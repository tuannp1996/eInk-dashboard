package web

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/viaiz/eink-dashboard/internal/cache"
	"github.com/viaiz/eink-dashboard/internal/config"
	"github.com/viaiz/eink-dashboard/internal/dashboard"
	"github.com/viaiz/eink-dashboard/internal/model"
	"github.com/viaiz/eink-dashboard/internal/render"
)

const devTok = "device-token-0123456789"
const ingTok = "ingest-token-0123456789"

type fakeFrames struct{}

func (fakeFrames) Frame(context.Context, string, bool) (render.Frame, error) {
	return render.Frame{PNG: []byte("\x89PNGfake"), RenderedAt: time.Now()}, nil
}
func (fakeFrames) Status() (string, time.Time) { return "", time.Time{} }

func newTestServer(t *testing.T) (*httptest.Server, cache.Store) {
	t.Helper()
	cfg := config.Default()
	cfg.DeviceTokens = []string{devTok}
	cfg.IngestTokens = []string{ingTok}
	store, _ := cache.NewMemory("")
	s := &Server{Cfg: &cfg, Store: store, Builder: &dashboard.Builder{Store: store, Cfg: &cfg},
		Frames: fakeFrames{}, Log: slog.New(slog.NewTextHandler(io.Discard, nil)), StartedAt: time.Now()}
	h, err := s.Handler()
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(h)
	t.Cleanup(ts.Close)
	return ts, store
}

func get(t *testing.T, url string, hdr map[string]string) (*http.Response, string) {
	t.Helper()
	req, _ := http.NewRequest("GET", url, nil)
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	return resp, string(b)
}

func TestAuth(t *testing.T) {
	ts, _ := newTestServer(t)
	for _, p := range []string{"/api/dashboard", "/api/eink/frame.png", "/dashboard", "/dashboard/preview", "/dashboard/image"} {
		if r, _ := get(t, ts.URL+p, nil); r.StatusCode != 401 {
			t.Errorf("%s without token: %d", p, r.StatusCode)
		}
		if r, _ := get(t, ts.URL+p+"?token=wrong-token-xxxxxxxx", nil); r.StatusCode != 401 {
			t.Errorf("%s wrong token: %d", p, r.StatusCode)
		}
		if r, _ := get(t, ts.URL+p+"?token="+devTok, nil); r.StatusCode != 200 {
			t.Errorf("%s query token: %d", p, r.StatusCode)
		}
	}
	if r, _ := get(t, ts.URL+"/api/dashboard", map[string]string{"Authorization": "Bearer " + devTok}); r.StatusCode != 200 {
		t.Error("bearer token rejected")
	}
	if r, _ := get(t, ts.URL+"/api/eink/health", nil); r.StatusCode != 200 {
		t.Error("health should be public")
	}
}

func TestDashboardHTMLNoFabrication(t *testing.T) {
	ts, _ := newTestServer(t)
	_, body := get(t, ts.URL+"/dashboard?token="+devTok, nil)
	for _, want := range []string{"Usage not available", "Weather unavailable", "Gold prices unavailable", "Trends unavailable", `content="width=1072"`, `http-equiv="refresh"`} {
		if !strings.Contains(body, want) {
			t.Errorf("missing %q", want)
		}
	}
	if strings.Contains(body, "% left") || strings.Contains(body, "<script") {
		t.Error("no quota numbers or scripts expected")
	}
	// links propagate token for stock Kindle
	if !strings.Contains(body, "token="+devTok) {
		t.Error("token not propagated on links")
	}
}

func TestIngestAndRender(t *testing.T) {
	ts, _ := newTestServer(t)
	now := time.Now()
	payload := map[string]any{
		"provider": "claude", "origin": "claude-statusline", "observed_at": now.UTC().Format(time.RFC3339),
		"five_hour": map[string]any{"used_percent": 37, "resets_at": now.Add(2 * time.Hour).Unix(), "window_minutes": 300},
		"weekly":    map[string]any{"used_percent": 58, "resets_at": now.Add(72 * time.Hour).Unix(), "window_minutes": 10080},
	}
	b, _ := json.Marshal(payload)
	post := func(tok string, body string) int {
		req, _ := http.NewRequest("POST", ts.URL+"/api/ingest/usage", strings.NewReader(body))
		if tok != "" {
			req.Header.Set("Authorization", "Bearer "+tok)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		return resp.StatusCode
	}
	if c := post("", string(b)); c != 401 {
		t.Fatalf("no token: %d", c)
	}
	if c := post(devTok, string(b)); c != 401 {
		t.Fatalf("device token must not allow ingest: %d", c)
	}
	if c := post(ingTok, `{"provider":"claude","bogus":1}`); c != 400 {
		t.Fatalf("unknown field: %d", c)
	}
	if c := post(ingTok, string(b)); c != 202 {
		t.Fatalf("ingest: %d", c)
	}

	_, js := get(t, ts.URL+"/api/dashboard?token="+devTok, nil)
	var d model.Dashboard
	if err := json.Unmarshal([]byte(js), &d); err != nil {
		t.Fatal(err)
	}
	if d.Claude.Meta.Status != model.StatusOK || *d.Claude.Data.FiveHour.UsedPercent != 37 {
		t.Fatalf("claude %+v", d.Claude)
	}
	_, html := get(t, ts.URL+"/dashboard?token="+devTok, nil)
	if !strings.Contains(html, `<span class="big">63</span>`) || !strings.Contains(html, "37% used") {
		t.Error("remaining (63) and used (37) should both be shown")
	}
	if !strings.Contains(html, "width:37%") {
		t.Error("bar fill should represent consumed share")
	}
}

func TestFrameAndPages(t *testing.T) {
	ts, _ := newTestServer(t)
	r, body := get(t, ts.URL+"/api/eink/frame.png?token="+devTok, nil)
	if r.StatusCode != 200 || r.Header.Get("Content-Type") != "image/png" || !strings.HasPrefix(body, "\x89PNG") {
		t.Fatalf("frame: %d %s", r.StatusCode, r.Header.Get("Content-Type"))
	}
	if r, _ := get(t, ts.URL+"/api/eink/frame.png?page=nope&token="+devTok, nil); r.StatusCode != 404 {
		t.Fatalf("unknown page: %d", r.StatusCode)
	}
}

func TestHelpers(t *testing.T) {
	if KVND(model.Ptr[int64](137_500_000)) != "137.500" || KVND(nil) != "—" {
		t.Error("KVND")
	}
	if Change(model.Ptr[int64](-200_000)) != "−200" || Change(model.Ptr[int64](0)) != "±0" || Change(nil) != "" {
		t.Error("Change")
	}
	loc, _ := time.LoadLocation("Asia/Bangkok")
	now := time.Date(2026, 11, 15, 13, 0, 0, 0, time.UTC) // 20:00 local
	q := &model.QuotaWindow{ResetsAt: model.Ptr(time.Date(2026, 11, 15, 15, 30, 0, 0, time.UTC))}
	if got := ResetText(q, now, loc); got != "resets 22:30 today (in 2h 30m)" {
		t.Errorf("ResetText %q", got)
	}
	q.ResetPassed = true
	if !strings.Contains(ResetText(q, now, loc), "awaiting new report") {
		t.Error("reset passed text")
	}
	if Ago(90*time.Minute) != "1h 30m ago" {
		t.Error(Ago(90 * time.Minute))
	}
}
