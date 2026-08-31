package backtest

import "testing"

func TestDownsampleEquity(t *testing.T) {
	var pts []EquityPoint
	for i := 0; i < 1000; i++ {
		ev := "mark:x"
		if i == 500 {
			ev = "entry"
		}
		pts = append(pts, EquityPoint{Equity: float64(i), Event: ev})
	}
	out := DownsampleEquity(pts, 50)
	if len(out) > 50 {
		t.Fatalf("len=%d", len(out))
	}
	if out[0].Equity != 0 || out[len(out)-1].Equity != 999 {
		t.Fatalf("endpoints not preserved")
	}
}
