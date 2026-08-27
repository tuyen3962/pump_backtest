package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/surt/pump_backtest/internal/backtest"
	"github.com/surt/pump_backtest/internal/jobs"
	"github.com/surt/pump_backtest/internal/signal"
	"github.com/surt/pump_backtest/internal/store"
	"github.com/surt/pump_backtest/internal/tokeninfo"
)

type backtestRequest struct {
	Source          string   `json:"source"`
	Label           string   `json:"label"`
	EntryKinds      []string `json:"entryKinds"`
	StartCash       float64  `json:"startCash"`
	NotionalUSD     float64  `json:"notionalUsd"`
	FeeBps          float64  `json:"feeBps"`
	MaxPositions    int      `json:"maxPositions"`
	CloseOpenAtEnd  bool     `json:"closeOpenAtEnd"`
	EnrichTokens    bool     `json:"enrichTokens"`
	SampleLive      bool     `json:"sampleLive"`
	AlsoExitMustOut *bool    `json:"alsoExitMustOut"`
	MinLiquidityUSD *float64 `json:"minLiquidityUsd"`
	MinVolumeUSD1h  *float64 `json:"minVolumeUsd1h"`
	LatencySec      *float64 `json:"latencySec"`
	StopLossPct     *float64 `json:"stopLossPct"`
	ScaleTriggerPct *float64 `json:"scaleTriggerPct"`
	TakeProfit2x    *float64 `json:"takeProfit2x"`
	DisableFilters  bool     `json:"disableFilters"`
	Async           bool     `json:"async"`
	// UpdateWatchlist: when false (async compare runs), skip mutating follow watchlist.
	UpdateWatchlist *bool `json:"updateWatchlist"`
}

type backtestOutcome struct {
	RunID   string
	Payload map[string]any
	Result  backtest.Result
	Path    string
}

type jobPayload struct {
	ReqBytes []byte
	Req      backtestRequest
}

// backtestHub owns shared pipeline + async jobs for the dashboard process.
type backtestHub struct {
	dataDir string
	st      *store.Store
	runner  *jobs.Runner
	mu      sync.Mutex
	pending map[string]jobPayload
}

func newBacktestHub(dataDir string, st *store.Store) *backtestHub {
	h := &backtestHub{
		dataDir: dataDir,
		st:      st,
		pending: map[string]jobPayload{},
	}
	h.runner = jobs.New(3, h.runJob)
	return h
}

func (h *backtestHub) runJob(ctx context.Context, job *jobs.Job) error {
	h.mu.Lock()
	p, ok := h.pending[job.ID]
	h.mu.Unlock()
	if !ok {
		return fmt.Errorf("missing job payload")
	}
	defer func() {
		h.mu.Lock()
		delete(h.pending, job.ID)
		h.mu.Unlock()
	}()

	h.runner.SetProgress(job.ID, "loading signals")
	out, err := h.execute(ctx, p.Req, p.ReqBytes, job.ID, false)
	if err != nil {
		return err
	}
	job.RunID = out.RunID
	if job.Label == "" {
		job.Label = p.Req.Label
	}
	h.runner.SetProgress(job.ID, "done")
	return nil
}

func (h *backtestHub) enqueue(req backtestRequest, reqBytes []byte) (*jobs.Job, error) {
	id := newRunID()
	if req.Label == "" {
		req.Label = "run " + id
	}
	h.mu.Lock()
	h.pending[id] = jobPayload{ReqBytes: reqBytes, Req: req}
	h.mu.Unlock()
	job, err := h.runner.Enqueue(id, req.Label)
	if err != nil {
		h.mu.Lock()
		delete(h.pending, id)
		h.mu.Unlock()
		return nil, err
	}
	return job, nil
}

func (h *backtestHub) execute(ctx context.Context, req backtestRequest, reqBytes []byte, preferredID string, updateWatchlist bool) (*backtestOutcome, error) {
	path := resolveSourcePath(h.dataDir, req.Source)
	records, err := signal.LoadNDJSON(path)
	if err != nil {
		return nil, fmt.Errorf("load: %w", err)
	}

	cfg := buildConfig(req)
	var market map[string]backtest.MarketSnapshot
	if req.EnrichTokens {
		ectx, cancel := context.WithTimeout(ctx, 3*time.Minute)
		market = backtest.MarketFromRecords(ectx, records, tokeninfo.Options{
			SampleLive: req.SampleLive,
			LiveWindow: 6 * time.Second,
		})
		cancel()
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
	}

	res, err := backtest.RunWithMarket(records, cfg, market)
	if err != nil {
		return nil, fmt.Errorf("run: %w", err)
	}
	if req.EnrichTokens {
		ectx, cancel := context.WithTimeout(ctx, 90*time.Second)
		backtest.EnrichCoins(ectx, &res, tokeninfo.Options{})
		cancel()
	}

	runID := preferredID
	if runID == "" {
		runID = newRunID()
	}
	payload := map[string]any{
		"id":      runID,
		"label":   req.Label,
		"source":  path,
		"loaded":  len(records),
		"result":  res,
		"equity":  equityDTO(res),
		"updated": time.Now().UTC().Format(time.RFC3339),
	}
	respBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	saved := store.SavedRun{
		ID:          runID,
		Label:       req.Label,
		Request:     reqBytes,
		Response:    respBytes,
		Source:      path,
		EndEquity:   res.EndEquity,
		TotalPnL:    res.TotalPnL,
		CoinCount:   len(res.Coins),
		WinRate:     res.WinRate,
		MaxDDPct:    res.MaxDrawdown,
		OpenCount:   res.OpenCount,
		ClosedCount: res.ClosedCount,
		Signals:     res.Signals,
		EntryKinds:  cfg.EntryKinds,
	}
	if err := h.st.SaveRun(saved); err != nil {
		log.Printf("save run %s: %v", runID, err)
	}

	if updateWatchlist {
		persistHistoryAndWatch(h.st, runID, res)
	} else {
		persistHistoryOnly(h.st, runID, res)
	}

	return &backtestOutcome{RunID: runID, Payload: payload, Result: res, Path: path}, nil
}

