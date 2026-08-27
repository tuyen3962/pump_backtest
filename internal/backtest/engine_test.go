package backtest_test

import (
	"encoding/json"
	"testing"

	"github.com/surt/pump_backtest/internal/backtest"
	"github.com/surt/pump_backtest/internal/signal"
)

func TestRunDemoFixture(t *testing.T) {
	recs, err := signal.LoadNDJSON("../../testdata/signals/demo.ndjson")
	if err != nil {
		t.Fatal(err)
	}
	cfg := backtest.DefaultConfig()
	cfg.EntryKinds = []string{signal.KindPump, signal.KindWhaleArmed}
	cfg.FeeBps = 0
	cfg.MinLiquidityUSD = 0
	cfg.MinVolumeUSD1h = 0
	cfg.LatencySec = 0
	cfg.LatencySlipBps = 0
	res, err := backtest.Run(recs, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Equity) < 2 {
		t.Fatalf("expected equity curve, got %d points", len(res.Equity))
	}
	if len(res.Coins) < 1 {
		t.Fatalf("want coins, got %d trades=%d", len(res.Coins), len(res.Trades))
	}
}

func TestStopLossAndWhaleEntry(t *testing.T) {
	recs := []signal.Record{
		rec("a1", signal.KindWhaleArmed, "M1", "AAA", 1_000_000, 100_000),
		rec("a2", signal.KindWhaleArmed, "M1", "AAA", 1_100_000, 35_000), // -65%
	}
	cfg := backtest.DefaultConfig()
	cfg.EntryKinds = []string{signal.KindWhaleArmed}
	cfg.FeeBps = 0
	cfg.MinLiquidityUSD = 0
	cfg.MinVolumeUSD1h = 0
	cfg.LatencySec = 0
	cfg.LatencySlipBps = 0
	cfg.CloseOpenAtEnd = false
	res, err := backtest.Run(recs, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Trades) != 1 {
		t.Fatalf("want 1 stop trade, got %+v", res.Trades)
	}
	if res.Trades[0].ExitReason != "stop_loss" {
		t.Fatalf("want stop_loss, got %s", res.Trades[0].ExitReason)
	}
	if res.Trades[0].ReturnPct > -60 {
		t.Fatalf("want <= -60%%, got %f", res.Trades[0].ReturnPct)
	}
}

func TestTakeProfitHalf(t *testing.T) {
	recs := []signal.Record{
		rec("b1", signal.KindWhaleArmed, "M2", "BBB", 1_000_000, 50_000),
		rec("b2", signal.KindMilestone, "M2", "BBB", 1_200_000, 120_000), // 2.4x
	}
	cfg := backtest.DefaultConfig()
	cfg.EntryKinds = []string{signal.KindWhaleArmed}
	cfg.FeeBps = 0
	cfg.MinLiquidityUSD = 0
	cfg.MinVolumeUSD1h = 0
	cfg.LatencySec = 0
	cfg.LatencySlipBps = 0
	cfg.CloseOpenAtEnd = true
	res, err := backtest.Run(recs, cfg)
	if err != nil {
		t.Fatal(err)
	}
	foundHalf := false
	for _, tr := range res.Trades {
		if tr.ExitReason == "tp_2x_half" {
			foundHalf = true
			if tr.Exit.Fraction < 0.49 || tr.Exit.Fraction > 0.51 {
				t.Fatalf("half fraction=%f", tr.Exit.Fraction)
			}
		}
	}
	if !foundHalf {
		t.Fatalf("missing tp_2x_half in %+v", res.Trades)
	}
}

