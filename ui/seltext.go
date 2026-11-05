package ui

import (
	"image"
	"image/color"
	"time"
	"unicode"

	"gioui.org/f32"
	"gioui.org/font"
	"gioui.org/io/event"
	"gioui.org/io/pointer"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/text"
	"gioui.org/unit"
	"golang.org/x/image/math/fixed"

	"github.com/smolworks/smolchat-android/app"
)

// SelText is read-only text with touch selection, which Gio's own widgets
// only offer for mouse drags: long-press selects a word, dragging extends
// it, and the two handles adjust either end.
type SelText struct {
	src     string
	runes   []rune
	glyphs  []text.Glyph
	gRune   []int // rune index where each glyph's cluster starts
	width   int
	size    unit.Sp
	height  int
	start   int // selection [start, end) in runes
	end     int
	pressed bool
	pressAt time.Time
	pressPt f32.Point
	pid     pointer.ID
	extend  bool // dragging after a long press
	anchor  [2]int
	handles [2]selHandle
}

type selHandle struct {
	dragging bool
}

func (s *SelText) SetText(t string) {
	if t != s.src {
		s.src, s.runes, s.glyphs = t, []rune(t), nil
		s.start, s.end = 0, 0
	}
}

func (s *SelText) HasSelection() bool { return s.end > s.start }
func (s *SelText) SelectAll()         { s.start, s.end = 0, len(s.runes) }
func (s *SelText) ClearSelection()    { s.start, s.end = 0, 0 }
func (s *SelText) Selected() string {
	if !s.HasSelection() {
		return ""
	}
	return string(s.runes[s.start:s.end])
}

func (s *SelText) shape(gtx C, th *Theme, f font.Font) {
	w := gtx.Constraints.Max.X
	if s.glyphs != nil && w == s.width {
		return
	}
	s.width = w
	sh := th.Shaper
	sh.LayoutString(text.Parameters{
		Font: f, PxPerEm: fixed.I(gtx.Sp(s.size)), MaxWidth: w, LineHeightScale: 1.45,
		WrapPolicy: text.WrapHeuristically, Locale: gtx.Locale,
	}, s.src)
	s.glyphs, s.gRune = s.glyphs[:0], s.gRune[:0]
	r := 0
	s.height = 0
	for g, ok := sh.NextGlyph(); ok; g, ok = sh.NextGlyph() {
		s.glyphs = append(s.glyphs, g)
		s.gRune = append(s.gRune, r)
		r += int(g.Runes)
		s.height = max(s.height, int(g.Y)+g.Descent.Ceil())
	}
	// Glyphs of one cluster share its start.
	for i := len(s.glyphs) - 2; i >= 0; i-- {
		if s.glyphs[i].Flags&text.FlagClusterBreak == 0 {
			s.gRune[i] = s.gRune[i+1]
		}
	}
}

// runeAt maps a point to the nearest rune index.
func (s *SelText) runeAt(p f32.Point) int {
	if len(s.glyphs) == 0 {
		return 0
	}
	// Closest line by baseline.
	best, bestD := 0, float32(1e9)
	for i, g := range s.glyphs {
		top := float32(int(g.Y) - g.Ascent.Ceil())
		bot := float32(int(g.Y) + g.Descent.Ceil())
		d := float32(0)
		switch {
		case p.Y < top:
			d = top - p.Y
		case p.Y > bot:
			d = p.Y - bot
		}
		x0 := fixedF(g.X)
		x1 := x0 + fixedF(g.Advance)
		switch {
		case p.X < x0:
			d += (x0 - p.X) * 0.01
		case p.X > x1:
			d += (p.X - x1) * 0.01
		}
		if d < bestD {
			best, bestD = i, d
		}
	}
	g := s.glyphs[best]
	if p.X > fixedF(g.X)+fixedF(g.Advance)/2 {
		return min(len(s.runes), s.gRune[best]+max(1, int(g.Runes)))
	}
	return s.gRune[best]
}

