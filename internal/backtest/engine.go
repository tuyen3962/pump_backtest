package backtest

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/surt/pump_backtest/internal/signal"
)

// Config controls strategy v1 (pump fire by default).
// Units default to SOL for bankroll / size (StartCash=1, Notional=0.05).
// Price proxy remains market-cap USD from signals (ratio ≈ price return).
// Entry fill uses signal mcap (for pump ≈ fireEntryMcap); TP/SL are vs that fill.
type Config struct {
	StartCash      float64  `json:"startCash"`
	EntryKinds     []string `json:"entryKinds"`
	NotionalUSD    float64  `json:"notionalUsd"` // size per entry (SOL in v1)
	FeeBps         float64  `json:"feeBps"`
	MaxPositions   int      `json:"maxPositions"` // 0 = unlimited (still capped by free bankroll)
	CloseOpenAtEnd bool     `json:"closeOpenAtEnd"`

	// ExitOutTriggers: close remaining on these out.trigger values.
	ExitOutTriggers []string `json:"exitOutTriggers"`
	// AlsoExitMustOut: also flatten on out tier=must (underwater/giveback).
	AlsoExitMustOut bool `json:"alsoExitMustOut"`

	// StopLossPct: cut all when mcap return <= -StopLossPct (60 => -60%).
	StopLossPct float64 `json:"stopLossPct"`
	// TakeProfit2x: when mcap/entry >= this, sell FirstTPFraction.
	TakeProfit2x float64 `json:"takeProfit2x"`
	// FirstTPFraction: portion of original size sold at 2x (0.5 = half).
	FirstTPFraction float64 `json:"firstTpFraction"`
	// ScaleTriggerPct: after first TP, each +ScaleTriggerPct from last TP mark sells again.
	ScaleTriggerPct float64 `json:"scaleTriggerPct"`
	// ScaleSellFraction: fraction of *remaining* size sold on each scale step.
	ScaleSellFraction float64 `json:"scaleSellFraction"`

	// LatencySec: assumed signal→fill delay. When LatencyLookahead is true, fill
	// uses the first post-delay mark; otherwise fill = signal×(1+LatencySlipBps).
	LatencySec float64 `json:"latencySec"`
	// LatencySlipBps: adverse entry slip if no look-ahead mark (or Lookahead off).
	LatencySlipBps float64 `json:"latencySlipBps"`
	// LatencyLookahead: if true, resolve fill from a later mark after LatencySec.
	// Default false — look-ahead often prices in the pump and makes 2x TP unreachable.
	LatencyLookahead bool `json:"latencyLookahead"`

	// MinLiquidityUSD / MinVolumeUSD1h: skip entry when market snapshot fails gate.
	MinLiquidityUSD float64 `json:"minLiquidityUsd"`
	MinVolumeUSD1h  float64 `json:"minVolumeUsd1h"`
}

// MarketSnapshot is optional per-mint gate data (from token enrich).
type MarketSnapshot struct {
	LiquidityUSD float64 `json:"liquidityUsd"`
	VolumeUSD1h  float64 `json:"volumeUsd1h"`
	VolumeUSD24h float64 `json:"volumeUsd24h"`
	RugScore     float64 `json:"rugScore"`
}

func DefaultConfig() Config {
	return Config{
		StartCash:         1,    // 1 SOL bankroll
		NotionalUSD:       0.05, // 0.05 SOL / trade → up to 20 concurrent if cash-gated
		EntryKinds:        []string{signal.KindPump},
		FeeBps:            100, // ~1% round-trip friction proxy
		MaxPositions:      0,   // 0 = no count cap (bankroll still limits)
		CloseOpenAtEnd:    false, // keep open for dashboard follow / multi-hold
		ExitOutTriggers:   []string{"stale", "dev_sold", "whale_dump"},
		AlsoExitMustOut:   true,
		StopLossPct:       60,
		TakeProfit2x:      2.0,
		FirstTPFraction:   0.5,
		ScaleTriggerPct:   15,  // mid of 10–20%
		ScaleSellFraction: 0.15,
		LatencySec:        5,
		LatencySlipBps:    50,
		LatencyLookahead:  false,
		MinLiquidityUSD:   5000,
		MinVolumeUSD1h:    2000,
	}
}

type Side string

const (
	SideBuy  Side = "buy"
	SideSell Side = "sell"
)

