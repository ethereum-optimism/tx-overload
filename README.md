# Transaction Overload

Generate load on Optimism Bedrock using transactions with random calldata.

## Usage

```go
go build 

./tx-overload \
    --eth-rpc http://localhost:8545 \
    --private-key <private_key> \
    --num-distributors 10 \
    --data-rate 1000 \
```

More options are avaiable:
```
./tx-overload --help
```
