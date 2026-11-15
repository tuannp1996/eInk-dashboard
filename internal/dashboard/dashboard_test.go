package dashboard

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/viaiz/eink-dashboard/internal/cache"
	"github.com/viaiz/eink-dashboard/internal/config"
	"github.com/viaiz/eink-dashboard/internal/model"
)

func ptr[T any](v T) *T { return &v }

func TestBuildStatuses(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 11, 15, 12, 0, 0, 0, time.UTC)
	store, _ := cache.NewMemory("")
	cfg := config.Default()
	cfg.Trends.Enabled = false

	fresh := now.Add(-10 * time.Minute)
	old := now.Add(-5 * time.Hour)
	wd, _ := json.Marshal(model.WeatherData{Location: "x"})
	gd, _ := json.Marshal(model.GoldData{})
	_ = store.Set(ctx, "weather", cache.Entry{Data: wd, UpdatedAt: &fresh})
	_ = store.Set(ctx, "gold", cache.Entry{Data: gd, UpdatedAt: ptr(now.Add(-7 * time.Hour)), LastError: "HTTP 500"})
	_ = old

	b := &Builder{Store: store, Cfg: &cfg, Now: func() time.Time { return now }}
	d, err := b.Build(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if d.Weather.Meta.Status != model.StatusOK {
		t.Errorf("weather: %s", d.Weather.Meta.Status)
	}
	if d.Gold.Meta.Status != model.StatusStale || d.Gold.Data == nil {
		t.Errorf("gold should be stale with cached fallback: %s", d.Gold.Meta.Status)
	}
	if d.Trends.Meta.Status != model.StatusDisabled {
		t.Errorf("trends: %s", d.Trends.Meta.Status)
	}
	if d.Claude.Meta.Status != model.StatusUnavailable || d.Claude.Data != nil {
		t.Errorf("claude must be unavailable without agent data")
	}
	if d.Health.SourcesTotal != 4 || d.Health.SourcesOK != 1 {
		t.Errorf("health %+v", d.Health)
	}
	if d.LastSync == nil || !d.LastSync.Equal(fresh) {
		t.Errorf("last sync %v", d.LastSync)
	}
}

func TestErrorWithoutData(t *testing.T) {
	ctx := context.Background()
	store, _ := cache.NewMemory("")
	cfg := config.Default()
	_ = store.Set(ctx, "weather", cache.Entry{LastError: "timeout"})
	d, _ := (&Builder{Store: store, Cfg: &cfg}).Build(ctx)
	if d.Weather.Meta.Status != model.StatusError || d.Weather.Data != nil {
		t.Fatalf("got %s", d.Weather.Meta.Status)
	}
}

func TestUsageValidateAndResetPassed(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 11, 15, 12, 0, 0, 0, time.UTC)
	store, _ := cache.NewMemory("")
	cfg := config.Default()

	rep := UsageReport{Provider: "claude", Origin: "claude-statusline", ObservedAt: now.Add(-time.Hour),
		FiveHour: &UsageWindowInput{UsedPercent: ptr(30.0), ResetsAt: ptr(now.Add(-time.Minute).Unix())},
		Weekly:   &UsageWindowInput{UsedPercent: ptr(55.5), ResetsAt: ptr(now.Add(48 * time.Hour).Unix())}}
	u, err := rep.Validate(now)
	if err != nil {
		t.Fatal(err)
	}
	if ok, err := StoreUsage(ctx, store, u, now); !ok || err != nil {
		t.Fatal(ok, err)
	}
	d, _ := (&Builder{Store: store, Cfg: &cfg, Now: func() time.Time { return now }}).Build(ctx)
	c := d.Claude.Data
	if d.Claude.Meta.Status != model.StatusOK || c == nil {
		t.Fatalf("status %s", d.Claude.Meta.Status)
	}
	if !c.FiveHour.ResetPassed || c.FiveHour.RemainingPercent() != nil {
		t.Fatal("past reset: remaining must be unknown, not 70%")
	}
	if r := c.Weekly.RemainingPercent(); r == nil || *r != 44.5 {
		t.Fatalf("remaining %v", r)
	}

	// older snapshot must not replace newer one
	older := rep
	older.ObservedAt = now.Add(-2 * time.Hour)
	ou, _ := older.Validate(now)
	if ok, _ := StoreUsage(ctx, store, ou, now); ok {
		t.Fatal("older snapshot stored")
	}
}

func TestUsageValidateRejects(t *testing.T) {
	now := time.Now()
	bad := []UsageReport{
		{Provider: "gpt", ObservedAt: now, FiveHour: &UsageWindowInput{UsedPercent: ptr(1.0)}},
		{Provider: "claude", FiveHour: &UsageWindowInput{UsedPercent: ptr(1.0)}},
		{Provider: "claude", ObservedAt: now},
		{Provider: "claude", ObservedAt: now, FiveHour: &UsageWindowInput{UsedPercent: ptr(140.0)}},
		{Provider: "claude", ObservedAt: now.Add(time.Hour), FiveHour: &UsageWindowInput{UsedPercent: ptr(1.0)}},
	}
	for i, r := range bad {
		if _, err := r.Validate(now); err == nil {
			t.Errorf("case %d accepted", i)
		}
	}
}

func TestUsageStaleAfter(t *testing.T) {
	ctx := context.Background()
	now := time.Now()
	store, _ := cache.NewMemory("")
	cfg := config.Default()
	rep := UsageReport{Provider: "codex", ObservedAt: now.Add(-7 * time.Hour), Weekly: &UsageWindowInput{UsedPercent: ptr(10.0)}}
	u, _ := rep.Validate(now)
	_, _ = StoreUsage(ctx, store, u, now)
	d, _ := (&Builder{Store: store, Cfg: &cfg}).Build(ctx)
	if d.Codex.Meta.Status != model.StatusStale {
		t.Fatalf("got %s", d.Codex.Meta.Status)
	}
}
