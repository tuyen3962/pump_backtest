# Deploy lên VPS

Hai cách: **Docker Compose** (nhanh) hoặc **systemd** (binary trên host).

## A) Docker Compose (khuyến nghị)

Trên VPS cần Docker + Docker Compose plugin.

```bash
git clone https://github.com/tuyen3962/pump_backtest.git /opt/pump_backtest
cd /opt/pump_backtest
cp .env.example .env
# sửa DASHBOARD_PORT nếu cần
docker compose up -d --build
```

- Dashboard: `http://<vps-ip>:8080`
- Recorder ghi NDJSON vào volume `signal_data` (`/app/data/signals`)
- Xem log: `docker compose logs -f`
- Update: `git pull && docker compose up -d --build`

## B) Systemd (binary)

### 1. Bootstrap (chạy trên VPS, một lần)

```bash
sudo ./scripts/bootstrap-vps.sh
```

### 2. Deploy từ máy local

```bash
./scripts/deploy-vps.sh user@your-vps
```

Script sẽ build Linux amd64, rsync `bin/` + unit files, enable:

- `pump-dashboard.service`
- `pump-signal-recorder.service`

Data: `/opt/pump_backtest/data/signals`  
Env: `/etc/pump-backtest.env`

```bash
sudo systemctl status pump-dashboard pump-signal-recorder
sudo journalctl -u pump-signal-recorder -f
```

## C) Nginx + HTTPS (optional)

```bash
sudo sed 's/DOMAIN/backtest.example.com/' deploy/nginx.conf \
  | sudo tee /etc/nginx/sites-available/pump-backtest
sudo ln -sf /etc/nginx/sites-available/pump-backtest /etc/nginx/sites-enabled/
sudo nginx -t && sudo systemctl reload nginx
sudo certbot --nginx -d backtest.example.com
```

Nên firewall chỉ mở 80/443, giữ dashboard listen `127.0.0.1:8080` khi đã có nginx (đổi `ExecStart` `-addr 127.0.0.1:8080`).

## Checklist bảo mật

- Không expose dashboard ra public nếu chưa auth — hiện API **không có login**
- Prefer nginx + firewall, hoặc VPN/SSH tunnel
- Backup volume/data signals định kỳ

## Local helper

```bash
make build          # web + go binaries
make docker-up      # compose
make deploy-help
```
