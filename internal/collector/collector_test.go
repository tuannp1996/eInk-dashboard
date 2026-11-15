package collector

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/viaiz/eink-dashboard/internal/cache"
	"github.com/viaiz/eink-dashboard/internal/config"
	"github.com/viaiz/eink-dashboard/internal/model"
)

func fixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestParseOpenMeteoFixture(t *testing.T) {
	d, err := ParseOpenMeteo(fixture(t, "openmeteo.json"), "Hà Nội", "Asia/Bangkok")
	if err != nil {
		t.Fatal(err)
	}
	if d.Location != "Hà Nội" || len(d.Daily) != 4 {
		t.Fatalf("unexpected: %+v", d)
	}
	if d.Current.Humidity <= 0 || d.Current.Humidity > 100 {
		t.Fatalf("humidity out of range: %v", d.Current.Humidity)
	}
	if d.Current.Condition == "" || d.Current.Icon == "" {
		t.Fatal("condition/icon not mapped")
	}
	if _, off := d.Current.Time.Zone(); off != 7*3600 {
		t.Fatalf("time not in UTC+7: %v", d.Current.Time)
	}
}

func TestParseOpenMeteoMissingFields(t *testing.T) {
	if _, err := ParseOpenMeteo([]byte(`{"current":{"time":"2026-01-01T00:00"}}`), "x", "Asia/Bangkok"); err == nil {
		t.Fatal("expected error for missing fields (must not default to 0)")
	}
}

func TestWMO(t *testing.T) {
	cases := map[int]string{0: "sun", 2: "partly", 3: "cloud", 45: "fog", 53: "drizzle", 63: "rain", 81: "rain", 75: "snow", 95: "storm"}
	for c, want := range cases {
		if got := WMOIcon(c); got != want {
			t.Errorf("WMOIcon(%d)=%s want %s", c, got, want)
		}
	}
	if WMOText(1234) != "WMO code 1234" {
		t.Error("unknown code should be shown verbatim")
	}
}

func TestParseVangTodayFixture(t *testing.T) {
	items := []config.GoldItemConfig{{Code: "SJL1L10", Label: "SJC"}, {Code: "NOPE", Label: "missing"}, {Code: "BTSJC"}}
	d, ts, err := ParseVangToday(fixture(t, "vangtoday.json"), items)
	if err != nil {
		t.Fatal(err)
	}
	if ts == nil {
		t.Fatal("expected source timestamp")
	}
	if len(d.Items) != 2 {
		t.Fatalf("missing codes must be skipped, not invented: got %d items", len(d.Items))
	}
	if d.Items[0].Label != "SJC" || d.Items[1].Label == "" {
		t.Fatalf("labels: %+v", d.Items)
	}
	for _, it := range d.Items {
		if it.Buy == nil || it.Sell == nil || *it.Buy < 10_000_000 || *it.Sell < *it.Buy {
			t.Fatalf("implausible price %+v", it)
		}
	}
}

func TestParseVangTodayZeroIsUnquoted(t *testing.T) {
	body := `{"success":true,"timestamp":1,"prices":{"X":{"name":"x","buy":100,"sell":0}}}`
	d, _, err := ParseVangToday([]byte(body), []config.GoldItemConfig{{Code: "X"}})
	if err != nil {
		t.Fatal(err)
	}
	if d.Items[0].Sell != nil {
		t.Fatal("sell=0 must be treated as not quoted")
	}
}

func TestParseGenericGold(t *testing.T) {
	body := `{"data":{"updated":"2026-11-15T08:00:00Z","list":[{"type":"SJC","b":"134,500","s":"137,500"},{"type":"PNJ","b":134000,"s":137000}]}}`
	gc := config.GenericGoldConfig{ItemsPath: "data.list", CodeField: "type", BuyField: "b", SellField: "s", Multiplier: 1000, TimeField: "data.updated"}
	d, ts, err := ParseGenericGold([]byte(body), gc, []config.GoldItemConfig{{Code: "SJC", Label: "SJC"}, {Code: "PNJ"}})
	if err != nil {
		t.Fatal(err)
	}
	if ts == nil || *d.Items[0].Buy != 134_500_000 || *d.Items[1].Sell != 137_000_000 {
		t.Fatalf("bad mapping: %+v %v", d.Items, ts)
	}
}

func TestGoldChangeVsPreviousFetch(t *testing.T) {
	g := &Gold{}
	d1 := &model.GoldData{Items: []model.GoldItem{{Code: "A", Buy: model.Ptr[int64](100), Sell: model.Ptr[int64](200)}}}
	g.fillChanges(d1)
	if d1.Items[0].ChangeBuy != nil {
		t.Fatal("first fetch has no basis for a change")
	}
	d2 := &model.GoldData{Items: []model.GoldItem{{Code: "A", Buy: model.Ptr[int64](150), Sell: model.Ptr[int64](190)}}}
	g.fillChanges(d2)
	if *d2.Items[0].ChangeBuy != 50 || *d2.Items[0].ChangeSell != -10 || d2.Items[0].ChangeBasis == "" {
		t.Fatalf("change: %+v", d2.Items[0])
	}
}

