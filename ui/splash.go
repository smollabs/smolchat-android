package ui

import (
	"image"
	"image/color"
	"math"
	"time"

	"gioui.org/f32"
	"gioui.org/font"
	"gioui.org/io/event"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/text"
	"gioui.org/unit"
)

// Splash is the opening sequence, ported from SmolChat for Mac: light
// blooms, the mascot springs into place with ripples and sparkles, the name
// writes itself in, then everything dissolves into the app. Tap to skip.
type Splash struct {
	start   time.Time
	outro   time.Time // zero until dismissal starts
	ready   bool
	skipped bool
}

const (
	splashMin   = 2200 * time.Millisecond // long enough to enjoy, short enough not to annoy
	splashOutro = 560 * time.Millisecond
)

// Finish marks initialisation as done; the outro starts once the intro has
// had its minimum time (or the user tapped).
func (s *Splash) Finish() { s.ready = true }

// Fading reports whether the outro has begun (the app is drawn underneath).
func (s *Splash) Fading() bool { return !s.outro.IsZero() }

// ---- easing ---------------------------------------------------------------

func clamp01(t float64) float64  { return math.Max(0, math.Min(1, t)) }
func outCubic(t float64) float64 { t = clamp01(t); return 1 - math.Pow(1-t, 3) }
func inCubic(t float64) float64  { t = clamp01(t); return t * t * t }
func outQuad(t float64) float64  { t = clamp01(t); return 1 - (1-t)*(1-t) }
func inQuad(t float64) float64   { t = clamp01(t); return t * t }
func inOutQuad(t float64) float64 {
	t = clamp01(t)
	if t < 0.5 {
		return 2 * t * t
	}
	return 1 - math.Pow(-2*t+2, 2)/2
}
func outBack(t, s float64) float64 {
	t = clamp01(t)
	c3 := s + 1
	return 1 + c3*math.Pow(t-1, 3) + s*math.Pow(t-1, 2)
}

// phase maps elapsed ms into 0..1 for an animation starting at from and
// lasting dur ms.
func phase(ms, from, dur float64) float64 { return clamp01((ms - from) / dur) }

