package web

import (
	"fmt"
	"html/template"
	"math"
	"strings"
	"time"

	"github.com/viaiz/eink-dashboard/internal/config"
	"github.com/viaiz/eink-dashboard/internal/model"
)

// View is the data passed to the dashboard template.
type View struct {
	D        *model.Dashboard
	Page     config.PageConfig
	Pages    []config.PageConfig
	Brand    string
	BasePath string
	TZLabel  string
	Fluid    bool   // responsive preview vs fixed 1072×1448 e-ink layout
	Render   bool   // being rendered by headless Chromium for PNG
	Token    string // device token propagated on links (stock Kindle cannot send headers)
	Refresh  int    // meta refresh seconds, 0 = none
	Now      time.Time
}

// Has reports whether a configured widget should be displayed. Usage cards
// appear once a report exists; stale reports stay visible with their warning.
func (v View) Has(w string) bool {
	for _, x := range v.Page.Widgets {
		if x == w {
			switch w {
			case "claude":
				return v.D.Claude.Data != nil
			case "codex":
				return v.D.Codex.Data != nil
			}
			return true
		}
	}
	return false
}

// VisibleHealth summarizes the displayed widgets without changing API health.
func (v View) VisibleHealth() model.DashboardHealthSummary {
	var health model.DashboardHealthSummary
	for _, s := range []struct {
		widget string
		name   string
		meta   model.Meta
	}{
		{"claude", "Claude", v.D.Claude.Meta},
		{"codex", "Codex", v.D.Codex.Meta},
		{"weather", "Weather", v.D.Weather.Meta},
		{"gold", "Gold", v.D.Gold.Meta},
		{"trends", "Trends", v.D.Trends.Meta},
	} {
		if !v.Has(s.widget) || s.meta.Status == model.StatusDisabled {
			continue
		}
		health.SourcesTotal++
		switch s.meta.Status {
		case model.StatusOK:
			health.SourcesOK++
		case model.StatusStale:
			health.Stale = append(health.Stale, s.name)
		default:
			health.Failing = append(health.Failing, s.name)
		}
	}
	return health
}

func funcMap(loc *time.Location, now func() time.Time) template.FuncMap {
	asTime := func(v any) (time.Time, bool) {
		switch t := v.(type) {
		case time.Time:
			return t, !t.IsZero()
		case *time.Time:
			if t == nil {
				return time.Time{}, false
			}
			return *t, true
		}
		return time.Time{}, false
	}
	return template.FuncMap{
		"icon": Icon,
		"tf": func(layout string, v any) string {
			t, ok := asTime(v)
			if !ok {
				return "—"
			}
			return t.In(loc).Format(layout)
		},
		"ago": func(v any) string {
			t, ok := asTime(v)
			if !ok {
				return "never"
			}
			return Ago(now().Sub(t))
		},
		"pct": func(p *float64) string {
			if p == nil {
				return "—"
			}
			return fmt.Sprintf("%.0f%%", math.Round(*p))
		},
		"pctnum": func(p *float64) string {
			if p == nil {
				return "—"
			}
			return fmt.Sprintf("%.0f", math.Round(*p))
		},
		"barw": func(p *float64) int { // used-fill width in percent, clamped
			if p == nil {
				return 0
			}
			return int(math.Max(0, math.Min(100, math.Round(*p))))
		},
		"remaining": func(q *model.QuotaWindow) *float64 { return q.RemainingPercent() },
		"resetText": func(q *model.QuotaWindow) string { return ResetText(q, now(), loc) },
		"kvnd":      KVND,
		"chg":       Change,
		"chgdir": func(v *int64) string {
			switch {
			case v == nil:
				return ""
			case *v > 0:
				return "up"
			case *v < 0:
				return "down"
			}
			return "flat"
		},
		"wday": func(date string) string {
			t, err := time.ParseInLocation("2006-01-02", date, loc)
			if err != nil {
				return date
			}
			today := now().In(loc).Format("2006-01-02")
			if date == today {
				return "Today"
			}
			return t.Format("Mon")
		},
		"temp": func(f float64) string { return fmt.Sprintf("%.0f°", math.Round(f)) },
		"num0": func(f *float64) string {
			if f == nil {
				return "—"
			}
			return fmt.Sprintf("%.0f", math.Round(*f))
		},
		"status": StatusLabel,
		"dict": func(kv ...any) (map[string]any, error) {
			if len(kv)%2 != 0 {
				return nil, fmt.Errorf("dict: odd number of args")
			}
			m := make(map[string]any, len(kv)/2)
			for i := 0; i < len(kv); i += 2 {
				k, ok := kv[i].(string)
				if !ok {
					return nil, fmt.Errorf("dict: key must be string")
				}
				m[k] = kv[i+1]
			}
			return m, nil
		},
		"join": strings.Join,
		"windowLabel": func(q *model.QuotaWindow, def string) string {
			if q == nil || q.WindowMinutes == 0 {
				return def
			}
			switch q.WindowMinutes {
			case 300:
				return "5-hour"
			case 10080:
				return "Weekly"
			}
			if q.WindowMinutes%1440 == 0 {
				return fmt.Sprintf("%d-day", q.WindowMinutes/1440)
			}
			return fmt.Sprintf("%dh", q.WindowMinutes/60)
		},
	}
}

