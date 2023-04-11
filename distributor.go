package main

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"math/rand"
	"time"

	opcrypto "github.com/ethereum-optimism/optimism/op-service/crypto"
	"github.com/ethereum-optimism/optimism/op-service/txmgr"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/params"
)

type Shard struct {
	*txmgr.SimpleTxManager
	reqs chan txmgr.TxCandidate
}

const txBufferSize = 1000

var ErrQueueFull = errors.New("queue full")

type Distributor struct {
	m          *Metrics
	shards     []Shard
	client     *ethclient.Client
	rootSigner opcrypto.SignerFn
	from       common.Address
	logger     log.Logger
	chainID    *big.Int
	cancel     chan struct{}
}

func NewDistributor(txmgrCfg txmgr.CLIConfig, l log.Logger, m *Metrics) (*Distributor, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	client, err := ethclient.DialContext(ctx, txmgrCfg.L1RPCURL)
	if err != nil {
		return nil, err
	}

	ctx, cancel = context.WithTimeout(context.Background(), 5*time.Second)
	chainID, err := client.ChainID(ctx)
	defer cancel()
	if err != nil {
		return nil, err
	}

	signerFactory, from, err := opcrypto.SignerFactoryFromConfig(l, txmgrCfg.PrivateKey, txmgrCfg.Mnemonic, txmgrCfg.HDPath, txmgrCfg.SignerCLIConfig)
	if err != nil {
		return nil, err
	}

	var shards []Shard
	for i, key := range distributors {
		cfg := txmgr.CLIConfig{
			L1RPCURL:                  txmgrCfg.L1RPCURL,
			PrivateKey:                key,
			NumConfirmations:          1,
			NetworkTimeout:            time.Second * 4,
			ResubmissionTimeout:       time.Second * 8,
			ReceiptQueryInterval:      time.Second * 2,
			TxNotInMempoolTimeout:     time.Second * 12,
			SafeAbortNonceTooLowCount: 2,
		}
		if err := cfg.Check(); err != nil {
			panic(err) // bug
		}
		tm, err := txmgr.NewSimpleTxManager(fmt.Sprintf("%d", i), logger, m, cfg)
		if err != nil {
			return nil, err
		}
		shards = append(shards, Shard{tm, make(chan txmgr.TxCandidate, txBufferSize)})
	}

	return &Distributor{
		m:          m,
		shards:     shards,
		client:     client,
		rootSigner: signerFactory(chainID),
		from:       from,
		logger:     l,
		chainID:    chainID,
		cancel:     make(chan struct{}),
	}, nil
}

func (d *Distributor) Start() {
	go d.runShards()
	go d.airdrop()
}

func (d *Distributor) Stop() {
	close(d.cancel)
	for _, s := range d.shards {
		close(s.reqs)
	}
}

func (d *Distributor) runShards() {
	for _, s := range d.shards {
		s := s
		go func() {
			for req := range s.reqs {
				receipt, err := s.Send(context.Background(), req)
				if err != nil {
					logger.Warn("unable to publish tx", "err", err)
					continue
				} else {
					logger.Trace("tx successfully published", "tx_hash", receipt.TxHash)
					d.m.RecordTx(&req)
				}
			}
		}()
	}
}

func (d *Distributor) airdrop() {
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		<-d.cancel
		cancel()
	}()

	lowBalance := new(big.Int).Mul(big.NewInt(20_000_000), big.NewInt(params.GWei)) // 0.02 ETH
	topOffAmount := new(big.Int).Mul(lowBalance, big.NewInt(3))
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		for _, shard := range d.shards {
			recipient := shard.SimpleTxManager.From()
			bal, err := d.client.BalanceAt(ctx, recipient, nil)
			if err != nil {
				d.logger.Error("failed to get balance", "err", err, "account", recipient)
				continue
			}
			if bal.Cmp(lowBalance) < 0 {
				d.logger.Debug("initiating airdrop", "recipient", recipient, "old_balance", bal)
				if err := d.doAirdrop(ctx, recipient, topOffAmount); err != nil {
					d.logger.Error("failed to airdrop", "err", err, "recipient", recipient)
					continue
				}
				d.logger.Info("airdrop successful", "recipient", recipient, "old_balance", bal)
			} else {
				d.logger.Debug("balance is high enough", "account", recipient, "balance", bal)
			}
		}
	}
}

func (d *Distributor) Send(ctx context.Context, tx txmgr.TxCandidate) error {
	shard := d.shards[rand.Intn(len(d.shards))]
	select {
	case shard.reqs <- tx:
		d.m.RecordQueuedTx(&tx)
		return nil
	default:
		d.logger.Warn("shard channel is full. dropping", "shard_account", shard.From())
		d.m.RecordTxDrop()
		return ErrQueueFull
	}
}

func (d *Distributor) doAirdrop(ctx context.Context, recipient common.Address, amount *big.Int) error {
	gasTipCap, err := d.client.SuggestGasTipCap(ctx)
	if err != nil {
		return err
	}
	head, err := d.client.HeaderByNumber(ctx, nil)
	if err != nil {
		return err
	}
	gasFeeCap := new(big.Int).Add(gasTipCap, new(big.Int).Mul(head.BaseFee, big.NewInt(2)))

	nonce, err := d.client.NonceAt(ctx, d.from, nil)
	if err != nil {
		return err
	}
	tx := &types.DynamicFeeTx{
		ChainID:   d.chainID,
		Nonce:     nonce,
		To:        &recipient,
		GasTipCap: gasTipCap,
		GasFeeCap: gasFeeCap,
		Value:     amount,
		Gas:       21000,
	}
	signed, err := d.rootSigner(ctx, d.from, types.NewTx(tx))
	if err != nil {
		return err
	}
	if err = d.client.SendTransaction(ctx, signed); err == nil {
		_, err = bind.WaitMined(ctx, d.client, signed)
	}
	return err
}
