package captchaimg

import (
	"image"
	"image/color"
	"math"
	"math/rand/v2"

	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/gomedium"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
)

type Canvas struct{ Width, Height int }

const oversample = 2

const marginX = 14

var typeface = func() *opentype.Font {
	f, err := opentype.Parse(gomedium.TTF)
	if err != nil {
		panic("captchaimg: the built-in font does not parse: " + err.Error())
	}
	return f
}()

const maxWaveSlope = 0.5

func Draw(text string, seed uint64, c Canvas) *image.NRGBA {
	glyphs, noise := Layers(text, seed, c)

	for i := 3; i < len(glyphs.Pix); i += 4 {
		glyphs.Pix[i] = max(glyphs.Pix[i], noise.Pix[i])
	}
	return glyphs
}

func Layers(text string, seed uint64, c Canvas) (glyphs, noise *image.NRGBA) {
	rng := rand.New(rand.NewPCG(seed, seed^0x9e3779b97f4a7c15))
	glyphs = renderText(text, rng, c)
	noise = image.NewNRGBA(glyphs.Rect)
	drawNoise(noise, rng)
	return glyphs, noise
}

func renderText(text string, rng *rand.Rand, c Canvas) *image.NRGBA {
	pointsize := float64(32 + rng.IntN(16))
	amplitude := float64(2 + rng.IntN(4))
	wavelength := float64(30 + rng.IntN(40))

	amplitude = math.Min(amplitude, maxWaveSlope*wavelength/(2*math.Pi))
	swirl := float64(15+rng.IntN(25)) * math.Pi / 180
	if rng.IntN(2) == 0 {
		swirl = -swirl
	}

	mask := drawText(text, pointsize, amplitude, c)

	out := image.NewNRGBA(image.Rect(0, 0, c.Width, c.Height))
	cx, cy := float64(c.Width)/2, float64(c.Height)/2

	radius, scaleY := cx, float64(c.Width)/float64(c.Height)
	for y := 0; y < c.Height; y++ {
		for x := 0; x < c.Width; x++ {

			sx, sy := float64(x)+.5, float64(y)+.5
			dx, dy := sx-cx, scaleY*(sy-cy)
			if d := math.Hypot(dx, dy); d < radius {
				f := 1 - d/radius
				sin, cos := math.Sincos(swirl * f * f)
				sx = cos*dx - sin*dy + cx
				sy = (sin*dx+cos*dy)/scaleY + cy
			}
			sy -= amplitude * math.Sin(2*math.Pi*sx/wavelength)

			if a := sample(mask, sx*oversample, sy*oversample); a > 0 {
				out.SetNRGBA(x, y, color.NRGBA{A: a})
			}
		}
	}
	return out
}

func drawText(text string, pointsize, amplitude float64, c Canvas) *image.Alpha {
	w, h := c.Width*oversample, c.Height*oversample
	mask := image.NewAlpha(image.Rect(0, 0, w, h))

	size := pointsize * oversample
	face := newFace(size)
	bounds, _ := font.BoundString(face, text)
	width := float64(bounds.Max.X-bounds.Min.X) / 64
	height := float64(bounds.Max.Y-bounds.Min.Y) / 64

	fit := math.Min(float64(w-2*marginX*oversample)/width, (float64(h)-2*(amplitude+4)*oversample)/height)
	if fit < 1 {
		face.Close()
		size *= fit
		face = newFace(size)
		bounds, _ = font.BoundString(face, text)
	}
	defer face.Close()

	inkW, inkH := bounds.Max.X-bounds.Min.X, bounds.Max.Y-bounds.Min.Y
	d := font.Drawer{
		Dst:  mask,
		Src:  image.Opaque,
		Face: face,
		Dot: fixed.Point26_6{
			X: (fixed.I(w)-inkW)/2 - bounds.Min.X,
			Y: (fixed.I(h)-inkH)/2 - bounds.Min.Y,
		},
	}
	start := d.Dot
	d.DrawString(text)
	slashZeros(mask, face, text, start, size)
	return mask
}

const zeroSlash = 0.10

