package render

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/png"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/viaiz/eink-dashboard/internal/config"
)

func gradient(w, h int) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			v := uint8(x * 255 / (w - 1))
			img.Set(x, y, color.RGBA{v, v / 2, 255 - v, 255})
		}
	}
	return img
}

func TestToGrayQuantizes16Levels(t *testing.T) {
	g := ToGray(gradient(256, 4), 16, false)
	seen := map[uint8]bool{}
	for _, p := range g.Pix {
		seen[p] = true
		if int(p)%17 != 0 { // 255/15 = 17
			t.Fatalf("value %d is not one of 16 levels", p)
		}
	}
	if len(seen) < 8 {
		t.Fatalf("too few levels used: %d", len(seen))
	}
}

func TestToGrayDitherKeepsLevels(t *testing.T) {
	g := ToGray(gradient(64, 64), 4, true)
	for _, p := range g.Pix {
		if p != 0 && p != 85 && p != 170 && p != 255 {
			t.Fatalf("dithered value %d not in 4-level palette", p)
		}
	}
}

func TestProcessScreenshotSizeAndFormat(t *testing.T) {
	var buf bytes.Buffer
	_ = png.Encode(&buf, gradient(100, 50))
	out, err := ProcessScreenshot(buf.Bytes(), 120, 60, 16, false)
	if err != nil {
		t.Fatal(err)
	}
	img, err := png.Decode(bytes.NewReader(out))
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := img.(*image.Gray); !ok {
		t.Fatalf("expected 8-bit gray PNG, got %T", img)
	}
	if b := img.Bounds(); b.Dx() != 120 || b.Dy() != 60 {
		t.Fatalf("size %v", b)
	}
	// padded area is white
	if img.(*image.Gray).GrayAt(110, 55).Y != 255 {
		t.Fatal("padding should be white")
	}
}

// TestChromiumRender runs only when CHROME_PATH points to a Chromium/Chrome binary.
func TestChromiumRender(t *testing.T) {
	chrome := os.Getenv("CHROME_PATH")
	if chrome == "" {
		t.Skip("CHROME_PATH not set")
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("token") != "tok" {
			http.Error(w, "no", 401)
			return
		}
		_, _ = w.Write([]byte(`<html><body style="margin:0;background:#fff"><div style="width:200px;height:100px;background:#000"></div></body></html>`))
	}))
	defer srv.Close()
	cfg := config.Default().Render
	cfg.Width, cfg.Height, cfg.ChromePath = 400, 300, chrome
	cfg.Timeout = config.Duration{Duration: 30 * time.Second}
	r := NewRenderer(cfg, srv.URL, "tok", slog.New(slog.NewTextHandler(io.Discard, nil)))
	defer r.Close()
	f, err := r.Frame(context.Background(), "main", false)
	if err != nil {
		t.Fatal(err)
	}
	img, err := png.Decode(bytes.NewReader(f.PNG))
	if err != nil {
		t.Fatal(err)
	}
	g := img.(*image.Gray)
	if g.Bounds().Dx() != 400 || g.GrayAt(50, 50).Y != 0 || g.GrayAt(350, 250).Y != 255 {
		t.Fatalf("unexpected render: %v black=%d white=%d", g.Bounds(), g.GrayAt(50, 50).Y, g.GrayAt(350, 250).Y)
	}
	// cached
	f2, _ := r.Frame(context.Background(), "main", false)
	if !f2.RenderedAt.Equal(f.RenderedAt) {
		t.Fatal("expected cached frame")
	}
}
