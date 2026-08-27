// Command signal-recorder connects to the pumpapi Signal Stream (Centrifugo)
// and appends every publication as one NDJSON line for later backtests.
//
// Docs: https://docs.memecatcher.bubudev.win/signal-stream
package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	ossignal "os/signal"
	"path/filepath"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/centrifugal/centrifuge-go"

	"github.com/surt/pump_backtest/internal/signal"
)

const defaultWSURL = "wss://memecatcher.bubudev.win/connection/websocket"
const defaultChannel = "signals"

// recordLine wraps the raw Centrifugo publication with local receive metadata.
type recordLine struct {
	ReceivedAt time.Time       `json:"receivedAt"`
	Channel    string          `json:"channel"`
	Offset     uint64          `json:"offset,omitempty"`
	Raw        json.RawMessage `json:"raw"`
}

func main() {
	wsURL := flag.String("url", defaultWSURL, "Centrifugo websocket endpoint")
	channel := flag.String("channel", defaultChannel, "Channel to subscribe (signals or signals:mint:<base58>)")
	outDir := flag.String("out", "data/signals", "Directory for NDJSON recordings")
	dedupe := flag.Bool("dedupe", true, "Skip duplicate envelope ids (recovery / multi-channel)")
	flag.Parse()

	if err := os.MkdirAll(*outDir, 0o755); err != nil {
		log.Fatalf("create out dir: %v", err)
	}

	outPath := filepath.Join(*outDir, fmt.Sprintf("signals_%s.ndjson", time.Now().UTC().Format("20060102_150405")))
	f, err := os.OpenFile(outPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		log.Fatalf("open output: %v", err)
	}
	defer f.Close()

	bw := bufio.NewWriterSize(f, 64*1024)
	var writeMu sync.Mutex
	flush := func() {
		writeMu.Lock()
		defer writeMu.Unlock()
		_ = bw.Flush()
		_ = f.Sync()
	}
	defer flush()

	log.Printf("recording → %s", outPath)
	log.Printf("connecting to %s channel=%s", *wsURL, *channel)

	seen := make(map[string]struct{})
	var seenMu sync.Mutex
	var count atomic.Int64

	client := centrifuge.NewJsonClient(*wsURL, centrifuge.Config{
		// Default handshake is 1s; Cloudflare + distant RTT often needs more.
		HandshakeTimeout:   15 * time.Second,
		ReadTimeout:        10 * time.Second,
		MaxServerPingDelay: 30 * time.Second,
		Name:               "pump_backtest",
		Version:            "0.1.0",
	})
	defer client.Close()

	client.OnConnecting(func(e centrifuge.ConnectingEvent) {
		log.Printf("connecting: %d (%s)", e.Code, e.Reason)
	})
	client.OnConnected(func(e centrifuge.ConnectedEvent) {
		log.Printf("connected clientID=%s", e.ClientID)
	})
	client.OnDisconnected(func(e centrifuge.DisconnectedEvent) {
		log.Printf("disconnected: %d (%s)", e.Code, e.Reason)
	})
	client.OnError(func(e centrifuge.ErrorEvent) {
		log.Printf("client error: %v", e.Error)
	})

	sub, err := client.NewSubscription(*channel, centrifuge.SubscriptionConfig{
		// Docs: at-least-once recovery up to 1000 msgs / 24h when reconnecting.
		Recoverable: true,
	})
	if err != nil {
		log.Fatalf("new subscription: %v", err)
	}

	sub.OnSubscribing(func(e centrifuge.SubscribingEvent) {
		log.Printf("subscribing %s: %d (%s)", sub.Channel, e.Code, e.Reason)
	})
	sub.OnSubscribed(func(e centrifuge.SubscribedEvent) {
		log.Printf("subscribed %s recovered=%v wasRecovering=%v", sub.Channel, e.Recovered, e.WasRecovering)
	})
	sub.OnUnsubscribed(func(e centrifuge.UnsubscribedEvent) {
		log.Printf("unsubscribed %s: %d (%s)", sub.Channel, e.Code, e.Reason)
	})
	sub.OnError(func(e centrifuge.SubscriptionErrorEvent) {
		log.Printf("subscription error %s: %v", sub.Channel, e.Error)
	})

	sub.OnPublication(func(e centrifuge.PublicationEvent) {
		// Keep this handler short — it runs on the Centrifuge read loop.
		raw := append(json.RawMessage(nil), e.Data...)

		var env signal.Envelope
		if err := json.Unmarshal(raw, &env); err != nil {
			log.Printf("envelope parse warn: %v (still recording raw)", err)
		}
		if env.V != 0 && env.V != 1 {
			log.Printf("unexpected envelope v=%d id=%s (continuing)", env.V, env.ID)
		}

		if *dedupe && env.ID != "" {
			seenMu.Lock()
			if _, ok := seen[env.ID]; ok {
				seenMu.Unlock()
				return
			}
			seen[env.ID] = struct{}{}
			seenMu.Unlock()
		}

		payload, _ := signal.DecodePayload(env.Payload)
		label := payload.Symbol
		if label == "" {
			label = env.Mint
		}
		if label == "" {
			label = "?"
		}
		kind := env.Kind
		if kind == "" {
			kind = "?"
		}
		n := count.Add(1)
		log.Printf("#%d %s → %s mcap=%.0f", n, kind, label, payload.McapUsd)

		rec := recordLine{
			ReceivedAt: time.Now().UTC(),
			Channel:    sub.Channel,
			Offset:     e.Offset,
			Raw:        raw,
		}
		b, err := json.Marshal(rec)
		if err != nil {
			log.Printf("marshal record: %v", err)
			return
		}
		b = append(b, '\n')

		writeMu.Lock()
		_, err = bw.Write(b)
		writeMu.Unlock()
		if err != nil {
			log.Printf("write error: %v", err)
		}
	})

	if err := client.Connect(); err != nil {
		log.Fatalf("connect: %v", err)
	}
	if err := sub.Subscribe(); err != nil {
		log.Fatalf("subscribe: %v", err)
	}

	log.Println("listening… Ctrl+C to stop")

	// Periodic flush so a crash still leaves recent lines on disk.
	stopFlush := make(chan struct{})
	go func() {
		t := time.NewTicker(2 * time.Second)
		defer t.Stop()
		for {
			select {
			case <-t.C:
				flush()
			case <-stopFlush:
				return
			}
		}
	}()

	stop := make(chan os.Signal, 1)
	ossignal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop

	log.Println("shutting down…")
	close(stopFlush)
	_ = sub.Unsubscribe()
	_ = client.Disconnect()
	flush()
	log.Printf("done, wrote %d signals → %s", count.Load(), outPath)
}