func TestParseTrendsFixture(t *testing.T) {
	d, err := ParseTrendsRSS(fixture(t, "trends_vn.xml"), "VN", 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Items) != 5 {
		t.Fatalf("want 5 got %d", len(d.Items))
	}
	for i, it := range d.Items {
		if it.Rank != i+1 || it.Title == "" {
			t.Fatalf("bad item %+v", it)
		}
		if it.ApproxTraffic == "" {
			t.Fatalf("approx traffic should come from feed: %+v", it)
		}
	}
}

func TestParseTrendsEmpty(t *testing.T) {
	if _, err := ParseTrendsRSS([]byte(`<rss><channel></channel></rss>`), "VN", 5); err == nil {
		t.Fatal("empty feed must be an error")
	}
}

// ---------- runner ----------

type fakeCollector struct {
	calls   atomic.Int32
	failFor int32
	perm    bool
}

func (f *fakeCollector) Key() string    { return "fake" }
func (f *fakeCollector) Source() string { return "fake" }
func (f *fakeCollector) Fetch(ctx context.Context) (Result, error) {
	n := f.calls.Add(1)
	if n <= f.failFor {
		if f.perm {
			return Result{}, &PermanentError{errors.New("bad request")}
		}
		return Result{}, errors.New("boom")
	}
	return Result{Data: map[string]int{"n": int(n)}}, nil
}

func newTestRunner(store cache.Store) *Runner {
	r := NewRunner(store, slog.New(slog.NewTextHandler(io.Discard, nil)))
	r.sleep = func(context.Context, time.Duration) error { return nil }
	return r
}

func TestRunnerRetriesThenSucceeds(t *testing.T) {
	store, _ := cache.NewMemory("")
	r := newTestRunner(store)
	f := &fakeCollector{failFor: 2}
	if err := r.RunOnce(context.Background(), Spec{Collector: f, Timeout: time.Second, Retries: 3}); err != nil {
		t.Fatal(err)
	}
	if f.calls.Load() != 3 {
		t.Fatalf("calls=%d", f.calls.Load())
	}
	e, ok, _ := store.Get(context.Background(), "fake")
	if !ok || e.UpdatedAt == nil || e.LastError != "" {
		t.Fatalf("entry %+v", e)
	}
}

func TestRunnerKeepsFallbackOnFailure(t *testing.T) {
	store, _ := cache.NewMemory("")
	r := newTestRunner(store)
	ok := &fakeCollector{}
	_ = r.RunOnce(context.Background(), Spec{Collector: ok, Timeout: time.Second})
	before, _, _ := store.Get(context.Background(), "fake")

	bad := &fakeCollector{failFor: 100}
	if err := r.RunOnce(context.Background(), Spec{Collector: bad, Timeout: time.Second, Retries: 2}); err == nil {
		t.Fatal("expected error")
	}
	if bad.calls.Load() != 3 {
		t.Fatalf("retries: %d", bad.calls.Load())
	}
	after, _, _ := store.Get(context.Background(), "fake")
	if string(after.Data) != string(before.Data) || !after.UpdatedAt.Equal(*before.UpdatedAt) {
		t.Fatal("failed fetch must not overwrite last good data")
	}
	if after.LastError == "" {
		t.Fatal("last error should be recorded")
	}
}

func TestRunnerPermanentErrorNoRetry(t *testing.T) {
	store, _ := cache.NewMemory("")
	r := newTestRunner(store)
	f := &fakeCollector{failFor: 100, perm: true}
	_ = r.RunOnce(context.Background(), Spec{Collector: f, Timeout: time.Second, Retries: 5})
	if f.calls.Load() != 1 {
		t.Fatalf("permanent errors must not retry, calls=%d", f.calls.Load())
	}
}

func TestBackoffGrowsAndCaps(t *testing.T) {
	for i := 1; i <= 3; i++ {
		d := Backoff(time.Second, i)
		base := time.Second << (i - 1)
		if d < base*8/10 || d > base*12/10 {
			t.Fatalf("attempt %d: %v outside jitter range of %v", i, d, base)
		}
	}
	if d := Backoff(time.Second, 30); d > 2*time.Minute*12/10 {
		t.Fatalf("not capped: %v", d)
	}
}

func TestCollectorTimeoutAndHTTPErrors(t *testing.T) {
	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-time.After(2 * time.Second):
		}
	}))
	defer slow.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	if _, err := (&Trends{BaseURL: slow.URL, Geo: "VN"}).Fetch(ctx); err == nil {
		t.Fatal("expected timeout")
	}

	notFound := httptest.NewServer(http.NotFoundHandler())
	defer notFound.Close()
	_, err := (&Trends{BaseURL: notFound.URL, Geo: "VN"}).Fetch(context.Background())
	var perm *PermanentError
	if !errors.As(err, &perm) {
		t.Fatalf("404 should be permanent, got %v", err)
	}
}

func TestWeatherEndToEndWithStub(t *testing.T) {
	body := fixture(t, "openmeteo.json")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("timezone") != "Asia/Bangkok" || r.URL.Query().Get("latitude") != "21.0285" {
			http.Error(w, "bad query", 400)
			return
		}
		_, _ = w.Write(body)
	}))
	defer srv.Close()
	w := &Weather{BaseURL: srv.URL, Location: "Hà Nội", Lat: 21.0285, Lon: 105.8542, Days: 4, Timezone: "Asia/Bangkok"}
	res, err := w.Fetch(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(res.Data)
	if len(b) < 50 || res.SourceTime == nil {
		t.Fatal("unexpected result")
	}
}
