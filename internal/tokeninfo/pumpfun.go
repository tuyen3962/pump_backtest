package tokeninfo

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

type pumpFunCoin struct {
	Mint              string  `json:"mint"`
	Name              string  `json:"name"`
	Symbol            string  `json:"symbol"`
	Description       string  `json:"description"`
	ImageURI          string  `json:"image_uri"`
	Twitter           string  `json:"twitter"`
	Website           string  `json:"website"`
	Creator           string  `json:"creator"`
	CreatedTimestamp  int64   `json:"created_timestamp"`
	Complete          bool    `json:"complete"`
	IsBanned          bool    `json:"is_banned"`
	MarketCap         float64 `json:"market_cap"`
	USDMarketCap      float64 `json:"usd_market_cap"`
	MarketCapUSD      float64 `json:"market_cap_usd"`
	ATHMarketCap      float64 `json:"ath_market_cap"`
	PumpSwapPool      string  `json:"pump_swap_pool"`
}

func fetchPumpFunCoin(ctx context.Context, mint string) (pumpFunCoin, error) {
	url := "https://frontend-api-v3.pump.fun/coins/" + mint
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return pumpFunCoin{}, err
	}
	req.Header.Set("User-Agent", "pump_backtest/0.1")
	req.Header.Set("Accept", "application/json")

	client := &http.Client{Timeout: 12 * time.Second}
	res, err := client.Do(req)
	if err != nil {
		return pumpFunCoin{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return pumpFunCoin{}, fmt.Errorf("status %d", res.StatusCode)
	}
	var coin pumpFunCoin
	if err := json.NewDecoder(res.Body).Decode(&coin); err != nil {
		return pumpFunCoin{}, err
	}
	if coin.Mint == "" {
		coin.Mint = mint
	}
	return coin, nil
}

func applyPumpFun(info *Info, coin pumpFunCoin) {
	info.Name = coin.Name
	info.Symbol = coin.Symbol
	info.Creator = coin.Creator
	info.Complete = coin.Complete
	info.IsBanned = coin.IsBanned
	info.CreatedAt = coin.CreatedTimestamp
	info.ImageURI = coin.ImageURI
	info.Twitter = coin.Twitter
	info.Website = coin.Website
	mcap := coin.USDMarketCap
	if mcap <= 0 {
		mcap = coin.MarketCapUSD
	}
	info.MarketCapUSD = mcap
	info.ATHMarketCapUSD = coin.ATHMarketCap
}
