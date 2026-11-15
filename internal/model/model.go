// Package model defines the normalized data model shared by collectors,
// the cache, the HTML dashboard and the PNG renderer.
//
// Rule: every value is either observed from a source or nil. Nothing is
// estimated or filled in; widgets render "—" / "unavailable" for nil.
package model

import "time"

// Status of a data section.
type Status string

const (
	StatusOK          Status = "ok"          // fresh data
	StatusStale       Status = "stale"       // data older than its stale threshold
	StatusError       Status = "error"       // last fetch failed and no cached data exists
	StatusUnavailable Status = "unavailable" // never received (e.g. usage agent not running)
	StatusDisabled    Status = "disabled"    // collector disabled in config
)

// Meta describes provenance and freshness of a section.
type Meta struct {
	Source      string     `json:"source"`
	Status      Status     `json:"status"`
	UpdatedAt   *time.Time `json:"updated_at,omitempty"`   // when data was last successfully obtained
	SourceTime  *time.Time `json:"source_time,omitempty"`  // timestamp reported by the source itself, if any
	LastAttempt *time.Time `json:"last_attempt,omitempty"` // last fetch attempt
	LastError   string     `json:"last_error,omitempty"`
	StaleAfter  string     `json:"stale_after,omitempty"`
}

// Section wraps data with metadata. Data is nil when unavailable.
type Section[T any] struct {
	Meta Meta `json:"meta"`
	Data *T   `json:"data,omitempty"`
}

// ---------- AI usage (Claude Code / Codex) ----------

// QuotaWindow is a single rate-limit window as reported by the tool.
type QuotaWindow struct {
	WindowMinutes int        `json:"window_minutes,omitempty"`
	UsedPercent   *float64   `json:"used_percent,omitempty"`
	ResetsAt      *time.Time `json:"resets_at,omitempty"`
	// ResetPassed is set at read time when ResetsAt is in the past: the
	// reported percentage no longer applies and current usage is unknown.
	ResetPassed bool `json:"reset_passed,omitempty"`
}

// RemainingPercent derives remaining = 100 - used (only when used is known).
func (q *QuotaWindow) RemainingPercent() *float64 {
	if q == nil || q.UsedPercent == nil || q.ResetPassed {
		return nil
	}
	r := 100 - *q.UsedPercent
	if r < 0 {
		r = 0
	}
	return &r
}

// UsageData is the normalized plan-usage snapshot of one tool.
// It describes subscription rate-limit windows, NOT API billing.
type UsageData struct {
	Provider   string       `json:"provider"` // "claude" | "codex"
	FiveHour   *QuotaWindow `json:"five_hour,omitempty"`
	Weekly     *QuotaWindow `json:"weekly,omitempty"`
	ObservedAt time.Time    `json:"observed_at"` // when the agent observed the values
	Origin     string       `json:"origin"`      // e.g. "claude-statusline", "codex-rollout"
	Host       string       `json:"host,omitempty"`
}

// ---------- Weather ----------

type WeatherCurrent struct {
	Time        time.Time `json:"time"`
	Temperature float64   `json:"temperature_c"`
	FeelsLike   *float64  `json:"feels_like_c,omitempty"`
	Humidity    float64   `json:"humidity_pct"`
	WindKmh     *float64  `json:"wind_kmh,omitempty"`
	Code        int       `json:"weather_code"`
	Condition   string    `json:"condition"`
	Icon        string    `json:"icon"`
}

type WeatherDay struct {
	Date       string   `json:"date"` // YYYY-MM-DD in location timezone
	Code       int      `json:"weather_code"`
	Condition  string   `json:"condition"`
	Icon       string   `json:"icon"`
	MaxC       float64  `json:"max_c"`
	MinC       float64  `json:"min_c"`
	PrecipProb *float64 `json:"precip_prob_pct,omitempty"`
}

type WeatherData struct {
	Location string         `json:"location"`
	Current  WeatherCurrent `json:"current"`
	Daily    []WeatherDay   `json:"daily"`
}

// ---------- Gold ----------

type GoldItem struct {
	Code       string `json:"code"`
	Label      string `json:"label"`
	Buy        *int64 `json:"buy,omitempty"`  // VND per lượng (tael)
	Sell       *int64 `json:"sell,omitempty"` // VND per lượng (tael)
	ChangeBuy  *int64 `json:"change_buy,omitempty"`
	ChangeSell *int64 `json:"change_sell,omitempty"`
	// ChangeBasis explains what the change is relative to.
	ChangeBasis string `json:"change_basis,omitempty"`
}

type GoldData struct {
	Unit  string     `json:"unit"`
	Items []GoldItem `json:"items"`
}

// ---------- Google Trends ----------

type TrendItem struct {
	Rank          int        `json:"rank"`
	Title         string     `json:"title"`
	ApproxTraffic string     `json:"approx_traffic,omitempty"` // verbatim from Google, e.g. "2000+"
	PubDate       *time.Time `json:"pub_date,omitempty"`
	NewsTitle     string     `json:"news_title,omitempty"`
	NewsSource    string     `json:"news_source,omitempty"`
}

type TrendsData struct {
	Geo   string      `json:"geo"`
	Items []TrendItem `json:"items"`
}

// ---------- Dashboard ----------

type Dashboard struct {
	GeneratedAt time.Time              `json:"generated_at"`
	Timezone    string                 `json:"timezone"`
	Claude      Section[UsageData]     `json:"claude"`
	Codex       Section[UsageData]     `json:"codex"`
	Weather     Section[WeatherData]   `json:"weather"`
	Gold        Section[GoldData]      `json:"gold"`
	Trends      Section[TrendsData]    `json:"trends"`
	LastSync    *time.Time             `json:"last_sync,omitempty"`
	Health      DashboardHealthSummary `json:"health"`
}

type DashboardHealthSummary struct {
	SourcesOK    int      `json:"sources_ok"`
	SourcesTotal int      `json:"sources_total"`
	Stale        []string `json:"stale,omitempty"`
	Failing      []string `json:"failing,omitempty"`
}

// Ptr is a small helper for optional values.
func Ptr[T any](v T) *T { return &v }
