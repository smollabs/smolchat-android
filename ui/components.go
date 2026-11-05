package ui

import (
	"image"
	"image/color"
	"math"
	"time"

	"gioui.org/f32"
	"gioui.org/font"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/text"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
	"golang.org/x/exp/shiny/materialdesign/icons"
)

type (
	C = layout.Context
	D = layout.Dimensions
)

func mustIcon(data []byte) *widget.Icon {
	ic, err := widget.NewIcon(data)
	if err != nil {
		panic(err)
	}
	return ic
}

var ic = struct {
	Menu, NewChat, Send, Stop, Copy, Refresh, Settings, Library, Search, Close, Back,
	Download, Delete, Check, ChevronDown, Edit, Auto, Memory, Storage, Offline, More,
	Pause, Play, Info, Chat, Lock, Phone, Bolt, Shield, Tune, Done,
	Lightbulb, Compass, Code, Sun, Moon, Monitor, Folder, ChevronRight, Plus *widget.Icon
}{
	Lightbulb:    mustIcon(icons.ActionLightbulbOutline),
	Compass:      mustIcon(icons.ActionExplore),
	Code:         mustIcon(icons.ActionCode),
	Sun:          mustIcon(icons.ImageWBSunny),
	Moon:         mustIcon(icons.ImageBrightness3),
	Monitor:      mustIcon(icons.HardwareComputer),
	Folder:       mustIcon(icons.FileFolderOpen),
	ChevronRight: mustIcon(icons.NavigationChevronRight),
	Plus:         mustIcon(icons.ContentAdd),
	Menu:         mustIcon(icons.NavigationMenu),
	NewChat:      mustIcon(icons.EditorModeEdit),
	Send:         mustIcon(icons.NavigationArrowUpward),
	Stop:         mustIcon(icons.AVStop),
	Copy:         mustIcon(icons.ContentContentCopy),
	Refresh:      mustIcon(icons.NavigationRefresh),
	Settings:     mustIcon(icons.ActionSettings),
	Library:      mustIcon(icons.ActionViewModule),
	Search:       mustIcon(icons.ActionSearch),
	Close:        mustIcon(icons.NavigationClose),
	Back:         mustIcon(icons.NavigationArrowBack),
	Download:     mustIcon(icons.FileFileDownload),
	Delete:       mustIcon(icons.ActionDelete),
	Check:        mustIcon(icons.ActionCheckCircle),
	ChevronDown:  mustIcon(icons.NavigationExpandMore),
	Edit:         mustIcon(icons.ContentCreate),
	Auto:         mustIcon(icons.ActionAutorenew),
	Memory:       mustIcon(icons.HardwareMemory),
	Storage:      mustIcon(icons.DeviceStorage),
	Offline:      mustIcon(icons.FileCloudOff),
	More:         mustIcon(icons.NavigationMoreVert),
	Pause:        mustIcon(icons.AVPause),
	Play:         mustIcon(icons.AVPlayArrow),
	Info:         mustIcon(icons.ActionInfoOutline),
	Chat:         mustIcon(icons.CommunicationChatBubbleOutline),
	Lock:         mustIcon(icons.ActionLock),
	Phone:        mustIcon(icons.HardwarePhoneAndroid),
	Bolt:         mustIcon(icons.ImageFlashOn),
	Shield:       mustIcon(icons.ActionVerifiedUser),
	Tune:         mustIcon(icons.ImageTune),
	Done:         mustIcon(icons.NavigationCheck),
}

// ---- painting -------------------------------------------------------------

func fillRRect(gtx C, sz image.Point, r int, col color.NRGBA) {
	defer clip.UniformRRect(image.Rectangle{Max: sz}, r).Push(gtx.Ops).Pop()
	paint.ColorOp{Color: col}.Add(gtx.Ops)
	paint.PaintOp{}.Add(gtx.Ops)
}

func strokeRRect(gtx C, sz image.Point, r int, width float32, col color.NRGBA) {
	rr := clip.UniformRRect(image.Rectangle{Max: sz}, r)
	paint.FillShape(gtx.Ops, col, clip.Stroke{Path: rr.Path(gtx.Ops), Width: width}.Op())
}

// Surface draws w on a rounded background with an optional hairline border.
type Surface struct {
	Bg     color.NRGBA
	Border color.NRGBA
	Radius unit.Dp
}

func (s Surface) Layout(gtx C, w layout.Widget) D {
	return layout.Background{}.Layout(gtx,
		func(gtx C) D {
			sz := gtx.Constraints.Min
			r := gtx.Dp(s.Radius)
			fillRRect(gtx, sz, r, s.Bg)
			if s.Border.A > 0 {
				strokeRRect(gtx, sz, r, float32(gtx.Dp(1)), s.Border)
			}
			return D{Size: sz}
		}, w)
}

