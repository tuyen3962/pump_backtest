package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/surt/pump_backtest/internal/backtest"
	"github.com/surt/pump_backtest/internal/store"
	"github.com/surt/pump_backtest/internal/tokeninfo"
	"github.com/surt/pump_backtest/web"
)

func main() {
	addr := flag.String("addr", "127.0.0.1:8080", "Listen address")
	dataDir := flag.String("data", "data/signals", "Signal recordings directory")
	storeRoot := flag.String("store", "", "Persistence root (default: parent of -data)")
	flag.Parse()

	if err := os.MkdirAll(*dataDir, 0o755); err != nil {
		log.Fatalf("create data dir: %v", err)
	}
	root := *storeRoot
	if root == "" {
		root = filepath.Clean(filepath.Join(*dataDir, ".."))
	}
	st, err := store.New(root)
	if err != nil {
		log.Fatalf("store: %v", err)
	}
	hub := newBacktestHub(*dataDir, st)

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
		switch r.Method {
		case http.MethodPost:
			handleBacktest(w, r, hub)
		case http.MethodGet:
			handleLastBacktest(w, st)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	})
	mux.HandleFunc("/api/jobs", func(w http.ResponseWriter, r *http.Request) {
		handleJobs(w, r, hub)
	})
	mux.HandleFunc("/api/jobs/", func(w http.ResponseWriter, r *http.Request) {
		handleJobByID(w, r, hub)
	})
	mux.HandleFunc("/api/runs", func(w http.ResponseWriter, r *http.Request) {
		handleRuns(w, r, st)
	})
	mux.HandleFunc("/api/runs/", func(w http.ResponseWriter, r *http.Request) {
		handleRunByID(w, r, st)
	})
	mux.HandleFunc("/api/history", func(w http.ResponseWriter, r *http.Request) {
		handleHistory(w, r, st)
	})
	mux.HandleFunc("/api/watchlist", func(w http.ResponseWriter, r *http.Request) {
		handleWatchlist(w, r, st)
	})
	mux.HandleFunc("/api/follow", func(w http.ResponseWriter, r *http.Request) {
		handleFollow(w, r, st)
	})
	mux.HandleFunc("/api/follow/stream", func(w http.ResponseWriter, r *http.Request) {
		handleFollowStream(w, r, st)
	})
	mux.Handle("/", spaHandler(static))

	log.Printf("dashboard http://%s  signals=%s  store=%s", *addr, *dataDir, root)
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
				out = append(out, source{ID: "file:" + name, Label: name, Path: p})
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

func handleLastBacktest(w http.ResponseWriter, st *store.Store) {
	run, err := st.LoadLastRun()
	if err != nil {
		if os.IsNotExist(err) {
			writeJSON(w, map[string]any{"found": false})
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	var resp any
	if err := json.Unmarshal(run.Response, &resp); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]any{
		"found":   true,
		"id":      run.ID,
		"savedAt": run.SavedAt.UTC().Format(time.RFC3339),
		"run":     resp,
	})
}

func handleBacktest(w http.ResponseWriter, r *http.Request, hub *backtestHub) {
	var req backtestRequest
	reqBytes, err := readBody(r)
	if err != nil {
		http.Error(w, "bad json: "+err.Error(), http.StatusBadRequest)
		return
	}
	if err := json.Unmarshal(reqBytes, &req); err != nil {
		http.Error(w, "bad json: "+err.Error(), http.StatusBadRequest)
		return
	}
	// Stability sessions always run async (periodic reload until SessionEndAt).
	async := req.Async || r.URL.Query().Get("async") == "1" || strings.TrimSpace(req.SessionEndAt) != ""
	if async {
		// Compare/lab runs should not clobber the live follow watchlist.
		// Session jobs default to updating watchlist so Live Follow tracks opens.
		if req.UpdateWatchlist == nil {
			f := strings.TrimSpace(req.SessionEndAt) != ""
			req.UpdateWatchlist = &f
		}
		job, err := hub.enqueue(req, reqBytes)
		if err != nil {
			http.Error(w, err.Error(), http.StatusTooManyRequests)
			return
		}
		writeJSON(w, map[string]any{"job": job})
		return
	}

	updateWatch := true
	if req.UpdateWatchlist != nil {
		updateWatch = *req.UpdateWatchlist
	}
	out, err := hub.execute(r.Context(), req, reqBytes, "", updateWatch, persistFull)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, out.Payload)
}

