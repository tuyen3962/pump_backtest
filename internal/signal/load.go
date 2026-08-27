package signal

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Record is one line from signal-recorder NDJSON output.
type Record struct {
	ReceivedAt time.Time `json:"receivedAt"`
	Channel    string    `json:"channel"`
	Offset     uint64    `json:"offset,omitempty"`
	Raw        Envelope  `json:"raw"`
	Payload    Payload   `json:"-"`
	Source     string    `json:"-"`
}

type fileLine struct {
	ReceivedAt time.Time       `json:"receivedAt"`
	Channel    string          `json:"channel"`
	Offset     uint64          `json:"offset,omitempty"`
	Raw        json.RawMessage `json:"raw"`
}

// LoadNDJSON reads one or more .ndjson/.jsonl files (or directories of them),
// dedupes by envelope id, and sorts by event time (ts, then receivedAt).
func LoadNDJSON(paths ...string) ([]Record, error) {
	var files []string
	for _, p := range paths {
		info, err := os.Stat(p)
		if err != nil {
			return nil, err
		}
		if info.IsDir() {
			entries, err := os.ReadDir(p)
			if err != nil {
				return nil, err
			}
			for _, e := range entries {
				if e.IsDir() {
					continue
				}
				name := e.Name()
				if strings.HasSuffix(name, ".ndjson") || strings.HasSuffix(name, ".jsonl") {
					files = append(files, filepath.Join(p, name))
				}
			}
			continue
		}
		files = append(files, p)
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("no ndjson files found in %v", paths)
	}

	seen := make(map[string]struct{})
	var out []Record
	for _, path := range files {
		recs, err := loadFile(path, seen)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		out = append(out, recs...)
	}

	sort.SliceStable(out, func(i, j int) bool {
		ti, tj := out[i].EventTime(), out[j].EventTime()
		if !ti.Equal(tj) {
			return ti.Before(tj)
		}
		return out[i].Offset < out[j].Offset
	})
	return out, nil
}

func loadFile(path string, seen map[string]struct{}) ([]Record, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var out []Record
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	lineNo := 0
	for sc.Scan() {
		lineNo++
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var fl fileLine
		if err := json.Unmarshal([]byte(line), &fl); err != nil {
			return nil, fmt.Errorf("line %d: %w", lineNo, err)
		}
		var env Envelope
		if err := json.Unmarshal(fl.Raw, &env); err != nil {
			return nil, fmt.Errorf("line %d raw: %w", lineNo, err)
		}
		if env.ID != "" {
			if _, ok := seen[env.ID]; ok {
				continue
			}
			seen[env.ID] = struct{}{}
		}
		payload, _ := DecodePayload(env.Payload)
		out = append(out, Record{
			ReceivedAt: fl.ReceivedAt,
			Channel:    fl.Channel,
			Offset:     fl.Offset,
			Raw:        env,
			Payload:    payload,
			Source:     path,
		})
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// EventTime prefers envelope ts; falls back to receivedAt.
func (r Record) EventTime() time.Time {
	if r.Raw.Ts > 0 {
		sec := int64(r.Raw.Ts) / 1000
		nsec := int64(r.Raw.Ts*1e6) % 1e9
		return time.Unix(sec, nsec).UTC()
	}
	return r.ReceivedAt.UTC()
}

func (r Record) Symbol() string {
	if r.Payload.Symbol != "" {
		return r.Payload.Symbol
	}
	return r.Raw.Mint
}

func (r Record) Mcap() float64 {
	if r.Payload.McapUsd > 0 {
		return r.Payload.McapUsd
	}
	// out signals often carry mid/entry/peak instead of mcapUsd
	if r.Payload.MidMcap > 0 {
		return r.Payload.MidMcap
	}
	return 0
}
