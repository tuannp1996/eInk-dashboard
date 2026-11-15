// Package collector contains independent data collectors and the scheduler
// that runs them with timeouts, retries (exponential backoff) and a cached
// fallback: a failed fetch never overwrites the last good data.
package collector

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math/rand"
	"net/http"
	"sync"
	"time"

	"github.com/viaiz/eink-dashboard/internal/cache"
)

// Result is what a collector returns on success.
type Result struct {
	Data       any
	SourceTime *time.Time // timestamp reported by the upstream source, if any
}

// Collector fetches one data section.
type Collector interface {
	Key() string    // cache key, e.g. "weather"
	Source() string // human-readable source, e.g. "Open-Meteo"
	Fetch(ctx context.Context) (Result, error)
}

// Spec configures how a collector is scheduled.
type Spec struct {
	Collector Collector
	Interval  time.Duration
	Timeout   time.Duration
	Retries   int // attempts = Retries+1
}

// Runner schedules collectors.
type Runner struct {
	store  cache.Store
	log    *slog.Logger
	specs  []Spec
	now    func() time.Time
	sleep  func(ctx context.Context, d time.Duration) error
	baseBO time.Duration
	wg     sync.WaitGroup
}

func NewRunner(store cache.Store, log *slog.Logger, specs ...Spec) *Runner {
	return &Runner{store: store, log: log, specs: specs, now: time.Now, sleep: sleepCtx, baseBO: 2 * time.Second}
}

// Start launches one goroutine per collector. The first run happens immediately.
func (r *Runner) Start(ctx context.Context) {
	for _, s := range r.specs {
		s := s
		r.wg.Add(1)
		go func() {
			defer r.wg.Done()
			r.loop(ctx, s)
		}()
	}
}

func (r *Runner) Wait() { r.wg.Wait() }

func (r *Runner) loop(ctx context.Context, s Spec) {
	t := time.NewTicker(s.Interval)
	defer t.Stop()
	for {
		r.RunOnce(ctx, s)
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

// RunOnce performs one fetch cycle with retries and stores the outcome.
func (r *Runner) RunOnce(ctx context.Context, s Spec) error {
	key := s.Collector.Key()
	log := r.log.With("collector", key)
	res, attempts, err := r.fetchWithRetry(ctx, s)
	now := r.now()

	prev, _, gerr := r.store.Get(ctx, key)
	if gerr != nil {
		log.Warn("cache read failed", "err", gerr)
	}
	e := prev
	e.Source = s.Collector.Source()
	e.LastAttempt = &now
	if err != nil {
		// Keep previous Data/UpdatedAt as cached fallback.
		e.LastError = err.Error()
		log.Warn("collect failed", "attempts", attempts, "err", err, "has_fallback", prev.Data != nil)
	} else {
		b, merr := json.Marshal(res.Data)
		if merr != nil {
			return merr
		}
		e.Data = b
		e.UpdatedAt = &now
		e.SourceTime = res.SourceTime
		e.LastError = ""
		log.Info("collect ok", "attempts", attempts)
	}
	if serr := r.store.Set(ctx, key, e); serr != nil {
		log.Error("cache write failed", "err", serr)
		return serr
	}
	return err
}

func (r *Runner) fetchWithRetry(ctx context.Context, s Spec) (Result, int, error) {
	var lastErr error
	attempts := s.Retries + 1
	if attempts < 1 {
		attempts = 1
	}
	for i := 0; i < attempts; i++ {
		if i > 0 {
			if err := r.sleep(ctx, Backoff(r.baseBO, i)); err != nil {
				return Result{}, i, err
			}
		}
		cctx, cancel := context.WithTimeout(ctx, s.Timeout)
		res, err := s.Collector.Fetch(cctx)
		cancel()
		if err == nil {
			return res, i + 1, nil
		}
		lastErr = err
		var perm *PermanentError
		if errors.As(err, &perm) {
			return Result{}, i + 1, err
		}
	}
	return Result{}, attempts, lastErr
}

// Backoff returns base*2^(attempt-1) with ±20% jitter, capped at 2 minutes.
func Backoff(base time.Duration, attempt int) time.Duration {
	d := base << (attempt - 1)
	if d > 2*time.Minute || d <= 0 {
		d = 2 * time.Minute
	}
	jitter := time.Duration(float64(d) * (rand.Float64()*0.4 - 0.2))
	return d + jitter
}

func sleepCtx(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// PermanentError stops retries (e.g. HTTP 4xx, parse errors).
type PermanentError struct{ Err error }

func (e *PermanentError) Error() string { return e.Err.Error() }
func (e *PermanentError) Unwrap() error { return e.Err }

var httpClient = &http.Client{Timeout: 30 * time.Second}

const userAgent = "ViaiZ-EInk-Dashboard/1.0 (+self-hosted)"

// getBody performs a GET and returns the body, classifying errors.
func getBody(ctx context.Context, url string, accept string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, &PermanentError{err}
	}
	req.Header.Set("User-Agent", userAgent)
	if accept != "" {
		req.Header.Set("Accept", accept)
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 400 && resp.StatusCode < 500 && resp.StatusCode != 429 {
		return nil, &PermanentError{fmt.Errorf("GET %s: HTTP %d", redactURL(url), resp.StatusCode)}
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET %s: HTTP %d", redactURL(url), resp.StatusCode)
	}
	return body, nil
}

// redactURL strips query strings (may contain API keys) from logged URLs.
func redactURL(u string) string {
	for i := 0; i < len(u); i++ {
		if u[i] == '?' {
			return u[:i] + "?…"
		}
	}
	return u
}
