package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/surt/pump_backtest/internal/tokeninfo"
)

func main() {
	mint := flag.String("mint", "", "Token mint (base58 contract)")
	live := flag.Bool("live", false, "Also sample PumpDev WebSocket volume for a few seconds")
	window := flag.Duration("window", 8*time.Second, "Live sample window")
	flag.Parse()
	if *mint == "" && flag.NArg() > 0 {
		*mint = flag.Arg(0)
	}
	if *mint == "" {
		log.Fatal("usage: token-info -mint <base58> [-live]")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	info, err := tokeninfo.Fetch(ctx, *mint, tokeninfo.Options{
		SampleLive: *live,
		LiveWindow: *window,
	})
	if err != nil {
		log.Printf("warn: %v", err)
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(info); err != nil {
		log.Fatal(err)
	}
	fmt.Fprintf(os.Stderr, "rug=%s(%.0f) vol24h=$%.0f sell1h=%.0f%% sources=%v\n",
		info.RugLabel, info.RugScore, info.VolumeUSD24h, info.SellRatio1h*100, info.Sources)
}
