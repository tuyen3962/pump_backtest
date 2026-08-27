package tokeninfo

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// Info is a merged snapshot for one mint: metadata + volume + rug heuristics.
type Info struct {
	Mint   string `json:"mint"`
	Symbol string `json:"symbol,omitempty"`
	Name   string `json:"name,omitempty"`

	// Snapshot (pump.fun frontend API)
	Creator        string  `json:"creator,omitempty"`
	Complete       bool    `json:"complete"`
	IsBanned       bool    `json:"isBanned"`
	MarketCapUSD   float64 `json:"marketCapUsd"`
	ATHMarketCapUSD float64 `json:"athMarketCapUsd,omitempty"`
	ATHDrawdownPct float64 `json:"athDrawdownPct,omitempty"` // (ath-current)/ath * 100
	CreatedAt      int64   `json:"createdAt,omitempty"`
	ImageURI       string  `json:"imageUri,omitempty"`
	Twitter        string  `json:"twitter,omitempty"`
	Website        string  `json:"website,omitempty"`

	// Volume (DexScreener, USD)
	VolumeUSD5m  float64 `json:"volumeUsd5m"`
	VolumeUSD1h  float64 `json:"volumeUsd1h"`
	VolumeUSD6h  float64 `json:"volumeUsd6h"`
	VolumeUSD24h float64 `json:"volumeUsd24h"`
	LiquidityUSD float64 `json:"liquidityUsd"`
	PriceUSD     float64 `json:"priceUsd"`
	DexID        string  `json:"dexId,omitempty"`
	PairAddress  string  `json:"pairAddress,omitempty"`

	// Trade pressure (DexScreener txns)
	Buys1h   int `json:"buys1h"`
	Sells1h  int `json:"sells1h"`
	Buys24h  int `json:"buys24h"`
	Sells24h int `json:"sells24h"`

	// Rug / risk heuristics (0–100, higher = riskier)
	SellRatio1h  float64 `json:"sellRatio1h"`  // sells/(buys+sells)
	SellRatio24h float64 `json:"sellRatio24h"`
	RugScore     float64 `json:"rugScore"`
	RugLabel     string  `json:"rugLabel"` // low|medium|high|critical
	RiskNotes    []string `json:"riskNotes,omitempty"`

	// Optional live sample from PumpDev WS (docs: https://pumpdev.io/data-api/)
	LiveSample *LiveVolume `json:"liveSample,omitempty"`

	FetchedAt time.Time `json:"fetchedAt"`
	Sources   []string  `json:"sources"`
	Errors    []string  `json:"errors,omitempty"`
}

type LiveVolume struct {
	WindowSec     int     `json:"windowSec"`
	Trades        int     `json:"trades"`
	BuySOL        float64 `json:"buySol"`
	SellSOL       float64 `json:"sellSol"`
	VolumeSOL     float64 `json:"volumeSol"`
	SellRatio     float64 `json:"sellRatio"`
	LastMarketCap float64 `json:"lastMarketCapSol,omitempty"`
}

type Options struct {
	// SampleLive enables a short PumpDev WebSocket sample (subscribeTokenTrade).
	SampleLive bool
	// LiveWindow how long to sample live trades.
	LiveWindow time.Duration
}

func DefaultOptions() Options {
	return Options{
		SampleLive: false,
		LiveWindow: 8 * time.Second,
	}
}

