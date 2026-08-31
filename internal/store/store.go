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
	ID         string          `json:"id"`
	Label      string          `json:"label,omitempty"`
	SavedAt    time.Time       `json:"savedAt"`
	Request    json.RawMessage `json:"request,omitempty"`
	Response   json.RawMessage `json:"response"`
	Source     string          `json:"source,omitempty"`
	EndEquity  float64         `json:"endEquity,omitempty"`
	TotalPnL   float64         `json:"totalPnl,omitempty"`
	CoinCount  int             `json:"coinCount,omitempty"`
	WinRate    float64         `json:"winRate,omitempty"`
	MaxDDPct   float64         `json:"maxDrawdownPct,omitempty"`
	OpenCount  int             `json:"openCount,omitempty"`
	ClosedCount int            `json:"closedCount,omitempty"`
	Signals    int             `json:"signals,omitempty"`
	EntryKinds []string        `json:"entryKinds,omitempty"`
}

type RunSummary struct {
	ID          string    `json:"id"`
	Label       string    `json:"label,omitempty"`
	SavedAt     time.Time `json:"savedAt"`
	Source      string    `json:"source,omitempty"`
	EndEquity   float64   `json:"endEquity"`
	TotalPnL    float64   `json:"totalPnl"`
	CoinCount   int       `json:"coinCount"`
	WinRate     float64   `json:"winRate"`
	MaxDDPct    float64   `json:"maxDrawdownPct"`
	OpenCount   int       `json:"openCount"`
	ClosedCount int       `json:"closedCount"`
	Signals     int       `json:"signals"`
	EntryKinds  []string  `json:"entryKinds,omitempty"`
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
	b, err := json.Marshal(run)
	if err != nil {
		return err
	}
	last := filepath.Join(s.Root, "runs", "last.json")
	archive := filepath.Join(s.Root, "runs", "run_"+run.ID+".json")
	if err := os.WriteFile(last, b, 0o644); err != nil {
		return err
	}
	if err := os.WriteFile(archive, b, 0o644); err != nil {
		return err
	}
	return s.upsertRunIndexLocked(metaFromRun(run))
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

func (s *Store) LoadRun(id string) (*SavedRun, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, os.ErrNotExist
	}
	// Prevent path traversal.
	id = filepath.Base(id)
	s.mu.Lock()
	defer s.mu.Unlock()
	b, err := os.ReadFile(filepath.Join(s.Root, "runs", "run_"+id+".json"))
	if err != nil {
		return nil, err
	}
	var run SavedRun
	if err := json.Unmarshal(b, &run); err != nil {
		return nil, err
	}
	return &run, nil
}

// ListRuns returns newest-first run summaries from index.json (fast).
func (s *Store) ListRuns(limit int) ([]RunSummary, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out, err := s.loadRunIndexLocked()
	if err != nil {
		return nil, err
	}
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (s *Store) DeleteRun(id string) error {
	id = strings.TrimSpace(filepath.Base(id))
	if id == "" {
		return os.ErrNotExist
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	path := filepath.Join(s.Root, "runs", "run_"+id+".json")
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	_ = os.Remove(s.runDetailPath(id))
	_ = s.removeFromRunIndexLocked(id)
	lastPath := filepath.Join(s.Root, "runs", "last.json")
	b, err := os.ReadFile(lastPath)
	if err == nil {
		var run SavedRun
		if json.Unmarshal(b, &run) == nil && run.ID == id {
			_ = os.Remove(lastPath)
		}
	}
	return nil
}

// DeleteRunsBefore removes archived runs with SavedAt strictly before cutoff.
func (s *Store) DeleteRunsBefore(cutoff time.Time) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	dir := filepath.Join(s.Root, "runs")
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}
	n := 0
	var lastID string
	if b, err := os.ReadFile(filepath.Join(dir, "last.json")); err == nil {
		var last SavedRun
		if json.Unmarshal(b, &last) == nil {
			lastID = last.ID
		}
	}
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasPrefix(name, "run_") || !strings.HasSuffix(name, ".json") {
			continue
		}
		p := filepath.Join(dir, name)
		b, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		var run SavedRun
		if err := json.Unmarshal(b, &run); err != nil {
			continue
		}
		if run.SavedAt.IsZero() || !run.SavedAt.Before(cutoff) {
			continue
		}
		if err := os.Remove(p); err != nil {
			continue
		}
		_ = os.Remove(s.runDetailPath(run.ID))
		n++
		if run.ID == lastID {
			_ = os.Remove(filepath.Join(dir, "last.json"))
		}
	}
	_, _ = s.rebuildRunIndexLocked()
	return n, nil
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

// IsRuggedTrade reports whether a closed leg should appear under the rugged filter.
func IsRuggedTrade(exitReason string, rugScore float64, rugLabel string) bool {
	low := strings.ToLower(strings.TrimSpace(exitReason))
	switch {
	case strings.Contains(low, "whale_dump"),
		strings.Contains(low, "dev_sold"),
		strings.HasPrefix(low, "out:must:"):
		return true
	case strings.HasPrefix(low, "out:") && strings.Contains(low, "stale"):
		return true
	}
	switch strings.ToLower(strings.TrimSpace(rugLabel)) {
	case "critical", "high":
		return true
	}
	return rugScore >= 70
}

// ClassifyStatus maps exit reason / rug metrics to history status.
func ClassifyStatus(exitReason string, rugScore float64, rugLabel string, open bool) string {
	if open {
		return "open_saved"
	}
	if IsRuggedTrade(exitReason, rugScore, rugLabel) {
		return "rugged"
	}
	return "closed"
}
