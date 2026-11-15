package render

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/chromedp/cdproto/emulation"
	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/chromedp"
	"golang.org/x/sync/singleflight"

	"github.com/viaiz/eink-dashboard/internal/config"
)

// Frame is a rendered PNG.
type Frame struct {
	PNG        []byte
	RenderedAt time.Time
}

// Renderer screenshots the server's own /dashboard page with headless
// Chromium (via the DevTools protocol — the same engine Playwright drives)
// and converts the result to an e-ink friendly grayscale PNG.
type Renderer struct {
	cfg     config.RenderConfig
	baseURL string // e.g. http://127.0.0.1:8080
	token   string // internal render token
	log     *slog.Logger

	mu        sync.Mutex
	allocCtx  context.Context
	allocStop context.CancelFunc
	browser   context.Context
	brStop    context.CancelFunc

	cacheMu sync.Mutex
	cache   map[string]Frame
	sf      singleflight.Group

	lastErr   error
	lastErrAt time.Time
}

func NewRenderer(cfg config.RenderConfig, baseURL, token string, log *slog.Logger) *Renderer {
	return &Renderer{cfg: cfg, baseURL: baseURL, token: token, log: log, cache: map[string]Frame{}}
}

// Frame returns a cached frame for the page if younger than CacheTTL,
// otherwise renders a new one. Concurrent requests share one render.
func (r *Renderer) Frame(ctx context.Context, pageName string, force bool) (Frame, error) {
	r.cacheMu.Lock()
	f, ok := r.cache[pageName]
	r.cacheMu.Unlock()
	if ok && !force && time.Since(f.RenderedAt) < r.cfg.CacheTTL.Duration {
		return f, nil
	}
	v, err, _ := r.sf.Do(pageName, func() (any, error) {
		png, err := r.render(ctx, pageName)
		if err != nil {
			r.mu.Lock()
			r.lastErr, r.lastErrAt = err, time.Now()
			r.mu.Unlock()
			return nil, err
		}
		nf := Frame{PNG: png, RenderedAt: time.Now()}
		r.cacheMu.Lock()
		r.cache[pageName] = nf
		r.cacheMu.Unlock()
		r.mu.Lock()
		r.lastErr = nil
		r.mu.Unlock()
		return nf, nil
	})
	if err != nil {
		// Serve the last good frame rather than nothing (it shows its own timestamp).
		if ok {
			r.log.Warn("render failed, serving previous frame", "page", pageName, "err", err)
			return f, nil
		}
		return Frame{}, err
	}
	return v.(Frame), nil
}

// Status reports the last render error, if any.
func (r *Renderer) Status() (lastErr string, at time.Time) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.lastErr == nil {
		return "", time.Time{}
	}
	return r.lastErr.Error(), r.lastErrAt
}

func (r *Renderer) ensureBrowser() (context.Context, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.browser != nil && r.browser.Err() == nil {
		return r.browser, nil
	}
	if r.allocStop != nil {
		r.brStop()
		r.allocStop()
	}
	if r.cfg.ChromeRemote != "" {
		r.allocCtx, r.allocStop = chromedp.NewRemoteAllocator(context.Background(), r.cfg.ChromeRemote)
	} else {
		opts := append(chromedp.DefaultExecAllocatorOptions[:],
			chromedp.Flag("headless", true),
			chromedp.Flag("hide-scrollbars", true),
			chromedp.Flag("font-render-hinting", "none"),
			chromedp.Flag("disable-gpu", true),
			chromedp.Flag("disable-dev-shm-usage", true),
			chromedp.NoSandbox, // container runs as non-root without user namespaces
			chromedp.WindowSize(r.cfg.Width, r.cfg.Height),
		)
		if r.cfg.ChromePath != "" {
			opts = append(opts, chromedp.ExecPath(r.cfg.ChromePath))
		}
		r.allocCtx, r.allocStop = chromedp.NewExecAllocator(context.Background(), opts...)
	}
	r.browser, r.brStop = chromedp.NewContext(r.allocCtx, chromedp.WithErrorf(func(format string, args ...any) {
		msg := fmt.Sprintf(format, args...)
		// Newer Chromium emits CDP enum values unknown to the pinned cdproto; harmless.
		if strings.Contains(msg, "could not unmarshal event") {
			return
		}
		r.log.Warn("chromedp", "msg", msg)
	}))
	if err := chromedp.Run(r.browser); err != nil { // starts the browser
		r.brStop()
		r.allocStop()
		r.browser, r.allocStop = nil, nil
		return nil, fmt.Errorf("start chromium: %w", err)
	}
	r.log.Info("chromium started")
	return r.browser, nil
}

func (r *Renderer) render(ctx context.Context, pageName string) ([]byte, error) {
	br, err := r.ensureBrowser()
	if err != nil {
		return nil, err
	}
	tab, cancel := chromedp.NewContext(br)
	defer cancel()
	tab, cancelT := context.WithTimeout(tab, r.cfg.Timeout.Duration)
	defer cancelT()
	stop := context.AfterFunc(ctx, cancelT)
	defer stop()

	u := fmt.Sprintf("%s/dashboard?render=1&page=%s&token=%s", r.baseURL, url.QueryEscape(pageName), url.QueryEscape(r.token))
	var raw []byte
	err = chromedp.Run(tab,
		emulation.SetDeviceMetricsOverride(int64(r.cfg.Width), int64(r.cfg.Height), 1, false),
		chromedp.Navigate(u),
		chromedp.WaitReady("body", chromedp.ByQuery),
		chromedp.ActionFunc(func(ctx context.Context) error {
			var e error
			raw, e = page.CaptureScreenshot().
				WithFormat(page.CaptureScreenshotFormatPng).
				WithClip(&page.Viewport{X: 0, Y: 0, Width: float64(r.cfg.Width), Height: float64(r.cfg.Height), Scale: 1}).
				WithCaptureBeyondViewport(false).
				Do(ctx)
			return e
		}),
	)
	if err != nil {
		if errors.Is(err, context.Canceled) && br.Err() != nil {
			r.mu.Lock()
			r.browser = nil // force restart next time
			r.mu.Unlock()
		}
		return nil, fmt.Errorf("render page %q: %w", pageName, err)
	}
	return ProcessScreenshot(raw, r.cfg.Width, r.cfg.Height, r.cfg.GrayLevels, r.cfg.Dither)
}

// Close shuts down Chromium.
func (r *Renderer) Close() {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.brStop != nil {
		r.brStop()
	}
	if r.allocStop != nil {
		r.allocStop()
	}
}