// Lbl is a styled label.
func Lbl(th *Theme, size unit.Sp, col color.NRGBA, w font.Weight, s string) material.LabelStyle {
	l := material.Label(th.Theme, size, s)
	l.Color = col
	l.Font.Weight = w
	return l
}

func Gap(h unit.Dp) layout.Widget {
	return func(gtx C) D { return D{Size: image.Pt(gtx.Dp(h), gtx.Dp(h))} }
}

func Divider(gtx C, col color.NRGBA) D {
	h := max(1, gtx.Dp(unit.Dp(0.5)))
	sz := image.Pt(gtx.Constraints.Max.X, h)
	fillRRect(gtx, sz, 0, col)
	return D{Size: sz}
}

// ---- buttons --------------------------------------------------------------

// pressOverlay paints the touch feedback for a clickable surface.
func pressOverlay(gtx C, clk *widget.Clickable, sz image.Point, r int) {
	if clk.Pressed() {
		fillRRect(gtx, sz, r, pal.Pressed)
	}
}

// IconButton is a 44dp circular touch target with a centred icon.
func IconButton(gtx C, clk *widget.Clickable, icon *widget.Icon, col color.NRGBA) D {
	return IconButtonSized(gtx, clk, icon, col, 22, 44)
}

func IconButtonSized(gtx C, clk *widget.Clickable, icon *widget.Icon, col color.NRGBA, iconDp, boxDp unit.Dp) D {
	return clk.Layout(gtx, func(gtx C) D {
		box := gtx.Dp(boxDp)
		sz := image.Pt(box, box)
		pressOverlay(gtx, clk, sz, box/2)
		is := gtx.Dp(iconDp)
		off := (box - is) / 2
		t := op.Offset(image.Pt(off, off)).Push(gtx.Ops)
		gtx.Constraints = layout.Exact(image.Pt(is, is))
		icon.Layout(gtx, col)
		t.Pop()
		return D{Size: sz}
	})
}

// ButtonKind selects a button style.
type ButtonKind int

const (
	BtnPrimary ButtonKind = iota
	BtnNeutral
	BtnSecondary
	BtnGhost
	BtnDanger
)

// Button is a pill-shaped labelled button.
type Button struct {
	Kind     ButtonKind
	Text     string
	Icon     *widget.Icon
	Disabled bool
	Wide     bool // fill the available width
	Small    bool
}

func (b Button) Layout(gtx C, th *Theme, clk *widget.Clickable) D {
	var bg, fg, border color.NRGBA
	gradient := false
	switch b.Kind {
	case BtnPrimary: // accent gradient, as the Mac's GlowButton
		bg, fg, gradient = th.Pal.Accent, th.Pal.OnAccent, true
	case BtnNeutral:
		bg, fg = th.Pal.Primary, th.Pal.OnPrimary
	case BtnSecondary:
		fg, border = th.Pal.Text, th.Pal.BorderStrong
	case BtnGhost:
		fg = th.Pal.TextMuted
	case BtnDanger:
		bg, fg = withAlpha(th.Pal.Danger, 0x26), th.Pal.Danger
	}
	if b.Disabled {
		bg, fg, gradient = withAlpha(bg, 0x60), withAlpha(fg, 0x80), false
		gtx = gtx.Disabled()
	}
	h := unit.Dp(44)
	textSize := unit.Sp(15)
	padX := unit.Dp(18)
	if b.Small {
		h, textSize, padX = 34, 13.5, 14
	}
	return clk.Layout(gtx, func(gtx C) D {
		gtx.Constraints.Min.Y = gtx.Dp(h)
		if b.Wide {
			gtx.Constraints.Min.X = gtx.Constraints.Max.X
		}
		return layout.Background{}.Layout(gtx,
			func(gtx C) D {
				sz := gtx.Constraints.Min
				r := sz.Y / 2
				switch {
				case gradient:
					func() {
						defer clip.UniformRRect(image.Rectangle{Max: sz}, r).Push(gtx.Ops).Pop()
						paint.LinearGradientOp{
							Stop1: f32.Pt(0, 0), Color1: mix(bg, rgb(0xffffff), 0.12),
							Stop2: f32.Pt(0, float32(sz.Y)), Color2: mix(bg, rgb(0x000000), 0.22),
						}.Add(gtx.Ops)
						paint.PaintOp{}.Add(gtx.Ops)
					}()
				case bg.A > 0:
					fillRRect(gtx, sz, r, bg)
				}
				if border.A > 0 {
					strokeRRect(gtx, sz, r, float32(gtx.Dp(1)), border)
				}
				pressOverlay(gtx, clk, sz, r)
				return D{Size: sz}
			},
			func(gtx C) D {
				gtx.Constraints.Min.Y = gtx.Dp(h)
				return layout.Center.Layout(gtx, func(gtx C) D {
					return layout.Inset{Left: padX, Right: padX}.Layout(gtx, func(gtx C) D {
						return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
							layout.Rigid(func(gtx C) D {
								if b.Icon == nil {
									return D{}
								}
								return layout.Inset{Right: 8}.Layout(gtx, func(gtx C) D {
									s := gtx.Dp(unit.Dp(textSize) + 3)
									gtx.Constraints = layout.Exact(image.Pt(s, s))
									return b.Icon.Layout(gtx, fg)
								})
							}),
							layout.Rigid(func(gtx C) D {
								l := Lbl(th, textSize, fg, font.SemiBold, b.Text)
								l.MaxLines = 1
								return l.Layout(gtx)
							}),
						)
					})
				})
			})
	})
}

