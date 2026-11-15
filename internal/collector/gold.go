package collector

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/viaiz/eink-dashboard/internal/config"
	"github.com/viaiz/eink-dashboard/internal/model"
)

// Gold collects Vietnamese gold prices from a configurable API.
//
// Providers:
//   - "vangtoday": https://www.vang.today/api/prices (verified format:
//     {"timestamp":…, "prices":{"CODE":{"name","buy","sell","change_buy","change_sell"}}},
//     prices in VND per lượng).
//   - "generic": any JSON API, mapped with dot-paths in config.
type Gold struct {
	Cfg config.GoldConfig

	mu   sync.Mutex
	prev map[string][2]int64 // last seen buy/sell, used when API gives no change
}

func (g *Gold) Key() string { return "gold" }
func (g *Gold) Source() string {
	if g.Cfg.Provider == "vangtoday" {
		return "vang.today"
	}
	return hostOf(g.Cfg.URL)
}

func (g *Gold) Fetch(ctx context.Context) (Result, error) {
	body, err := getBody(ctx, g.Cfg.URL, "application/json")
	if err != nil {
		return Result{}, err
	}
	var data *model.GoldData
	var ts *time.Time
	switch g.Cfg.Provider {
	case "vangtoday", "":
		data, ts, err = ParseVangToday(body, g.Cfg.Items)
	case "generic":
		data, ts, err = ParseGenericGold(body, g.Cfg.Generic, g.Cfg.Items)
	default:
		err = fmt.Errorf("unknown gold provider %q", g.Cfg.Provider)
	}
	if err != nil {
		return Result{}, &PermanentError{err}
	}
	g.fillChanges(data)
	return Result{Data: data, SourceTime: ts}, nil
}

// fillChanges computes change vs the previous fetch for items where the
// API did not provide a change value.
func (g *Gold) fillChanges(d *model.GoldData) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.prev == nil {
		g.prev = map[string][2]int64{}
	}
	for i := range d.Items {
		it := &d.Items[i]
		if it.Buy == nil || it.Sell == nil {
			continue
		}
		p, had := g.prev[it.Code]
		if it.ChangeBuy == nil && it.ChangeSell == nil && had {
			it.ChangeBuy = model.Ptr(*it.Buy - p[0])
			it.ChangeSell = model.Ptr(*it.Sell - p[1])
			it.ChangeBasis = "vs previous fetch"
		}
		g.prev[it.Code] = [2]int64{*it.Buy, *it.Sell}
	}
}

type vangTodayResp struct {
	Success   bool  `json:"success"`
	Timestamp int64 `json:"timestamp"`
	Prices    map[string]struct {
		Name       string   `json:"name"`
		Buy        *float64 `json:"buy"`
		Sell       *float64 `json:"sell"`
		ChangeBuy  *float64 `json:"change_buy"`
		ChangeSell *float64 `json:"change_sell"`
	} `json:"prices"`
}

// ParseVangToday parses the vang.today response, keeping configured items in order.
func ParseVangToday(body []byte, items []config.GoldItemConfig) (*model.GoldData, *time.Time, error) {
	var r vangTodayResp
	if err := json.Unmarshal(body, &r); err != nil {
		return nil, nil, fmt.Errorf("vang.today: %w", err)
	}
	if !r.Success || len(r.Prices) == 0 {
		return nil, nil, fmt.Errorf("vang.today: unsuccessful or empty response")
	}
	out := &model.GoldData{Unit: "VND/lượng"}
	for _, ic := range items {
		p, ok := r.Prices[ic.Code]
		if !ok {
			continue // not offered by the source right now; do not invent
		}
		label := ic.Label
		if label == "" {
			label = p.Name
		}
		it := model.GoldItem{Code: ic.Code, Label: label, Buy: posInt(p.Buy), Sell: posInt(p.Sell)}
		if p.ChangeBuy != nil || p.ChangeSell != nil {
			it.ChangeBuy, it.ChangeSell = intPtr(p.ChangeBuy), intPtr(p.ChangeSell)
			it.ChangeBasis = "reported by source"
		}
		out.Items = append(out.Items, it)
	}
	if len(out.Items) == 0 {
		return nil, nil, fmt.Errorf("vang.today: none of the configured item codes present")
	}
	var ts *time.Time
	if r.Timestamp > 0 {
		ts = model.Ptr(time.Unix(r.Timestamp, 0))
	}
	return out, ts, nil
}

