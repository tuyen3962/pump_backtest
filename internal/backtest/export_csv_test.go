package backtest_test

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/surt/pump_backtest/internal/backtest"
)

func TestWriteTradesCSV(t *testing.T) {
	res := backtest.Result{
		Trades: []backtest.Trade{{
			Mint:   "Mint1",
			Symbol: "AAA",
			Entry: backtest.Fill{
				Time:     time.Unix(100, 0).UTC(),
				Kind:     "pump",
				Mcap:     100,
				Notional: 0.05,
			},
			Exit: backtest.Fill{
				Time:     time.Unix(200, 0).UTC(),
				Mcap:     200,
				Notional: 0.05,
				Fraction: 1,
			},
			ReturnPct:  100,
			PnLUSD:     0.05,
			HoldSec:    100,
			ExitReason: "tp_2x_half",
		}},
	}
	var buf bytes.Buffer
	if err := backtest.WriteTradesCSV(&buf, res); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "mint,symbol,entry_kind") {
		t.Fatalf("missing header: %s", out)
	}
	if !strings.Contains(out, "AAA") || !strings.Contains(out, "tp_2x_half") {
		t.Fatalf("missing row: %s", out)
	}
}