func handleJobs(w http.ResponseWriter, r *http.Request, hub *backtestHub) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, map[string]any{"items": hub.runner.List(50)})
}

func handleJobByID(w http.ResponseWriter, r *http.Request, hub *backtestHub) {
	id := strings.TrimPrefix(r.URL.Path, "/api/jobs/")
	id = filepath.Base(id)
	switch r.Method {
	case http.MethodGet:
		job, ok := hub.runner.Get(id)
		if !ok {
			http.Error(w, "job not found", http.StatusNotFound)
			return
		}
		writeJSON(w, job)
	case http.MethodDelete:
		ok := hub.runner.Cancel(id)
		if !ok {
			http.Error(w, "job not found or not cancellable", http.StatusNotFound)
			return
		}
		writeJSON(w, map[string]any{"ok": true, "id": id})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func handleRuns(w http.ResponseWriter, r *http.Request, st *store.Store) {
	switch r.Method {
	case http.MethodGet:
		if r.URL.Query().Get("compare") != "" {
			handleCompareRuns(w, r, st)
			return
		}
		items, err := st.ListRuns(100)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, map[string]any{"items": items, "count": len(items)})
	case http.MethodDelete:
		before := strings.TrimSpace(r.URL.Query().Get("before"))
		if before == "" {
			http.Error(w, "before=RFC3339 required", http.StatusBadRequest)
			return
		}
		cutoff, err := parseRFC3339(before)
		if err != nil || cutoff.IsZero() {
			http.Error(w, "bad before time", http.StatusBadRequest)
			return
		}
		n, err := st.DeleteRunsBefore(cutoff)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, map[string]any{"deleted": n, "before": cutoff.UTC().Format(time.RFC3339)})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func handleCompareRuns(w http.ResponseWriter, r *http.Request, st *store.Store) {
	raw := r.URL.Query().Get("compare")
	ids := strings.Split(raw, ",")
	type row struct {
		ID          string   `json:"id"`
		Label       string   `json:"label"`
		SavedAt     string   `json:"savedAt"`
		Source      string   `json:"source"`
		EntryKinds  []string `json:"entryKinds"`
		Signals     int      `json:"signals"`
		Coins       int      `json:"coins"`
		Closed      int      `json:"closedCount"`
		Open        int      `json:"openCount"`
		WinRate     float64  `json:"winRate"`
		AvgReturn   float64  `json:"avgReturn"`
		TotalPnL    float64  `json:"totalPnl"`
		EndEquity   float64  `json:"endEquity"`
		MaxDDPct    float64  `json:"maxDrawdownPct"`
		Skipped     int      `json:"skippedEntries"`
		StopLossPct float64  `json:"stopLossPct"`
		TP2x        float64  `json:"takeProfit2x"`
		FeeBps      float64  `json:"feeBps"`
		LatencySec  float64  `json:"latencySec"`
		Notional    float64  `json:"notionalUsd"`
		StartCash   float64  `json:"startCash"`
	}
	var rows []row
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		run, err := st.LoadRun(id)
		if err != nil {
			continue
		}
		res, _, err := resultFromSaved(run)
		if err != nil {
			rows = append(rows, row{
				ID: run.ID, Label: run.Label, Source: run.Source,
				TotalPnL: run.TotalPnL, EndEquity: run.EndEquity, WinRate: run.WinRate,
				MaxDDPct: run.MaxDDPct, Coins: run.CoinCount, EntryKinds: run.EntryKinds,
				SavedAt: run.SavedAt.UTC().Format(time.RFC3339),
			})
			continue
		}
		cfg := res.Config
		rows = append(rows, row{
			ID:          run.ID,
			Label:       run.Label,
			SavedAt:     run.SavedAt.UTC().Format(time.RFC3339),
			Source:      run.Source,
			EntryKinds:  cfg.EntryKinds,
			Signals:     res.Signals,
			Coins:       len(res.Coins),
			Closed:      res.ClosedCount,
			Open:        res.OpenCount,
			WinRate:     res.WinRate,
			AvgReturn:   res.AvgReturn,
			TotalPnL:    res.TotalPnL,
			EndEquity:   res.EndEquity,
			MaxDDPct:    res.MaxDrawdown,
			Skipped:     res.Skipped,
			StopLossPct: cfg.StopLossPct,
			TP2x:        cfg.TakeProfit2x,
			FeeBps:      cfg.FeeBps,
			LatencySec:  cfg.LatencySec,
			Notional:    cfg.NotionalUSD,
			StartCash:   cfg.StartCash,
		})
	}
	writeJSON(w, map[string]any{"items": rows, "count": len(rows)})
}

func handleRunByID(w http.ResponseWriter, r *http.Request, st *store.Store) {
	path := strings.TrimPrefix(r.URL.Path, "/api/runs/")
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) == 0 || parts[0] == "" {
		http.Error(w, "run id required", http.StatusBadRequest)
		return
	}
	id := filepath.Base(parts[0])
	run, err := st.LoadRun(id)
	if err != nil {
		http.Error(w, "run not found", http.StatusNotFound)
		return
	}

	if len(parts) >= 2 && parts[1] == "export.csv" {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		res, _, err := resultFromSaved(run)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		part := r.URL.Query().Get("part")
		if part == "" {
			part = "trades"
		}
		filename := fmt.Sprintf("%s_%s.csv", id, part)
		writeCSVDownload(w, filename, func(w http.ResponseWriter) error {
			switch part {
			case "coins":
				return backtest.WriteCoinsCSV(w, res)
			case "equity":
				return backtest.WriteEquityCSV(w, res)
			case "summary":
				return backtest.WriteSummaryCSV(w, res, run.ID, run.Label, run.Source)
			default:
				return backtest.WriteTradesCSV(w, res)
			}
		})
		return
	}

	if r.Method == http.MethodDelete {
		if err := st.DeleteRun(id); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, map[string]any{"ok": true, "id": id})
		return
	}

	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var resp any
	if err := json.Unmarshal(run.Response, &resp); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]any{
		"id":      run.ID,
		"label":   run.Label,
		"savedAt": run.SavedAt.UTC().Format(time.RFC3339),
		"source":  run.Source,
		"run":     resp,
	})
}

