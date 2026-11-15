// Package agent implements the local usage collectors that run on the
// developer workstation (where Claude Code / Codex are used) and push
// normalized usage snapshots to the dashboard server.
//
// Data sources (no credentials are ever read or transmitted):
//
//   - Claude Code: the documented statusLine JSON on stdin contains
//     rate_limits.five_hour / rate_limits.seven_day with used_percentage
//     (0–100) and resets_at (Unix seconds). Present only for claude.ai
//     Pro/Max subscribers and only after the first API response of a session.
//     See https://code.claude.com/docs/en/statusline
//   - Codex CLI: session rollout files (~/.codex/sessions/**/rollout-*.jsonl)
//     include token_count events carrying rate_limits.primary/secondary with
//     used_percent, window_minutes and resets_at / resets_in_seconds. This
//     format is not a documented API; the parser is tolerant and reports
//     nothing rather than guessing when fields are missing.
package agent

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/viaiz/eink-dashboard/internal/dashboard"
)

// ---------- Claude Code statusLine ----------

type claudeWindow struct {
	UsedPercentage *float64 `json:"used_percentage"`
	ResetsAt       *float64 `json:"resets_at"`
}

type claudeStatusInput struct {
	Model *struct {
		DisplayName string `json:"display_name"`
	} `json:"model"`
	RateLimits *struct {
		FiveHour *claudeWindow `json:"five_hour"`
		SevenDay *claudeWindow `json:"seven_day"`
	} `json:"rate_limits"`
}

// ClaudeSnapshot is what the statusline command persists locally.
type ClaudeSnapshot struct {
	ObservedAt time.Time                   `json:"observed_at"`
	FiveHour   *dashboard.UsageWindowInput `json:"five_hour,omitempty"`
	Weekly     *dashboard.UsageWindowInput `json:"weekly,omitempty"`
}

// ParseClaudeStatusline extracts rate limits from statusLine stdin JSON.
// Returns (nil, model, nil) when the payload carries no rate_limits.
func ParseClaudeStatusline(b []byte, now time.Time) (*ClaudeSnapshot, string, error) {
	var in claudeStatusInput
	if err := json.Unmarshal(b, &in); err != nil {
		return nil, "", err
	}
	model := ""
	if in.Model != nil {
		model = in.Model.DisplayName
	}
	if in.RateLimits == nil {
		return nil, model, nil
	}
	conv := func(w *claudeWindow, minutes int) *dashboard.UsageWindowInput {
		if w == nil || w.UsedPercentage == nil {
			return nil
		}
		p := *w.UsedPercentage
		if p < 0 || p > 100 || math.IsNaN(p) {
			return nil
		}
		out := &dashboard.UsageWindowInput{UsedPercent: &p, WindowMinutes: minutes}
		if w.ResetsAt != nil && *w.ResetsAt > 0 {
			r := int64(*w.ResetsAt)
			if r > 1e12 { // tolerate milliseconds
				r /= 1000
			}
			out.ResetsAt = &r
		}
		return out
	}
	s := &ClaudeSnapshot{ObservedAt: now.UTC(), FiveHour: conv(in.RateLimits.FiveHour, 300), Weekly: conv(in.RateLimits.SevenDay, 10080)}
	if s.FiveHour == nil && s.Weekly == nil {
		return nil, model, nil
	}
	return s, model, nil
}

// StatusLineText renders the one-line status shown inside Claude Code.
func StatusLineText(model string, s *ClaudeSnapshot) string {
	var parts []string
	if model != "" {
		parts = append(parts, model)
	}
	if s != nil {
		if s.FiveHour != nil && s.FiveHour.UsedPercent != nil {
			parts = append(parts, fmt.Sprintf("5h %.0f%%", *s.FiveHour.UsedPercent))
		}
		if s.Weekly != nil && s.Weekly.UsedPercent != nil {
			parts = append(parts, fmt.Sprintf("7d %.0f%%", *s.Weekly.UsedPercent))
		}
	}
	return strings.Join(parts, " · ")
}