func resolveSourcePath(dataDir, source string) string {
	switch {
	case source == "" || source == "live":
		return dataDir
	case source == "demo":
		return "testdata/signals/demo.ndjson"
	case strings.HasPrefix(source, "file:"):
		name := filepath.Base(strings.TrimPrefix(source, "file:"))
		return filepath.Join(dataDir, name)
	default:
		return source
	}
}

func newRunID() string {
	var b [4]byte
	_, _ = rand.Read(b[:])
	return time.Now().UTC().Format("20060102_150405") + "_" + hex.EncodeToString(b[:])
}

func persistHistoryOnly(st *store.Store, runID string, res backtest.Result) {
	now := time.Now().UTC()
	var hist []store.HistoryEntry
	for _, t := range res.Trades {
		if t.Open {
			continue
		}
		status := store.ClassifyStatus(t.ExitReason, 0, false)
		var rugScore float64
		var rugLabel string
		for _, c := range res.Coins {
			if c.Mint == t.Mint {
				rugScore = c.RugScore
				rugLabel = c.RugLabel
				status = store.ClassifyStatus(t.ExitReason, rugScore, false)
				break
			}
		}
		hist = append(hist, store.HistoryEntry{
			Mint:       t.Mint,
			Symbol:     t.Symbol,
			Status:     status,
			ExitReason: t.ExitReason,
			EntryKind:  t.Entry.Kind,
			EntryMcap:  t.Entry.Mcap,
			ExitMcap:   t.Exit.Mcap,
			ReturnPct:  t.ReturnPct,
			PnLSOL:     t.PnLUSD,
			RugScore:   rugScore,
			RugLabel:   rugLabel,
			HoldSec:    t.HoldSec,
			ClosedAt:   now,
			RunID:      runID,
		})
	}
	if err := st.AppendHistory(hist); err != nil {
		log.Printf("history append (%d): %v", len(hist), err)
	}
}

func persistHistoryAndWatch(st *store.Store, runID string, res backtest.Result) {
	persistHistoryOnly(st, runID, res)
	now := time.Now().UTC()
	var openWatch []store.WatchItem
	seenOpen := map[string]bool{}
	for _, c := range res.Coins {
		if !c.Open || c.Mint == "" || seenOpen[c.Mint] {
			continue
		}
		seenOpen[c.Mint] = true
		openWatch = append(openWatch, store.WatchItem{
			Mint:      c.Mint,
			Symbol:    c.Symbol,
			EntryMcap: c.EntryMcap,
			AddedAt:   now,
			Source:    "backtest_open",
		})
	}
	for _, t := range res.Trades {
		if t.ExitReason != "eod_mark" || t.Mint == "" || seenOpen[t.Mint] {
			continue
		}
		seenOpen[t.Mint] = true
		openWatch = append(openWatch, store.WatchItem{
			Mint:      t.Mint,
			Symbol:    t.Symbol,
			EntryMcap: t.Entry.Mcap,
			AddedAt:   now,
			Source:    "backtest_open",
		})
	}
	if _, err := st.ReplaceSourceWatches("backtest_open", openWatch); err != nil {
		log.Printf("watchlist sync: %v", err)
	}
}

func resultFromSaved(run *store.SavedRun) (backtest.Result, map[string]any, error) {
	var payload map[string]any
	if err := json.Unmarshal(run.Response, &payload); err != nil {
		return backtest.Result{}, nil, err
	}
	raw, ok := payload["result"]
	if !ok {
		return backtest.Result{}, payload, fmt.Errorf("no result in run")
	}
	b, err := json.Marshal(raw)
	if err != nil {
		return backtest.Result{}, payload, err
	}
	var res backtest.Result
	if err := json.Unmarshal(b, &res); err != nil {
		return backtest.Result{}, payload, err
	}
	return res, payload, nil
}

func writeCSVDownload(w http.ResponseWriter, filename string, writeFn func(http.ResponseWriter) error) {
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filename))
	if err := writeFn(w); err != nil {
		log.Printf("csv export: %v", err)
	}
}