// ParseGenericGold maps an arbitrary JSON document using dot-paths.
func ParseGenericGold(body []byte, gc config.GenericGoldConfig, items []config.GoldItemConfig) (*model.GoldData, *time.Time, error) {
	var root any
	if err := json.Unmarshal(body, &root); err != nil {
		return nil, nil, fmt.Errorf("gold generic: %w", err)
	}
	list, ok := lookup(root, gc.ItemsPath).([]any)
	if !ok {
		return nil, nil, fmt.Errorf("gold generic: items_path %q is not an array", gc.ItemsPath)
	}
	mult := gc.Multiplier
	if mult == 0 {
		mult = 1
	}
	byCode := map[string]map[string]any{}
	for _, el := range list {
		m, ok := el.(map[string]any)
		if !ok {
			continue
		}
		if code := fmt.Sprint(lookup(m, gc.CodeField)); code != "" {
			byCode[code] = m
		}
	}
	out := &model.GoldData{Unit: "VND/lượng"}
	for _, ic := range items {
		m, ok := byCode[ic.Code]
		if !ok {
			continue
		}
		label := ic.Label
		if label == "" {
			label = ic.Code
		}
		out.Items = append(out.Items, model.GoldItem{
			Code: ic.Code, Label: label,
			Buy:  scaled(lookup(m, gc.BuyField), mult),
			Sell: scaled(lookup(m, gc.SellField), mult),
		})
	}
	if len(out.Items) == 0 {
		return nil, nil, fmt.Errorf("gold generic: no configured items found")
	}
	var ts *time.Time
	if gc.TimeField != "" {
		ts = parseAnyTime(lookup(root, gc.TimeField))
	}
	return out, ts, nil
}

func lookup(v any, path string) any {
	if path == "" {
		return v
	}
	for _, part := range strings.Split(path, ".") {
		m, ok := v.(map[string]any)
		if !ok {
			return nil
		}
		v = m[part]
	}
	return v
}

func scaled(v any, mult float64) *int64 {
	var f float64
	switch x := v.(type) {
	case float64:
		f = x
	case string:
		s := strings.NewReplacer(",", "", ".", "", " ", "").Replace(x)
		p, err := strconv.ParseFloat(s, 64)
		if err != nil {
			return nil
		}
		f = p
	default:
		return nil
	}
	if f <= 0 {
		return nil
	}
	return model.Ptr(int64(math.Round(f * mult)))
}

func parseAnyTime(v any) *time.Time {
	switch x := v.(type) {
	case float64:
		if x > 1e12 {
			x /= 1000
		}
		return model.Ptr(time.Unix(int64(x), 0))
	case string:
		for _, layout := range []string{time.RFC3339, "2006-01-02 15:04:05", "02/01/2006 15:04"} {
			if t, err := time.Parse(layout, x); err == nil {
				return &t
			}
		}
	}
	return nil
}

func posInt(f *float64) *int64 {
	if f == nil || *f <= 0 {
		return nil // 0 means "not quoted" in vang.today (e.g. XAUUSD sell)
	}
	return model.Ptr(int64(math.Round(*f)))
}

func intPtr(f *float64) *int64 {
	if f == nil {
		return nil
	}
	return model.Ptr(int64(math.Round(*f)))
}

func hostOf(u string) string {
	s := strings.TrimPrefix(strings.TrimPrefix(u, "https://"), "http://")
	if i := strings.IndexAny(s, "/?"); i >= 0 {
		s = s[:i]
	}
	return s
}
