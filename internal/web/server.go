// Package web exposes the HTTP API, the Kindle-compatible HTML dashboard,
// the responsive preview and the PNG frame endpoint.
package web

import (
	"context"
	"crypto/subtle"
	"embed"
	"encoding/json"
	"errors"
	"html/template"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/viaiz/eink-dashboard/internal/cache"
	"github.com/viaiz/eink-dashboard/internal/config"
	"github.com/viaiz/eink-dashboard/internal/dashboard"
	"github.com/viaiz/eink-dashboard/internal/render"
)

//go:embed templates/*.html
var templateFS embed.FS

//go:embed static
var staticFS embed.FS

// FrameSource renders PNG frames (implemented by render.Renderer).
type FrameSource interface {
	Frame(ctx context.Context, page string, force bool) (render.Frame, error)
	Status() (string, time.Time)
}

type Server struct {
	Cfg          *config.Config
	Store        cache.Store
	Builder      *dashboard.Builder
	Frames       FrameSource // may be nil when rendering is disabled
	Log          *slog.Logger
	RenderToken  string // internal token used by the headless browser
	Now          func() time.Time
	StartedAt    time.Time
	ReadOnlyMock bool

	tmpl *template.Template
	loc  *time.Location
}

func (s *Server) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

// Handler builds the HTTP handler.
func (s *Server) Handler() (http.Handler, error) {
	loc, err := time.LoadLocation(s.Cfg.Timezone)
	if err != nil {
		return nil, err
	}
	s.loc = loc
	s.tmpl, err = template.New("").Funcs(funcMap(loc, s.now)).ParseFS(templateFS, "templates/*.html")
	if err != nil {
		return nil, err
	}
	st, _ := fs.Sub(staticFS, "static")

	mux := http.NewServeMux()
	mux.Handle("GET /static/", http.StripPrefix("/static/", cacheFor(http.FileServer(http.FS(st)), time.Hour)))
	mux.HandleFunc("GET /api/eink/health", s.health)
	mux.HandleFunc("GET /healthz", s.health)
	mux.Handle("GET /api/dashboard", s.device(http.HandlerFunc(s.apiDashboard)))
	mux.Handle("GET /api/eink/frame.png", s.device(http.HandlerFunc(s.frame)))
	mux.Handle("GET /dashboard", s.device(http.HandlerFunc(s.htmlDashboard)))
	mux.Handle("GET /dashboard/preview", s.device(http.HandlerFunc(s.preview)))
	mux.Handle("GET /dashboard/image", s.device(http.HandlerFunc(s.imagePage)))
	mux.Handle("POST /api/ingest/usage", s.ingest(http.HandlerFunc(s.ingestUsage)))
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, s.Cfg.BasePath+"/dashboard/preview"+tokenSuffix(r, "?"), http.StatusFound)
	})
	var handler http.Handler = mux
	if prefix := s.Cfg.BasePath; prefix != "" {
		mounted := http.NewServeMux()
		mounted.Handle(prefix+"/", http.StripPrefix(prefix, mux))
		mounted.HandleFunc("GET "+prefix, func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, prefix+"/dashboard/preview"+tokenSuffix(r, "?"), http.StatusFound)
		})
		handler = mounted
	}
	return s.logging(securityHeaders(handler)), nil
}

// ---------- auth ----------

func bearerOrQuery(r *http.Request) string {
	if h := r.Header.Get("Authorization"); strings.HasPrefix(h, "Bearer ") {
		return strings.TrimSpace(h[7:])
	}
	if t := r.Header.Get("X-Device-Token"); t != "" {
		return t
	}
	return r.URL.Query().Get("token")
}

func tokenMatch(got string, allowed []string) bool {
	if got == "" {
		return false
	}
	ok := 0
	for _, a := range allowed {
		ok |= subtle.ConstantTimeCompare([]byte(got), []byte(a))
	}
	return ok == 1
}

