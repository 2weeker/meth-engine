package captchaimg

import (
	"bytes"
	"image"
	"testing"
)

var v1 = Canvas{192, 64}

func TestRenderStaysOnCanvas(t *testing.T) {
	for seed := uint64(1); seed <= 300; seed++ {

		text := []string{"wmwmwm", "bdfhkt", "gypygp", "dgbpky", "w8m4w8", "926457", "0o0o00"}[seed%7]
		img, _ := Layers(text, seed, v1)
		b := img.Bounds()
		ink, clipped := 0, 0
		for y := b.Min.Y; y < b.Max.Y; y++ {
			for x := b.Min.X; x < b.Max.X; x++ {
				c := img.NRGBAAt(x, y)
				if c.R != 0 || c.G != 0 || c.B != 0 {
					t.Fatalf("seed %d: pixel (%d,%d) is %v; glyphs are black", seed, x, y, c)
				}
				if c.A > 128 {
					ink++
					if x == b.Min.X || x == b.Max.X-1 || y == b.Min.Y || y == b.Max.Y-1 {
						clipped++
					}
				}
			}
		}
		if ink < 300 {
			t.Errorf("seed %d: only %d inked pixels; the text is missing", seed, ink)
		}
		if ink > v1.Width*v1.Height/2 {
			t.Errorf("seed %d: %d inked pixels; the canvas is mostly ink, not transparent", seed, ink)
		}
		if clipped > 0 {
			t.Errorf("seed %d: %d inked pixels on the canvas edge; a glyph is cut off", seed, clipped)
		}
	}
}

func TestRenderIsDistorted(t *testing.T) {

	a, b := Draw("abcdef", 1, v1), Draw("abcdef", 2, v1)
	if bytes.Equal(a.Pix, b.Pix) {
		t.Error("two seeds gave the same image")
	}
	var _ image.Image = a
}

func TestRenderAddsNoise(t *testing.T) {
	inked := func(img *image.NRGBA) (n int) {
		for i := 3; i < len(img.Pix); i += 4 {
			if img.Pix[i] > 64 {
				n++
			}
		}
		return n
	}
	for seed := uint64(1); seed <= 100; seed++ {
		text, noise := Layers("ab3cd7", seed, v1)
		final := Draw("ab3cd7", seed, v1)
		if n, bare := inked(final), inked(text); n <= bare+150 {
			t.Errorf("seed %d: noise added only %d inked pixels", seed, n-bare)
		}
		if n := inked(final); n > v1.Width*v1.Height*45/100 {
			t.Errorf("seed %d: %d of %d pixels inked; the text would drown", seed, n, v1.Width*v1.Height)
		}

		outside := 0
		for i := 3; i < len(noise.Pix); i += 4 {
			if noise.Pix[i] > 64 && text.Pix[i] == 0 {
				outside++
			}
		}
		if outside < 100 {
			t.Errorf("seed %d: only %d noise pixels away from the text", seed, outside)
		}

		for i := 0; i < len(final.Pix); i += 4 {
			if final.Pix[i] != 0 || final.Pix[i+1] != 0 || final.Pix[i+2] != 0 {
				t.Fatalf("seed %d: a pixel is not black: %v", seed, final.Pix[i:i+4])
			}
		}
	}

	if !bytes.Equal(Draw("ab3cd7", 42, v1).Pix, Draw("ab3cd7", 42, v1).Pix) {
		t.Error("the same seed must give the same image")
	}
	if bytes.Equal(Draw("ab3cd7", 42, v1).Pix, Draw("ab3cd7", 43, v1).Pix) {
		t.Error("different seeds must give different noise")
	}
}

func TestZeroIsSlashed(t *testing.T) {
	middle := func(ch string) int {
		m := drawText(ch, 44, 0, v1)
		minX, minY, maxX, maxY := m.Rect.Max.X, m.Rect.Max.Y, 0, 0
		for y := m.Rect.Min.Y; y < m.Rect.Max.Y; y++ {
			for x := m.Rect.Min.X; x < m.Rect.Max.X; x++ {
				if m.AlphaAt(x, y).A > 128 {
					minX, minY, maxX, maxY = min(minX, x), min(minY, y), max(maxX, x), max(maxY, y)
				}
			}
		}
		cx, cy, n := (minX+maxX)/2, (minY+maxY)/2, 0
		for y := cy - 4; y <= cy+4; y++ {
			for x := cx - 4; x <= cx+4; x++ {
				if m.AlphaAt(x, y).A > 128 {
					n++
				}
			}
		}
		return n
	}
	if zero, o := middle("0"), middle("o"); zero < 45 || o != 0 {
		t.Errorf("the zero's middle must be crossed by its slash and the o's empty: zero %d, o %d", zero, o)
	}
}
