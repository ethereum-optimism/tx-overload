package main

import (
	opservice "github.com/ethereum-optimism/optimism/op-service"
	oplog "github.com/ethereum-optimism/optimism/op-service/log"
	opmetrics "github.com/ethereum-optimism/optimism/op-service/metrics"
	"github.com/ethereum-optimism/optimism/op-service/txmgr"
	"github.com/urfave/cli"
)

const envVarPrefix = "TX_OVERLOAD"

var (
	EthRpcFlag = cli.StringFlag{
		Name:     "eth-rpc",
		Required: true,
		EnvVar:   opservice.PrefixEnvVar(envVarPrefix, "ETH_RPC"),
	}
	DataRateFlag = cli.Int64Flag{
		Name:   "data-rate",
		Usage:  "data rate in bytes per second.",
		Value:  5000,
		EnvVar: opservice.PrefixEnvVar(envVarPrefix, "DATA_RATE"),
	}
	NumDistributors = cli.Int64Flag{
		Name:   "num-distributors",
		Value:  20,
		EnvVar: opservice.PrefixEnvVar(envVarPrefix, "NUM_DISTRIBUTORS"),
	}
)

func init() {
	flags = append(flags, EthRpcFlag, DataRateFlag, NumDistributors)
	flags = append(flags, oplog.CLIFlags(envVarPrefix)...)
	flags = append(flags, txmgr.CLIFlags(envVarPrefix)...)
	flags = append(flags, opmetrics.CLIFlags(envVarPrefix)...)
}

var flags []cli.Flag