func fixedF(v fixed.Int26_6) float32 { return float32(v) / 64 }

func isWordRune(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' || r == '\''
}

func (s *SelText) wordAt(i int) (int, int) {
	if len(s.runes) == 0 {
		return 0, 0
	}
	i = min(i, len(s.runes)-1)
	if !isWordRune(s.runes[i]) {
		return i, i + 1
	}
	a, b := i, i
	for a > 0 && isWordRune(s.runes[a-1]) {
		a--
	}
	for b < len(s.runes) && isWordRune(s.runes[b]) {
		b++
	}
	return a, b
}

// caretPos is where a selection handle attaches: the bottom-left of the
// first selected glyph (start) or the bottom-right of the last one (end).
func (s *SelText) caretPos(i int, endSide bool) f32.Point {
	bottom := func(g text.Glyph) float32 { return float32(int(g.Y) + g.Descent.Ceil()) }
	if !endSide {
		for k, g := range s.glyphs {
			if s.gRune[k] >= i {
				return f32.Pt(fixedF(g.X), bottom(g))
			}
		}
	} else {
		for k := len(s.glyphs) - 1; k >= 0; k-- {
			if g := s.glyphs[k]; s.gRune[k] < i {
				return f32.Pt(fixedF(g.X)+fixedF(g.Advance), bottom(g))
			}
		}
	}
	return f32.Point{}
}

// Layout draws the text with selection and handles.
func (s *SelText) Layout(gtx C, th *Theme, size unit.Sp, col, selCol color.NRGBA) D {
	if s.size != size {
		s.size, s.glyphs = size, nil
	}
	f := font.Font{Typeface: uiFace}
	s.shape(gtx, th, f)
	sz := image.Pt(gtx.Constraints.Max.X, s.height+gtx.Dp(28)) // room for handles
	s.events(gtx)

	// Selection highlight.
	if s.HasSelection() {
		for k, g := range s.glyphs {
			if s.gRune[k] < s.start || s.gRune[k] >= s.end {
				continue
			}
			r := image.Rect(g.X.Floor(), int(g.Y)-g.Ascent.Ceil(), (g.X + g.Advance).Ceil(), int(g.Y)+g.Descent.Ceil())
			paint.FillShape(gtx.Ops, selCol, clip.Rect(r).Op())
		}
	}
	// Text, one line at a time (as widget.Label does).
	var line []text.Glyph
	flush := func() {
		if len(line) == 0 {
			return
		}
		off := f32.Pt(fixedF(line[0].X), float32(line[0].Y))
		t := op.Affine(f32.AffineId().Offset(off)).Push(gtx.Ops)
		path := th.Shaper.Shape(line)
		o := clip.Outline{Path: path}.Op().Push(gtx.Ops)
		paint.ColorOp{Color: col}.Add(gtx.Ops)
		paint.PaintOp{}.Add(gtx.Ops)
		o.Pop()
		if call := th.Shaper.Bitmaps(line); call != (op.CallOp{}) {
			call.Add(gtx.Ops)
		}
		t.Pop()
		line = line[:0]
	}
	for _, g := range s.glyphs {
		line = append(line, g)
		if g.Flags&text.FlagLineBreak != 0 {
			flush()
		}
	}
	flush()

	// Press area for long-press selection.
	func() {
		defer clip.Rect{Max: sz}.Push(gtx.Ops).Pop()
		event.Op(gtx.Ops, s)
	}()
	// Handles.
	if s.HasSelection() {
		for i := range s.handles {
			idx, endSide := s.start, false
			if i == 1 {
				idx, endSide = s.end, true
			}
			p := s.caretPos(idx, endSide)
			r := float32(gtx.Dp(9))
			c := f32.Pt(p.X, p.Y+r)
			paint.FillShape(gtx.Ops, pal.Accent, clip.Ellipse{Min: image.Pt(int(c.X-r), int(c.Y-r)), Max: image.Pt(int(c.X+r), int(c.Y+r))}.Op(gtx.Ops))
			paint.FillShape(gtx.Ops, pal.Accent, clip.Rect{Min: image.Pt(int(p.X)-gtx.Dp(1), int(p.Y)-gtx.Dp(18)), Max: image.Pt(int(p.X)+gtx.Dp(1), int(p.Y)+2)}.Op())
			hit := gtx.Dp(22)
			func() {
				defer clip.Rect{Min: image.Pt(int(c.X)-hit, int(c.Y)-hit), Max: image.Pt(int(c.X)+hit, int(c.Y)+hit)}.Push(gtx.Ops).Pop()
				event.Op(gtx.Ops, &s.handles[i])
			}()
		}
	}
	return D{Size: sz}
}