// Chip is a small rounded tag.
func Chip(gtx C, th *Theme, s string, fg, bg color.NRGBA) D {
	return Surface{Bg: bg, Radius: 8}.Layout(gtx, func(gtx C) D {
		return layout.Inset{Top: 3, Bottom: 3, Left: 8, Right: 8}.Layout(gtx, func(gtx C) D {
			l := Lbl(th, 12, fg, font.Medium, s)
			l.MaxLines = 1
			return l.Layout(gtx)
		})
	})
}

// Dot draws a small status dot.
func Dot(gtx C, col color.NRGBA, d unit.Dp) D {
	s := gtx.Dp(d)
	fillRRect(gtx, image.Pt(s, s), s/2, col)
	return D{Size: image.Pt(s, s)}
}

// ProgressBar draws a rounded determinate bar.
func ProgressBar(gtx C, frac float32, col, track color.NRGBA) D {
	h := gtx.Dp(5)
	w := gtx.Constraints.Max.X
	fillRRect(gtx, image.Pt(w, h), h/2, track)
	frac = float32(math.Max(0, math.Min(1, float64(frac))))
	if fw := int(float32(w) * frac); fw > 0 {
		fillRRect(gtx, image.Pt(max(fw, h), h), h/2, col)
	}
	return D{Size: image.Pt(w, h)}
}

// Indeterminate draws a sliding bar for work of unknown length.
func Indeterminate(gtx C, col, track color.NRGBA) D {
	h := gtx.Dp(5)
	w := gtx.Constraints.Max.X
	fillRRect(gtx, image.Pt(w, h), h/2, track)
	t := float64(gtx.Now.UnixMilli()%1400) / 1400
	seg := w / 3
	x := int(float64(w+seg)*easeInOut(t)) - seg
	func() {
		defer clip.UniformRRect(image.Rectangle{Max: image.Pt(w, h)}, h/2).Push(gtx.Ops).Pop()
		defer op.Offset(image.Pt(x, 0)).Push(gtx.Ops).Pop()
		fillRRect(gtx, image.Pt(seg, h), h/2, col)
	}()
	gtx.Execute(op.InvalidateCmd{})
	return D{Size: image.Pt(w, h)}
}

// TypingDots is the three-dot "thinking" animation.
func TypingDots(gtx C, col color.NRGBA) D {
	d := gtx.Dp(6)
	gap := gtx.Dp(5)
	t := float64(gtx.Now.UnixMilli()%1200) / 1200
	for i := 0; i < 3; i++ {
		ph := math.Mod(t-float64(i)*0.15+1, 1)
		a := 0.3 + 0.7*math.Max(0, math.Sin(ph*math.Pi))
		off := op.Offset(image.Pt(i*(d+gap), 0)).Push(gtx.Ops)
		fillRRect(gtx, image.Pt(d, d), d/2, withAlpha(col, uint8(255*a)))
		off.Pop()
	}
	gtx.Execute(op.InvalidateCmd{})
	return D{Size: image.Pt(3*d+2*gap, d)}
}

// ---- animation ------------------------------------------------------------

// Anim eases a value toward Target with critically damped exponential
// smoothing; it requests redraws only while moving.
type Anim struct {
	V, Target float32
	Rate      float32 // per second; higher is faster (default 14)
	last      time.Time
}

func (a *Anim) Set(v float32) { a.V, a.Target = v, v }

