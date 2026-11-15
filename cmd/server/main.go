// Command server runs the ViaiZ e-ink dashboard backend:
// collectors → cache → HTML dashboard → Chromium renderer → PNG endpoint.
package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"flag"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
	_ "time/tzdata" // embed tz database (Asia/Bangkok) for minimal containers

	"github.com/viaiz/eink-dashboard/internal/cache"
	"github.com/viaiz/eink-dashboard/internal/collector"
	"github.com/viaiz/eink-dashboard/internal/config"
	"github.com/viaiz/eink-dashboard/internal/dashboard"
	"github.com/viaiz/eink-dashboard/internal/mock"
	"github.com/viaiz/eink-dashboard/internal/render"
	"github.com/viaiz/eink-dashboard/internal/web"
)

func main() {
	cfgPath := flag.String("config", envOr("CONFIG_FILE", "config/config.yaml"), "path to YAML config")
	noRender := flag.Bool("no-render", os.Getenv("RENDER_DISABLED") == "true", "disable Chromium PNG rendering")
	flag.Parse()

	cfg, err := config.Load(*cfgPath)
	log := newLogger(cfg.LogLevel)
	if err != nil {
		log.Error("config", "err", err)
		os.Exit(2)
	}
	if cfg.AuthDisabled {
		log.Warn("AUTH_DISABLED=true: all endpoints are public. Use only for local development.")
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	store, err := openStore(cfg)
	if err != nil {
		log.Error("cache", "err", err)
		os.Exit(1)
	}
	defer store.Close()

	var runner *collector.Runner
	if cfg.MockData != "" {
		mem, _ := cache.NewMemory("") // never mix mock data into persistent storage
		store = mem
		if err := mock.Load(ctx, cfg.MockData, store, time.Now()); err != nil {
			log.Error("mock", "err", err)
			os.Exit(1)
		}
		log.Warn("MOCK MODE: serving example data, collectors disabled", "file", cfg.MockData)
	} else {
		runner = collector.NewRunner(store, log, specs(&cfg)...)
		runner.Start(ctx)
	}

	renderToken := randomToken()
	var frames web.FrameSource
	var renderer *render.Renderer
	if !*noRender {
		base := cfg.BaseURL
		if base == "" {
			base = "http://" + loopback(cfg.Listen) + cfg.BasePath
		}
		renderer = render.NewRenderer(cfg.Render, strings.TrimRight(base, "/"), renderToken, log.With("component", "renderer"))
		frames = renderer
		defer renderer.Close()
	}

	srv := &web.Server{
		Cfg: &cfg, Store: store, Log: log, Frames: frames, RenderToken: renderToken,
		Builder:   &dashboard.Builder{Store: store, Cfg: &cfg},
		StartedAt: time.Now(), ReadOnlyMock: cfg.MockData != "",
	}
	h, err := srv.Handler()
	if err != nil {
		log.Error("handler", "err", err)
		os.Exit(1)
	}
	hs := &http.Server{
		Addr: cfg.Listen, Handler: h,
		ReadHeaderTimeout: 10 * time.Second, ReadTimeout: 30 * time.Second,
		WriteTimeout: 90 * time.Second, IdleTimeout: 120 * time.Second,
	}
	go func() {
		<-ctx.Done()
		sctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = hs.Shutdown(sctx)
	}()
	log.Info("listening", "addr", cfg.Listen, "pages", len(cfg.Pages), "render", !*noRender)
	if err := hs.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Error("server", "err", err)
		os.Exit(1)
	}
	if runner != nil {
		runner.Wait()
	}
}

func specs(cfg *config.Config) []collector.Spec {
	var out []collector.Spec
	add := func(b config.CollectorBase, c collector.Collector) {
		if b.Enabled {
			out = append(out, collector.Spec{Collector: c, Interval: b.Interval.Duration, Timeout: b.Timeout.Duration, Retries: b.Retries})
		}
	}
	w := cfg.Weather
	add(w.CollectorBase, &collector.Weather{BaseURL: w.URL, Location: w.Location, Lat: w.Latitude, Lon: w.Longitude, Days: w.Days, Timezone: cfg.Timezone})
	add(cfg.Gold.CollectorBase, &collector.Gold{Cfg: cfg.Gold})
	t := cfg.Trends
	add(t.CollectorBase, &collector.Trends{BaseURL: t.URL, Geo: t.Geo, Limit: t.Limit})
	return out
}

func openStore(cfg config.Config) (cache.Store, error) {
	if cfg.RedisURL != "" {
		return cache.NewRedis(cfg.RedisURL)
	}
	return cache.NewMemory(cfg.CacheFile)
}

func loopback(listen string) string {
	host, port, err := net.SplitHostPort(listen)
	if err != nil {
		return "127.0.0.1:8080"
	}
	if host == "" || host == "0.0.0.0" || host == "::" {
		host = "127.0.0.1"
	}
	return net.JoinHostPort(host, port)
}

func randomToken() string {
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}

func newLogger(level string) *slog.Logger {
	var l slog.Level
	_ = l.UnmarshalText([]byte(level))
	return slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: l}))
}

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
