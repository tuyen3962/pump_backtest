// Command backtest replays recorded Signal Stream NDJSON with strategy v1.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/surt/pump_backtest/internal/backtest"
	"github.com/surt/pump_backtest/internal/signal"
	"github.com/surt/pump_backtest/internal/tokeninfo"
)

func main() {
	in := flag.String("in", "data/signals", "NDJSON file or directory from signal-recorder")
	entry := flag.String("entry", "whale_armed", "Comma-separated entry signal kinds")
	bankroll := flag.Float64("bankroll", 1, "Start bankroll in SOL")
	size := flag.Float64("size", 0.05, "SOL size per entry")
	feeBps := flag.Float64("fee-bps", 100, "Round-trip fee/slippage proxy in basis points")
	latency := flag.Float64("latency", 5, "Entry latency seconds (look-ahead fill / slip)")
	minLiq := flag.Float64("min-liq", 5000, "Min liquidity USD to enter (0=off)")
	minVol1h := flag.Float64("min-vol1h", 2000, "Min 1h volume USD to enter (0=off)")
	noFilter := flag.Bool("no-filter", false, "Disable liquidity/volume gates")
	enrich := flag.Bool("enrich", true, "Fetch live volume/rug + apply entry filters")
	live := flag.Bool("live-sample", false, "Also sample PumpDev WS while enriching")
	noEOD := flag.Bool("no-eod", false, "Do not mark-to-market open positions at end")
	flag.Parse()

	records, err := signal.LoadNDJSON(*in)
	if err != nil {
		log.Fatalf("load: %v", err)
	}

	cfg := backtest.DefaultConfig()
	cfg.EntryKinds = splitCSV(*entry)
	cfg.StartCash = *bankroll
	cfg.NotionalUSD = *size
	cfg.FeeBps = *feeBps
	cfg.LatencySec = *latency
	cfg.CloseOpenAtEnd = !*noEOD
	if *noFilter {
		cfg.MinLiquidityUSD = 0
		cfg.MinVolumeUSD1h = 0
	} else {
		cfg.MinLiquidityUSD = *minLiq
		cfg.MinVolumeUSD1h = *minVol1h
	}

	var market map[string]backtest.MarketSnapshot
	if *enrich {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		market = backtest.MarketFromRecords(ctx, records, tokeninfo.Options{
			SampleLive: *live,
			LiveWindow: 6 * time.Second,
		})
		cancel()
	}

	res, err := backtest.RunWithMarket(records, cfg, market)
	if err != nil {
		log.Fatalf("run: %v", err)
	}
	if *enrich {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		backtest.EnrichCoins(ctx, &res, tokeninfo.Options{})
		cancel()
	}
	backtest.WriteReport(os.Stdout, res)
	fmt.Fprintf(os.Stderr, "\nlatency tip: default 5s (aggressive 2s / congested 10s). Filter look-ahead: enrich is live snapshot, not historical.\n")
}

func splitCSV(s string) []string {
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}
