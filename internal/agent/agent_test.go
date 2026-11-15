package agent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestParseClaudeStatusline(t *testing.T) {
	now := time.Unix(1_794_700_000, 0)
	in := `{"model":{"display_name":"Opus"},"session_id":"x","rate_limits":{"five_hour":{"used_percentage":23.5,"resets_at":1794707200},"seven_day":{"used_percentage":41,"resets_at":1795000000}}}`
	s, model, err := ParseClaudeStatusline([]byte(in), now)
	if err != nil || s == nil {
		t.Fatal(err)
	}
	if model != "Opus" || *s.FiveHour.UsedPercent != 23.5 || *s.FiveHour.ResetsAt != 1794707200 || s.FiveHour.WindowMinutes != 300 {
		t.Fatalf("%+v", s.FiveHour)
	}
	if *s.Weekly.UsedPercent != 41 || s.Weekly.WindowMinutes != 10080 {
		t.Fatalf("%+v", s.Weekly)
	}
	if got := StatusLineText(model, s); got != "Opus · 5h 24% · 7d 41%" {
		t.Fatalf("statusline text %q", got)
	}
}

func TestParseClaudeStatuslineNoLimits(t *testing.T) {
	// API-key users / before first response: rate_limits absent → nothing stored.
	s, model, err := ParseClaudeStatusline([]byte(`{"model":{"display_name":"Sonnet"}}`), time.Now())
	if err != nil || s != nil || model != "Sonnet" {
		t.Fatal(s, err)
	}
	s, _, _ = ParseClaudeStatusline([]byte(`{"rate_limits":{"five_hour":{"used_percentage":150}}}`), time.Now())
	if s != nil {
		t.Fatal("out-of-range value accepted")
	}
}

func TestParseCodexRollout(t *testing.T) {
	lines := []string{
		`{"timestamp":"2026-11-15T10:00:00.000Z","type":"session_meta","payload":{}}`,
		`{"timestamp":"2026-11-15T10:01:00.000Z","type":"event_msg","payload":{"type":"token_count","rate_limits":{"primary":{"used_percent":5,"window_minutes":300,"resets_in_seconds":100}}}}`,
		`{"timestamp":"2026-11-15T10:05:00.000Z","type":"event_msg","payload":{"type":"token_count","info":{},"rate_limits":{"primary":{"used_percent":12.5,"window_minutes":300,"resets_in_seconds":3600},"secondary":{"used_percent":66,"window_minutes":10080,"resets_at":1795000000}}}}`,
		`{"timestamp":"2026-11-15T10:06:00.000Z","type":"response_item","payload":{}}`,
	}
	rep, err := ParseCodexRollout(strings.NewReader(strings.Join(lines, "\n")), "h")
	if err != nil || rep == nil {
		t.Fatal(err)
	}
	obs := time.Date(2026, 11, 15, 10, 5, 0, 0, time.UTC)
	if !rep.ObservedAt.Equal(obs) {
		t.Fatalf("observed_at must be the event timestamp, got %v", rep.ObservedAt)
	}
	if *rep.FiveHour.UsedPercent != 12.5 || *rep.FiveHour.ResetsAt != obs.Add(time.Hour).Unix() {
		t.Fatalf("five hour %+v", rep.FiveHour)
	}
	if *rep.Weekly.UsedPercent != 66 || *rep.Weekly.ResetsAt != 1795000000 {
		t.Fatalf("weekly %+v", rep.Weekly)
	}
}

func TestParseCodexRolloutNone(t *testing.T) {
	rep, err := ParseCodexRollout(strings.NewReader(`{"timestamp":"2026-11-15T10:00:00Z","type":"x"}`), "h")
	if err != nil || rep != nil {
		t.Fatal("expected no report")
	}
}

func TestLatestRollout(t *testing.T) {
	dir := t.TempDir()
	a := filepath.Join(dir, "2026", "11", "14", "rollout-a.jsonl")
	b := filepath.Join(dir, "2026", "11", "15", "rollout-b.jsonl")
	for _, p := range []string{a, b} {
		_ = os.MkdirAll(filepath.Dir(p), 0o755)
		_ = os.WriteFile(p, []byte("{}"), 0o644)
	}
	_ = os.Chtimes(a, time.Now().Add(-time.Hour), time.Now().Add(-time.Hour))
	got, err := LatestRollout(dir)
	if err != nil || got != b {
		t.Fatalf("got %s %v", got, err)
	}
	if _, err := LatestRollout(filepath.Join(dir, "missing")); err == nil {
		t.Fatal("expected error for missing dir")
	}
}
