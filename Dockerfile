# syntax=docker/dockerfile:1

# --- frontend ---
FROM node:22-bookworm-slim AS web
WORKDIR /src/webapp
COPY webapp/package.json ./
# lockfile optional (may be missing if global gitignore excluded it)
COPY webapp/package-lock.json* ./
RUN if [ -f package-lock.json ]; then npm ci; else npm install; fi
COPY webapp/ ./
RUN npm run build

# --- go binaries ---
FROM golang:1.25-bookworm AS go-build
WORKDIR /src
ENV GOTOOLCHAIN=auto
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=web /src/web/dist ./web/dist
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/dashboard ./cmd/dashboard \
 && CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/signal-recorder ./cmd/signal-recorder \
 && CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/backtest ./cmd/backtest \
 && CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/token-info ./cmd/token-info

# --- runtime ---
# Run as root so Docker named volumes (/app/data) are writable on VPS.
FROM gcr.io/distroless/static-debian12:latest AS runtime
WORKDIR /app
COPY --from=go-build /out/dashboard /app/dashboard
COPY --from=go-build /out/signal-recorder /app/signal-recorder
COPY --from=go-build /out/backtest /app/backtest
COPY --from=go-build /out/token-info /app/token-info
COPY testdata /app/testdata
EXPOSE 8080
VOLUME ["/app/data"]
ENTRYPOINT ["/app/dashboard"]
CMD ["-addr", "0.0.0.0:8080", "-data", "/app/data/signals", "-store", "/app/data"]