func (s *Server) device(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.Cfg.AuthDisabled {
			next.ServeHTTP(w, r)
			return
		}
		allowed := s.Cfg.DeviceTokens
		if s.RenderToken != "" {
			allowed = append(append([]string{}, allowed...), s.RenderToken)
		}
		if !tokenMatch(bearerOrQuery(r), allowed) {
			w.Header().Set("WWW-Authenticate", `Bearer realm="viaiz"`)
			http.Error(w, "unauthorized: device token required (?token=… or Authorization: Bearer …)", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) ingest(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if len(s.Cfg.IngestTokens) == 0 && !s.Cfg.AuthDisabled {
			http.Error(w, "ingest disabled: INGEST_TOKENS not configured", http.StatusForbidden)
			return
		}
		// Ingest tokens are only accepted from headers (never query strings, which get logged).
		h := r.Header.Get("Authorization")
		if !s.Cfg.AuthDisabled && (!strings.HasPrefix(h, "Bearer ") || !tokenMatch(strings.TrimSpace(h[7:]), s.Cfg.IngestTokens)) {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// tokenSuffix propagates the query token on links (stock Kindle browser
// cannot send headers). sep is "?" or "&".
func tokenSuffix(r *http.Request, sep string) string {
	t := r.URL.Query().Get("token")
	if t == "" {
		return ""
	}
	return sep + "token=" + url.QueryEscape(t)
}

// ---------- handlers ----------

func (s *Server) apiDashboard(w http.ResponseWriter, r *http.Request) {
	d, err := s.Builder.Build(r.Context())
	if err != nil {
		s.fail(w, r, http.StatusInternalServerError, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, d)
}

func (s *Server) view(r *http.Request, fluid bool) (View, int, error) {
	p, ok := s.Cfg.Page(r.URL.Query().Get("page"))
	if !ok {
		return View{}, http.StatusNotFound, errors.New("unknown page")
	}
	d, err := s.Builder.Build(r.Context())
	if err != nil {
		return View{}, http.StatusInternalServerError, err
	}
	refresh := s.Cfg.Kindle.HTMLRefreshSeconds
	if v, err := strconv.Atoi(r.URL.Query().Get("refresh")); err == nil && v >= 0 {
		refresh = v
	}
	isRender := r.URL.Query().Get("render") == "1"
	if isRender {
		refresh = 0
	}
	_, off := s.now().In(s.loc).Zone()
	tz := s.Cfg.Timezone + " (UTC" + offsetString(off) + ")"
	return View{
		D: d, Page: p, Pages: s.Cfg.Pages, Brand: s.Cfg.Brand, BasePath: s.Cfg.BasePath, TZLabel: tz,
		Fluid: fluid, Render: isRender, Token: r.URL.Query().Get("token"), Refresh: refresh, Now: s.now(),
	}, 0, nil
}

func offsetString(sec int) string {
	sign := "+"
	if sec < 0 {
		sign, sec = "-", -sec
	}
	h, m := sec/3600, (sec%3600)/60
	if m == 0 {
		return sign + strconv.Itoa(h)
	}
	return sign + strconv.Itoa(h) + ":" + strconv.Itoa(m)
}

func (s *Server) htmlDashboard(w http.ResponseWriter, r *http.Request) {
	v, code, err := s.view(r, false)
	if err != nil {
		s.fail(w, r, code, err)
		return
	}
	s.exec(w, "dashboard", v)
}

func (s *Server) preview(w http.ResponseWriter, r *http.Request) {
	v, code, err := s.view(r, true)
	if err != nil {
		s.fail(w, r, code, err)
		return
	}
	v.Refresh = 0
	s.exec(w, "dashboard", v)
}

func (s *Server) imagePage(w http.ResponseWriter, r *http.Request) {
	v, code, err := s.view(r, false)
	if err != nil {
		s.fail(w, r, code, err)
		return
	}
	s.exec(w, "image", v)
}

func (s *Server) exec(w http.ResponseWriter, name string, v View) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	if err := s.tmpl.ExecuteTemplate(w, name, v); err != nil {
		s.Log.Error("template", "err", err)
	}
}

func (s *Server) frame(w http.ResponseWriter, r *http.Request) {
	if s.Frames == nil {
		http.Error(w, "renderer disabled", http.StatusServiceUnavailable)
		return
	}
	p, ok := s.Cfg.Page(r.URL.Query().Get("page"))
	if !ok {
		http.Error(w, "unknown page", http.StatusNotFound)
		return
	}
	f, err := s.Frames.Frame(r.Context(), p.Name, r.URL.Query().Get("force") == "1")
	if err != nil {
		s.fail(w, r, http.StatusServiceUnavailable, err)
		return
	}
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Last-Modified", f.RenderedAt.UTC().Format(http.TimeFormat))
	w.Header().Set("X-Rendered-At", f.RenderedAt.UTC().Format(time.RFC3339))
	w.Header().Set("Content-Length", strconv.Itoa(len(f.PNG)))
	_, _ = w.Write(f.PNG)
}

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()
	resp := map[string]any{"status": "ok", "time": s.now().UTC(), "uptime_s": int(s.now().Sub(s.StartedAt).Seconds())}
	code := http.StatusOK
	if err := s.Store.Ping(ctx); err != nil {
		resp["status"], resp["cache"] = "degraded", "error"
		code = http.StatusServiceUnavailable
	} else {
		resp["cache"] = "ok"
	}
	if s.Frames != nil {
		if e, at := s.Frames.Status(); e != "" {
			resp["renderer"] = map[string]any{"status": "error", "at": at.UTC()}
			resp["status"] = "degraded"
		} else {
			resp["renderer"] = map[string]any{"status": "ok"}
		}
	}
	// Per-source status without data or error details (endpoint is unauthenticated).
	if d, err := s.Builder.Build(ctx); err == nil {
		resp["sources"] = map[string]string{
			"claude": string(d.Claude.Meta.Status), "codex": string(d.Codex.Meta.Status),
			"weather": string(d.Weather.Meta.Status), "gold": string(d.Gold.Meta.Status), "trends": string(d.Trends.Meta.Status),
		}
		resp["last_sync"] = d.LastSync
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, code, resp)
}

func (s *Server) ingestUsage(w http.ResponseWriter, r *http.Request) {
	if s.ReadOnlyMock {
		http.Error(w, "mock mode: ingest disabled", http.StatusConflict)
		return
	}
	var rep dashboard.UsageReport
	dec := json.NewDecoder(io.LimitReader(r.Body, 64<<10))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&rep); err != nil {
		http.Error(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}
	now := s.now()
	u, err := rep.Validate(now)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	stored, err := dashboard.StoreUsage(r.Context(), s.Store, u, now)
	if err != nil {
		s.fail(w, r, http.StatusInternalServerError, err)
		return
	}
	s.Log.Info("usage ingested", "provider", u.Provider, "origin", u.Origin, "stored", stored)
	writeJSON(w, http.StatusAccepted, map[string]any{"stored": stored})
}

// ---------- helpers ----------

func (s *Server) fail(w http.ResponseWriter, r *http.Request, code int, err error) {
	s.Log.Error("request failed", "path", r.URL.Path, "code", code, "err", err)
	http.Error(w, http.StatusText(code)+": "+err.Error(), code)
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	_ = enc.Encode(v)
}

func cacheFor(h http.Handler, d time.Duration) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "public, max-age="+strconv.Itoa(int(d.Seconds())))
		h.ServeHTTP(w, r)
	})
}

func securityHeaders(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("X-Frame-Options", "SAMEORIGIN")
		h.ServeHTTP(w, r)
	})
}

type statusRecorder struct {
	http.ResponseWriter
	code int
}

func (s *statusRecorder) WriteHeader(c int) { s.code = c; s.ResponseWriter.WriteHeader(c) }

func (s *Server) logging(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, code: 200}
		h.ServeHTTP(rec, r)
		if r.URL.Path == "/healthz" || strings.HasPrefix(r.URL.Path, "/static/") {
			return
		}
		// Never log query strings (they may carry device tokens).
		s.Log.Info("http", "method", r.Method, "path", r.URL.Path, "code", rec.code, "dur_ms", time.Since(start).Milliseconds(), "ua", truncate(r.UserAgent(), 80))
	})
}

func truncate(s string, n int) string {
	if len(s) > n {
		return s[:n]
	}
	return s
}