func TestMultiPositionPumpHold(t *testing.T) {
	recs := []signal.Record{
		rec("p1", signal.KindPump, "M1", "AAA", 1_000_000, 100_000),
		rec("p2", signal.KindPump, "M2", "BBB", 1_001_000, 110_000),
		rec("p3", signal.KindPump, "M3", "CCC", 1_002_000, 120_000),
		rec("m1", signal.KindMilestone, "M1", "AAA", 1_010_000, 150_000),
	}
	cfg := backtest.DefaultConfig()
	cfg.EntryKinds = []string{signal.KindPump}
	cfg.StartCash = 1
	cfg.NotionalUSD = 0.05
	cfg.FeeBps = 0
	cfg.MinLiquidityUSD = 0
	cfg.MinVolumeUSD1h = 0
	cfg.LatencySec = 0
	cfg.LatencySlipBps = 0
	cfg.CloseOpenAtEnd = false
	cfg.MaxPositions = 0
	res, err := backtest.Run(recs, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if res.OpenCount < 3 {
		t.Fatalf("want ≥3 open holds, got open=%d coins=%+v", res.OpenCount, res.Coins)
	}
	if res.Equity[len(res.Equity)-1].OpenPositions < 3 {
		t.Fatalf("equity openPositions=%d", res.Equity[len(res.Equity)-1].OpenPositions)
	}
}

func TestBankrollCapsConcurrentEntries(t *testing.T) {
	recs := []signal.Record{
		rec("p1", signal.KindPump, "M1", "AAA", 1_000_000, 100_000),
		rec("p2", signal.KindPump, "M2", "BBB", 1_001_000, 110_000),
		rec("p3", signal.KindPump, "M3", "CCC", 1_002_000, 120_000),
	}
	cfg := backtest.DefaultConfig()
	cfg.EntryKinds = []string{signal.KindPump}
	cfg.StartCash = 0.1
	cfg.NotionalUSD = 0.05 // only 2 concurrent
	cfg.FeeBps = 0
	cfg.MinLiquidityUSD = 0
	cfg.MinVolumeUSD1h = 0
	cfg.LatencySec = 0
	cfg.LatencySlipBps = 0
	cfg.CloseOpenAtEnd = false
	res, err := backtest.Run(recs, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if res.OpenCount != 2 {
		t.Fatalf("want 2 open (bankroll cap), got %d coins=%+v skipped=%d", res.OpenCount, res.Coins, res.Skipped)
	}
	if res.Skipped < 1 {
		t.Fatalf("want skipped cash-gated entry, got %d", res.Skipped)
	}
}

func TestEodMarkIsClosedTrade(t *testing.T) {
	recs := []signal.Record{
		rec("p1", signal.KindPump, "M1", "AAA", 1_000_000, 100_000),
		rec("m1", signal.KindMilestone, "M1", "AAA", 1_010_000, 120_000),
	}
	cfg := backtest.DefaultConfig()
	cfg.EntryKinds = []string{signal.KindPump}
	cfg.FeeBps = 0
	cfg.MinLiquidityUSD = 0
	cfg.MinVolumeUSD1h = 0
	cfg.LatencySec = 0
	cfg.LatencySlipBps = 0
	cfg.CloseOpenAtEnd = true
	res, err := backtest.Run(recs, cfg)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, tr := range res.Trades {
		if tr.ExitReason == "eod_mark" {
			found = true
			if tr.Open {
				t.Fatalf("eod_mark must be closed (Open=false) for history, got %+v", tr)
			}
		}
	}
	if !found {
		t.Fatalf("missing eod_mark in %+v", res.Trades)
	}
	if res.OpenCount != 0 {
		t.Fatalf("want 0 open after eod, got %d", res.OpenCount)
	}
	if res.ClosedCount < 1 {
		t.Fatalf("want closed coins, got %d", res.ClosedCount)
	}
}

func TestTakeProfitUsesThresholdFill(t *testing.T) {
	recs := []signal.Record{
		rec("e1", signal.KindPump, "M9", "PPP", 1_000_000, 100_000),
		rec("e2", signal.KindMilestone, "M9", "PPP", 1_010_000, 250_000), // 2.5x — should TP at 2.0x
	}
	cfg := backtest.DefaultConfig()
	cfg.EntryKinds = []string{signal.KindPump}
	cfg.FeeBps = 0
	cfg.MinLiquidityUSD = 0
	cfg.MinVolumeUSD1h = 0
	cfg.LatencySec = 0
	cfg.LatencySlipBps = 0
	cfg.CloseOpenAtEnd = false
	cfg.TakeProfit2x = 2.0
	cfg.FirstTPFraction = 0.5
	res, err := backtest.Run(recs, cfg)
	if err != nil {
		t.Fatal(err)
	}
	var tp *backtest.Trade
	for i := range res.Trades {
		if res.Trades[i].ExitReason == "tp_2x_half" {
			tp = &res.Trades[i]
			break
		}
	}
	if tp == nil {
		t.Fatalf("expected tp_2x_half, trades=%+v", res.Trades)
	}
	if tp.Exit.Mcap < 199_000 || tp.Exit.Mcap > 201_000 {
		t.Fatalf("TP fill should be ~2x entry (200k), got %f", tp.Exit.Mcap)
	}
	if res.OpenCount != 1 {
		t.Fatalf("want 1 open remainder, got %d", res.OpenCount)
	}
}

func rec(id, kind, mint, sym string, tsMs, mcap float64) signal.Record {
	p := signal.Payload{Kind: kind, Symbol: sym, McapUsd: mcap}
	raw, _ := json.Marshal(p)
	return signal.Record{
		Raw: signal.Envelope{
			V:       1,
			ID:      id,
			Kind:    kind,
			Ts:      tsMs,
			Mint:    mint,
			Payload: raw,
		},
		Payload: p,
	}
}
