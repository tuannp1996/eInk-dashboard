// Package dashboard assembles the normalized Dashboard from cached entries,
// computing staleness at read time, and validates pushed usage snapshots.
package dashboard

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"time"

	"github.com/viaiz/eink-dashboard/internal/cache"
	"github.com/viaiz/eink-dashboard/internal/config"
	"github.com/viaiz/eink-dashboard/internal/model"
)

type Builder struct {
	Store cache.Store
	Cfg   *config.Config
	Now   func() time.Time
}

func (b *Builder) now() time.Time {
	if b.Now != nil {
		return b.Now()
	}
	return time.Now()
}

// Build reads all sections and returns the dashboard.
func (b *Builder) Build(ctx context.Context) (*model.Dashboard, error) {
	now := b.now()
	c := b.Cfg
	d := &model.Dashboard{GeneratedAt: now, Timezone: c.Timezone}
	var err error
	if d.Claude, err = section[model.UsageData](ctx, b.Store, "usage:claude", true, c.Usage.StaleAfter.Duration, now); err != nil {
		return nil, err
	}
	if d.Codex, err = section[model.UsageData](ctx, b.Store, "usage:codex", true, c.Usage.StaleAfter.Duration, now); err != nil {
		return nil, err
	}
	if d.Weather, err = section[model.WeatherData](ctx, b.Store, "weather", c.Weather.Enabled, c.Weather.StaleAfter.Duration, now); err != nil {
		return nil, err
	}
	if d.Gold, err = section[model.GoldData](ctx, b.Store, "gold", c.Gold.Enabled, c.Gold.StaleAfter.Duration, now); err != nil {
		return nil, err
	}
	if d.Trends, err = section[model.TrendsData](ctx, b.Store, "trends", c.Trends.Enabled, c.Trends.StaleAfter.Duration, now); err != nil {
		return nil, err
	}
	for _, u := range []*model.Section[model.UsageData]{&d.Claude, &d.Codex} {
		if u.Data != nil {
			markResets(u.Data, now)
		}
	}
	type named struct {
		name string
		m    model.Meta
	}
	all := []named{{"Claude", d.Claude.Meta}, {"Codex", d.Codex.Meta}, {"Weather", d.Weather.Meta}, {"Gold", d.Gold.Meta}, {"Trends", d.Trends.Meta}}
	for _, s := range all {
		if s.m.Status == model.StatusDisabled {
			continue
		}
		d.Health.SourcesTotal++
		switch s.m.Status {
		case model.StatusOK:
			d.Health.SourcesOK++
		case model.StatusStale:
			d.Health.Stale = append(d.Health.Stale, s.name)
		default:
			d.Health.Failing = append(d.Health.Failing, s.name)
		}
		if s.m.UpdatedAt != nil && (d.LastSync == nil || s.m.UpdatedAt.After(*d.LastSync)) {
			d.LastSync = s.m.UpdatedAt
		}
	}
	return d, nil
}

func section[T any](ctx context.Context, s cache.Store, key string, enabled bool, staleAfter time.Duration, now time.Time) (model.Section[T], error) {
	var out model.Section[T]
	if !enabled {
		out.Meta.Status = model.StatusDisabled
		return out, nil
	}
	e, ok, err := s.Get(ctx, key)
	if err != nil {
		// Cache backend failure: report as error but do not fail the whole dashboard.
		out.Meta.Status = model.StatusError
		out.Meta.LastError = "cache: " + err.Error()
		return out, nil
	}
	out.Meta.StaleAfter = staleAfter.String()
	if !ok {
		out.Meta.Status = model.StatusUnavailable
		return out, nil
	}
	out.Meta.Source = e.Source
	out.Meta.UpdatedAt = e.UpdatedAt
	out.Meta.SourceTime = e.SourceTime
	out.Meta.LastAttempt = e.LastAttempt
	out.Meta.LastError = e.LastError
	if len(e.Data) > 0 {
		var v T
		if err := json.Unmarshal(e.Data, &v); err != nil {
			return out, fmt.Errorf("decode %s: %w", key, err)
		}
		out.Data = &v
	}
	switch {
	case out.Data == nil && e.LastError != "":
		out.Meta.Status = model.StatusError
	case out.Data == nil:
		out.Meta.Status = model.StatusUnavailable
	case e.UpdatedAt == nil || (staleAfter > 0 && now.Sub(*e.UpdatedAt) > staleAfter):
		out.Meta.Status = model.StatusStale
	default:
		out.Meta.Status = model.StatusOK
	}
	return out, nil
}

