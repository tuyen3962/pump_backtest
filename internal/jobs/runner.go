package jobs

import (
	"context"
	"sync"
	"time"
)

type Status string

const (
	StatusQueued    Status = "queued"
	StatusRunning   Status = "running"
	StatusDone      Status = "done"
	StatusFailed    Status = "failed"
	StatusCancelled Status = "cancelled"
)

type Job struct {
	ID        string    `json:"id"`
	Label     string    `json:"label,omitempty"`
	Status    Status    `json:"status"`
	CreatedAt time.Time `json:"createdAt"`
	StartedAt time.Time `json:"startedAt,omitempty"`
	FinishedAt time.Time `json:"finishedAt,omitempty"`
	Error     string    `json:"error,omitempty"`
	RunID     string    `json:"runId,omitempty"`
	Progress  string    `json:"progress,omitempty"`
}

type Runner struct {
	mu      sync.Mutex
	jobs    map[string]*Job
	queue   chan string
	workers int
	work    func(ctx context.Context, job *Job) error
	cancel  map[string]context.CancelFunc
}

func New(workers int, work func(ctx context.Context, job *Job) error) *Runner {
	if workers < 1 {
		workers = 2
	}
	r := &Runner{
		jobs:    map[string]*Job{},
		queue:   make(chan string, 256),
		workers: workers,
		work:    work,
		cancel:  map[string]context.CancelFunc{},
	}
	for i := 0; i < workers; i++ {
		go r.loop()
	}
	return r
}

func (r *Runner) Enqueue(id, label string) (*Job, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.jobs[id]; ok {
		return nil, errExists
	}
	j := &Job{
		ID:        id,
		Label:     label,
		Status:    StatusQueued,
		CreatedAt: time.Now().UTC(),
	}
	r.jobs[id] = j
	select {
	case r.queue <- id:
	default:
		j.Status = StatusFailed
		j.Error = "job queue full"
		return j, errQueueFull
	}
	return cloneJob(j), nil
}

func (r *Runner) Get(id string) (*Job, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	j, ok := r.jobs[id]
	if !ok {
		return nil, false
	}
	return cloneJob(j), true
}

func (r *Runner) List(limit int) []*Job {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]*Job, 0, len(r.jobs))
	for _, j := range r.jobs {
		out = append(out, cloneJob(j))
	}
	// newest first
	for i := 0; i < len(out); i++ {
		for k := i + 1; k < len(out); k++ {
			if out[k].CreatedAt.After(out[i].CreatedAt) {
				out[i], out[k] = out[k], out[i]
			}
		}
	}
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out
}

func (r *Runner) SetProgress(id, msg string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if j, ok := r.jobs[id]; ok {
		j.Progress = msg
	}
}

// SetRunID updates the linked run id while a long session job is still running.
func (r *Runner) SetRunID(id, runID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if j, ok := r.jobs[id]; ok {
		j.RunID = runID
	}
}

// Cancel stops a queued or running job. Returns false if missing or already finished.
func (r *Runner) Cancel(id string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	j, ok := r.jobs[id]
	if !ok {
		return false
	}
	switch j.Status {
	case StatusQueued:
		j.Status = StatusCancelled
		j.FinishedAt = time.Now().UTC()
		j.Error = "cancelled"
		j.Progress = ""
		return true
	case StatusRunning:
		if cancel, ok := r.cancel[id]; ok {
			cancel()
		}
		return true
	default:
		return false
	}
}

func (r *Runner) loop() {
	for id := range r.queue {
		r.mu.Lock()
		j, ok := r.jobs[id]
		if !ok || j.Status != StatusQueued {
			r.mu.Unlock()
			continue
		}
		j.Status = StatusRunning
		j.StartedAt = time.Now().UTC()
		j.Progress = "starting"
		ctx, cancel := context.WithCancel(context.Background())
		r.cancel[id] = cancel
		jobCopy := *j
		r.mu.Unlock()

		err := r.work(ctx, &jobCopy)

		r.mu.Lock()
		if cur, ok := r.jobs[id]; ok {
			cur.FinishedAt = time.Now().UTC()
			cur.Progress = ""
			if err != nil {
				if ctx.Err() != nil {
					cur.Status = StatusCancelled
					cur.Error = "cancelled"
				} else {
					cur.Status = StatusFailed
					cur.Error = err.Error()
				}
			} else {
				cur.Status = StatusDone
				cur.RunID = jobCopy.RunID
				cur.Label = jobCopy.Label
			}
		}
		delete(r.cancel, id)
		cancel()
		r.mu.Unlock()
	}
}

var (
	errExists    = errString("job already exists")
	errQueueFull = errString("job queue full")
)

type errString string

func (e errString) Error() string { return string(e) }

func cloneJob(j *Job) *Job {
	c := *j
	return &c
}
