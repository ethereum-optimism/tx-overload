package main

import (
	"context"

	opmetrics "github.com/ethereum-optimism/optimism/op-service/metrics"
	"github.com/ethereum-optimism/optimism/op-service/txmgr"
	txmetrics "github.com/ethereum-optimism/optimism/op-service/txmgr/metrics"
	"github.com/prometheus/client_golang/prometheus"
)

type Metrics struct {
	registry               *prometheus.Registry
	factory                opmetrics.Factory
	DataBytes              prometheus.Counter
	DataBytesQueued        prometheus.Counter
	TransactionCount       prometheus.Counter
	TransactionCountQueued prometheus.Counter
	TxDrops                prometheus.Counter

	txmetrics.TxMetrics
}

func NewMetrics() *Metrics {
	registry := opmetrics.NewRegistry()
	factory := opmetrics.With(registry)
	ns := "tx_overload_default"

	return &Metrics{
		registry:  registry,
		factory:   factory,
		TxMetrics: txmetrics.MakeTxMetrics(ns, factory),
		DataBytes: factory.NewCounter(prometheus.CounterOpts{
			Namespace: ns,
			Name:      "data_bytes",
			Help:      "Number of confirmed bytes in transaction calldata",
		}),
		DataBytesQueued: factory.NewCounter(prometheus.CounterOpts{
			Namespace: ns,
			Name:      "data_bytes_queued",
			Help:      "Number of queued bytes in transaction calldata",
		}),
		TransactionCount: factory.NewCounter(prometheus.CounterOpts{
			Namespace: ns,
			Name:      "tx_count",
			Help:      "Number of confirmed transactions",
		}),
		TransactionCountQueued: factory.NewCounter(prometheus.CounterOpts{
			Namespace: ns,
			Name:      "tx_count_queued",
			Help:      "Number of queued transactions",
		}),
		TxDrops: factory.NewCounter(prometheus.CounterOpts{
			Namespace: ns,
			Name:      "tx_drops",
			Help:      "Number of dropped transactions",
		}),
	}
}

func (m *Metrics) Serve(ctx context.Context, host string, port int) error {
	return opmetrics.ListenAndServe(ctx, m.registry, host, port)
}

func (m *Metrics) RecordQueuedTx(tx *txmgr.TxCandidate) {
	m.DataBytesQueued.Add(float64(len(tx.TxData)))
	m.TransactionCountQueued.Inc()
}

func (m *Metrics) RecordTx(tx *txmgr.TxCandidate) {
	m.DataBytes.Add(float64(len(tx.TxData)))
	m.TransactionCount.Inc()
}

func (m *Metrics) RecordTxDrop() {
	m.TxDrops.Inc()
}
