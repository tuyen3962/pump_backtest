#!/usr/bin/env bash
# Deploy to a VPS over SSH (rsync + systemd).
#
# Usage:
#   ./scripts/deploy-vps.sh user@vps.example.com
#   DEPLOY_PATH=/opt/pump_backtest ./scripts/deploy-vps.sh root@1.2.3.4
#
# First-time on VPS (once):
#   sudo ./scripts/bootstrap-vps.sh
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
HOST="${1:-}"
DEPLOY_PATH="${DEPLOY_PATH:-/opt/pump_backtest}"

if [[ -z "$HOST" ]]; then
  echo "usage: $0 user@host" >&2
  exit 1
fi

echo "==> local build (linux/amd64)"
GOOS=linux GOARCH=amd64 "$ROOT/scripts/build.sh"

echo "==> rsync to ${HOST}:${DEPLOY_PATH}"
ssh "$HOST" "sudo mkdir -p '$DEPLOY_PATH'/{bin,data/signals,testdata} && sudo chown -R \"\$(whoami)\" '$DEPLOY_PATH' || true"
rsync -avz --delete \
  --exclude '.git' \
  --exclude 'webapp/node_modules' \
  --exclude 'data/signals/*.ndjson' \
  "$ROOT/bin/" "$HOST:$DEPLOY_PATH/bin/"
rsync -avz \
  "$ROOT/testdata/" "$HOST:$DEPLOY_PATH/testdata/"
rsync -avz \
  "$ROOT/deploy/" "$HOST:$DEPLOY_PATH/deploy/"
rsync -avz \
  "$ROOT/.env.example" "$HOST:$DEPLOY_PATH/.env.example"

echo "==> install/restart systemd units"
ssh "$HOST" bash -s <<EOF
set -euo pipefail
sudo mkdir -p /opt/pump_backtest/data/signals
if ! id pump >/dev/null 2>&1; then
  sudo useradd --system --home /opt/pump_backtest --shell /usr/sbin/nologin pump || true
fi
sudo chown -R pump:pump /opt/pump_backtest
sudo cp /opt/pump_backtest/deploy/pump-dashboard.service /etc/systemd/system/
sudo cp /opt/pump_backtest/deploy/pump-signal-recorder.service /etc/systemd/system/
if [[ ! -f /etc/pump-backtest.env ]]; then
  sudo cp /opt/pump_backtest/.env.example /etc/pump-backtest.env
  sudo chmod 600 /etc/pump-backtest.env
fi
sudo systemctl daemon-reload
sudo systemctl enable --now pump-dashboard.service pump-signal-recorder.service
sudo systemctl restart pump-dashboard.service pump-signal-recorder.service
sudo systemctl --no-pager --full status pump-dashboard.service pump-signal-recorder.service || true
EOF

echo "deployed. open http://<vps-ip>:8080  (or put nginx in front)"
