// Package config loads YAML configuration with environment-variable
// overrides. Secrets (tokens, Redis URL) should come from the environment.
package config

import (
	"fmt"
	"os"
	"path"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

type Duration struct{ time.Duration }

func (d *Duration) UnmarshalYAML(n *yaml.Node) error {
	v, err := time.ParseDuration(n.Value)
	if err != nil {
		return fmt.Errorf("invalid duration %q: %w", n.Value, err)
	}
	d.Duration = v
	return nil
}

func (d Duration) MarshalYAML() (any, error) { return d.String(), nil }

type CollectorBase struct {
	Enabled    bool     `yaml:"enabled"`
	Interval   Duration `yaml:"interval"`
	Timeout    Duration `yaml:"timeout"`
	StaleAfter Duration `yaml:"stale_after"`
	Retries    int      `yaml:"retries"`
}

type WeatherConfig struct {
	CollectorBase `yaml:",inline"`
	URL           string  `yaml:"url"`
	Location      string  `yaml:"location"`
	Latitude      float64 `yaml:"latitude"`
	Longitude     float64 `yaml:"longitude"`
	Days          int     `yaml:"days"`
}

type GoldItemConfig struct {
	Code  string `yaml:"code"`
	Label string `yaml:"label"`
}

// GenericGoldConfig maps an arbitrary JSON API onto GoldItems.
// Paths are dot-separated (e.g. "data.items"); fields are relative to each item.
type GenericGoldConfig struct {
	ItemsPath  string  `yaml:"items_path"`
	CodeField  string  `yaml:"code_field"`
	BuyField   string  `yaml:"buy_field"`
	SellField  string  `yaml:"sell_field"`
	Multiplier float64 `yaml:"multiplier"` // e.g. 1000 if API returns thousand VND
	TimeField  string  `yaml:"time_field"` // optional top-level path
}

type GoldConfig struct {
	CollectorBase `yaml:",inline"`
	Provider      string            `yaml:"provider"` // "vangtoday" | "generic"
	URL           string            `yaml:"url"`
	Items         []GoldItemConfig  `yaml:"items"`
	Generic       GenericGoldConfig `yaml:"generic"`
}

type TrendsConfig struct {
	CollectorBase `yaml:",inline"`
	URL           string `yaml:"url"`
	Geo           string `yaml:"geo"`
	Limit         int    `yaml:"limit"`
}

type UsageConfig struct {
	// StaleAfter for pushed usage snapshots (agents only report while tools run).
	StaleAfter Duration `yaml:"stale_after"`
}

type PageConfig struct {
	Name    string   `yaml:"name"`
	Title   string   `yaml:"title"`
	Widgets []string `yaml:"widgets"`
}

type RenderConfig struct {
	Width        int      `yaml:"width"`
	Height       int      `yaml:"height"`
	GrayLevels   int      `yaml:"gray_levels"` // 16 for PW3
	Dither       bool     `yaml:"dither"`
	CacheTTL     Duration `yaml:"cache_ttl"`
	Timeout      Duration `yaml:"timeout"`
	ChromePath   string   `yaml:"chrome_path"`
	ChromeRemote string   `yaml:"chrome_remote"` // optional ws:// URL of a remote Chromium
}

type KindleConfig struct {
	// Meta refresh (seconds) on stock-browser pages; 0 disables.
	HTMLRefreshSeconds int `yaml:"html_refresh_seconds"`
}

type Config struct {
	Listen       string `yaml:"listen"`
	BaseURL      string `yaml:"base_url"`  // internal URL Chromium uses to reach this server
	BasePath     string `yaml:"base_path"` // URL prefix, empty or e.g. /elink-dashboard
	Timezone     string `yaml:"timezone"`
	Brand        string `yaml:"brand"`
	DeviceTokens []string
	IngestTokens []string
	AuthDisabled bool
	RedisURL     string `yaml:"redis_url"`
	CacheFile    string `yaml:"cache_file"`
	MockData     string `yaml:"mock_data"`
	LogLevel     string `yaml:"log_level"`

	Weather WeatherConfig `yaml:"weather"`
	Gold    GoldConfig    `yaml:"gold"`
	Trends  TrendsConfig  `yaml:"trends"`
	Usage   UsageConfig   `yaml:"usage"`
	Pages   []PageConfig  `yaml:"pages"`
	Render  RenderConfig  `yaml:"render"`
	Kindle  KindleConfig  `yaml:"kindle"`
}

func d(v time.Duration) Duration { return Duration{v} }

// Default returns a configuration that works out of the box.
func Default() Config {
	return Config{
		Listen:   ":8080",
		BaseURL:  "",
		Timezone: "Asia/Bangkok",
		Brand:    "ViaiZ",
		LogLevel: "info",
		Weather: WeatherConfig{
			CollectorBase: CollectorBase{Enabled: true, Interval: d(30 * time.Minute), Timeout: d(15 * time.Second), StaleAfter: d(2 * time.Hour), Retries: 3},
			URL:           "https://api.open-meteo.com/v1/forecast",
			Location:      "Hà Nội", Latitude: 21.0285, Longitude: 105.8542, Days: 4,
		},
		Gold: GoldConfig{
			CollectorBase: CollectorBase{Enabled: true, Interval: d(15 * time.Minute), Timeout: d(15 * time.Second), StaleAfter: d(6 * time.Hour), Retries: 3},
			Provider:      "vangtoday",
			URL:           "https://www.vang.today/api/prices",
			Items: []GoldItemConfig{
				{Code: "SJL1L10", Label: "SJC miếng 9999"},
				{Code: "SJ9999", Label: "SJC nhẫn 9999"},
				{Code: "BTSJC", Label: "Bảo Tín · SJC"},
				{Code: "BT9999NTT", Label: "Bảo Tín · nhẫn 9999"},
			},
			Generic: GenericGoldConfig{Multiplier: 1},
		},
		Trends: TrendsConfig{
			CollectorBase: CollectorBase{Enabled: true, Interval: d(30 * time.Minute), Timeout: d(15 * time.Second), StaleAfter: d(4 * time.Hour), Retries: 3},
			URL:           "https://trends.google.com/trending/rss",
			Geo:           "VN", Limit: 5,
		},
		Usage: UsageConfig{StaleAfter: d(6 * time.Hour)},
		Pages: []PageConfig{
			{Name: "main", Title: "Overview", Widgets: []string{"claude", "codex", "weather", "gold", "trends"}},
		},
		Render: RenderConfig{
			Width: 1072, Height: 1448, GrayLevels: 16, Dither: false,
			CacheTTL: d(60 * time.Second), Timeout: d(30 * time.Second),
		},
		Kindle: KindleConfig{HTMLRefreshSeconds: 900},
	}
}

// Load reads the YAML file (if path is non-empty and exists) and applies env overrides.
func Load(path string) (Config, error) {
	cfg := Default()
	if path != "" {
		b, err := os.ReadFile(path)
		switch {
		case err == nil:
			if err := yaml.Unmarshal(b, &cfg); err != nil {
				return cfg, fmt.Errorf("parse %s: %w", path, err)
			}
		case os.IsNotExist(err):
			// fall back to defaults
		default:
			return cfg, err
		}
	}
	applyEnv(&cfg)
	return cfg, cfg.Validate()
}

func applyEnv(c *Config) {
	str := func(key string, dst *string) {
		if v, ok := os.LookupEnv(key); ok && v != "" {
			*dst = v
		}
	}
	str("LISTEN", &c.Listen)
	if p := os.Getenv("PORT"); p != "" {
		c.Listen = ":" + p
	}
	str("BASE_URL", &c.BaseURL)
	str("BASE_PATH", &c.BasePath)
	str("DISPLAY_TZ", &c.Timezone)
	str("BRAND", &c.Brand)
	str("REDIS_URL", &c.RedisURL)
	str("CACHE_FILE", &c.CacheFile)
	str("MOCK_DATA", &c.MockData)
	str("LOG_LEVEL", &c.LogLevel)
	str("CHROME_PATH", &c.Render.ChromePath)
	str("CHROME_REMOTE", &c.Render.ChromeRemote)
	str("GOLD_API_URL", &c.Gold.URL)
	str("GOLD_PROVIDER", &c.Gold.Provider)
	c.DeviceTokens = splitList(os.Getenv("DEVICE_TOKENS"))
	c.IngestTokens = splitList(os.Getenv("INGEST_TOKENS"))
	if v, err := strconv.ParseBool(os.Getenv("AUTH_DISABLED")); err == nil {
		c.AuthDisabled = v
	}
	if v, err := strconv.Atoi(os.Getenv("KINDLE_HTML_REFRESH_SECONDS")); err == nil {
		c.Kindle.HTMLRefreshSeconds = v
	}
}

func splitList(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// Validate checks the configuration for obvious mistakes.
func (c *Config) Validate() error {
	if c.BasePath != "" && (c.BasePath == "/" || !strings.HasPrefix(c.BasePath, "/") || path.Clean(c.BasePath) != c.BasePath || strings.ContainsAny(c.BasePath, "?#\\%") || strings.Contains(c.BasePath, "//")) {
		return fmt.Errorf("base_path must be empty or a clean absolute path without a trailing slash")
	}
	if _, err := time.LoadLocation(c.Timezone); err != nil {
		return fmt.Errorf("timezone %q: %w", c.Timezone, err)
	}
	if !c.AuthDisabled && len(c.DeviceTokens) == 0 {
		return fmt.Errorf("DEVICE_TOKENS is empty: set at least one device token or AUTH_DISABLED=true (local dev only)")
	}
	for _, t := range append(append([]string{}, c.DeviceTokens...), c.IngestTokens...) {
		if len(t) < 16 {
			return fmt.Errorf("tokens must be at least 16 characters")
		}
	}
	if len(c.Pages) == 0 {
		return fmt.Errorf("at least one page must be configured")
	}
	known := map[string]bool{"claude": true, "codex": true, "weather": true, "gold": true, "trends": true}
	for _, p := range c.Pages {
		if p.Name == "" {
			return fmt.Errorf("page without name")
		}
		for _, w := range p.Widgets {
			if !known[w] {
				return fmt.Errorf("page %q: unknown widget %q", p.Name, w)
			}
		}
	}
	if c.Render.Width <= 0 || c.Render.Height <= 0 {
		return fmt.Errorf("render size must be positive")
	}
	if c.Render.GrayLevels < 2 || c.Render.GrayLevels > 256 {
		return fmt.Errorf("render.gray_levels must be 2..256")
	}
	return nil
}

// Page returns the named page or the first page.
func (c *Config) Page(name string) (PageConfig, bool) {
	if name == "" {
		return c.Pages[0], true
	}
	for _, p := range c.Pages {
		if p.Name == name {
			return p, true
		}
	}
	return PageConfig{}, false
}