// Fetch merges pump.fun metadata + DexScreener volume, optionally PumpDev live sample.
func Fetch(ctx context.Context, mint string, opt Options) (Info, error) {
	if mint == "" {
		return Info{}, fmt.Errorf("mint is required")
	}
	info := Info{
		Mint:      mint,
		FetchedAt: time.Now().UTC(),
	}

	var (
		wg   sync.WaitGroup
		mu   sync.Mutex
		addE = func(s string) {
			mu.Lock()
			info.Errors = append(info.Errors, s)
			mu.Unlock()
		}
		addS = func(s string) {
			mu.Lock()
			info.Sources = append(info.Sources, s)
			mu.Unlock()
		}
	)

	wg.Add(2)
	go func() {
		defer wg.Done()
		coin, err := fetchPumpFunCoin(ctx, mint)
		if err != nil {
			addE("pump.fun: " + err.Error())
			return
		}
		mu.Lock()
		applyPumpFun(&info, coin)
		mu.Unlock()
		addS("pump.fun")
	}()
	go func() {
		defer wg.Done()
		dex, err := fetchDexScreener(ctx, mint)
		if err != nil {
			addE("dexscreener: " + err.Error())
			return
		}
		mu.Lock()
		applyDex(&info, dex)
		mu.Unlock()
		addS("dexscreener")
	}()
	wg.Wait()

	if opt.SampleLive {
		win := opt.LiveWindow
		if win <= 0 {
			win = 8 * time.Second
		}
		liveCtx, cancel := context.WithTimeout(ctx, win+2*time.Second)
		live, err := samplePumpDevVolume(liveCtx, mint, win)
		cancel()
		if err != nil {
			info.Errors = append(info.Errors, "pumpdev: "+err.Error())
		} else {
			info.LiveSample = &live
			info.Sources = append(info.Sources, "pumpdev-ws")
		}
	}

	scoreRug(&info)
	if len(info.Sources) == 0 {
		return info, fmt.Errorf("no token sources available for %s: %v", mint, info.Errors)
	}
	return info, nil
}

// FetchMany looks up several mints with a small worker pool.
func FetchMany(ctx context.Context, mints []string, opt Options, workers int) map[string]Info {
	if workers <= 0 {
		workers = 4
	}
	out := make(map[string]Info, len(mints))
	var mu sync.Mutex
	ch := make(chan string)
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for mint := range ch {
				info, err := Fetch(ctx, mint, opt)
				if err != nil && info.Mint == "" {
					info = Info{Mint: mint, Errors: []string{err.Error()}, FetchedAt: time.Now().UTC()}
				}
				mu.Lock()
				out[mint] = info
				mu.Unlock()
			}
		}()
	}
	for _, m := range mints {
		if m == "" {
			continue
		}
		select {
		case <-ctx.Done():
		case ch <- m:
		}
	}
	close(ch)
	wg.Wait()
	return out
}

func scoreRug(info *Info) {
	notes := []string{}
	score := 0.0

	if info.IsBanned {
		score += 50
		notes = append(notes, "token flagged banned on pump.fun")
	}
	if info.ATHMarketCapUSD > 0 && info.MarketCapUSD > 0 {
		dd := (info.ATHMarketCapUSD - info.MarketCapUSD) / info.ATHMarketCapUSD * 100
		if dd < 0 {
			dd = 0
		}
		info.ATHDrawdownPct = dd
		switch {
		case dd >= 90:
			score += 35
			notes = append(notes, fmt.Sprintf("ATH drawdown %.0f%%", dd))
		case dd >= 70:
			score += 25
			notes = append(notes, fmt.Sprintf("ATH drawdown %.0f%%", dd))
		case dd >= 50:
			score += 15
			notes = append(notes, fmt.Sprintf("ATH drawdown %.0f%%", dd))
		}
	}
	if info.Buys1h+info.Sells1h > 0 {
		info.SellRatio1h = float64(info.Sells1h) / float64(info.Buys1h+info.Sells1h)
		if info.SellRatio1h >= 0.65 {
			score += 20
			notes = append(notes, fmt.Sprintf("1h sell pressure %.0f%%", info.SellRatio1h*100))
		} else if info.SellRatio1h >= 0.55 {
			score += 10
			notes = append(notes, fmt.Sprintf("1h sell pressure %.0f%%", info.SellRatio1h*100))
		}
	}
	if info.Buys24h+info.Sells24h > 0 {
		info.SellRatio24h = float64(info.Sells24h) / float64(info.Buys24h+info.Sells24h)
	}
	if info.LiveSample != nil && info.LiveSample.Trades >= 3 {
		if info.LiveSample.SellRatio >= 0.7 {
			score += 15
			notes = append(notes, fmt.Sprintf("live sell ratio %.0f%%", info.LiveSample.SellRatio*100))
		}
	}
	if info.LiquidityUSD > 0 && info.LiquidityUSD < 3000 && info.VolumeUSD24h > 50000 {
		score += 10
		notes = append(notes, "thin liquidity vs 24h volume")
	}
	if score > 100 {
		score = 100
	}
	info.RugScore = score
	switch {
	case score >= 70:
		info.RugLabel = "critical"
	case score >= 45:
		info.RugLabel = "high"
	case score >= 25:
		info.RugLabel = "medium"
	default:
		info.RugLabel = "low"
	}
	info.RiskNotes = notes
}
