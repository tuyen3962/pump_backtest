#!/usr/bin/env bash
# One-time VPS bootstrap (run ON the server as root/sudo).
set -euo pipefail

apt-get update
apt-get install -y --no-install-recommends ca-certificates curl rsync nginx

if ! id pump >/dev/null 2>&1; then
  useradd --system --home /opt/pump_backtest --shell /usr/sbin/nologin pump
fi

mkdir -p /opt/pump_backtest/{bin,data/signals,testdata,deploy}
chown -R pump:pump /opt/pump_backtest

# Open firewall if ufw is active
if command -v ufw >/dev/null 2>&1 && ufw status | grep -q "Status: active"; then
  ufw allow 80/tcp
  ufw allow 443/tcp
  ufw allow 8080/tcp || true
fi

echo "bootstrap ok. Next from laptop:"
echo "  ./scripts/deploy-vps.sh user@$(hostname -f)"