// WriteJSONAtomic writes v to path via a temp file + rename.
func WriteJSONAtomic(path string, v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// ReadClaudeSnapshot loads the snapshot written by the statusline command.
func ReadClaudeSnapshot(path string) (*ClaudeSnapshot, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var s ClaudeSnapshot
	if err := json.Unmarshal(b, &s); err != nil {
		return nil, err
	}
	return &s, nil
}

// ClaudeReport converts a snapshot into an ingest report.
func ClaudeReport(s *ClaudeSnapshot, host string) dashboard.UsageReport {
	return dashboard.UsageReport{Provider: "claude", Origin: "claude-statusline", Host: host, ObservedAt: s.ObservedAt, FiveHour: s.FiveHour, Weekly: s.Weekly}
}

// ---------- Codex rollout ----------

// LatestRollout returns the most recently modified rollout-*.jsonl under dir.
func LatestRollout(dir string) (string, error) {
	var best string
	var bestT time.Time
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				return fs.SkipAll
			}
			return nil
		}
		if d.IsDir() || !strings.HasSuffix(d.Name(), ".jsonl") || !strings.HasPrefix(d.Name(), "rollout-") {
			return nil
		}
		info, err := d.Info()
		if err == nil && info.ModTime().After(bestT) {
			best, bestT = p, info.ModTime()
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	if best == "" {
		return "", fmt.Errorf("no rollout-*.jsonl files under %s", dir)
	}
	return best, nil
}

type codexWindow struct {
	UsedPercent     *float64 `json:"used_percent"`
	WindowMinutes   *float64 `json:"window_minutes"`
	ResetsAt        any      `json:"resets_at"`
	ResetsInSeconds *float64 `json:"resets_in_seconds"`
}

type codexLimits struct {
	Primary   *codexWindow `json:"primary"`
	Secondary *codexWindow `json:"secondary"`
}

// ParseCodexRollout scans a rollout JSONL stream and returns a report built
// from the LAST event that contains rate_limits. Returns (nil, nil) if none.
func ParseCodexRollout(r io.Reader, host string) (*dashboard.UsageReport, error) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 1<<20), 16<<20)
	var lastLine []byte
	for sc.Scan() {
		if strings.Contains(sc.Text(), `"rate_limits"`) {
			lastLine = append(lastLine[:0], sc.Bytes()...)
		}
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	if lastLine == nil {
		return nil, nil
	}
	var raw map[string]any
	if err := json.Unmarshal(lastLine, &raw); err != nil {
		return nil, err
	}
	rl := findKey(raw, "rate_limits", 6)
	if rl == nil {
		return nil, nil
	}
	b, _ := json.Marshal(rl)
	var lim codexLimits
	if err := json.Unmarshal(b, &lim); err != nil {
		return nil, nil
	}
	ts := time.Time{}
	if s, ok := raw["timestamp"].(string); ok {
		ts, _ = time.Parse(time.RFC3339Nano, s)
	}
	if ts.IsZero() {
		return nil, fmt.Errorf("codex rollout: rate_limits event without timestamp")
	}
	rep := &dashboard.UsageReport{Provider: "codex", Origin: "codex-rollout", Host: host, ObservedAt: ts.UTC()}
	for _, w := range []*codexWindow{lim.Primary, lim.Secondary} {
		in := convCodex(w, ts)
		if in == nil {
			continue
		}
		switch {
		case in.WindowMinutes == 300:
			rep.FiveHour = in
		case in.WindowMinutes == 10080:
			rep.Weekly = in
		case w == lim.Primary && rep.FiveHour == nil:
			rep.FiveHour = in
		case w == lim.Secondary && rep.Weekly == nil:
			rep.Weekly = in
		}
	}
	if rep.FiveHour == nil && rep.Weekly == nil {
		return nil, nil
	}
	return rep, nil
}

func convCodex(w *codexWindow, observed time.Time) *dashboard.UsageWindowInput {
	if w == nil || w.UsedPercent == nil || *w.UsedPercent < 0 || *w.UsedPercent > 100 {
		return nil
	}
	p := *w.UsedPercent
	out := &dashboard.UsageWindowInput{UsedPercent: &p}
	if w.WindowMinutes != nil {
		out.WindowMinutes = int(*w.WindowMinutes)
	}
	switch v := w.ResetsAt.(type) {
	case float64:
		r := int64(v)
		if r > 1e12 {
			r /= 1000
		}
		out.ResetsAt = &r
	case string:
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			r := t.Unix()
			out.ResetsAt = &r
		}
	}
	if out.ResetsAt == nil && w.ResetsInSeconds != nil {
		r := observed.Add(time.Duration(*w.ResetsInSeconds * float64(time.Second))).Unix()
		out.ResetsAt = &r
	}
	return out
}

func findKey(v any, key string, depth int) any {
	if depth < 0 {
		return nil
	}
	m, ok := v.(map[string]any)
	if !ok {
		return nil
	}
	if x, ok := m[key]; ok && x != nil {
		return x
	}
	for _, c := range m {
		if r := findKey(c, key, depth-1); r != nil {
			return r
		}
	}
	return nil
}