type Fill struct {
	Time     time.Time `json:"time"`
	Side     Side      `json:"side"`
	Mint     string    `json:"mint"`
	Symbol   string    `json:"symbol"`
	Kind     string    `json:"kind"`
	Mcap     float64   `json:"mcap"`
	Notional float64   `json:"notional"` // SOL size for this fill
	FeeUSD   float64   `json:"feeUsd"`   // fee in SOL units
	SignalID string    `json:"signalId"`
	Fraction float64   `json:"fraction"` // of original position
}

type Trade struct {
	Mint       string  `json:"mint"`
	Symbol     string  `json:"symbol"`
	Entry      Fill    `json:"entry"`
	Exit       Fill    `json:"exit"`
	ReturnPct  float64 `json:"returnPct"`
	PnLUSD     float64 `json:"pnlUsd"` // PnL in SOL
	HoldSec    float64 `json:"holdSec"`
	ExitReason string  `json:"exitReason"`
	Open       bool    `json:"open"`
}

type EquityPoint struct {
	Time          time.Time `json:"time"`
	Equity        float64   `json:"equity"`
	RealizedPnL   float64   `json:"realizedPnl"`
	UnrealizedPnL float64   `json:"unrealizedPnl"`
	OpenPositions int       `json:"openPositions"`
	Event         string    `json:"event"`
	Symbol        string    `json:"symbol,omitempty"`
}

type CoinStat struct {
	Mint       string  `json:"mint"`
	Symbol     string  `json:"symbol"`
	Trades     int     `json:"trades"`
	Open       bool    `json:"open"`
	PnLUSD     float64 `json:"pnlUsd"`
	ReturnPct  float64 `json:"returnPct"`
	EntryMcap  float64 `json:"entryMcap"`
	ExitMcap   float64 `json:"exitMcap"`
	EntryKind  string  `json:"entryKind"`
	ExitReason string  `json:"exitReason"`
	HoldSec    float64 `json:"holdSec"`
	Remaining  float64 `json:"remaining,omitempty"`

	VolumeUSD24h   float64 `json:"volumeUsd24h,omitempty"`
	VolumeUSD1h    float64 `json:"volumeUsd1h,omitempty"`
	LiquidityUSD   float64 `json:"liquidityUsd,omitempty"`
	SellRatio1h    float64 `json:"sellRatio1h,omitempty"`
	RugScore       float64 `json:"rugScore,omitempty"`
	RugLabel       string  `json:"rugLabel,omitempty"`
	MarketCapUSD   float64 `json:"marketCapUsd,omitempty"`
	ATHDrawdownPct float64 `json:"athDrawdownPct,omitempty"`
	Complete       bool    `json:"complete,omitempty"`
}

type Result struct {
	Config      Config        `json:"config"`
	Signals     int           `json:"signals"`
	Trades      []Trade       `json:"trades"`
	Coins       []CoinStat    `json:"coins"`
	Equity      []EquityPoint `json:"equity"`
	OpenCount   int           `json:"openCount"`
	ClosedCount int           `json:"closedCount"`
	Wins        int           `json:"wins"`
	Losses      int           `json:"losses"`
	TotalPnL    float64       `json:"totalPnl"`
	TotalFees   float64       `json:"totalFees"`
	WinRate     float64       `json:"winRate"`
	AvgReturn   float64       `json:"avgReturn"`
	EndEquity   float64       `json:"endEquity"`
	MaxEquity   float64       `json:"maxEquity"`
	MinEquity   float64       `json:"minEquity"`
	MaxDrawdown float64       `json:"maxDrawdownPct"`
	Skipped     int           `json:"skippedEntries"`
}

type position struct {
	entry      Fill
	sizeSOL    float64
	remaining  float64 // 0..1 of original
	lastMcap   float64
	peakMcap   float64 // high-water since entry (for TP cross detection)
	lastTime   time.Time
	lastKind   string
	lastSigID  string
	halfTaken  bool
	lastTPMcap float64
	realized   float64
}

func (p *position) unrealized() float64 {
	if p.remaining <= 0 || p.entry.Mcap <= 0 || p.lastMcap <= 0 {
		return 0
	}
	ret := (p.lastMcap - p.entry.Mcap) / p.entry.Mcap
	return p.sizeSOL * p.remaining * ret
}

// Run replays records with strategy v1 rules.
func Run(records []signal.Record, cfg Config) (Result, error) {
	return RunWithMarket(records, cfg, nil)
}

