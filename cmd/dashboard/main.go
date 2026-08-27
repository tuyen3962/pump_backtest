package main

import (
	"context"
	"encoding/json"
	"flag"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/surt/pump_backtest/internal/backtest"
	"github.com/surt/pump_backtest/internal/signal"
	"github.com/surt/pump_backtest/internal/tokeninfo"
	"github.com/surt/pump_backtest/web"
)

func main() {
	addr := flag.String("addr", "127.0.0.1:8080", "Listen address")
	dataDir := flag.String("data", "data/signals", "Default recordings directory")
	flag.Parse()

	static, err := fs.Sub(web.Dist, "dist")
	if err != nil {
		log.Fatal(err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/api/sources", func(w http.ResponseWriter, r *http.Request) {
		handleSources(w, r, *dataDir)
	})
	mux.HandleFunc("/api/token", handleToken)
	mux.HandleFunc("/api/backtest", func(w http.ResponseWriter, r *http.Request) {
		handleBacktest(w, r, *dataDir)
	})
	mux.Handle("/", spaHandler(static))

	log.Printf("dashboard http://%s  data=%s", *addr, *dataDir)
	if err := http.ListenAndServe(*addr, mux); err != nil {
		log.Fatal(err)
	}
}

func spaHandler(static fs.FS) http.Handler {
	fileServer := http.FileServer(http.FS(static))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/")
		if path != "" {
			if _, err := fs.Stat(static, path); err == nil {
				fileServer.ServeHTTP(w, r)
				return
			}
			// Missing asset (not SPA route) → 404
			if strings.Contains(path, ".") {
				http.NotFound(w, r)
				return
			}
		}
		data, err := fs.ReadFile(static, "index.html")
		if err != nil {
			http.Error(w, "ui not built — run: cd webapp && npm run build", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(data)
	})
}

type backtestRequest struct {
	Source            string   `json:"source"`
	EntryKinds        []string `json:"entryKinds"`
	StartCash         float64  `json:"startCash"`
	NotionalUSD       float64  `json:"notionalUsd"`
	FeeBps            float64  `json:"feeBps"`
	MaxPositions      int      `json:"maxPositions"`
	CloseOpenAtEnd    bool     `json:"closeOpenAtEnd"`
	EnrichTokens      bool     `json:"enrichTokens"`
	SampleLive        bool     `json:"sampleLive"`
	AlsoExitMustOut   *bool    `json:"alsoExitMustOut"`
	MinLiquidityUSD   *float64 `json:"minLiquidityUsd"`
	MinVolumeUSD1h    *float64 `json:"minVolumeUsd1h"`
	LatencySec        *float64 `json:"latencySec"`
	StopLossPct       *float64 `json:"stopLossPct"`
	ScaleTriggerPct   *float64 `json:"scaleTriggerPct"`
	DisableFilters    bool     `json:"disableFilters"`
}

func handleSources(w http.ResponseWriter, r *http.Request, dataDir string) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	type source struct {
		ID    string `json:"id"`
		Label string `json:"label"`
		Path  string `json:"path"`
	}
	out := []source{
		{ID: "live", Label: "Live recordings (data/signals)", Path: dataDir},
		{ID: "demo", Label: "Demo fixture (pump → out)", Path: "testdata/signals/demo.ndjson"},
	}
	if entries, err := os.ReadDir(dataDir); err == nil {
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			name := e.Name()
			if strings.HasSuffix(name, ".ndjson") || strings.HasSuffix(name, ".jsonl") {
				p := filepath.Join(dataDir, name)
				out = append(out, source{
					ID:    "file:" + name,
					Label: name,
					Path:  p,
				})
			}
		}
	}
	writeJSON(w, out)
}

