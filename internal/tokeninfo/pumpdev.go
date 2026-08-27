package tokeninfo

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/gorilla/websocket"
)

// samplePumpDevVolume connects to wss://pumpdev.io/ws and aggregates
// subscribeTokenTrade events for a short window.
// Docs: https://pumpdev.io/data-api/
func samplePumpDevVolume(ctx context.Context, mint string, window time.Duration) (LiveVolume, error) {
	out := LiveVolume{WindowSec: int(window.Seconds())}
	dialer := websocket.Dialer{HandshakeTimeout: 10 * time.Second}
	conn, _, err := dialer.DialContext(ctx, "wss://pumpdev.io/ws", nil)
	if err != nil {
		return out, err
	}
	defer conn.Close()

	sub, _ := json.Marshal(map[string]any{
		"method": "subscribeTokenTrade",
		"keys":   []string{mint},
	})
	if err := conn.WriteMessage(websocket.TextMessage, sub); err != nil {
		return out, err
	}

	deadline := time.Now().Add(window)
	for {
		select {
		case <-ctx.Done():
			return finalizeLive(out), ctx.Err()
		default:
		}
		if time.Now().After(deadline) {
			return finalizeLive(out), nil
		}
		_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
		_, msg, err := conn.ReadMessage()
		if err != nil {
			continue
		}
		var ev map[string]any
		if err := json.Unmarshal(msg, &ev); err != nil {
			continue
		}
		txType, _ := ev["txType"].(string)
		if txType != "buy" && txType != "sell" {
			continue
		}
		if m, _ := ev["mint"].(string); m != "" && m != mint {
			continue
		}
		sol := asFloat(ev["solAmount"])
		if sol <= 0 {
			sol = asFloat(ev["quoteAmount"])
		}
		out.Trades++
		out.VolumeSOL += sol
		if txType == "buy" {
			out.BuySOL += sol
		} else {
			out.SellSOL += sol
		}
		if mc := asFloat(ev["marketCapSol"]); mc > 0 {
			out.LastMarketCap = mc
		}
	}
}

func finalizeLive(v LiveVolume) LiveVolume {
	total := v.BuySOL + v.SellSOL
	if total > 0 {
		v.SellRatio = v.SellSOL / total
	}
	return v
}

func asFloat(v any) float64 {
	switch t := v.(type) {
	case float64:
		return t
	case json.Number:
		f, _ := t.Float64()
		return f
	case string:
		var f float64
		_, _ = fmt.Sscanf(t, "%f", &f)
		return f
	default:
		return 0
	}
}