func (a *Anim) Step(gtx C) float32 {
	if a.V == a.Target {
		a.last = time.Time{}
		return a.V
	}
	if a.last.IsZero() {
		a.last = gtx.Now
	}
	dt := float32(gtx.Now.Sub(a.last).Seconds())
	a.last = gtx.Now
	rate := a.Rate
	if rate == 0 {
		rate = 14
	}
	a.V += (a.Target - a.V) * float32(1-math.Exp(float64(-dt*rate)))
	if math.Abs(float64(a.Target-a.V)) < 0.002 {
		a.V = a.Target
	}
	gtx.Execute(op.InvalidateCmd{})
	return a.V
}

func (a *Anim) Moving() bool { return a.V != a.Target }

func easeInOut(t float64) float64 { return 0.5 - 0.5*math.Cos(math.Pi*t) }
func easeOut(t float64) float64   { return 1 - math.Pow(1-t, 3) }

// ---- text input -------------------------------------------------------------

// Field draws a rounded single- or multi-line text field.
func Field(gtx C, th *Theme, ed *widget.Editor, hint string, icon *widget.Icon) D {
	return Surface{Bg: th.Pal.Glass, Border: th.Pal.BorderStrong, Radius: 12}.Layout(gtx, func(gtx C) D {
		return layout.Inset{Left: 12, Right: 12, Top: 10, Bottom: 10}.Layout(gtx, func(gtx C) D {
			return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
				layout.Rigid(func(gtx C) D {
					if icon == nil {
						return D{}
					}
					return layout.Inset{Right: 8}.Layout(gtx, func(gtx C) D {
						s := gtx.Dp(18)
						gtx.Constraints = layout.Exact(image.Pt(s, s))
						return icon.Layout(gtx, th.Pal.TextFaint)
					})
				}),
				layout.Flexed(1, func(gtx C) D {
					e := material.Editor(th.Theme, ed, hint)
					e.Color, e.HintColor = th.Pal.Text, th.Pal.TextFaint
					e.SelectionColor = withAlpha(th.Pal.Accent, 0x60)
					e.TextSize = 15
					return e.Layout(gtx)
				}),
			)
		})
	})
}

// Section renders an uppercase section caption.
func Section(gtx C, th *Theme, s string) D {
	return layout.Inset{Top: 22, Bottom: 10}.Layout(gtx, func(gtx C) D {
		return Lbl(th, 12, th.Pal.TextFaint, font.SemiBold, s).Layout(gtx)
	})
}

// Toggle is an iOS-style switch; it returns true when flipped.
type Toggle struct {
	clk  widget.Clickable
	anim Anim
	init bool
}

func (t *Toggle) Layout(gtx C, th *Theme, on bool) (D, bool) {
	changed := t.clk.Clicked(gtx)
	if changed {
		on = !on
	}
	target := float32(0)
	if on {
		target = 1
	}
	if !t.init {
		t.anim.Set(target)
		t.init = true
	}
	t.anim.Target = target
	t.anim.Rate = 18
	p := t.anim.Step(gtx)
	d := t.clk.Layout(gtx, func(gtx C) D {
		w, h := gtx.Dp(46), gtx.Dp(28)
		track := mix(th.Pal.Elevated, th.Pal.Accent, p)
		if !th.Pal.Dark && p == 0 {
			track = argb(0x26000000)
		}
		fillRRect(gtx, image.Pt(w, h), h/2, track)
		k := h - gtx.Dp(6)
		x := gtx.Dp(3) + int(float32(w-k-gtx.Dp(6))*p)
		off := op.Offset(image.Pt(x, gtx.Dp(3))).Push(gtx.Ops)
		fillRRect(gtx, image.Pt(k, k), k/2, rgb(0xffffff))
		off.Pop()
		return D{Size: image.Pt(w, h)}
	})
	return d, changed
}

// centered aligns a single line label in its constraints.
func centered(l material.LabelStyle) material.LabelStyle {
	l.Alignment = text.Middle
	return l
}

// Wrap lays items out left to right, wrapping onto new lines.
func Wrap(gtx C, gapX, gapY unit.Dp, items []layout.Widget) D {
	maxW := gtx.Constraints.Max.X
	gx, gy := gtx.Dp(gapX), gtx.Dp(gapY)
	cgtx := gtx
	cgtx.Constraints.Min = image.Point{}
	x, y, rowH := 0, 0, 0
	for _, w := range items {
		m := op.Record(gtx.Ops)
		d := w(cgtx)
		call := m.Stop()
		if x > 0 && x+d.Size.X > maxW {
			x, y, rowH = 0, y+rowH+gy, 0
		}
		t := op.Offset(image.Pt(x, y)).Push(gtx.Ops)
		call.Add(gtx.Ops)
		t.Pop()
		x += d.Size.X + gx
		rowH = max(rowH, d.Size.Y)
	}
	return D{Size: image.Pt(maxW, y+rowH)}
}
