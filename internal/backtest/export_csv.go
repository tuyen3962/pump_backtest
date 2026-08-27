package backtest

import (
	"encoding/csv"
	"fmt"
	"io"
	"strconv"
	"time"
)

// WriteTradesCSV exports closed/open exit legs.
func WriteTradesCSV(w io.Writer, res Result) error {
	cw := csv.NewWriter(w)
	defer cw.Flush()
	if err := cw.Write([]string{
		"mint", "symbol", "entry_kind", "exit_reason", "open",
		"entry_time", "exit_time", "hold_sec",
		"entry_mcap", "exit_mcap", "return_pct", "pnl_sol",
		"entry_notional_sol", "exit_notional_sol", "fraction",
	}); err != nil {
		return err
	}
	for _, t := range res.Trades {
		row := []string{
			t.Mint,
			t.Symbol,
			t.Entry.Kind,
			t.ExitReason,
			strconv.FormatBool(t.Open),
			t.Entry.Time.UTC().Format(time.RFC3339Nano),
			t.Exit.Time.UTC().Format(time.RFC3339Nano),
			fmt.Sprintf("%.3f", t.HoldSec),
			fmt.Sprintf("%.6f", t.Entry.Mcap),
			fmt.Sprintf("%.6f", t.Exit.Mcap),
			fmt.Sprintf("%.6f", t.ReturnPct),
			fmt.Sprintf("%.8f", t.PnLUSD),
			fmt.Sprintf("%.6f", t.Entry.Notional),
			fmt.Sprintf("%.6f", t.Exit.Notional),
			fmt.Sprintf("%.6f", t.Exit.Fraction),
		}
		if err := cw.Write(row); err != nil {
			return err
		}
	}
	return cw.Error()
}

// WriteCoinsCSV exports per-mint aggregates.
func WriteCoinsCSV(w io.Writer, res Result) error {
	cw := csv.NewWriter(w)
	defer cw.Flush()
	if err := cw.Write([]string{
		"mint", "symbol", "entry_kind", "exit_reason", "open", "trades",
		"entry_mcap", "exit_mcap", "return_pct", "pnl_sol", "hold_sec",
		"remaining", "volume_usd_24h", "volume_usd_1h", "liquidity_usd",
		"sell_ratio_1h", "rug_score", "rug_label", "ath_drawdown_pct",
	}); err != nil {
		return err
	}
	for _, c := range res.Coins {
		row := []string{
			c.Mint,
			c.Symbol,
			c.EntryKind,
			c.ExitReason,
			strconv.FormatBool(c.Open),
			strconv.Itoa(c.Trades),
			fmt.Sprintf("%.6f", c.EntryMcap),
			fmt.Sprintf("%.6f", c.ExitMcap),
			fmt.Sprintf("%.6f", c.ReturnPct),
			fmt.Sprintf("%.8f", c.PnLUSD),
			fmt.Sprintf("%.3f", c.HoldSec),
			fmt.Sprintf("%.6f", c.Remaining),
			fmt.Sprintf("%.2f", c.VolumeUSD24h),
			fmt.Sprintf("%.2f", c.VolumeUSD1h),
			fmt.Sprintf("%.2f", c.LiquidityUSD),
			fmt.Sprintf("%.4f", c.SellRatio1h),
			fmt.Sprintf("%.1f", c.RugScore),
			c.RugLabel,
			fmt.Sprintf("%.2f", c.ATHDrawdownPct),
		}
		if err := cw.Write(row); err != nil {
			return err
		}
	}
	return cw.Error()
}

// WriteEquityCSV exports the equity curve.
func WriteEquityCSV(w io.Writer, res Result) error {
	cw := csv.NewWriter(w)
	defer cw.Flush()
	if err := cw.Write([]string{
		"time", "equity_sol", "realized_pnl", "unrealized_pnl", "open_positions", "event", "symbol",
	}); err != nil {
		return err
	}
	for _, p := range res.Equity {
		row := []string{
			p.Time.UTC().Format(time.RFC3339Nano),
			fmt.Sprintf("%.8f", p.Equity),
			fmt.Sprintf("%.8f", p.RealizedPnL),
			fmt.Sprintf("%.8f", p.UnrealizedPnL),
			strconv.Itoa(p.OpenPositions),
			p.Event,
			p.Symbol,
		}
		if err := cw.Write(row); err != nil {
			return err
		}
	}
	return cw.Error()
}

// WriteSummaryCSV one-row strategy summary for compare spreadsheets.
func WriteSummaryCSV(w io.Writer, res Result, runID, label, source string) error {
	cw := csv.NewWriter(w)
	defer cw.Flush()
	if err := cw.Write([]string{
		"run_id", "label", "source", "entry_kinds",
		"signals", "coins", "closed", "open",
		"win_rate_pct", "avg_return_pct", "total_pnl_sol", "total_fees_sol",
		"start_cash", "end_equity", "max_drawdown_pct", "skipped_entries",
		"stop_loss_pct", "take_profit_2x", "fee_bps", "latency_sec",
		"min_liq_usd", "min_vol1h_usd", "notional_sol",
	}); err != nil {
		return err
	}
	cfg := res.Config
	row := []string{
		runID,
		label,
		source,
		fmt.Sprintf("%v", cfg.EntryKinds),
		strconv.Itoa(res.Signals),
		strconv.Itoa(len(res.Coins)),
		strconv.Itoa(res.ClosedCount),
		strconv.Itoa(res.OpenCount),
		fmt.Sprintf("%.2f", res.WinRate),
		fmt.Sprintf("%.2f", res.AvgReturn),
		fmt.Sprintf("%.8f", res.TotalPnL),
		fmt.Sprintf("%.8f", res.TotalFees),
		fmt.Sprintf("%.6f", cfg.StartCash),
		fmt.Sprintf("%.8f", res.EndEquity),
		fmt.Sprintf("%.2f", res.MaxDrawdown),
		strconv.Itoa(res.Skipped),
		fmt.Sprintf("%.1f", cfg.StopLossPct),
		fmt.Sprintf("%.2f", cfg.TakeProfit2x),
		fmt.Sprintf("%.0f", cfg.FeeBps),
		fmt.Sprintf("%.1f", cfg.LatencySec),
		fmt.Sprintf("%.0f", cfg.MinLiquidityUSD),
		fmt.Sprintf("%.0f", cfg.MinVolumeUSD1h),
		fmt.Sprintf("%.6f", cfg.NotionalUSD),
	}
	if err := cw.Write(row); err != nil {
		return err
	}
	return cw.Error()
}
