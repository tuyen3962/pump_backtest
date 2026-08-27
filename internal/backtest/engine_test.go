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