func handleToken(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	mint := strings.TrimSpace(r.URL.Query().Get("mint"))
	if mint == "" {
		http.Error(w, "mint query required", http.StatusBadRequest)
		return
	}
	live := r.URL.Query().Get("live") == "1" || r.URL.Query().Get("live") == "true"
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	info, err := tokeninfo.Fetch(ctx, mint, tokeninfo.Options{
		SampleLive: live,
		LiveWindow: 8 * time.Second,
	})
	if err != nil && len(info.Sources) == 0 {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	writeJSON(w, info)
}

func handleBacktest(w http.ResponseWriter, r *http.Request, dataDir string) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req backtestRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad json: "+err.Error(), http.StatusBadRequest)
		return
	}

	path := dataDir
	switch {
	case req.Source == "" || req.Source == "live":
		path = dataDir
	case req.Source == "demo":
		path = "testdata/signals/demo.ndjson"
	case strings.HasPrefix(req.Source, "file:"):
		name := filepath.Base(strings.TrimPrefix(req.Source, "file:"))
		path = filepath.Join(dataDir, name)
	default:
		path = req.Source
	}

	records, err := signal.LoadNDJSON(path)
	if err != nil {
		http.Error(w, "load: "+err.Error(), http.StatusBadRequest)
		return
	}

	cfg := backtest.DefaultConfig()
	if len(req.EntryKinds) > 0 {
		cfg.EntryKinds = req.EntryKinds
	}
	if req.StartCash > 0 {
		cfg.StartCash = req.StartCash
	}
	if req.NotionalUSD > 0 {
		cfg.NotionalUSD = req.NotionalUSD
	}
	if req.FeeBps >= 0 {
		cfg.FeeBps = req.FeeBps
	}
	cfg.MaxPositions = req.MaxPositions
	cfg.CloseOpenAtEnd = req.CloseOpenAtEnd
	if req.AlsoExitMustOut != nil {
		cfg.AlsoExitMustOut = *req.AlsoExitMustOut
	}
	if req.MinLiquidityUSD != nil {
		cfg.MinLiquidityUSD = *req.MinLiquidityUSD
	}
	if req.MinVolumeUSD1h != nil {
		cfg.MinVolumeUSD1h = *req.MinVolumeUSD1h
	}
	if req.LatencySec != nil {
		cfg.LatencySec = *req.LatencySec
	}
	if req.StopLossPct != nil {
		cfg.StopLossPct = *req.StopLossPct
	}
	if req.ScaleTriggerPct != nil {
		cfg.ScaleTriggerPct = *req.ScaleTriggerPct
	}
	if req.DisableFilters {
		cfg.MinLiquidityUSD = 0
		cfg.MinVolumeUSD1h = 0
	}

	var market map[string]backtest.MarketSnapshot
	if req.EnrichTokens {
		ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
		market = backtest.MarketFromRecords(ctx, records, tokeninfo.Options{
			SampleLive: req.SampleLive,
			LiveWindow: 6 * time.Second,
		})
		cancel()
	}

	res, err := backtest.RunWithMarket(records, cfg, market)
	if err != nil {
		http.Error(w, "run: "+err.Error(), http.StatusBadRequest)
		return
	}

	if req.EnrichTokens {
		ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
		backtest.EnrichCoins(ctx, &res, tokeninfo.Options{})
		cancel()
	}

	type equityDTO struct {
		Time          string  `json:"time"`
		Equity        float64 `json:"equity"`
		RealizedPnL   float64 `json:"realizedPnl"`
		UnrealizedPnL float64 `json:"unrealizedPnl"`
		OpenPositions int     `json:"openPositions"`
		Event         string  `json:"event"`
		Symbol        string  `json:"symbol,omitempty"`
	}
	eq := make([]equityDTO, 0, len(res.Equity))
	for _, p := range res.Equity {
		eq = append(eq, equityDTO{
			Time:          p.Time.UTC().Format(time.RFC3339Nano),
			Equity:        p.Equity,
			RealizedPnL:   p.RealizedPnL,
			UnrealizedPnL: p.UnrealizedPnL,
			OpenPositions: p.OpenPositions,
			Event:         p.Event,
			Symbol:        p.Symbol,
		})
	}

	writeJSON(w, map[string]any{
		"source":  path,
		"loaded":  len(records),
		"result":  res,
		"equity":  eq,
		"updated": time.Now().UTC().Format(time.RFC3339),
	})
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		log.Printf("json encode: %v", err)
	}
}
