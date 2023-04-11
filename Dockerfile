FROM golang:1.19.4-alpine3.17 AS builder

WORKDIR /build
# Copy and download dependencies using go mod
COPY go.mod .
COPY go.sum .
RUN go mod download

# Copy the code into the container
COPY . .

# Build the application
CGO_ENABLED=0 GOOS=linux go build -o tx-overload.bin .

FROM alpine:latest

COPY --from=builder /build/tx-overload.bin /tx-overload.bin

ENTRYPOINT ["/tx-overload.bin"]
