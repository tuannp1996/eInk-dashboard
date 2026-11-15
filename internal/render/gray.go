// Package render turns the dashboard HTML into an e-ink ready grayscale PNG.
package render

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"math"
)

// ToGray converts img to 8-bit grayscale quantized to `levels` evenly spaced
// gray levels (PW3 panel: 16). With dither, Floyd–Steinberg error diffusion
// is applied (good for photos, usually worse for text — off by default).
func ToGray(img image.Image, levels int, dither bool) *image.Gray {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	out := image.NewGray(image.Rect(0, 0, w, h))
	if levels < 2 {
		levels = 2
	}
	if levels > 256 {
		levels = 256
	}
	step := 255.0 / float64(levels-1)
	q := func(v float64) uint8 {
		if v < 0 {
			v = 0
		} else if v > 255 {
			v = 255
		}
		return uint8(math.Round(math.Round(v/step) * step))
	}
	lum := func(x, y int) float64 {
		g := color.GrayModel.Convert(img.At(b.Min.X+x, b.Min.Y+y)).(color.Gray)
		return float64(g.Y)
	}
	if !dither {
		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
				out.Pix[y*out.Stride+x] = q(lum(x, y))
			}
		}
		return out
	}
	cur := make([]float64, w+2)
	next := make([]float64, w+2)
	for x := 0; x < w; x++ {
		cur[x+1] = lum(x, 0)
	}
	for y := 0; y < h; y++ {
		for i := range next {
			next[i] = 0
		}
		if y+1 < h {
			for x := 0; x < w; x++ {
				next[x+1] = lum(x, y+1)
			}
		}
		for x := 0; x < w; x++ {
			old := cur[x+1]
			nv := q(old)
			out.Pix[y*out.Stride+x] = nv
			e := old - float64(nv)
			cur[x+2] += e * 7 / 16
			next[x] += e * 3 / 16
			next[x+1] += e * 5 / 16
			next[x+2] += e * 1 / 16
		}
		cur, next = next, cur
	}
	return out
}

// EncodeGrayPNG encodes a grayscale image (8-bit gray PNG, which FBInk and
// the Kindle browser both handle).
func EncodeGrayPNG(g *image.Gray) ([]byte, error) {
	var buf bytes.Buffer
	enc := png.Encoder{CompressionLevel: png.BestCompression}
	if err := enc.Encode(&buf, g); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// ProcessScreenshot decodes a PNG screenshot, crops/pads it to w×h and
// returns the quantized grayscale PNG bytes.
func ProcessScreenshot(raw []byte, w, h, levels int, dither bool) ([]byte, error) {
	src, err := png.Decode(bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	canvas := image.NewRGBA(image.Rect(0, 0, w, h))
	for i := range canvas.Pix {
		canvas.Pix[i] = 0xff
	}
	sb := src.Bounds()
	for y := 0; y < h && y < sb.Dy(); y++ {
		for x := 0; x < w && x < sb.Dx(); x++ {
			canvas.Set(x, y, src.At(sb.Min.X+x, sb.Min.Y+y))
		}
	}
	return EncodeGrayPNG(ToGray(canvas, levels, dither))
}