// Layout draws the splash; it reports false once it has fully faded.
func (s *Splash) Layout(gtx C, th *Theme, status string) bool {
	if s.start.IsZero() {
		s.start = gtx.Now
	}
	// Tap anywhere to skip the minimum time.
	for {
		ev, ok := gtx.Event(pointer.Filter{Target: s, Kinds: pointer.Press})
		if !ok {
			break
		}
		if e, ok := ev.(pointer.Event); ok && e.Kind == pointer.Press {
			s.skipped = true
		}
	}
	if s.outro.IsZero() && s.ready && (s.skipped || gtx.Now.Sub(s.start) >= splashMin) {
		s.outro = gtx.Now
	}
	ms := float64(gtx.Now.Sub(s.start).Milliseconds())
	out := 0.0
	if !s.outro.IsZero() {
		out = float64(gtx.Now.Sub(s.outro)) / float64(splashOutro)
		if out >= 1 {
			return false
		}
	}

	full := gtx.Constraints.Max
	glow := pal.Accent
	splashAlpha := float32(1 - inOutQuad(out))
	defer paint.PushOpacity(gtx.Ops, splashAlpha).Pop()
	paint.FillShape(gtx.Ops, rgb(0x04030b), clip.Rect{Max: full}.Op())
	func() {
		defer clip.Rect{Max: full}.Push(gtx.Ops).Pop()
		event.Op(gtx.Ops, s)
	}()

	// Light bloom.
	bloom := 0.35 + 0.65*outCubic(ms/1600)
	if out > 0 {
		bloom = 1 + 0.6*inCubic(out)
	}
	lights := float32(outCubic(ms / 900))
	m := float32(max(full.X, full.Y))
	W, H := float32(full.X), float32(full.Y)
	glowAt(gtx, f32.Pt(W*0.5, H*0.42), m*0.55*float32(bloom), glow, 0.30*lights)
	glowAt(gtx, f32.Pt(W*0.85, H*0.05), m*0.6*float32(bloom), glow, 0.22*lights)

	// Stage: 380×420 dp, scaled down to fit narrow phones.
	stageW, stageH := float32(gtx.Dp(380)), float32(gtx.Dp(420))
	sc := min(float32(1), (W-float32(gtx.Dp(16)))/stageW)
	ox := (W - stageW*sc) / 2
	oy := (H-stageH*sc)/2 - float32(gtx.Dp(20))*sc
	defer op.Affine(f32.AffineId().Scale(f32.Point{}, f32.Pt(sc, sc)).Offset(f32.Pt(ox, oy))).Push(gtx.Ops).Pop()
	stageAlpha := float32(1 - inQuad(out*560/380))
	defer paint.PushOpacity(gtx.Ops, stageAlpha).Pop()

	dp := func(v float32) float32 { return float32(gtx.Dp(unit.Dp(v))) }
	center := f32.Pt(stageW/2, dp(100))

	// Ripples radiating from the logo (cycle 2990 ms, staggered 520 ms).
	for i := 0; i < 3; i++ {
		start := 350 + float64(i)*520
		if ms < start {
			continue
		}
		p := math.Mod(ms-start, 2990) / 1600
		if p > 1 {
			continue
		}
		scale := float32(0.7 + 1.2*outCubic(p))
		alpha := 0.5 * (1 - outQuad(p))
		rw, rh := dp(120)*scale, dp(75)*scale
		r := image.Rect(int(center.X-rw), int(center.Y-rh), int(center.X+rw), int(center.Y+rh))
		paint.FillShape(gtx.Ops, withAlpha(glow, uint8(255*alpha)),
			clip.Stroke{Path: clip.UniformRRect(r, int(rh)).Path(gtx.Ops), Width: dp(1.5)}.Op())
	}

	// Twinkling sparkles.
	for i := 0; i < 14; i++ {
		fi := float64(i)
		angle := fi*2.4 + 0.6
		dist := 165 + float64((i*37)%60)
		pause1 := 500 + float64((i*173)%900)
		pause2 := 300 + float64((i*211)%700)
		cycle := pause1 + 420 + 700 + pause2
		t := math.Mod(ms, cycle) - pause1
		var a, sz, drift float64
		switch {
		case t < 0:
			continue
		case t < 420:
			sz, a = outBack(t/420, 1.70158), 0.95*t/420
			drift = 10 * outQuad(t/1300)
		case t < 1120:
			k := (t - 420) / 700
			sz, a = 1-inQuad(k), 0.95*(1-k)
			drift = 10 * outQuad(t/1300)
		default:
			continue
		}
		col := rgb(0xffffff)
		switch i % 3 {
		case 1:
			col = pal.FlameLight
		case 2:
			col = mix(glow, rgb(0xffffff), 0.35)
		}
		x := center.X + float32(math.Cos(angle)*dist)*dp(1)
		y := center.Y + float32(math.Sin(angle)*dist*0.7)*dp(1) - float32(drift)*dp(1)
		r := dp(float32(8+(i*5)%10)) * 0.5 * float32(sz)
		sparkle(gtx, f32.Pt(x, y), r, withAlpha(col, uint8(255*clamp01(a))))
	}

	// Mascot: springs from edge-on to facing the viewer.
	func() {
		p := ms - 150
		if p < 0 {
			return
		}
		scale := float32(0.4 + 0.6*outBack(p/820, 1.4))
		if out > 0 {
			scale = 1 + 0.35*float32(inCubic(out*560/520))
		}
		tilt := 80 * (1 - outCubic(p/820)) // degrees around the vertical axis
		sx := scale * float32(math.Cos(tilt*math.Pi/180))
		alpha := float32(clamp01(p / 380))
		w := dp(300)
		h := w * 479 / 720
		tr := f32.AffineId().Offset(f32.Pt(-w/2, -h/2)).Scale(f32.Point{}, f32.Pt(sx, scale)).Offset(center)
		defer op.Affine(tr).Push(gtx.Ops).Pop()
		defer paint.PushOpacity(gtx.Ops, alpha).Pop()
		Mascot(gtx, 300)
	}()

	// "SmolChat", written in letter by letter.
	func() {
		letters := []rune("SmolChat")
		var calls []op.CallOp
		var widths []int
		total := 0
		for range letters {
			calls = append(calls, op.CallOp{})
			widths = append(widths, 0)
		}
		lgtx := gtx
		lgtx.Constraints = layout.Constraints{Max: image.Pt(1<<20, 1<<20)}
		for i, r := range letters {
			mc := op.Record(gtx.Ops)
			l := Lbl(th, 38, rgb(0xf7f3ff), font.SemiBold, string(r))
			d := l.Layout(lgtx)
			calls[i] = mc.Stop()
			widths[i] = d.Size.X
			total += d.Size.X
		}
		x := (stageW - float32(total)) / 2
		for i := range letters {
			p := ms - (820 + float64(i)*55)
			if p > 0 {
				a := float32(outCubic(p / 380))
				lift := 18 * (1 - outBack(p/520, 1.6))
				t := op.Affine(f32.AffineId().Offset(f32.Pt(x, dp(236)+float32(lift)*dp(1)))).Push(gtx.Ops)
				o := paint.PushOpacity(gtx.Ops, a)
				calls[i].Add(gtx.Ops)
				o.Pop()
				t.Pop()
			}
			x += float32(widths[i])
		}
	}()

	// Tagline.
	textAt(gtx, th, "by SmolWorks  ·  private AI that lives on your phone", 13, rgb(0x8f8c9c), font.Normal,
		stageW, dp(292), float32(outCubic((ms-1250)/450)))

	// Shimmer line + status.
	prog := float32(clamp01((ms - 1500) / 350))
	if prog > 0 {
		lw, lh := dp(160), dp(2)
		x0 := (stageW - lw) / 2
		y0 := dp(338)
		func() {
			defer op.Offset(image.Pt(int(x0), int(y0))).Push(gtx.Ops).Pop()
			defer paint.PushOpacity(gtx.Ops, prog).Pop()
			defer clip.Rect{Max: image.Pt(int(lw), int(lh))}.Push(gtx.Ops).Pop()
			paint.Fill(gtx.Ops, argb(0x1fffffff))
			t := (1 - math.Cos(math.Pi*math.Mod(ms, 1200)/1200)) / 2 // InOutSine
			sx := float32(-60+220*t) * dp(1)
			sw := dp(60)
			paint.LinearGradientOp{Stop1: f32.Pt(sx, 0), Color1: withAlpha(glow, 0), Stop2: f32.Pt(sx+sw/2, 0), Color2: mix(glow, rgb(0xffffff), 0.4)}.Add(gtx.Ops)
			paint.PaintOp{}.Add(gtx.Ops)
		}()
		textAt(gtx, th, status, 11, rgb(0x5f5c6b), font.Normal, stageW, dp(352), prog)
	}

	gtx.Execute(op.InvalidateCmd{})
	return true
}

// textAt draws a centred single line within width w at y with opacity a.
func textAt(gtx C, th *Theme, s string, size unit.Sp, col color.NRGBA, weight font.Weight, w, y, a float32) {
	if a <= 0 || s == "" {
		return
	}
	defer op.Offset(image.Pt(0, int(y))).Push(gtx.Ops).Pop()
	defer paint.PushOpacity(gtx.Ops, min(1, a)).Pop()
	lgtx := gtx
	lgtx.Constraints = layout.Constraints{Min: image.Pt(int(w), 0), Max: image.Pt(int(w), gtx.Dp(40))}
	l := Lbl(th, size, col, weight, s)
	l.Alignment = text.Middle
	l.MaxLines = 1
	l.Layout(lgtx)
}
