package collector

import (
	"context"
	"encoding/xml"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/viaiz/eink-dashboard/internal/model"
)

// Trends reads the public Google Trends "Trending now" RSS feed.
// Traffic numbers are Google's own approximate buckets ("2000+"), shown
// verbatim; no search volumes are derived or estimated.
type Trends struct {
	BaseURL string
	Geo     string
	Limit   int
}

func (t *Trends) Key() string    { return "trends" }
func (t *Trends) Source() string { return "Google Trends RSS" }

func (t *Trends) URL() string {
	return t.BaseURL + "?geo=" + url.QueryEscape(t.Geo)
}

func (t *Trends) Fetch(ctx context.Context) (Result, error) {
	body, err := getBody(ctx, t.URL(), "application/rss+xml, application/xml")
	if err != nil {
		return Result{}, err
	}
	data, err := ParseTrendsRSS(body, t.Geo, t.Limit)
	if err != nil {
		return Result{}, &PermanentError{err}
	}
	return Result{Data: data}, nil
}

type rss struct {
	Channel struct {
		Items []struct {
			Title   string `xml:"title"`
			Traffic string `xml:"https://trends.google.com/trending/rss approx_traffic"`
			PubDate string `xml:"pubDate"`
			News    []struct {
				Title  string `xml:"https://trends.google.com/trending/rss news_item_title"`
				Source string `xml:"https://trends.google.com/trending/rss news_item_source"`
			} `xml:"https://trends.google.com/trending/rss news_item"`
		} `xml:"item"`
	} `xml:"channel"`
}

// ParseTrendsRSS parses the feed and returns the top `limit` items in feed order.
func ParseTrendsRSS(body []byte, geo string, limit int) (*model.TrendsData, error) {
	var r rss
	if err := xml.Unmarshal(body, &r); err != nil {
		return nil, fmt.Errorf("trends rss: %w", err)
	}
	if len(r.Channel.Items) == 0 {
		return nil, fmt.Errorf("trends rss: feed has no items")
	}
	if limit <= 0 {
		limit = 5
	}
	out := &model.TrendsData{Geo: geo}
	for i, it := range r.Channel.Items {
		if i >= limit {
			break
		}
		ti := model.TrendItem{Rank: i + 1, Title: strings.TrimSpace(it.Title), ApproxTraffic: strings.TrimSpace(it.Traffic)}
		if p, err := time.Parse(time.RFC1123Z, strings.TrimSpace(it.PubDate)); err == nil {
			ti.PubDate = &p
		}
		if len(it.News) > 0 {
			ti.NewsTitle = strings.TrimSpace(it.News[0].Title)
			ti.NewsSource = strings.TrimSpace(it.News[0].Source)
		}
		out.Items = append(out.Items, ti)
	}
	return out, nil
}
