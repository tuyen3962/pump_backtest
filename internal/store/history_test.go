package store

import "testing"

func TestIsRuggedTrade(t *testing.T) {
	tests := []struct {
		reason string
		score  float64
		label  string
		want   bool
	}{
		{"stop_loss", 0, "", false},
		{"stop_loss", 80, "critical", true},
		{"tp_2x_half", 0, "", false},
		{"out:must:whale_dump", 0, "", true},
		{"out:must:dev_sold", 0, "", true},
		{"out:warning:stale", 0, "", true},
		{"eod_mark", 0, "high", true},
		{"eod_mark", 55, "medium", false},
		{"eod_mark", 75, "", true},
	}
	for _, tc := range tests {
		got := IsRuggedTrade(tc.reason, tc.score, tc.label)
		if got != tc.want {
			t.Fatalf("IsRuggedTrade(%q, %.0f, %q) = %v want %v", tc.reason, tc.score, tc.label, got, tc.want)
		}
	}
}

func TestClassifyStatus(t *testing.T) {
	if got := ClassifyStatus("stop_loss", 0, "", false); got != "closed" {
		t.Fatalf("stop_loss want closed, got %s", got)
	}
	if got := ClassifyStatus("out:must:dev_sold", 0, "", false); got != "rugged" {
		t.Fatalf("dump exit want rugged, got %s", got)
	}
}
