package main

import (
	"context"

	opmetrics "github.com/ethereum-optimism/optimism/op-service/metrics"
	txmetrics "github.com/ethereum-optimism/optimism/op-service/txmgr/metrics"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/prometheus/client_golang/prometheus"
)

type Metrics struct {
	registry                  *prometheus.Registry
	factory                   opmetrics.Factory
	DataBytes                 prometheus.Counter
	DataBytesConfirmed        prometheus.Counter
	TransactionTotal          prometheus.Counter
	TransactionConfirmedTotal prometheus.Counter

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
			Help:      "Number of bytes in transaction",
		}),
		DataBytesConfirmed: factory.NewCounter(prometheus.CounterOpts{
			Namespace: ns,
			Name:      "data_bytes_confirmed",
			Help:      "Number of bytes in transaction",
		}),
		TransactionTotal: factory.NewCounter(prometheus.CounterOpts{
			Namespace: ns,
			Name:      "tx_total",
			Help:      "Number of transactions",
		}),
		TransactionConfirmedTotal: factory.NewCounter(prometheus.CounterOpts{
			Namespace: ns,
			Name:      "tx_confirmed_total",
			Help:      "Number of transactions",
		}),
	}
}

func (m *Metrics) Serve(ctx context.Context, host string, port int) error {
	return opmetrics.ListenAndServe(ctx, m.registry, host, port)
}

func (m *Metrics) RecordTx(tx *types.Transaction) {
	m.DataBytes.Add(float64(len(tx.Data())))
	m.TransactionTotal.Inc()
}
