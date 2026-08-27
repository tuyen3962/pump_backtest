package tokeninfo

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

type dexResponse struct {
	Pairs []dexPair `json:"pairs"`
}

type dexPair struct {
	ChainID     string `json:"chainId"`
	DexID       string `json:"dexId"`
	PairAddress string `json:"pairAddress"`
	PriceUSD    string `json:"priceUsd"`
	FDV         float64 `json:"fdv"`
	MarketCap   float64 `json:"marketCap"`
	Liquidity   struct {
		USD float64 `json:"usd"`
	} `json:"liquidity"`
	Volume struct {
		H24 float64 `json:"h24"`
		H6  float64 `json:"h6"`
		H1  float64 `json:"h1"`
		M5  float64 `json:"m5"`
	} `json:"volume"`
	Txns struct {
		M5  dexTxn `json:"m5"`
		H1  dexTxn `json:"h1"`
		H6  dexTxn `json:"h6"`
		H24 dexTxn `json:"h24"`
	} `json:"txns"`
	BaseToken struct {
		Address string `json:"address"`
		Symbol  string `json:"symbol"`
		Name    string `json:"name"`
	} `json:"baseToken"`
}

type dexTxn struct {
	Buys  int `json:"buys"`
	Sells int `json:"sells"`
}

func fetchDexScreener(ctx context.Context, mint string) (dexPair, error) {
	url := "https://api.dexscreener.com/latest/dex/tokens/" + mint
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return dexPair{}, err
	}
	req.Header.Set("User-Agent", "pump_backtest/0.1")
	req.Header.Set("Accept", "application/json")

	client := &http.Client{Timeout: 12 * time.Second}
	res, err := client.Do(req)
	if err != nil {
		return dexPair{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return dexPair{}, fmt.Errorf("status %d", res.StatusCode)
	}
	var payload dexResponse
	if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
		return dexPair{}, err
	}
	if len(payload.Pairs) == 0 {
		return dexPair{}, fmt.Errorf("no pairs")
	}

	// Prefer solana pumpswap / highest liquidity.
	best := payload.Pairs[0]
	bestScore := pairScore(best)
	for _, p := range payload.Pairs[1:] {
		if p.ChainID != "" && p.ChainID != "solana" {
			continue
		}
		s := pairScore(p)
		if s > bestScore {
			best = p
			bestScore = s
		}
	}
	return best, nil
}

func pairScore(p dexPair) float64 {
	score := p.Liquidity.USD + p.Volume.H24*0.05
	if p.DexID == "pumpswap" || p.DexID == "pumpfun" {
		score *= 1.25
	}
	if p.ChainID == "solana" {
		score *= 1.1
	}
	return score
}

func applyDex(info *Info, p dexPair) {
	if info.Symbol == "" {
		info.Symbol = p.BaseToken.Symbol
	}
	if info.Name == "" {
		info.Name = p.BaseToken.Name
	}
	info.VolumeUSD5m = p.Volume.M5
	info.VolumeUSD1h = p.Volume.H1
	info.VolumeUSD6h = p.Volume.H6
	info.VolumeUSD24h = p.Volume.H24
	info.LiquidityUSD = p.Liquidity.USD
	info.DexID = p.DexID
	info.PairAddress = p.PairAddress
	var price float64
	_, _ = fmt.Sscanf(p.PriceUSD, "%f", &price)
	info.PriceUSD = price
	if info.MarketCapUSD <= 0 && p.MarketCap > 0 {
		info.MarketCapUSD = p.MarketCap
	}
	info.Buys1h = p.Txns.H1.Buys
	info.Sells1h = p.Txns.H1.Sells
	info.Buys24h = p.Txns.H24.Buys
	info.Sells24h = p.Txns.H24.Sells
}
