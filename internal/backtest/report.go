package backtest

import (
	"fmt"
	"io"
	"strings"
	"time"
)

func WriteReport(w io.Writer, res Result) {
	cfg := res.Config
	fmt.Fprintf(w, "=== Strategy v1 (%s) ===\n", strings.Join(cfg.EntryKinds, ","))
	fmt.Fprintf(w, "signals=%d  entry=%s  bankroll=%.2f SOL  size=%.3f SOL  fee=%.0fbps\n",
		res.Signals, strings.Join(cfg.EntryKinds, ","), cfg.StartCash, cfg.NotionalUSD, cfg.FeeBps)
	fmt.Fprintf(w, "stop=-%.0f%%  tp2x=%.1fx→%.0f%%  scale=+%.0f%%→%.0f%% rem  latency=%.0fs\n",
		cfg.StopLossPct, cfg.TakeProfit2x, cfg.FirstTPFraction*100, cfg.ScaleTriggerPct, cfg.ScaleSellFraction*100, cfg.LatencySec)
	fmt.Fprintf(w, "filter liq>=$%.0f vol1h>=$%.0f  skipped_entries=%d  max_pos=%d\n",
		cfg.MinLiquidityUSD, cfg.MinVolumeUSD1h, res.Skipped, cfg.MaxPositions)
	fmt.Fprintf(w, "legs=%d coins=%d (closed=%d open=%d)  winrate=%.1f%%  avg_return=%.2f%%\n",
		len(res.Trades), len(res.Coins), res.ClosedCount, res.OpenCount, res.WinRate, res.AvgReturn)
	fmt.Fprintf(w, "total_pnl=%.4f SOL  fees=%.4f SOL\n", res.TotalPnL, res.TotalFees)
	fmt.Fprintf(w, "start=%.2f  end=%.4f SOL  maxDD=%.2f%%\n\n",
		cfg.StartCash, res.EndEquity, res.MaxDrawdown)

	if len(res.Coins) == 0 {
		fmt.Fprintf(w, "(no trades — need matching entry signals + free bankroll, passing filters)\n")
		return
	}

	fmt.Fprintf(w, "%-6s %-10s %-8s %10s %10s %9s %10s %8s %s\n",
		"#", "symbol", "hold", "entry_mcap", "exit_mcap", "return%", "pnl SOL", "vol24h", "exit")
	for i, c := range res.Coins {
		sym := c.Symbol
		if len(sym) > 10 {
			sym = sym[:10]
		}
		vol := ""
		if c.VolumeUSD24h > 0 {
			vol = fmt.Sprintf("$%.0f", c.VolumeUSD24h)
		}
		fmt.Fprintf(w, "%-6d %-10s %-8s %10.0f %10.0f %8.2f%% %10.4f %8s %s\n",
			i+1,
			sym,
			fmtDur(time.Duration(c.HoldSec*float64(time.Second))),
			c.EntryMcap,
			c.ExitMcap,
			c.ReturnPct,
			c.PnLUSD,
			vol,
			c.ExitReason,
		)
	}
}

func fmtDur(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm%ds", int(d.Minutes()), int(d.Seconds())%60)
	}
	return fmt.Sprintf("%dh%dm", int(d.Hours()), int(d.Minutes())%60)
}
