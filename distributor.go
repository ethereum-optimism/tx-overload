package main

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"math/rand"
	"time"

	"github.com/ethereum-optimism/optimism/op-service/txmgr"
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
	m      *Metrics
	root   *txmgr.SimpleTxManager
	shards []Shard
	client *ethclient.Client
	logger log.Logger
	cancel chan struct{}
}

func NewDistributor(txmgrCfg txmgr.CLIConfig, l log.Logger, m *Metrics) (*Distributor, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	client, err := ethclient.DialContext(ctx, txmgrCfg.L1RPCURL)
	if err != nil {
		return nil, err
	}

	// override L1 defaults
	txmgrCfg.NumConfirmations = 1
	txmgrCfg.NetworkTimeout = time.Second * 2
	txmgrCfg.ResubmissionTimeout = time.Second * 8
	txmgrCfg.ReceiptQueryInterval = time.Second * 2
	txmgrCfg.TxNotInMempoolTimeout = time.Second * 6
	txmgrCfg.SafeAbortNonceTooLowCount = 2
	root, err := txmgr.NewSimpleTxManager("root", logger, m, txmgrCfg)
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
		tm, err := txmgr.NewSimpleTxManager(fmt.Sprintf("shard-%d", i), logger, m, cfg)
		if err != nil {
			return nil, err
		}
		shards = append(shards, Shard{tm, make(chan txmgr.TxCandidate, txBufferSize)})
	}

	return &Distributor{
		m:      m,
		root:   root,
		shards: shards,
		client: client,
		logger: l,
		cancel: make(chan struct{}),
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

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			for _, shard := range d.shards {
				recipient := shard.SimpleTxManager.From()
				bal, err := d.client.BalanceAt(ctx, recipient, nil)
				if err != nil {
					d.logger.Error("failed to get balance", "err", err, "account", recipient)
					continue
				}
				if bal.Cmp(lowBalance) < 0 {
					d.logger.Debug("initiating airdrop", "recipient", recipient, "old_balance", bal)
					_, err := d.root.Send(ctx, txmgr.TxCandidate{
						To:       &recipient,
						Value:    topOffAmount,
						GasLimit: params.TxGas,
					})
					if err != nil {
						d.logger.Error("failed to send airdrop tx", "err", err, "recipient", recipient)
						continue
					} else {
						d.logger.Info("airdrop successful", "recipient", recipient)
					}
				} else {
					d.logger.Debug("balance is high enough", "account", recipient, "balance", bal)
				}
			}
		}
	}
}