func RunWithMarket(records []signal.Record, cfg Config, market map[string]MarketSnapshot) (Result, error) {
	cfg = applyConfigDefaults(cfg)
	if cfg.NotionalUSD <= 0 {
		return Result{}, fmt.Errorf("notional must be > 0")
	}
	entrySet := toSet(cfg.EntryKinds)
	if len(entrySet) == 0 {
		return Result{}, fmt.Errorf("EntryKinds is empty")
	}
	outTriggers := toSet(cfg.ExitOutTriggers)
	feeRate := cfg.FeeBps / 10_000 / 2

	positions := map[string]*position{}
	var trades []Trade
	var equity []EquityPoint
	var totalFees float64
	realized := 0.0
	skipped := 0

	snapshot := func(t time.Time, event, symbol string) {
		unreal := 0.0
		openN := 0
		for _, pos := range positions {
			if pos.remaining > 1e-9 {
				unreal += pos.unrealized()
				openN++
			}
		}
		equity = append(equity, EquityPoint{
			Time:          t,
			Equity:        cfg.StartCash + realized + unreal,
			RealizedPnL:   realized,
			UnrealizedPnL: unreal,
			OpenPositions: openN,
			Event:         event,
			Symbol:        symbol,
		})
	}

	if len(records) > 0 {
		snapshot(records[0].EventTime(), "start", "")
	} else {
		snapshot(time.Now().UTC(), "start", "")
	}

	sellFrac := func(mint string, fracOfOriginal float64, exit Fill, reason string, open bool) {
		pos := positions[mint]
		if pos == nil || pos.remaining <= 1e-9 || fracOfOriginal <= 0 {
			return
		}
		if fracOfOriginal > pos.remaining {
			fracOfOriginal = pos.remaining
		}
		ret := 0.0
		if pos.entry.Mcap > 0 && exit.Mcap > 0 {
			ret = (exit.Mcap - pos.entry.Mcap) / pos.entry.Mcap
		}
		notional := pos.sizeSOL * fracOfOriginal
		exit.Notional = notional
		exit.Fraction = fracOfOriginal
		exit.FeeUSD = notional * feeRate
		// Charge entry fee once on the first exit leg.
		feeEntry := 0.0
		if pos.remaining >= 0.999 {
			feeEntry = pos.entry.FeeUSD
		}
		pnl := notional*ret - feeEntry - exit.FeeUSD
		tr := Trade{
			Mint:       mint,
			Symbol:     pos.entry.Symbol,
			Entry:      pos.entry,
			Exit:       exit,
			ReturnPct:  ret * 100,
			PnLUSD:     pnl,
			HoldSec:    exit.Time.Sub(pos.entry.Time).Seconds(),
			ExitReason: reason,
			Open:       open,
		}
		trades = append(trades, tr)
		totalFees += feeEntry + exit.FeeUSD
		realized += pnl
		pos.realized += pnl
		pos.remaining -= fracOfOriginal
		if pos.remaining <= 1e-9 {
			pos.remaining = 0
			delete(positions, mint)
		}
		ev := "exit"
		if open {
			ev = "eod"
		} else if strings.HasPrefix(reason, "tp_") || strings.HasPrefix(reason, "scale_") {
			ev = "tp"
		}
		snapshot(exit.Time, ev, pos.entry.Symbol)
	}

	closeAll := func(mint string, exit Fill, reason string, open bool) {
		pos := positions[mint]
		if pos == nil {
			return
		}
		sellFrac(mint, pos.remaining, exit, reason, open)
	}

	for i, rec := range records {
		mint := rec.Raw.Mint
		if mint == "" {
			continue
		}
		kind := rec.Raw.Kind
		mcap := rec.Mcap()
		ts := rec.EventTime()
		sym := rec.Symbol()

		if pos, ok := positions[mint]; ok && pos.remaining > 0 {
			if mcap > 0 {
				pos.lastMcap = mcap
				pos.lastTime = ts
				pos.lastKind = kind
				pos.lastSigID = rec.Raw.ID
				if mcap > pos.peakMcap {
					pos.peakMcap = mcap
				}
				// Out/payload may carry a session peak higher than this tick.
				if rec.Payload.PeakMcap > pos.peakMcap {
					pos.peakMcap = rec.Payload.PeakMcap
				}
				snapshot(ts, "mark:"+kind, sym)

				mult := mcap / pos.entry.Mcap
				peakMult := 0.0
				if pos.entry.Mcap > 0 {
					peakMult = pos.peakMcap / pos.entry.Mcap
				}
				// Hard stop -60% (on current mark)
				if cfg.StopLossPct > 0 && mult <= (1-cfg.StopLossPct/100) {
					closeAll(mint, sellFill(ts, mint, sym, kind, mcap, rec.Raw.ID), "stop_loss", false)
					continue
				}
				// First TP at 2x → sell half. If peak crossed TP but this tick is
				// still ≥ TP, fill at threshold; if tick already below TP after a
				// gap, we missed the print (no synthetic fill above last mark).
				if !pos.halfTaken && cfg.TakeProfit2x > 0 && peakMult >= cfg.TakeProfit2x {
					exitMcap := mcap
					target := pos.entry.Mcap * cfg.TakeProfit2x
					if mcap >= target {
						exitMcap = target // crossed this tick — fill at TP
					} else if peakMult >= cfg.TakeProfit2x && mult >= cfg.TakeProfit2x {
						exitMcap = mcap
					} else {
						// Peak was ≥ TP earlier but we never observed a mark ≥ TP
						// (shouldn't happen if peak comes from marks). If peak only
						// from PeakMcap field, sell at target capped by peak.
						if rec.Payload.PeakMcap >= target {
							exitMcap = target
						} else {
							exitMcap = 0 // skip — cannot honestly fill
						}
					}
					if exitMcap > 0 {
						sellFrac(mint, cfg.FirstTPFraction, sellFill(ts, mint, sym, kind, exitMcap, rec.Raw.ID), "tp_2x_half", false)
						if p2 := positions[mint]; p2 != nil {
							p2.halfTaken = true
							p2.lastTPMcap = exitMcap
						}
					}
				} else if pos.halfTaken && cfg.ScaleTriggerPct > 0 && pos.lastTPMcap > 0 && mcap >= pos.lastTPMcap*(1+cfg.ScaleTriggerPct/100) {
					fracOrig := pos.remaining * cfg.ScaleSellFraction
					sellFrac(mint, fracOrig, sellFill(ts, mint, sym, kind, mcap, rec.Raw.ID), "scale_tp", false)
					if p2 := positions[mint]; p2 != nil {
						p2.lastTPMcap = mcap
					}
				}
			}

			if kind == signal.KindOut {
				trigger := rec.Payload.Trigger
				tier := rec.Payload.Tier
				should := false
				if _, ok := outTriggers[trigger]; ok {
					should = true
				}
				if cfg.AlsoExitMustOut && tier == "must" {
					should = true
				}
				if should {
					exitMcap := mcap
					if exitMcap <= 0 {
						exitMcap = pos.lastMcap
					}
					if exitMcap <= 0 {
						exitMcap = pos.entry.Mcap
					}
					reason := "out:" + tier + ":" + trigger
					closeAll(mint, sellFill(ts, mint, sym, kind, exitMcap, rec.Raw.ID), reason, false)
				}
			}
			continue
		}

		if _, want := entrySet[kind]; !want {
			continue
		}
		if mcap <= 0 {
			continue
		}
		if cfg.MaxPositions > 0 && countOpen(positions) >= cfg.MaxPositions {
			skipped++
			continue
		}
		// Reserve notional from bankroll so multiple concurrent holds share StartCash.
		deployed := deployedNotional(positions)
		freeCash := cfg.StartCash + realized - deployed
		if freeCash+1e-12 < cfg.NotionalUSD {
			skipped++
			continue
		}
		if !passMarketGate(mint, market, cfg) {
			skipped++
			continue
		}

		fillMcap := resolveEntryMcap(records, i, mint, mcap, ts, cfg)
		fee := cfg.NotionalUSD * feeRate
		positions[mint] = &position{
			entry: Fill{
				Time:     ts,
				Side:     SideBuy,
				Mint:     mint,
				Symbol:   sym,
				Kind:     kind,
				Mcap:     fillMcap,
				Notional: cfg.NotionalUSD,
				FeeUSD:   fee,
				SignalID: rec.Raw.ID,
				Fraction: 1,
			},
			sizeSOL:   cfg.NotionalUSD,
			remaining: 1,
			lastMcap:  fillMcap,
			peakMcap:  fillMcap,
			lastTime:  ts,
			lastKind:  kind,
			lastSigID: rec.Raw.ID,
		}
		snapshot(ts, "entry:"+kind, sym)
	}

	if cfg.CloseOpenAtEnd {
		var mints []string
		for mint := range positions {
			mints = append(mints, mint)
		}
		sort.Slice(mints, func(i, j int) bool {
			return positions[mints[i]].entry.Time.Before(positions[mints[j]].entry.Time)
		})
		for _, mint := range mints {
			pos := positions[mint]
			exitMcap := pos.lastMcap
			if exitMcap <= 0 {
				exitMcap = pos.entry.Mcap
			}
			closeAll(mint, sellFill(pos.lastTime, mint, pos.entry.Symbol, pos.lastKind, exitMcap, pos.lastSigID), "eod_mark", false)
		}
	}

	res := Result{
		Config:    cfg,
		Signals:   len(records),
		Trades:    trades,
		Equity:    equity,
		TotalFees: totalFees,
		Skipped:   skipped,
		EndEquity: cfg.StartCash + realized,
	}
	finalizeStats(&res)
	res.Coins = aggregateCoins(trades, positions)
	// Open/closed counts from coin rows (includes never-exited holds).
	res.OpenCount = 0
	res.ClosedCount = 0
	for _, c := range res.Coins {
		if c.Open {
			res.OpenCount++
		} else {
			res.ClosedCount++
		}
	}
	return res, nil
}

