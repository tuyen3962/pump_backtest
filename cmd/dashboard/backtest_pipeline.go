package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
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

	// FromTime / ToTime: RFC3339 window filter on signal EventTime (historical slice).
	FromTime string `json:"fromTime"`
	ToTime   string `json:"toTime"`
	// SessionEndAt: RFC3339 — if set with async, keep re-running until this time.
	SessionEndAt string `json:"sessionEndAt"`
	// SessionRefreshSec: how often to reload live signals during a session (default 60).
	SessionRefreshSec int `json:"sessionRefreshSec"`
}

type persistKind int

const (
	persistFull persistKind = iota
	// persistSnapshot: overwrite run + optional watchlist; skip history append spam.
	persistSnapshot
)

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
	dataDir       string
	st            *store.Store
	runner        *jobs.Runner
	mu            sync.Mutex
	pending       map[string]jobPayload
	sessionMu     sync.Mutex
	activeSession string // job id of running stability session
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

	req := p.Req
	sessionEndRaw := strings.TrimSpace(req.SessionEndAt)
	endAt, endErr := parseRFC3339(sessionEndRaw)
	if sessionEndRaw != "" {
		defer func() {
			h.sessionMu.Lock()
			if h.activeSession == job.ID {
				h.activeSession = ""
			}
			h.sessionMu.Unlock()
		}()
		if endErr != nil || endAt.IsZero() {
			return fmt.Errorf("invalid sessionEndAt %q: %w", sessionEndRaw, endErr)
		}
		if !endAt.After(time.Now().UTC()) {
			return fmt.Errorf("sessionEndAt is in the past")
		}
	} else if endAt.IsZero() {
		updateWatch := false
		if req.UpdateWatchlist != nil {
			updateWatch = *req.UpdateWatchlist
		}
		h.runner.SetProgress(job.ID, "loading signals")
		out, err := h.execute(ctx, req, p.ReqBytes, job.ID, updateWatch, persistFull)
		if err != nil {
			return err
		}
		job.RunID = out.RunID
		h.runner.SetRunID(job.ID, out.RunID)
		if job.Label == "" {
			job.Label = req.Label
		}
		h.runner.SetProgress(job.ID, "done")
		return nil
	}

	// Live stability session: from now (or FromTime) until SessionEndAt.
	if strings.TrimSpace(req.FromTime) == "" {
		req.FromTime = time.Now().UTC().Format(time.RFC3339)
	}
	refresh := req.SessionRefreshSec
	if refresh < 30 {
		refresh = 60
	}
	updateWatch := true
	if req.UpdateWatchlist != nil {
		updateWatch = *req.UpdateWatchlist
	}

	sessionCtx, sessionCancel := context.WithDeadline(ctx, endAt)
	defer sessionCancel()

	for tickN := 0; ; tickN++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		now := time.Now().UTC()
		final := !now.Before(endAt)
		tick := req
		if final {
			tick.ToTime = endAt.UTC().Format(time.RFC3339)
			tick.CloseOpenAtEnd = true
		} else {
			tick.ToTime = now.Format(time.RFC3339)
			tick.CloseOpenAtEnd = false
		}
		phase := "tick"
		if final {
			phase = "final"
		}
		h.runner.SetProgress(job.ID, fmt.Sprintf("session %s → %s · %s %s",
			phase, endAt.Local().Format("15:04"), now.Local().Format("15:04:05"),
			map[bool]string{true: "(closing)", false: ""}[final]))

		mode := persistSnapshot
		if final {
			mode = persistFull
		}
		out, err := h.execute(sessionCtx, tick, p.ReqBytes, job.ID, updateWatch, mode)
		if err != nil {
			if final && (errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled)) {
				// Deadline hit mid-final — still treat as complete.
				h.runner.SetProgress(job.ID, "session complete")
				return nil
			}
			return err
		}
		job.RunID = out.RunID
		h.runner.SetRunID(job.ID, out.RunID)
		if job.Label == "" {
			job.Label = req.Label
		}
		if final {
			h.runner.SetProgress(job.ID, "session complete")
			return nil
		}

		// execute (enrich) may take minutes — re-check wall clock before sleeping.
		now = time.Now().UTC()
		if !now.Before(endAt) {
			continue
		}
		sleep := time.Duration(refresh) * time.Second
		if d := time.Until(endAt); d < sleep {
			sleep = d
		}
		if sleep <= 0 {
			continue
		}
		timer := time.NewTimer(sleep)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-sessionCtx.Done():
			timer.Stop()
			if errors.Is(sessionCtx.Err(), context.DeadlineExceeded) {
				continue // run final tick
			}
			return sessionCtx.Err()
		case <-timer.C:
		}
		_ = tickN
	}
}

func (h *backtestHub) enqueue(req backtestRequest, reqBytes []byte) (*jobs.Job, error) {
	id := newRunID()
	if req.Label == "" {
		if req.SessionEndAt != "" {
			req.Label = "session " + id
		} else {
			req.Label = "run " + id
		}
	}

	if strings.TrimSpace(req.SessionEndAt) != "" {
		endAt, err := parseRFC3339(req.SessionEndAt)
		if err != nil || endAt.IsZero() {
			return nil, fmt.Errorf("invalid sessionEndAt: %w", err)
		}
		if !endAt.After(time.Now().UTC()) {
			return nil, fmt.Errorf("sessionEndAt must be in the future")
		}
		h.sessionMu.Lock()
		if prev := h.activeSession; prev != "" {
			h.runner.Cancel(prev)
		}
		h.activeSession = id
		h.sessionMu.Unlock()
	}

	h.mu.Lock()
	h.pending[id] = jobPayload{ReqBytes: reqBytes, Req: req}
	h.mu.Unlock()
	job, err := h.runner.Enqueue(id, req.Label)
	if err != nil {
		h.mu.Lock()
		delete(h.pending, id)
		h.mu.Unlock()
		if strings.TrimSpace(req.SessionEndAt) != "" {
			h.sessionMu.Lock()
			if h.activeSession == id {
				h.activeSession = ""
			}
			h.sessionMu.Unlock()
		}
		return nil, err
	}
	return job, nil
}

