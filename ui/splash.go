package ui

import (
	"image"
	"math"
	"time"

	"gioui.org/f32"
	"gioui.org/font"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
)

// Logo draws the SmolChat mark: a squircle with a chat bubble holding three
// dots. With animate set, the dots pulse (used as the loading indicator).
func Logo(gtx C, size unit.Dp, animate bool) D {
	s := gtx.Dp(size)
	fs := float32(s)
	sz := image.Pt(s, s)

	// Squircle background with a subtle vertical gradient.
	func() {
		defer clip.UniformRRect(image.Rectangle{Max: sz}, int(fs*0.3)).Push(gtx.Ops).Pop()
		paint.LinearGradientOp{
			Stop1: f32.Pt(0, 0), Color1: rgb(0x9d97ff),
			Stop2: f32.Pt(fs, fs), Color2: rgb(0x6a5cf5),
		}.Add(gtx.Ops)
		paint.PaintOp{}.Add(gtx.Ops)
	}()

	// Bubble body.
	white := rgb(0xffffff)
	body := image.Rect(int(fs*0.21), int(fs*0.25), int(fs*0.79), int(fs*0.66))
	paint.FillShape(gtx.Ops, white, clip.UniformRRect(body, int(fs*0.15)).Op(gtx.Ops))
	// Tail.
	var p clip.Path
	p.Begin(gtx.Ops)
	p.MoveTo(f32.Pt(fs*0.30, fs*0.60))
	p.LineTo(f32.Pt(fs*0.27, fs*0.78))
	p.LineTo(f32.Pt(fs*0.46, fs*0.64))
	p.Close()
	paint.FillShape(gtx.Ops, white, clip.Outline{Path: p.End()}.Op())

	// Dots.
	d := fs * 0.085
	cy := float32(body.Min.Y+body.Max.Y) / 2
	t := float64(gtx.Now.UnixMilli()%1200) / 1200
	for i := 0; i < 3; i++ {
		cx := fs*0.5 + float32(i-1)*d*1.9
		a := 1.0
		if animate {
			ph := math.Mod(t-float64(i)*0.16+1, 1)
			a = 0.35 + 0.65*math.Max(0, math.Sin(ph*math.Pi))
		}
		r := image.Rect(int(cx-d/2), int(cy-d/2), int(cx+d/2), int(cy+d/2))
		paint.FillShape(gtx.Ops, withAlpha(rgb(0x6a5cf5), uint8(255*a)), clip.Ellipse(r).Op(gtx.Ops))
	}
	if animate {
		gtx.Execute(op.InvalidateCmd{})
	}
	return D{Size: sz}
}

// Splash is shown only while the app core initialises.
type Splash struct {
	start   time.Time
	doneAt  time.Time
	Visible bool
}

const splashFade = 280 * time.Millisecond

// Finish starts the fade-out.
func (s *Splash) Finish(now time.Time) {
	if s.doneAt.IsZero() {
		s.doneAt = now
	}
}

// Layout draws the splash over the app; it reports false once fully faded.
func (s *Splash) Layout(gtx C, th *Theme) bool {
	if s.start.IsZero() {
		s.start = gtx.Now
	}
	alpha := float32(1)
	if !s.doneAt.IsZero() {
		p := float64(gtx.Now.Sub(s.doneAt)) / float64(splashFade)
		if p >= 1 {
			return false
		}
		alpha = float32(1 - easeOut(p))
	}
	in := math.Min(1, float64(gtx.Now.Sub(s.start))/float64(450*time.Millisecond))
	scale := float32(0.88 + 0.12*easeOut(in))

	defer paint.PushOpacity(gtx.Ops, alpha).Pop()
	paint.Fill(gtx.Ops, th.Pal.Bg)
	layout.Center.Layout(gtx, func(gtx C) D {
		return layout.Flex{Axis: layout.Vertical, Alignment: layout.Middle}.Layout(gtx,
			layout.Rigid(func(gtx C) D {
				s := gtx.Dp(88)
				c := f32.Pt(float32(s)/2, float32(s)/2)
				defer op.Affine(f32.AffineId().Scale(c, f32.Pt(scale, scale))).Push(gtx.Ops).Pop()
				defer paint.PushOpacity(gtx.Ops, float32(in)).Pop()
				return Logo(gtx, 88, true)
			}),
			layout.Rigid(Gap(22)),
			layout.Rigid(Lbl(th, 28, th.Pal.Text, font.Bold, "SmolChat").Layout),
			layout.Rigid(Gap(6)),
			layout.Rigid(Lbl(th, 13, th.Pal.TextMuted, font.Medium, "by SmolWorks").Layout),
		)
	})
	layout.S.Layout(gtx, func(gtx C) D {
		return layout.Inset{Bottom: 48}.Layout(gtx,
			centered(Lbl(th, 12, th.Pal.TextFaint, font.Medium, "Private · Offline · On-device")).Layout)
	})
	gtx.Execute(op.InvalidateCmd{})
	return true
}