func deployedNotional(positions map[string]*position) float64 {
	var sum float64
	for _, p := range positions {
		if p == nil || p.remaining <= 1e-9 {
			continue
		}
		sum += p.sizeSOL * p.remaining
	}
	return sum
}

func applyConfigDefaults(cfg Config) Config {
	d := DefaultConfig()
	if cfg.StartCash <= 0 {
		cfg.StartCash = d.StartCash
	}
	if cfg.NotionalUSD <= 0 {
		cfg.NotionalUSD = d.NotionalUSD
	}
	if len(cfg.EntryKinds) == 0 {
		cfg.EntryKinds = d.EntryKinds
	}
	if len(cfg.ExitOutTriggers) == 0 {
		cfg.ExitOutTriggers = d.ExitOutTriggers
	}
	if cfg.StopLossPct <= 0 {
		cfg.StopLossPct = d.StopLossPct
	}
	if cfg.TakeProfit2x <= 0 {
		cfg.TakeProfit2x = d.TakeProfit2x
	}
	if cfg.FirstTPFraction <= 0 {
		cfg.FirstTPFraction = d.FirstTPFraction
	}
	if cfg.ScaleTriggerPct <= 0 {
		cfg.ScaleTriggerPct = d.ScaleTriggerPct
	}
	if cfg.ScaleSellFraction <= 0 {
		cfg.ScaleSellFraction = d.ScaleSellFraction
	}
	if cfg.LatencySec < 0 {
		cfg.LatencySec = d.LatencySec
	}
	if cfg.LatencySlipBps < 0 {
		cfg.LatencySlipBps = d.LatencySlipBps
	}
	return cfg
}

