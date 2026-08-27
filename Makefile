.PHONY: build web docker-up docker-down docker-logs deploy-help

web:
	cd webapp && npm ci && npm run build

build: web
	mkdir -p bin
	CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o bin/dashboard ./cmd/dashboard
	CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o bin/signal-recorder ./cmd/signal-recorder
	CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o bin/backtest ./cmd/backtest
	CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o bin/token-info ./cmd/token-info

docker-up:
	docker compose up -d --build

docker-down:
	docker compose down

docker-logs:
	docker compose logs -f --tail=200

deploy-help:
	@echo "Docker:  cp .env.example .env && make docker-up"
	@echo "Systemd: ./scripts/bootstrap-vps.sh  # on VPS"
	@echo "         ./scripts/deploy-vps.sh user@vps"
