// Package mock seeds the cache with example data for local UI preview.
// Every section's source is labelled "MOCK" so it can never be mistaken
// for real data on screen.
package mock

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/viaiz/eink-dashboard/internal/cache"
	"github.com/viaiz/eink-dashboard/internal/model"
)

type window struct {
	UsedPercent     *float64 `json:"used_percent"`
	ResetsInMinutes *int     `json:"resets_in_minutes"`
	WindowMinutes   int      `json:"window_minutes"`
}

type usage struct {
	FiveHour           *window `json:"five_hour"`
	Weekly             *window `json:"weekly"`
	Origin             string  `json:"origin"`
	ObservedMinutesAgo int     `json:"observed_minutes_ago"`
}

type File struct {
	Claude  *usage             `json:"claude"`
	Codex   *usage             `json:"codex"`
	Weather *model.WeatherData `json:"weather"`
	Gold    *model.GoldData    `json:"gold"`
	Trends  *model.TrendsData  `json:"trends"`
	// Optional: make a section look old to preview the stale indicator.
	StaleSections []string `json:"stale_sections"`
}

// Load reads a mock file and writes all sections into the store.
func Load(ctx context.Context, path string, s cache.Store, now time.Time) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var f File
	if err := json.Unmarshal(b, &f); err != nil {
		return fmt.Errorf("mock %s: %w", path, err)
	}
	stale := map[string]bool{}
	for _, k := range f.StaleSections {
		stale[k] = true
	}
	put := func(key string, v any) error {
		if v == nil {
			return nil
		}
		data, err := json.Marshal(v)
		if err != nil {
			return err
		}
		t := now.Add(-2 * time.Minute)
		if stale[key] {
			t = now.Add(-30 * time.Hour)
		}
		return s.Set(ctx, key, cache.Entry{Data: data, Source: "MOCK data", UpdatedAt: &t, SourceTime: &t, LastAttempt: &t})
	}
	conv := func(p string, u *usage) *model.UsageData {
		if u == nil {
			return nil
		}
		w := func(x *window) *model.QuotaWindow {
			if x == nil {
				return nil
			}
			q := &model.QuotaWindow{UsedPercent: x.UsedPercent, WindowMinutes: x.WindowMinutes}
			if x.ResetsInMinutes != nil {
				q.ResetsAt = model.Ptr(now.Add(time.Duration(*x.ResetsInMinutes) * time.Minute).Truncate(time.Minute))
			}
			return q
		}
		return &model.UsageData{Provider: p, FiveHour: w(u.FiveHour), Weekly: w(u.Weekly),
			ObservedAt: now.Add(-time.Duration(u.ObservedMinutesAgo) * time.Minute), Origin: "MOCK " + u.Origin}
	}
	if f.Weather != nil {
		f.Weather.Current.Time = now.Truncate(15 * time.Minute)
		// Re-date the forecast so "Today" lines up with the current date.
		for i := range f.Weather.Daily {
			f.Weather.Daily[i].Date = now.AddDate(0, 0, i).Format("2006-01-02")
		}
	}
	for _, kv := range []struct {
		k string
		v any
	}{
		{"usage:claude", nilIfEmpty(conv("claude", f.Claude))},
		{"usage:codex", nilIfEmpty(conv("codex", f.Codex))},
		{"weather", nilIfEmpty(f.Weather)},
		{"gold", nilIfEmpty(f.Gold)},
		{"trends", nilIfEmpty(f.Trends)},
	} {
		if err := put(kv.k, kv.v); err != nil {
			return err
		}
	}
	return nil
}

func nilIfEmpty[T any](p *T) any {
	if p == nil {
		return nil
	}
	return p
}
