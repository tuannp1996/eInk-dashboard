// Package cache stores the latest successful result of each collector.
// Backends: in-memory (optionally persisted to a JSON file) or Redis.
package cache

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

// Entry is what a collector stores. Data holds the last *successful* payload
// so it can be served as a fallback when later fetches fail.
type Entry struct {
	Data        json.RawMessage `json:"data,omitempty"`
	Source      string          `json:"source"`
	UpdatedAt   *time.Time      `json:"updated_at,omitempty"`
	SourceTime  *time.Time      `json:"source_time,omitempty"`
	LastAttempt *time.Time      `json:"last_attempt,omitempty"`
	LastError   string          `json:"last_error,omitempty"`
}

type Store interface {
	Get(ctx context.Context, key string) (Entry, bool, error)
	Set(ctx context.Context, key string, e Entry) error
	Ping(ctx context.Context) error
	Close() error
}

// ---------- memory (+ optional file persistence) ----------

type Memory struct {
	mu   sync.RWMutex
	m    map[string]Entry
	path string
}

// NewMemory creates an in-memory store. If path is non-empty the store is
// loaded from and persisted to that JSON file so data survives restarts.
func NewMemory(path string) (*Memory, error) {
	s := &Memory{m: map[string]Entry{}, path: path}
	if path != "" {
		b, err := os.ReadFile(path)
		if err == nil {
			if err := json.Unmarshal(b, &s.m); err != nil {
				return nil, err
			}
		} else if !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
	}
	return s, nil
}

func (s *Memory) Get(_ context.Context, key string) (Entry, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	e, ok := s.m[key]
	return e, ok, nil
}

func (s *Memory) Set(_ context.Context, key string, e Entry) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.m[key] = e
	if s.path == "" {
		return nil
	}
	b, err := json.Marshal(s.m)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}

func (s *Memory) Ping(context.Context) error { return nil }
func (s *Memory) Close() error               { return nil }

// ---------- Redis ----------

type Redis struct {
	c      *redis.Client
	prefix string
}

func NewRedis(url string) (*Redis, error) {
	opt, err := redis.ParseURL(url)
	if err != nil {
		return nil, err
	}
	return &Redis{c: redis.NewClient(opt), prefix: "viaiz:"}, nil
}

func (r *Redis) Get(ctx context.Context, key string) (Entry, bool, error) {
	b, err := r.c.Get(ctx, r.prefix+key).Bytes()
	if errors.Is(err, redis.Nil) {
		return Entry{}, false, nil
	}
	if err != nil {
		return Entry{}, false, err
	}
	var e Entry
	if err := json.Unmarshal(b, &e); err != nil {
		return Entry{}, false, err
	}
	return e, true, nil
}

func (r *Redis) Set(ctx context.Context, key string, e Entry) error {
	b, err := json.Marshal(e)
	if err != nil {
		return err
	}
	// No TTL: last good value is kept as fallback; staleness is computed on read.
	return r.c.Set(ctx, r.prefix+key, b, 0).Err()
}

func (r *Redis) Ping(ctx context.Context) error { return r.c.Ping(ctx).Err() }
func (r *Redis) Close() error                   { return r.c.Close() }
