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
- Update thủ công: `git pull && docker compose up -d --build`

### Auto deploy (GitHub Actions + Telegram)

Mỗi lần push lên `main`, workflow `.github/workflows/deploy.yml` SSH vào VPS, `git pull` + `docker compose up -d --build`, và gửi Telegram khi bắt đầu / thành công / thất bại.

#### 1) Chuẩn bị VPS (một lần)

```bash
# clone nếu chưa có
sudo mkdir -p /opt/pump_backtest
sudo chown "$USER":"$USER" /opt/pump_backtest
git clone https://github.com/tuyen3962/pump_backtest.git /opt/pump_backtest
cd /opt/pump_backtest
cp .env.example .env
docker compose up -d --build
```

Tạo SSH key riêng cho CI (trên máy local hoặc VPS):

```bash
ssh-keygen -t ed25519 -C "github-actions-deploy" -f ~/.ssh/pump_deploy -N ""
# public key lên VPS
cat ~/.ssh/pump_deploy.pub >> ~/.ssh/authorized_keys
# private key → GitHub secret VPS_SSH_KEY (copy nguyên nội dung ~/.ssh/pump_deploy)
```

User trên VPS cần quyền chạy Docker không cần sudo (hoặc thêm vào group `docker`):

```bash
sudo usermod -aG docker "$USER"
# logout/login lại
```

#### 2) Tạo Telegram bot

1. Chat với [@BotFather](https://t.me/BotFather) → `/newbot` → lấy **bot token**
2. Chat với bot vừa tạo (gửi bất kỳ tin nhắn)
3. Lấy `chat_id`:

```bash
curl "https://api.telegram.org/bot<TOKEN>/getUpdates"
# xem result[].message.chat.id
```

Hoặc dùng group: thêm bot vào group, gửi tin, rồi gọi `getUpdates` như trên.

#### 3) GitHub Secrets

Repo → **Settings → Secrets and variables → Actions** → thêm:

| Secret | Ví dụ |
|--------|--------|
| `VPS_HOST` | `1.2.3.4` hoặc hostname |
| `VPS_USER` | `ubuntu` / `root` / user deploy |
| `VPS_SSH_KEY` | nội dung private key (`-----BEGIN ...`) |
| `VPS_PORT` | `22` (optional; mặc định 22) |
| `DEPLOY_PATH` | `/opt/pump_backtest` (optional; mặc định path này) |
| `TELEGRAM_BOT_TOKEN` | token từ BotFather |
| `TELEGRAM_CHAT_ID` | chat id số |

**Repo private:** trên VPS cần quyền `git fetch` (deploy key read-only hoặc HTTPS + PAT).

Sau đó push lên `main` (hoặc **Actions → Deploy to VPS → Run workflow**).
Kiểm tra Telegram + `docker compose ps` trên VPS.

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