func toSet(items []string) map[string]struct{} {
	out := make(map[string]struct{}, len(items))
	for _, k := range items {
		k = strings.TrimSpace(k)
		if k != "" {
			out[k] = struct{}{}
		}
	}
	return out
}

func countOpen(positions map[string]*position) int {
	n := 0
	for _, p := range positions {
		if p.remaining > 1e-9 {
			n++
		}
	}
	return n
}

func sellFill(ts time.Time, mint, sym, kind string, mcap float64, id string) Fill {
	return Fill{
		Time:     ts,
		Side:     SideSell,
		Mint:     mint,
		Symbol:   sym,
		Kind:     kind,
		Mcap:     mcap,
		SignalID: id,
	}
}

func resolveEntryMcap(records []signal.Record, idx int, mint string, signalMcap float64, ts time.Time, cfg Config) float64 {
	slip := cfg.LatencySlipBps / 10_000
	adverse := signalMcap * (1 + slip)
	if !cfg.LatencyLookahead || cfg.LatencySec <= 0 {
		return adverse
	}
	deadline := ts.Add(time.Duration(cfg.LatencySec * float64(time.Second)))
	for j := idx + 1; j < len(records); j++ {
		r := records[j]
		if r.Raw.Mint != mint {
			continue
		}
		if r.EventTime().Before(deadline) {
			continue
		}
		if m := r.Mcap(); m > 0 {
			// Cap look-ahead chase so a 5s pump doesn't become the entry basis for 2x TP.
			cap := signalMcap * 1.25
			if m > cap {
				return cap
			}
			if m < signalMcap {
				return adverse // worse than signal — keep adverse slip
			}
			return m
		}
	}
	return adverse
}

