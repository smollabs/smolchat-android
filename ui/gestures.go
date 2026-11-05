package ui

import (
	"image"
	"time"

	"gioui.org/f32"
	"gioui.org/io/event"
	"gioui.org/io/pointer"
	"gioui.org/op"
	"gioui.org/op/clip"

	"github.com/smolworks/smolchat-android/app"
)

const longPressDelay = 450 * time.Millisecond

// longPress detects a press held still for longPressDelay. It listens
// alongside other handlers (PassOp); when it fires it grabs the pointer so the
// widget underneath does not also register a click.
type longPress struct {
	pressed bool
	id      pointer.ID
	start   f32.Point
	at      time.Time
}

// Update reports whether a long press fired in this frame.
func (l *longPress) Update(gtx C) bool {
	slop := float32(gtx.Dp(8))
	for {
		ev, ok := gtx.Event(pointer.Filter{Target: l, Kinds: pointer.Press | pointer.Drag | pointer.Release | pointer.Cancel})
		if !ok {
			break
		}
		e, ok := ev.(pointer.Event)
		if !ok {
			continue
		}
		switch e.Kind {
		case pointer.Press:
			if e.Source == pointer.Mouse && e.Buttons != pointer.ButtonPrimary {
				continue
			}
			l.pressed, l.id, l.start, l.at = true, e.PointerID, e.Position, gtx.Now
		case pointer.Drag:
			if d := e.Position.Sub(l.start); d.X*d.X+d.Y*d.Y > slop*slop {
				l.pressed = false
			}
		case pointer.Release, pointer.Cancel:
			l.pressed = false
		}
	}
	if !l.pressed {
		return false
	}
	if gtx.Now.Sub(l.at) >= longPressDelay {
		l.pressed = false
		gtx.Execute(pointer.GrabCmd{Tag: l, ID: l.id})
		app.Haptic()
		return true
	}
	gtx.Execute(op.InvalidateCmd{At: l.at.Add(longPressDelay)})
	return false
}

// Add registers the gesture area (sz from the origin).
func (l *longPress) Add(gtx C, sz image.Point) {
	defer clip.Rect{Max: sz}.Push(gtx.Ops).Pop()
	defer pointer.PassOp{}.Push(gtx.Ops).Pop()
	event.Op(gtx.Ops, l)
}

// withLongPress lays out w and makes it long-pressable.
func withLongPress(gtx C, l *longPress, w func(C) D) D {
	d := w(gtx)
	l.Add(gtx, d.Size)
	return d
}
