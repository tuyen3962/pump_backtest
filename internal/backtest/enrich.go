package backtest

import (
	"context"
	"time"

	"github.com/surt/pump_backtest/internal/signal"
	"github.com/surt/pump_backtest/internal/tokeninfo"
)

// EnrichCoins attaches live volume / rug metrics to each coin in the result.
func EnrichCoins(ctx context.Context, res *Result, opt tokeninfo.Options) map[string]MarketSnapshot {
	out := map[string]MarketSnapshot{}
	if res == nil || len(res.Coins) == 0 {
		return out
	}
	mints := make([]string, 0, len(res.Coins))
	seen := map[string]struct{}{}
	for _, c := range res.Coins {
		if c.Mint == "" {
			continue
		}
		if _, ok := seen[c.Mint]; ok {
			continue
		}
		seen[c.Mint] = struct{}{}
		mints = append(mints, c.Mint)
	}
	if len(mints) == 0 {
		return out
	}
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, 45*time.Second)
		defer cancel()
	}
	infos := tokeninfo.FetchMany(ctx, mints, opt, 4)
	for mint, info := range infos {
		out[mint] = MarketSnapshot{
			LiquidityUSD: info.LiquidityUSD,
			VolumeUSD1h:  info.VolumeUSD1h,
			VolumeUSD24h: info.VolumeUSD24h,
			RugScore:     info.RugScore,
		}
	}
	for i := range res.Coins {
		info, ok := infos[res.Coins[i].Mint]
		if !ok {
			continue
		}
		res.Coins[i].VolumeUSD24h = info.VolumeUSD24h
		res.Coins[i].VolumeUSD1h = info.VolumeUSD1h
		res.Coins[i].LiquidityUSD = info.LiquidityUSD
		res.Coins[i].SellRatio1h = info.SellRatio1h
		res.Coins[i].RugScore = info.RugScore
		res.Coins[i].RugLabel = info.RugLabel
		res.Coins[i].MarketCapUSD = info.MarketCapUSD
		res.Coins[i].ATHDrawdownPct = info.ATHDrawdownPct
		res.Coins[i].Complete = info.Complete
		if res.Coins[i].Symbol == "" || res.Coins[i].Symbol == res.Coins[i].Mint {
			if info.Symbol != "" {
				res.Coins[i].Symbol = info.Symbol
			}
		}
	}
	return out
}

// MarketFromRecords prefetches market snapshots for all mints in the recording
// so entry filters can run without look-ahead through trade outcomes.
func MarketFromRecords(ctx context.Context, records []signal.Record, opt tokeninfo.Options) map[string]MarketSnapshot {
	seen := map[string]struct{}{}
	var mints []string
	for _, r := range records {
		m := r.Raw.Mint
		if m == "" {
			continue
		}
		if _, ok := seen[m]; ok {
			continue
		}
		seen[m] = struct{}{}
		mints = append(mints, m)
	}
	out := map[string]MarketSnapshot{}
	if len(mints) == 0 {
		return out
	}
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, 45*time.Second)
		defer cancel()
	}
	infos := tokeninfo.FetchMany(ctx, mints, opt, 4)
	for mint, info := range infos {
		out[mint] = MarketSnapshot{
			LiquidityUSD: info.LiquidityUSD,
			VolumeUSD1h:  info.VolumeUSD1h,
			VolumeUSD24h: info.VolumeUSD24h,
			RugScore:     info.RugScore,
		}
	}
	return out
}

// ApplyMarketToCoins copies snapshot fields onto coin rows.
func ApplyMarketToCoins(res *Result, market map[string]MarketSnapshot, infos map[string]tokeninfo.Info) {
	if res == nil {
		return
	}
	for i := range res.Coins {
		mint := res.Coins[i].Mint
		if info, ok := infos[mint]; ok {
			res.Coins[i].VolumeUSD24h = info.VolumeUSD24h
			res.Coins[i].VolumeUSD1h = info.VolumeUSD1h
			res.Coins[i].LiquidityUSD = info.LiquidityUSD
			res.Coins[i].SellRatio1h = info.SellRatio1h
			res.Coins[i].RugScore = info.RugScore
			res.Coins[i].RugLabel = info.RugLabel
			res.Coins[i].MarketCapUSD = info.MarketCapUSD
			res.Coins[i].ATHDrawdownPct = info.ATHDrawdownPct
			res.Coins[i].Complete = info.Complete
			if info.Symbol != "" {
				res.Coins[i].Symbol = info.Symbol
			}
			continue
		}
		if snap, ok := market[mint]; ok {
			res.Coins[i].VolumeUSD24h = snap.VolumeUSD24h
			res.Coins[i].VolumeUSD1h = snap.VolumeUSD1h
			res.Coins[i].LiquidityUSD = snap.LiquidityUSD
			res.Coins[i].RugScore = snap.RugScore
		}
	}
}