// Ago formats a duration as a short relative age.
func Ago(d time.Duration) string {
	switch {
	case d < 0:
		return "just now"
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%d min ago", int(d.Minutes()))
	case d < 48*time.Hour:
		h := int(d.Hours())
		m := int(d.Minutes()) % 60
		if m == 0 || h >= 10 {
			return fmt.Sprintf("%dh ago", h)
		}
		return fmt.Sprintf("%dh %02dm ago", h, m)
	}
	return fmt.Sprintf("%dd ago", int(d.Hours()/24))
}

// ResetText describes when a window resets, in the display timezone.
func ResetText(q *model.QuotaWindow, now time.Time, loc *time.Location) string {
	if q == nil {
		return "not reported"
	}
	if q.ResetsAt == nil {
		return "reset time not reported"
	}
	t := q.ResetsAt.In(loc)
	if q.ResetPassed {
		return "reset at " + t.Format("15:04 Mon 02 Jan") + " · awaiting new report"
	}
	n := now.In(loc)
	day := t.Format("Mon 02 Jan")
	switch {
	case sameDay(t, n):
		day = "today"
	case sameDay(t, n.AddDate(0, 0, 1)):
		day = "tomorrow"
	}
	left := t.Sub(now)
	in := ""
	if left < 24*time.Hour {
		in = fmt.Sprintf(" (in %dh %02dm)", int(left.Hours()), int(left.Minutes())%60)
	}
	return "resets " + t.Format("15:04") + " " + day + in
}

func sameDay(a, b time.Time) bool {
	return a.Year() == b.Year() && a.YearDay() == b.YearDay()
}

// KVND formats VND as thousands with Vietnamese dot grouping: 137500000 → "137.500".
func KVND(v *int64) string {
	if v == nil {
		return "—"
	}
	return group(int64(math.Round(float64(*v) / 1000)))
}

// Change formats a price change in thousand VND with a sign.
func Change(v *int64) string {
	if v == nil {
		return ""
	}
	k := int64(math.Round(float64(*v) / 1000))
	switch {
	case k > 0:
		return "+" + group(k)
	case k < 0:
		return "−" + group(-k)
	}
	return "±0"
}

func group(n int64) string {
	neg := n < 0
	if neg {
		n = -n
	}
	s := fmt.Sprintf("%d", n)
	var b strings.Builder
	for i, c := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			b.WriteByte('.')
		}
		b.WriteRune(c)
	}
	if neg {
		return "-" + b.String()
	}
	return b.String()
}

// StatusLabel returns a short badge for non-OK sections ("" when OK).
func StatusLabel(m model.Meta) string {
	switch m.Status {
	case model.StatusStale:
		return "STALE"
	case model.StatusError:
		return "ERROR"
	case model.StatusUnavailable:
		return "NO DATA"
	case model.StatusDisabled:
		return "OFF"
	}
	if m.LastError != "" {
		return "RETRYING"
	}
	return ""
}