func slashZeros(mask *image.Alpha, face font.Face, text string, start fixed.Point26_6, size float64) {
	dot, prev := start, rune(-1)
	for _, r := range text {
		if prev >= 0 {
			dot.X += face.Kern(prev, r)
		}
		prev = r
		b, advance, ok := face.GlyphBounds(r)
		if ok && r == '0' {

			minX, maxX := float64(dot.X+b.Min.X)/64, float64(dot.X+b.Max.X)/64
			minY, maxY := float64(dot.Y+b.Min.Y)/64, float64(dot.Y+b.Max.Y)/64
			gw, gh := maxX-minX, maxY-minY

			stroke(mask, minX+gw*.22, maxY-gh*.14, maxX-gw*.22, minY+gh*.14, size*zeroSlash/2)
		}
		dot.X += advance
	}
}

func stroke(mask *image.Alpha, x0, y0, x1, y1, r float64) {
	steps := int(math.Hypot(x1-x0, y1-y0)*2) + 1
	for s := 0; s <= steps; s++ {
		t := float64(s) / float64(steps)
		x, y := x0+(x1-x0)*t, y0+(y1-y0)*t
		for py := int(math.Floor(y - r - 1)); py <= int(math.Ceil(y+r+1)); py++ {
			for px := int(math.Floor(x - r - 1)); px <= int(math.Ceil(x+r+1)); px++ {
				if !(image.Point{px, py}).In(mask.Rect) {
					continue
				}
				cover := r + .5 - math.Hypot(float64(px)+.5-x, float64(py)+.5-y)
				if cover <= 0 {
					continue
				}
				i := mask.PixOffset(px, py)
				mask.Pix[i] = max(mask.Pix[i], uint8(math.Min(cover, 1)*255))
			}
		}
	}
}

func newFace(size float64) font.Face {
	face, err := opentype.NewFace(typeface, &opentype.FaceOptions{Size: size, DPI: 72, Hinting: font.HintingNone})
	if err != nil {
		panic("captchaimg: " + err.Error())
	}
	return face
}

func sample(m *image.Alpha, x, y float64) uint8 {
	x, y = x-.5, y-.5
	x0, y0 := int(math.Floor(x)), int(math.Floor(y))
	fx, fy := x-float64(x0), y-float64(y0)
	at := func(px, py int) float64 {
		if !(image.Point{px, py}).In(m.Rect) {
			return 0
		}
		return float64(m.Pix[py*m.Stride+px])
	}
	v := at(x0, y0)*(1-fx)*(1-fy) + at(x0+1, y0)*fx*(1-fy) + at(x0, y0+1)*(1-fx)*fy + at(x0+1, y0+1)*fx*fy
	return uint8(v + .5)
}

const (
	minLines, maxLines = 3, 5
	minDots, maxDots   = 70, 130
)

func drawNoise(img *image.NRGBA, rng *rand.Rand) {
	w, h := float64(img.Rect.Dx()), float64(img.Rect.Dy())
	for i, n := 0, minLines+rng.IntN(maxLines-minLines+1); i < n; i++ {

		x0, x1 := -4.0, w+4
		if rng.IntN(2) == 0 {
			x0, x1 = x1, x0
		}
		y0, y1 := rng.Float64()*h, rng.Float64()*h
		cx, cy := w*(.3+.4*rng.Float64()), h*(.25+.5*rng.Float64())
		width := .9 + rng.Float64()*.9
		ink := uint8(170 + rng.IntN(86))
		steps := int(math.Hypot(x1-x0, y1-y0) * 3)
		for s := 0; s <= steps; s++ {
			t := float64(s) / float64(steps)

			x := (1-t)*(1-t)*x0 + 2*(1-t)*t*cx + t*t*x1
			y := (1-t)*(1-t)*y0 + 2*(1-t)*t*cy + t*t*y1
			dot(img, x, y, width/2, ink)
		}
	}
	for i, n := 0, minDots+rng.IntN(maxDots-minDots+1); i < n; i++ {
		r := .6 + rng.Float64()*1.1
		dot(img, rng.Float64()*w, rng.Float64()*h, r, uint8(150+rng.IntN(106)))
	}
}

func dot(img *image.NRGBA, x, y, r float64, ink uint8) {
	b := img.Rect
	for py := int(math.Floor(y - r - 1)); py <= int(math.Ceil(y+r+1)); py++ {
		for px := int(math.Floor(x - r - 1)); px <= int(math.Ceil(x+r+1)); px++ {
			if !(image.Point{px, py}).In(b) {
				continue
			}

			cover := r + .5 - math.Hypot(float64(px)+.5-x, float64(py)+.5-y)
			if cover <= 0 {
				continue
			}
			a := uint8(math.Min(cover, 1) * float64(ink))
			i := img.PixOffset(px, py) + 3
			img.Pix[i] = max(img.Pix[i], a)
		}
	}
}
