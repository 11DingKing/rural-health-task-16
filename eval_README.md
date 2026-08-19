# Evaluation Build Guide

This document describes how to build and run the rehabilitation device certification platform for evaluation.

## Project Purpose

A Go backend service for managing medical device (exoskeleton rehabilitation robot) certification submissions, dispatch to testing agencies, rule-based standard evaluation, and result consistency enforcement.

## Data Directory

The service persists data to a configurable directory (default: `./data`). This contains:
- `certify.wal` — Write-ahead log for the KV store
- `certify.snapshot` — Periodic KV store snapshots
- `certify.meta` — Store metadata and data version
- `audit.db` — SQLite database for audit logs

## Standard Commands

```bash
# Build all Go packages
go build ./...

# Run the gateway service (port 52661)
go run ./cmd/certgateway -config configs/config.yaml

# Run an upstream simulator (port 52662)
go run ./cmd/certupstream -addr :52662 -name "Primary Testing Center"

# Run all tests
go test ./...

# Run tests with race detector
go test -race ./...
```

## Running with Port 52661

```bash
# Start the gateway service
CERT_HTTP_ADDR=:52661 CERT_DATA_DIR=./data go run ./cmd/certgateway

# Or with config file
go run ./cmd/certgateway -config configs/config.yaml

# Start an upstream simulator
go run ./cmd/certupstream -addr :52662 -name "Primary Testing Center"

# Start a backup upstream
go run ./cmd/certupstream -addr :52663 -name "Backup Testing Center"
```

## Docker Build

### Build for linux/amd64
```bash
./build_eval_docker.sh rehabcert-eval linux/amd64
```

### Build for linux/arm64
```bash
./build_eval_docker.sh rehabcert-eval linux/arm64
```

The `eval.Dockerfile` uses `golang:1.26` as the base image, installs Node.js 20, downloads Go dependencies, builds the Go binaries, and runs frontend type-checking and build. All dependencies are downloaded during build — no network access needed at runtime.
