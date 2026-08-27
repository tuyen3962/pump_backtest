package store

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// Store persists dashboard runs, coin history, and watchlist under a data root.
type Store struct {
	Root string
	mu   sync.Mutex
}

func New(root string) (*Store, error) {
	s := &Store{Root: root}
	for _, d := range []string{
		filepath.Join(root, "runs"),
		filepath.Join(root, "history"),
	} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return nil, err
		}
	}
	return s, nil
}

type SavedRun struct {
	ID        string          `json:"id"`
	SavedAt   time.Time       `json:"savedAt"`
	Request   json.RawMessage `json:"request,omitempty"`
	Response  json.RawMessage `json:"response"`
	Source    string          `json:"source,omitempty"`
	EndEquity float64         `json:"endEquity,omitempty"`
	TotalPnL  float64         `json:"totalPnl,omitempty"`
	CoinCount int             `json:"coinCount,omitempty"`
}

type HistoryEntry struct {
	ID         string    `json:"id"`
	Mint       string    `json:"mint"`
	Symbol     string    `json:"symbol"`
	Status     string    `json:"status"` // closed | rugged | open_saved
	ExitReason string    `json:"exitReason"`
	EntryKind  string    `json:"entryKind"`
	EntryMcap  float64   `json:"entryMcap"`
	ExitMcap   float64   `json:"exitMcap"`
	ReturnPct  float64   `json:"returnPct"`
	PnLSOL     float64   `json:"pnlSol"`
	RugScore   float64   `json:"rugScore,omitempty"`
	RugLabel   string    `json:"rugLabel,omitempty"`
	HoldSec    float64   `json:"holdSec"`
	ClosedAt   time.Time `json:"closedAt"`
	RunID      string    `json:"runId,omitempty"`
}

type WatchItem struct {
	Mint      string    `json:"mint"`
	Symbol    string    `json:"symbol,omitempty"`
	EntryMcap float64   `json:"entryMcap,omitempty"`
	AddedAt   time.Time `json:"addedAt"`
	Source    string    `json:"source,omitempty"` // backtest_open | manual
}

func (s *Store) SaveRun(run SavedRun) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if run.ID == "" {
		run.ID = time.Now().UTC().Format("20060102_150405")
	}
	if run.SavedAt.IsZero() {
		run.SavedAt = time.Now().UTC()
	}
	b, err := json.MarshalIndent(run, "", "  ")
	if err != nil {
		return err
	}
	last := filepath.Join(s.Root, "runs", "last.json")
	archive := filepath.Join(s.Root, "runs", "run_"+run.ID+".json")
	if err := os.WriteFile(last, b, 0o644); err != nil {
		return err
	}
	return os.WriteFile(archive, b, 0o644)
}

func (s *Store) LoadLastRun() (*SavedRun, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	b, err := os.ReadFile(filepath.Join(s.Root, "runs", "last.json"))
	if err != nil {
		return nil, err
	}
	var run SavedRun
	if err := json.Unmarshal(b, &run); err != nil {
		return nil, err
	}
	return &run, nil
}

func (s *Store) AppendHistory(entries []HistoryEntry) error {
	if len(entries) == 0 {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	path := filepath.Join(s.Root, "history", "coins.ndjson")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	for _, e := range entries {
		if e.ID == "" {
			e.ID = e.Mint + "|" + e.ClosedAt.UTC().Format(time.RFC3339Nano)
		}
		if e.ClosedAt.IsZero() {
			e.ClosedAt = time.Now().UTC()
		}
		if err := enc.Encode(e); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) ListHistory(limit int) ([]HistoryEntry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	path := filepath.Join(s.Root, "history", "coins.ndjson")
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return []HistoryEntry{}, nil
		}
		return nil, err
	}
	defer f.Close()

	var all []HistoryEntry
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 2*1024*1024)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var e HistoryEntry
		if err := json.Unmarshal([]byte(line), &e); err != nil {
			continue
		}
		all = append(all, e)
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	sort.Slice(all, func(i, j int) bool {
		return all[i].ClosedAt.After(all[j].ClosedAt)
	})
	if limit > 0 && len(all) > limit {
		all = all[:limit]
	}
	return all, nil
}

func (s *Store) watchPath() string {
	return filepath.Join(s.Root, "watchlist.json")
}

func (s *Store) loadWatchlistUnlocked() ([]WatchItem, error) {
	b, err := os.ReadFile(s.watchPath())
	if err != nil {
		if os.IsNotExist(err) {
			return []WatchItem{}, nil
		}
		return nil, err
	}
	var items []WatchItem
	if err := json.Unmarshal(b, &items); err != nil {
		return nil, err
	}
	return items, nil
}

func (s *Store) saveWatchlistUnlocked(items []WatchItem) error {
	b, err := json.MarshalIndent(items, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.watchPath(), b, 0o644)
}

func (s *Store) LoadWatchlist() ([]WatchItem, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.loadWatchlistUnlocked()
}

func (s *Store) SaveWatchlist(items []WatchItem) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.saveWatchlistUnlocked(items)
}

func (s *Store) UpsertWatch(item WatchItem) ([]WatchItem, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	items, err := s.loadWatchlistUnlocked()
	if err != nil {
		return nil, err
	}
	found := false
	for i := range items {
		if items[i].Mint == item.Mint {
			if item.Symbol != "" {
				items[i].Symbol = item.Symbol
			}
			if item.EntryMcap > 0 {
				items[i].EntryMcap = item.EntryMcap
			}
			if item.Source != "" {
				items[i].Source = item.Source
			}
			found = true
			break
		}
	}
	if !found {
		if item.AddedAt.IsZero() {
			item.AddedAt = time.Now().UTC()
		}
		items = append(items, item)
	}
	if err := s.saveWatchlistUnlocked(items); err != nil {
		return nil, err
	}
	return items, nil
}

func (s *Store) RemoveWatch(mint string) ([]WatchItem, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	items, err := s.loadWatchlistUnlocked()
	if err != nil {
		return nil, err
	}
	out := items[:0]
	for _, it := range items {
		if it.Mint != mint {
			out = append(out, it)
		}
	}
	if err := s.saveWatchlistUnlocked(out); err != nil {
		return nil, err
	}
	return out, nil
}

// ReplaceSourceWatches keeps items from other sources and replaces those matching source.
func (s *Store) ReplaceSourceWatches(source string, next []WatchItem) ([]WatchItem, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	items, err := s.loadWatchlistUnlocked()
	if err != nil {
		return nil, err
	}
	out := make([]WatchItem, 0, len(items)+len(next))
	for _, it := range items {
		if it.Source != source {
			out = append(out, it)
		}
	}
	seen := map[string]bool{}
	for _, it := range next {
		if it.Mint == "" || seen[it.Mint] {
			continue
		}
		seen[it.Mint] = true
		if it.Source == "" {
			it.Source = source
		}
		if it.AddedAt.IsZero() {
			it.AddedAt = time.Now().UTC()
		}
		out = append(out, it)
	}
	if err := s.saveWatchlistUnlocked(out); err != nil {
		return nil, err
	}
	return out, nil
}

// ClassifyStatus maps exit reason / rug metrics to history status.
func ClassifyStatus(exitReason string, rugScore float64, open bool) string {
	if open {
		return "open_saved"
	}
	low := strings.ToLower(exitReason)
	if strings.Contains(low, "whale_dump") || strings.Contains(low, "dev_sold") ||
		strings.Contains(low, "stop_loss") || rugScore >= 70 {
		return "rugged"
	}
	return "closed"
}