func (h *backtestHub) execute(ctx context.Context, req backtestRequest, reqBytes []byte, preferredID string, updateWatchlist bool, persist persistKind) (*backtestOutcome, error) {
	path := resolveSourcePath(h.dataDir, req.Source)
	records, err := signal.LoadNDJSON(path)
	if err != nil {
		return nil, fmt.Errorf("load: %w", err)
	}
	from, _ := parseRFC3339(req.FromTime)
	to, _ := parseRFC3339(req.ToTime)
	before := len(records)
	records = signal.FilterTimeRange(records, from, to)

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

	slimRes := res
	slimRes.Trades = nil
	slimRes.Equity = backtest.DownsampleEquity(res.Equity, 400)

	payload := map[string]any{
		"id":           runID,
		"label":        req.Label,
		"source":       path,
		"loaded":       len(records),
		"loadedAll":    before,
		"fromTime":     req.FromTime,
		"toTime":       req.ToTime,
		"sessionEndAt": req.SessionEndAt,
		"result":       slimRes,
		"equity":       equityDTO(slimRes),
		"updated":      time.Now().UTC().Format(time.RFC3339),
	}
	respBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	if tradesJSON, err := json.Marshal(res.Trades); err == nil {
		if eqJSON, err := json.Marshal(res.Equity); err == nil {
			if err := h.st.SaveRunDetail(runID, store.RunDetail{
				Trades: tradesJSON,
				Equity: eqJSON,
			}); err != nil {
				log.Printf("save run detail %s: %v", runID, err)
			}
		}
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

	switch persist {
	case persistSnapshot:
		if updateWatchlist {
			persistWatchOnly(h.st, res)
		}
	default:
		if updateWatchlist {
			persistHistoryAndWatch(h.st, runID, res)
		} else {
			persistHistoryOnly(h.st, runID, res)
		}
	}

	return &backtestOutcome{RunID: runID, Payload: payload, Result: res, Path: path}, nil
}

func parseRFC3339(s string) (time.Time, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, nil
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t.UTC(), nil
	}
	// datetime-local from browsers: 2026-08-27T21:00
	if t, err := time.ParseInLocation("2006-01-02T15:04", s, time.Local); err == nil {
		return t.UTC(), nil
	}
	if t, err := time.ParseInLocation("2006-01-02T15:04:05", s, time.Local); err == nil {
		return t.UTC(), nil
	}
	return time.Time{}, fmt.Errorf("bad time %q", s)
}

func persistWatchOnly(st *store.Store, res backtest.Result) {
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
	if _, err := st.ReplaceSourceWatches("backtest_open", openWatch); err != nil {
		log.Printf("watchlist sync: %v", err)
	}
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
		status := store.ClassifyStatus(t.ExitReason, 0, "", false)
		var rugScore float64
		var rugLabel string
		for _, c := range res.Coins {
			if c.Mint == t.Mint {
				rugScore = c.RugScore
				rugLabel = c.RugLabel
				status = store.ClassifyStatus(t.ExitReason, rugScore, rugLabel, false)
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

func resultFromSaved(run *store.SavedRun, st *store.Store) (backtest.Result, map[string]any, error) {
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
	if st != nil {
		if detail, err := st.LoadRunDetail(run.ID); err == nil && detail != nil {
			if len(detail.Trades) > 0 {
				_ = json.Unmarshal(detail.Trades, &res.Trades)
			}
			if len(detail.Equity) > 0 {
				var fullEq []backtest.EquityPoint
				if json.Unmarshal(detail.Equity, &fullEq) == nil && len(fullEq) > 0 {
					res.Equity = fullEq
				}
			}
		}
	}
	return res, payload, nil
}

// slimRunPayloadForAPI strips heavy fields from legacy run blobs before sending to the UI.
func slimRunPayloadForAPI(resp map[string]any) map[string]any {
	if resp == nil {
		return resp
	}
	raw, ok := resp["result"]
	if !ok {
		return resp
	}
	b, err := json.Marshal(raw)
	if err != nil {
		return resp
	}
	var res backtest.Result
	if err := json.Unmarshal(b, &res); err != nil {
		return resp
	}
	if len(res.Trades) > 0 {
		res.Trades = nil
	}
	if len(res.Equity) > 400 {
		res.Equity = backtest.DownsampleEquity(res.Equity, 400)
	}
	resp["result"] = res
	if eq, ok := resp["equity"].([]any); ok && len(eq) > 400 {
		resp["equity"] = equityDTO(res)
	} else if eqSlice, ok := resp["equity"].([]map[string]any); ok && len(eqSlice) > 400 {
		resp["equity"] = equityDTO(res)
	}
	return resp
}

func writeCSVDownload(w http.ResponseWriter, filename string, writeFn func(http.ResponseWriter) error) {
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filename))
	if err := writeFn(w); err != nil {
		log.Printf("csv export: %v", err)
	}
}
