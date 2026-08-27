#!/usr/bin/env bash
# Build Linux binaries on the current machine (or VPS) into ./bin
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

echo "==> building web UI"
(cd webapp && npm ci && npm run build)

echo "==> building go binaries"
mkdir -p bin
GOOS="${GOOS:-linux}" GOARCH="${GOARCH:-amd64}" CGO_ENABLED=0 \
  go build -trimpath -ldflags="-s -w" -o bin/dashboard ./cmd/dashboard
GOOS="${GOOS:-linux}" GOARCH="${GOARCH:-amd64}" CGO_ENABLED=0 \
  go build -trimpath -ldflags="-s -w" -o bin/signal-recorder ./cmd/signal-recorder
GOOS="${GOOS:-linux}" GOARCH="${GOARCH:-amd64}" CGO_ENABLED=0 \
  go build -trimpath -ldflags="-s -w" -o bin/backtest ./cmd/backtest
GOOS="${GOOS:-linux}" GOARCH="${GOARCH:-amd64}" CGO_ENABLED=0 \
  go build -trimpath -ldflags="-s -w" -o bin/token-info ./cmd/token-info

echo "done:"
ls -lh bin/