func handleHistory(w http.ResponseWriter, r *http.Request, st *store.Store) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	items, err := st.ListHistory(200)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	status := r.URL.Query().Get("status")
	if status != "" {
		filtered := items[:0]
		for _, it := range items {
			if it.Status == status {
				filtered = append(filtered, it)
			}
		}
		items = filtered
	}
	writeJSON(w, map[string]any{"items": items, "count": len(items)})
}

func handleWatchlist(w http.ResponseWriter, r *http.Request, st *store.Store) {
	switch r.Method {
	case http.MethodGet:
		items, err := st.LoadWatchlist()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, map[string]any{"items": items})
	case http.MethodPost:
		var body struct {
			Mint      string  `json:"mint"`
			Symbol    string  `json:"symbol"`
			EntryMcap float64 `json:"entryMcap"`
			Source    string  `json:"source"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		body.Mint = strings.TrimSpace(body.Mint)
		if body.Mint == "" {
			http.Error(w, "mint required", http.StatusBadRequest)
			return
		}
		if body.Source == "" {
			body.Source = "manual"
		}
		items, err := st.UpsertWatch(store.WatchItem{
			Mint:      body.Mint,
			Symbol:    body.Symbol,
			EntryMcap: body.EntryMcap,
			AddedAt:   time.Now().UTC(),
			Source:    body.Source,
		})
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, map[string]any{"items": items})
	case http.MethodDelete:
		mint := strings.TrimSpace(r.URL.Query().Get("mint"))
		if mint == "" {
			http.Error(w, "mint required", http.StatusBadRequest)
			return
		}
		items, err := st.RemoveWatch(mint)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, map[string]any{"items": items})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func handleFollow(w http.ResponseWriter, r *http.Request, st *store.Store) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 45*time.Second)
	defer cancel()
	out, err := buildFollowRows(ctx, st)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]any{
		"items":     out,
		"updatedAt": time.Now().UTC().Format(time.RFC3339),
	})
}

func handleFollowStream(w http.ResponseWriter, r *http.Request, st *store.Store) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	send := func() bool {
		ctx, cancel := context.WithTimeout(r.Context(), 40*time.Second)
		rows, err := buildFollowRows(ctx, st)
		cancel()
		payload := map[string]any{
			"items":     rows,
			"updatedAt": time.Now().UTC().Format(time.RFC3339),
		}
		if err != nil {
			payload["error"] = err.Error()
			payload["items"] = []followRow{}
		}
		b, _ := json.Marshal(payload)
		if _, err := fmt.Fprintf(w, "data: %s\n\n", b); err != nil {
			return false
		}
		flusher.Flush()
		return true
	}

	if !send() {
		return
	}
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
			if !send() {
				return
			}
		}
	}
}

type followRow struct {
	Mint           string  `json:"mint"`
	Symbol         string  `json:"symbol"`
	EntryMcap      float64 `json:"entryMcap"`
	LiveMcap       float64 `json:"liveMcap"`
	ReturnPct      float64 `json:"returnPct"`
	VolumeUSD1h    float64 `json:"volumeUsd1h"`
	VolumeUSD24h   float64 `json:"volumeUsd24h"`
	LiquidityUSD   float64 `json:"liquidityUsd"`
	SellRatio1h    float64 `json:"sellRatio1h"`
	RugScore       float64 `json:"rugScore"`
	RugLabel       string  `json:"rugLabel"`
	ATHDrawdownPct float64 `json:"athDrawdownPct"`
	Source         string  `json:"source"`
	Error          string  `json:"error,omitempty"`
}

func buildFollowRows(ctx context.Context, st *store.Store) ([]followRow, error) {
	items, err := st.LoadWatchlist()
	if err != nil {
		return nil, err
	}
	out := make([]followRow, 0, len(items))
	for _, it := range items {
		row := followRow{
			Mint:      it.Mint,
			Symbol:    it.Symbol,
			EntryMcap: it.EntryMcap,
			Source:    it.Source,
		}
		info, err := tokeninfo.Fetch(ctx, it.Mint, tokeninfo.Options{})
		if err != nil && len(info.Sources) == 0 {
			row.Error = err.Error()
			out = append(out, row)
			continue
		}
		if info.Symbol != "" {
			row.Symbol = info.Symbol
		}
		row.LiveMcap = info.MarketCapUSD
		row.VolumeUSD1h = info.VolumeUSD1h
		row.VolumeUSD24h = info.VolumeUSD24h
		row.LiquidityUSD = info.LiquidityUSD
		row.SellRatio1h = info.SellRatio1h
		row.RugScore = info.RugScore
		row.RugLabel = info.RugLabel
		row.ATHDrawdownPct = info.ATHDrawdownPct
		if it.EntryMcap > 0 && info.MarketCapUSD > 0 {
			row.ReturnPct = (info.MarketCapUSD - it.EntryMcap) / it.EntryMcap * 100
		}
		out = append(out, row)
	}
	return out, nil
}

func buildConfig(req backtestRequest) backtest.Config {
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
	if req.TakeProfit2x != nil && *req.TakeProfit2x > 0 {
		cfg.TakeProfit2x = *req.TakeProfit2x
	}
	if req.DisableFilters {
		cfg.MinLiquidityUSD = 0
		cfg.MinVolumeUSD1h = 0
	}
	return cfg
}

func equityDTO(res backtest.Result) []map[string]any {
	eq := make([]map[string]any, 0, len(res.Equity))
	for _, p := range res.Equity {
		eq = append(eq, map[string]any{
			"time":          p.Time.UTC().Format(time.RFC3339Nano),
			"equity":        p.Equity,
			"realizedPnl":   p.RealizedPnL,
			"unrealizedPnl": p.UnrealizedPnL,
			"openPositions": p.OpenPositions,
			"event":         p.Event,
			"symbol":        p.Symbol,
		})
	}
	return eq
}

func readBody(r *http.Request) ([]byte, error) {
	defer r.Body.Close()
	return io.ReadAll(r.Body)
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		log.Printf("json encode: %v", err)
	}
}