func markResets(u *model.UsageData, now time.Time) {
	for _, w := range []*model.QuotaWindow{u.FiveHour, u.Weekly} {
		if w != nil && w.ResetsAt != nil && !w.ResetsAt.After(now) {
			w.ResetPassed = true
		}
	}
}

// ---------- usage ingest ----------

// UsageReport is the payload a local usage agent POSTs to /api/ingest/usage.
type UsageReport struct {
	Provider   string            `json:"provider"`
	Origin     string            `json:"origin"`
	Host       string            `json:"host,omitempty"`
	ObservedAt time.Time         `json:"observed_at"`
	FiveHour   *UsageWindowInput `json:"five_hour,omitempty"`
	Weekly     *UsageWindowInput `json:"weekly,omitempty"`
}

type UsageWindowInput struct {
	UsedPercent   *float64 `json:"used_percent"`
	ResetsAt      *int64   `json:"resets_at,omitempty"` // unix seconds
	WindowMinutes int      `json:"window_minutes,omitempty"`
}

// Validate checks the report and converts it to UsageData.
func (r *UsageReport) Validate(now time.Time) (*model.UsageData, error) {
	if r.Provider != "claude" && r.Provider != "codex" {
		return nil, fmt.Errorf("provider must be 'claude' or 'codex'")
	}
	if r.ObservedAt.IsZero() {
		return nil, fmt.Errorf("observed_at is required")
	}
	if r.ObservedAt.After(now.Add(5 * time.Minute)) {
		return nil, fmt.Errorf("observed_at is in the future")
	}
	if r.FiveHour == nil && r.Weekly == nil {
		return nil, fmt.Errorf("at least one of five_hour or weekly is required")
	}
	if len(r.Origin) > 64 || len(r.Host) > 64 {
		return nil, fmt.Errorf("origin/host too long")
	}
	conv := func(name string, w *UsageWindowInput) (*model.QuotaWindow, error) {
		if w == nil {
			return nil, nil
		}
		q := &model.QuotaWindow{WindowMinutes: w.WindowMinutes}
		if w.UsedPercent != nil {
			p := *w.UsedPercent
			if math.IsNaN(p) || p < 0 || p > 100 {
				return nil, fmt.Errorf("%s.used_percent must be within 0..100", name)
			}
			q.UsedPercent = &p
		}
		if w.ResetsAt != nil {
			if *w.ResetsAt <= 0 {
				return nil, fmt.Errorf("%s.resets_at invalid", name)
			}
			q.ResetsAt = model.Ptr(time.Unix(*w.ResetsAt, 0).UTC())
		}
		return q, nil
	}
	fh, err := conv("five_hour", r.FiveHour)
	if err != nil {
		return nil, err
	}
	wk, err := conv("weekly", r.Weekly)
	if err != nil {
		return nil, err
	}
	return &model.UsageData{Provider: r.Provider, FiveHour: fh, Weekly: wk, ObservedAt: r.ObservedAt.UTC(), Origin: r.Origin, Host: r.Host}, nil
}

// StoreUsage writes a validated snapshot. Older snapshots never replace newer ones.
func StoreUsage(ctx context.Context, s cache.Store, u *model.UsageData, now time.Time) (bool, error) {
	key := "usage:" + u.Provider
	prev, ok, err := s.Get(ctx, key)
	if err != nil {
		return false, err
	}
	if ok && len(prev.Data) > 0 {
		var p model.UsageData
		if json.Unmarshal(prev.Data, &p) == nil && p.ObservedAt.After(u.ObservedAt) {
			return false, nil
		}
	}
	b, err := json.Marshal(u)
	if err != nil {
		return false, err
	}
	src := "local agent"
	if u.Origin != "" {
		src = u.Origin
	}
	obs := u.ObservedAt
	return true, s.Set(ctx, key, cache.Entry{Data: b, Source: src, UpdatedAt: &obs, SourceTime: &obs, LastAttempt: &now})
}