func passMarketGate(mint string, market map[string]MarketSnapshot, cfg Config) bool {
	if market == nil {
		return true
	}
	snap, ok := market[mint]
	if !ok {
		// No snapshot yet — allow (recorder-only runs); enrich path should prefill.
		return true
	}
	if cfg.MinLiquidityUSD > 0 && snap.LiquidityUSD > 0 && snap.LiquidityUSD < cfg.MinLiquidityUSD {
		return false
	}
	if cfg.MinVolumeUSD1h > 0 && snap.VolumeUSD1h > 0 && snap.VolumeUSD1h < cfg.MinVolumeUSD1h {
		return false
	}
	// If thresholds set but snapshot zeros (failed fetch), don't block.
	return true
}

func finalizeStats(res *Result) {
	if len(res.Equity) > 0 {
		res.EndEquity = res.Equity[len(res.Equity)-1].Equity
		res.MaxEquity = res.Equity[0].Equity
		res.MinEquity = res.Equity[0].Equity
		peak := res.Equity[0].Equity
		maxDD := 0.0
		for _, p := range res.Equity {
			if p.Equity > res.MaxEquity {
				res.MaxEquity = p.Equity
			}
			if p.Equity < res.MinEquity {
				res.MinEquity = p.Equity
			}
			if p.Equity > peak {
				peak = p.Equity
			}
			if peak > 0 {
				dd := (peak - p.Equity) / peak * 100
				if dd > maxDD {
					maxDD = dd
				}
			}
		}
		res.MaxDrawdown = maxDD
	}
	var sumRet float64
	for _, t := range res.Trades {
		res.TotalPnL += t.PnLUSD
		sumRet += t.ReturnPct
		if t.PnLUSD >= 0 {
			res.Wins++
		} else {
			res.Losses++
		}
	}
	n := len(res.Trades)
	if n > 0 {
		res.WinRate = float64(res.Wins) / float64(n) * 100
		res.AvgReturn = sumRet / float64(n)
	}
}

func aggregateCoins(trades []Trade, stillOpen map[string]*position) []CoinStat {
	type agg struct {
		stat CoinStat
	}
	order := []string{}
	m := map[string]*agg{}
	for _, t := range trades {
		key := t.Entry.SignalID
		if key == "" {
			key = t.Mint + "|" + t.Entry.Time.String()
		}
		a, ok := m[key]
		if !ok {
			a = &agg{stat: CoinStat{
				Mint:      t.Mint,
				Symbol:    t.Symbol,
				EntryMcap: t.Entry.Mcap,
				EntryKind: t.Entry.Kind,
			}}
			m[key] = a
			order = append(order, key)
		}
		a.stat.Trades++
		a.stat.PnLUSD += t.PnLUSD
		a.stat.ExitMcap = t.Exit.Mcap
		a.stat.ExitReason = t.ExitReason
		a.stat.HoldSec = t.HoldSec
		a.stat.Open = t.Open
		if t.Entry.Mcap > 0 && t.Exit.Mcap > 0 {
			a.stat.ReturnPct = (t.Exit.Mcap - t.Entry.Mcap) / t.Entry.Mcap * 100
		}
	}
	for _, pos := range stillOpen {
		if pos.remaining <= 0 {
			continue
		}
		key := pos.entry.SignalID
		if key == "" {
			key = pos.entry.Mint + "|" + pos.entry.Time.String()
		}
		a, ok := m[key]
		if !ok {
			exitMcap := pos.lastMcap
			if exitMcap <= 0 {
				exitMcap = pos.entry.Mcap
			}
			ret := 0.0
			if pos.entry.Mcap > 0 && exitMcap > 0 {
				ret = (exitMcap - pos.entry.Mcap) / pos.entry.Mcap * 100
			}
			a = &agg{stat: CoinStat{
				Mint:       pos.entry.Mint,
				Symbol:     pos.entry.Symbol,
				EntryMcap:  pos.entry.Mcap,
				EntryKind:  pos.entry.Kind,
				ExitMcap:   exitMcap,
				ExitReason: "open",
				ReturnPct:  ret,
				HoldSec:    pos.lastTime.Sub(pos.entry.Time).Seconds(),
				Open:       true,
				Remaining:  pos.remaining,
				Trades:     0,
			}}
			m[key] = a
			order = append(order, key)
			continue
		}
		a.stat.Open = true
		a.stat.Remaining = pos.remaining
	}
	out := make([]CoinStat, 0, len(order))
	for _, key := range order {
		out = append(out, m[key].stat)
	}
	return out
}
