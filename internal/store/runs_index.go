package store

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type runIndexFile struct {
	Runs []RunSummary `json:"runs"`
}

// runFileMeta is SavedRun header without the heavy response blob.
type runFileMeta struct {
	ID          string    `json:"id"`
	Label       string    `json:"label,omitempty"`
	SavedAt     time.Time `json:"savedAt"`
	Source      string    `json:"source,omitempty"`
	EndEquity   float64   `json:"endEquity,omitempty"`
	TotalPnL    float64   `json:"totalPnl,omitempty"`
	CoinCount   int       `json:"coinCount,omitempty"`
	WinRate     float64   `json:"winRate,omitempty"`
	MaxDDPct    float64   `json:"maxDrawdownPct,omitempty"`
	OpenCount   int       `json:"openCount,omitempty"`
	ClosedCount int       `json:"closedCount,omitempty"`
	Signals     int       `json:"signals,omitempty"`
	EntryKinds  []string  `json:"entryKinds,omitempty"`
}

func metaFromRun(run SavedRun) RunSummary {
	return RunSummary{
		ID:          run.ID,
		Label:       run.Label,
		SavedAt:     run.SavedAt,
		Source:      run.Source,
		EndEquity:   run.EndEquity,
		TotalPnL:    run.TotalPnL,
		CoinCount:   run.CoinCount,
		WinRate:     run.WinRate,
		MaxDDPct:    run.MaxDDPct,
		OpenCount:   run.OpenCount,
		ClosedCount: run.ClosedCount,
		Signals:     run.Signals,
		EntryKinds:  run.EntryKinds,
	}
}

func metaFromFileMeta(m runFileMeta) RunSummary {
	return RunSummary{
		ID:          m.ID,
		Label:       m.Label,
		SavedAt:     m.SavedAt,
		Source:      m.Source,
		EndEquity:   m.EndEquity,
		TotalPnL:    m.TotalPnL,
		CoinCount:   m.CoinCount,
		WinRate:     m.WinRate,
		MaxDDPct:    m.MaxDDPct,
		OpenCount:   m.OpenCount,
		ClosedCount: m.ClosedCount,
		Signals:     m.Signals,
		EntryKinds:  m.EntryKinds,
	}
}

func (s *Store) indexPath() string {
	return filepath.Join(s.Root, "runs", "index.json")
}

func (s *Store) loadRunIndexLocked() ([]RunSummary, error) {
	b, err := os.ReadFile(s.indexPath())
	if err != nil {
		if os.IsNotExist(err) {
			return s.rebuildRunIndexLocked()
		}
		return nil, err
	}
	var idx runIndexFile
	if err := json.Unmarshal(b, &idx); err != nil {
		return s.rebuildRunIndexLocked()
	}
	return idx.Runs, nil
}

func (s *Store) writeRunIndexLocked(runs []RunSummary) error {
	sort.Slice(runs, func(i, j int) bool {
		return runs[i].SavedAt.After(runs[j].SavedAt)
	})
	b, err := json.Marshal(runIndexFile{Runs: runs})
	if err != nil {
		return err
	}
	return os.WriteFile(s.indexPath(), b, 0o644)
}

func (s *Store) rebuildRunIndexLocked() ([]RunSummary, error) {
	dir := filepath.Join(s.Root, "runs")
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return []RunSummary{}, nil
		}
		return nil, err
	}
	var out []RunSummary
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasPrefix(name, "run_") || !strings.HasSuffix(name, ".json") {
			continue
		}
		if strings.HasSuffix(name, ".detail.json") {
			continue
		}
		b, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			continue
		}
		var meta runFileMeta
		if err := json.Unmarshal(b, &meta); err != nil || meta.ID == "" {
			continue
		}
		out = append(out, metaFromFileMeta(meta))
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].SavedAt.After(out[j].SavedAt)
	})
	_ = s.writeRunIndexLocked(out)
	return out, nil
}

func (s *Store) upsertRunIndexLocked(summary RunSummary) error {
	runs, err := s.loadRunIndexLocked()
	if err != nil {
		runs = []RunSummary{}
	}
	found := false
	for i := range runs {
		if runs[i].ID == summary.ID {
			runs[i] = summary
			found = true
			break
		}
	}
	if !found {
		runs = append(runs, summary)
	}
	return s.writeRunIndexLocked(runs)
}

func (s *Store) removeFromRunIndexLocked(id string) error {
	runs, err := s.loadRunIndexLocked()
	if err != nil {
		return err
	}
	out := runs[:0]
	for _, r := range runs {
		if r.ID != id {
			out = append(out, r)
		}
	}
	return s.writeRunIndexLocked(out)
}

// RunDetail holds full trades/equity for CSV export (kept out of the main run file).
type RunDetail struct {
	Trades json.RawMessage `json:"trades,omitempty"`
	Equity json.RawMessage `json:"equity,omitempty"`
}

func (s *Store) runDetailPath(id string) string {
	return filepath.Join(s.Root, "runs", "run_"+id+".detail.json")
}

func (s *Store) SaveRunDetail(id string, detail RunDetail) error {
	if id == "" {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	b, err := json.Marshal(detail)
	if err != nil {
		return err
	}
	return os.WriteFile(s.runDetailPath(id), b, 0o644)
}

func (s *Store) LoadRunDetail(id string) (*RunDetail, error) {
	id = filepath.Base(strings.TrimSpace(id))
	if id == "" {
		return nil, os.ErrNotExist
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	b, err := os.ReadFile(s.runDetailPath(id))
	if err != nil {
		return nil, err
	}
	var d RunDetail
	if err := json.Unmarshal(b, &d); err != nil {
		return nil, err
	}
	return &d, nil
}