func (s *SelText) events(gtx C) {
	slop := float32(gtx.Dp(8))
	lineH := float32(gtx.Sp(s.size))
	for {
		ev, ok := gtx.Event(pointer.Filter{Target: s, Kinds: pointer.Press | pointer.Drag | pointer.Release | pointer.Cancel})
		if !ok {
			break
		}
		e, ok := ev.(pointer.Event)
		if !ok {
			continue
		}
		switch e.Kind {
		case pointer.Press:
			s.pressed, s.pressAt, s.pressPt, s.pid, s.extend = true, gtx.Now, e.Position, e.PointerID, false
		case pointer.Drag:
			if s.extend {
				r := s.runeAt(e.Position)
				if r < s.anchor[0] {
					s.start, s.end = s.wordStart(r), s.anchor[1]
				} else {
					s.start, s.end = s.anchor[0], max(s.anchor[1], r)
				}
				gtx.Execute(op.InvalidateCmd{})
			} else if d := e.Position.Sub(s.pressPt); d.X*d.X+d.Y*d.Y > slop*slop {
				s.pressed = false // a scroll, not a press
			}
		case pointer.Release, pointer.Cancel:
			if s.pressed && !s.extend && e.Kind == pointer.Release && gtx.Now.Sub(s.pressAt) < longPressDelay {
				s.ClearSelection() // a tap clears the selection
			}
			s.pressed, s.extend = false, false
		}
	}
	if s.pressed && !s.extend {
		if gtx.Now.Sub(s.pressAt) >= longPressDelay {
			a, b := s.wordAt(s.runeAt(s.pressPt))
			s.start, s.end, s.anchor = a, b, [2]int{a, b}
			s.extend = true
			gtx.Execute(pointer.GrabCmd{Tag: s, ID: s.pid})
			app.Haptic()
		} else {
			gtx.Execute(op.InvalidateCmd{At: s.pressAt.Add(longPressDelay)})
		}
	}
	for i := range s.handles {
		h := &s.handles[i]
		for {
			ev, ok := gtx.Event(pointer.Filter{Target: h, Kinds: pointer.Press | pointer.Drag | pointer.Release | pointer.Cancel})
			if !ok {
				break
			}
			e, ok := ev.(pointer.Event)
			if !ok {
				continue
			}
			switch e.Kind {
			case pointer.Press:
				h.dragging = true
				gtx.Execute(pointer.GrabCmd{Tag: h, ID: e.PointerID})
			case pointer.Drag:
				if !h.dragging {
					continue
				}
				// The finger sits below the text; aim at the line above it.
				r := s.runeAt(e.Position.Sub(f32.Pt(0, lineH)))
				if i == 0 {
					s.start = min(r, s.end-1)
				} else {
					s.end = max(r, s.start+1)
				}
				gtx.Execute(op.InvalidateCmd{})
			case pointer.Release, pointer.Cancel:
				h.dragging = false
			}
		}
	}
}

func (s *SelText) wordStart(i int) int {
	a, _ := s.wordAt(i)
	return a
}
