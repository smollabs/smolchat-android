package ui

import (
	"image"
	"image/color"
	"image/png"
	"math"
	"sync"
	"sync/atomic"

	"gioui.org/f32"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"

	"github.com/smolworks/smolchat-android/assets"
)

// ---- brand artwork ---------------------------------------------------------

// The artwork is decoded on a background goroutine (started by NewTheme) so
// it never delays the first frame; until it is ready nothing is drawn.
var (
	artOnce   sync.Once
	artReady  atomic.Bool
	mascotImg paint.ImageOp // smollabs mascot, 720×479
	logoImg   paint.ImageOp // smollabs app logo, 314×314
)

func loadArt(invalidate func()) {
	artOnce.Do(func() {
		go func() {
			mascotImg = loadImage("images/mascot.png")
			logoImg = loadImage("images/logo.png")
			artReady.Store(true)
			if invalidate != nil {
				invalidate()
			}
		}()
	})
}

func loadImage(name string) paint.ImageOp {
	f, err := assets.Images.Open(name)
	if err != nil {
		return paint.ImageOp{}
	}
	defer f.Close()
	img, err := png.Decode(f)
	if err != nil {
		return paint.ImageOp{}
	}
	op := paint.NewImageOp(img)
	op.Filter = paint.FilterLinear
	return op
}

// drawImage paints img scaled to width w (px), keeping its aspect ratio.
func drawImage(gtx C, img paint.ImageOp, w int) D {
	if !artReady.Load() {
		gtx.Execute(op.InvalidateCmd{})
		return D{}
	}
	sz := img.Size()
	if sz.X == 0 {
		return D{}
	}
	s := float32(w) / float32(sz.X)
	h := int(float32(sz.Y) * s)
	defer op.Affine(f32.AffineId().Scale(f32.Point{}, f32.Pt(s, s))).Push(gtx.Ops).Pop()
	img.Add(gtx.Ops)
	paint.PaintOp{}.Add(gtx.Ops)
	return D{Size: image.Pt(w, h)}
}

// Mascot draws the mascot with its warm-core, accent-aura glow behind it.
func Mascot(gtx C, width unit.Dp) D {
	w := gtx.Dp(width)
	h := w * 479 / 720
	if !artReady.Load() {
		gtx.Execute(op.InvalidateCmd{})
		return D{Size: image.Pt(w, h)}
	}
	// Glow: warm flame core inside an accent aura (Mac: Mascot3D glow).
	cx, cy := float32(w)*0.5, float32(h)*0.5
	r := float32(w) * 0.62
	glowAt(gtx, f32.Pt(cx, cy), r, pal.Accent, pal.GlowStrength*0.75)
	glowAt(gtx, f32.Pt(cx, cy), r*0.45, pal.Flame, pal.GlowStrength*0.6)
	return drawImage(gtx, mascotImg, w)
}

// LogoTile draws the rounded app logo at size s.
func LogoTile(gtx C, s unit.Dp) D {
	px := gtx.Dp(s)
	defer clip.UniformRRect(image.Rectangle{Max: image.Pt(px, px)}, px*22/100).Push(gtx.Ops).Pop()
	return drawImage(gtx, logoImg, px)
}

// ---- radial glows -----------------------------------------------------------

type glowKey struct{ c color.NRGBA }

var glowCache = map[glowKey]paint.ImageOp{}

// glowTexture is a 128px radial falloff of c (alpha stops 1 → 0.35 at 45% →
// 0 at the rim, as in the Mac backdrop). Upscaled with linear filtering it
// stays smooth at any radius.
func glowTexture(c color.NRGBA) paint.ImageOp {
	k := glowKey{c}
	if img, ok := glowCache[k]; ok {
		return img
	}
	const n = 128
	img := image.NewNRGBA(image.Rect(0, 0, n, n))
	for y := 0; y < n; y++ {
		for x := 0; x < n; x++ {
			dx, dy := (float64(x)+0.5)/n*2-1, (float64(y)+0.5)/n*2-1
			d := math.Sqrt(dx*dx + dy*dy)
			var a float64
			switch {
			case d < 0.45:
				a = 1 - (1-0.35)*(d/0.45)
			case d < 1:
				t := (d - 0.45) / 0.55
				a = 0.35 * (1 - t) * (1 - t)
			}
			cc := c
			cc.A = uint8(float64(c.A) * a)
			img.SetNRGBA(x, y, cc)
		}
	}
	op := paint.NewImageOp(img)
	op.Filter = paint.FilterLinear
	glowCache[k] = op
	return op
}

// glowAt paints a radial glow of colour c at centre ctr with radius r (px).
func glowAt(gtx C, ctr f32.Point, r float32, c color.NRGBA, strength float32) {
	if strength <= 0 || r <= 0 {
		return
	}
	c.A = uint8(255*min(1, strength)) &^ 3 // quantised: animations reuse textures
	tex := glowTexture(c)
	s := 2 * r / 128
	tr := f32.AffineId().Scale(f32.Point{}, f32.Pt(s, s)).Offset(f32.Pt(ctr.X-r, ctr.Y-r))
	defer op.Affine(tr).Push(gtx.Ops).Pop()
	tex.Add(gtx.Ops)
	paint.PaintOp{}.Add(gtx.Ops)
}

// Backdrop paints the main panel: base colour with the accent glows in the
// upper-left and lower-right corners, as on the Mac.
func Backdrop(gtx C) {
	sz := gtx.Constraints.Max
	paint.FillShape(gtx.Ops, pal.Bg, clip.Rect{Max: sz}.Op())
	defer clip.Rect{Max: sz}.Push(gtx.Ops).Pop()
	m := float32(max(sz.X, sz.Y))
	w, h := float32(sz.X), float32(sz.Y)
	glowAt(gtx, f32.Pt(w*0.10, h*0.02), m*0.62, pal.Accent, pal.GlowStrength)
	glowAt(gtx, f32.Pt(w*0.94, h*0.98), m*0.60, pal.Accent, pal.GlowStrength*0.8)
}

// ---- sparkles -----------------------------------------------------------------

// sparkle fills a four-point star (✦) of radius r centred at c.
func sparkle(gtx C, c f32.Point, r float32, col color.NRGBA) {
	if r <= 0 {
		return
	}
	var p clip.Path
	p.Begin(gtx.Ops)
	p.MoveTo(f32.Pt(c.X, c.Y-r))
	p.QuadTo(c, f32.Pt(c.X+r, c.Y))
	p.QuadTo(c, f32.Pt(c.X, c.Y+r))
	p.QuadTo(c, f32.Pt(c.X-r, c.Y))
	p.QuadTo(c, f32.Pt(c.X, c.Y-r))
	p.Close()
	paint.FillShape(gtx.Ops, col, clip.Outline{Path: p.End()}.Op())
}

// SparkleIcon draws the composer's "✦ + small ✦" glyph in a size×size box.
func SparkleIcon(gtx C, size unit.Dp, col color.NRGBA) D {
	s := float32(gtx.Dp(size))
	sparkle(gtx, f32.Pt(s*0.42, s*0.58), s*0.40, col)
	sparkle(gtx, f32.Pt(s*0.80, s*0.20), s*0.18, col)
	return D{Size: image.Pt(int(s), int(s))}
}
