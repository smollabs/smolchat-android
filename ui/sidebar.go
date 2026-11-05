package ui

import (
	"image"
	"strings"
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
	"gioui.org/widget"
	"gioui.org/widget/material"

	"github.com/smolworks/smolchat-android/app"
)

// hdrag tracks a horizontal swipe. It listens alongside other handlers
// (PassOp) and grabs the pointer only once the motion is clearly
// horizontal, so vertical scrolling and taps keep working.
type hdrag struct {
	active bool
	locked int // 0 undecided, 1 horizontal, -1 vertical
	// rightOnly makes leftward swipes fall through (opening gesture), so
	// sideways-scrolling content such as code blocks keeps working.
	rightOnly bool
	id        pointer.ID
	start     f32.Point
	dx        float32
	lastDx    float32
	lastPos   f32.Point
}

// update processes events; it returns the horizontal delta while dragging
// and whether a drag just ended.
func (h *hdrag) update(gtx C) (dragging bool, released bool) {
	slop := float32(gtx.Dp(10))
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
			if e.Source == pointer.Mouse && e.Buttons != pointer.ButtonPrimary {
				continue
			}
			*h = hdrag{active: true, id: e.PointerID, start: e.Position, lastPos: e.Position}
		case pointer.Drag:
			if !h.active || e.PointerID != h.id {
				continue
			}
			d := e.Position.Sub(h.start)
			if h.locked == 0 {
				ax, ay := abs32(d.X), abs32(d.Y)
				switch {
				case ax > slop && ax > ay*1.3 && (!h.rightOnly || d.X > 0):
					h.locked = 1
					gtx.Execute(pointer.GrabCmd{Tag: h, ID: e.PointerID})
				case ay > slop || (h.rightOnly && d.X < -slop):
					h.locked = -1
				}
			}
			h.lastDx = e.Position.X - h.lastPos.X
			h.lastPos = e.Position
			h.dx = d.X
		case pointer.Release, pointer.Cancel:
			if h.active && h.locked == 1 {
				released = true
			}
			h.active = false
		}
	}
	return h.active && h.locked == 1, released
}

func (h *hdrag) add(gtx C, r image.Rectangle) {
	defer clip.Rect(r).Push(gtx.Ops).Pop()
	defer pointer.PassOp{}.Push(gtx.Ops).Pop()
	event.Op(gtx.Ops, h)
}

func abs32(v float32) float32 {
	if v < 0 {
		return -v
	}
	return v
}

type convRow struct {
	open, more widget.Clickable
	lp         longPress
}

// Drawer is the slide-in navigation panel (the Mac app's sidebar).
type Drawer struct {
	anim       Anim
	drag       hdrag
	dragBase   float32
	scrim      widget.Clickable
	search     widget.Editor
	newChat    widget.Clickable
	settings   widget.Clickable
	themeBtn   widget.Clickable
	status     widget.Clickable
	clearQuery widget.Clickable
	list       widget.List
	rows       map[string]*convRow
	lastQuery  string
}

func (d *Drawer) Open()        { d.anim.Target = 1 }
func (d *Drawer) Close()       { d.anim.Target = 0 }
func (d *Drawer) IsOpen() bool { return d.anim.Target == 1 }

func (d *Drawer) width(gtx C) int {
	return min(gtx.Constraints.Max.X*84/100, gtx.Dp(320))
}

// Layout draws the drawer over the screen. It must be called after the main
// content so it sits on top.
func (d *Drawer) Layout(gtx C, u *UI) D {
	w := d.width(gtx)
	full := gtx.Constraints.Max

	dragging, released := d.drag.update(gtx)
	if dragging {
		if d.dragBase < 0 {
			d.dragBase = d.anim.V
		}
		p := d.dragBase + d.drag.dx/float32(w)
		d.anim.Set(max(0, min(1, p)))
		gtx.Execute(op.InvalidateCmd{})
	} else {
		d.dragBase = -1
	}
	if released {
		switch {
		case d.drag.lastDx > 2:
			d.anim.Target = 1
		case d.drag.lastDx < -2:
			d.anim.Target = 0
		case d.anim.V > 0.5:
			d.anim.Target = 1
		default:
			d.anim.Target = 0
		}
	}
	if d.scrim.Clicked(gtx) {
		d.Close()
	}
	p := d.anim.Step(gtx)

	// Gesture area: when closed, a rightward swipe anywhere below the header
	// opens the drawer (Android reserves the screen edge for its Back
	// gesture); when open, any horizontal swipe moves it.
	if p == 0 && !dragging {
		if u.screen == screenChat {
			d.drag.rightOnly = true
			d.drag.add(gtx, image.Rect(0, gtx.Dp(u.insets.Top+64), full.X, full.Y-gtx.Dp(u.insets.Bottom+120)))
		}
		return D{Size: full}
	}
	d.drag.rightOnly = false

	func() {
		defer paint.PushOpacity(gtx.Ops, p).Pop()
		d.scrim.Layout(gtx, func(gtx C) D {
			paint.FillShape(gtx.Ops, u.th.Pal.Scrim, clip.Rect{Max: full}.Op())
			return D{Size: full}
		})
	}()

	x := int(float32(w) * (p - 1))
	func() {
		defer op.Offset(image.Pt(x, 0)).Push(gtx.Ops).Pop()
		pgtx := gtx
		pgtx.Constraints = layout.Exact(image.Pt(w, full.Y))
		paint.FillShape(gtx.Ops, u.th.Pal.Frame, clip.Rect{Max: image.Pt(w, full.Y)}.Op())
		func() { // swallow taps so they don't reach the scrim
			defer clip.Rect{Max: image.Pt(w, full.Y)}.Push(gtx.Ops).Pop()
			event.Op(gtx.Ops, &d.rows)
		}()
		d.panel(pgtx, u)
		off := op.Offset(image.Pt(w-1, 0)).Push(gtx.Ops)
		fillRRect(gtx, image.Pt(1, full.Y), 0, u.th.Pal.Border)
		off.Pop()
	}()
	d.drag.add(gtx, image.Rectangle{Max: full})
	return D{Size: full}
}

func (d *Drawer) panel(gtx C, u *UI) D {
	th, a := u.th, u.a
	// Actions below change state after this frame's animation step, so ask
	// for another frame (otherwise the change waits for the next input).
	defer func() {
		if d.anim.Target != d.anim.V {
			gtx.Execute(op.InvalidateCmd{})
		}
	}()
	if d.newChat.Clicked(gtx) {
		a.NewChat()
		u.chat.focusComposer = true
		d.Close()
	}
	if d.status.Clicked(gtx) {
		u.navigate(screenLibrary)
		d.Close()
	}
	if d.settings.Clicked(gtx) {
		u.navigate(screenSettings)
		d.Close()
	}
	if d.themeBtn.Clicked(gtx) {
		dark := !th.Pal.Dark
		a.ApplySettings(func(s *app.Settings) {
			s.Theme = "light"
			if dark {
				s.Theme = "dark"
			}
		})
	}
	if d.clearQuery.Clicked(gtx) {
		d.search.SetText("")
	}
	d.search.SingleLine = true
	for {
		ev, ok := d.search.Update(gtx)
		if !ok {
			break
		}
		if _, ok := ev.(widget.ChangeEvent); ok {
			if q := d.search.Text(); q != d.lastQuery {
				d.lastQuery = q
				a.SetSearch(q)
			}
		}
	}
	if d.rows == nil {
		d.rows = map[string]*convRow{}
	}
	convs, _ := a.Conversations()
	current := a.Chat().Conv.ID

	return layout.Inset{Top: u.insets.Top, Bottom: u.insets.Bottom}.Layout(gtx, func(gtx C) D {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			// Brand.
			layout.Rigid(func(gtx C) D {
				return layout.Inset{Left: 16, Right: 12, Top: 16, Bottom: 14}.Layout(gtx, func(gtx C) D {
					return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
						layout.Rigid(func(gtx C) D { return LogoTile(gtx, 38) }),
						layout.Rigid(Gap(12)),
						layout.Flexed(1, func(gtx C) D {
							return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
								layout.Rigid(Lbl(th, 17, th.Pal.Text, font.SemiBold, "SmolChat").Layout),
								layout.Rigid(Lbl(th, 12.5, th.Pal.TextFaint, font.Normal, "by SmolWorks").Layout),
							)
						}),
					)
				})
			}),
			// New chat.
			layout.Rigid(func(gtx C) D {
				return layout.Inset{Left: 10, Right: 10}.Layout(gtx, func(gtx C) D {
					return d.newChat.Layout(gtx, func(gtx C) D {
						return layout.Background{}.Layout(gtx,
							func(gtx C) D {
								sz := gtx.Constraints.Min
								r := gtx.Dp(12)
								fillRRect(gtx, sz, r, withAlpha(th.Pal.Glass, 0xf0))
								strokeRRect(gtx, sz, r, float32(gtx.Dp(1)), th.Pal.BorderStrong)
								pressOverlay(gtx, &d.newChat, sz, r)
								return D{Size: sz}
							},
							func(gtx C) D {
								gtx.Constraints.Min.X = gtx.Constraints.Max.X
								return layout.Inset{Left: 8, Right: 8, Top: 8, Bottom: 8}.Layout(gtx, func(gtx C) D {
									return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
										layout.Rigid(func(gtx C) D {
											s := gtx.Dp(28)
											fillRRect(gtx, image.Pt(s, s), gtx.Dp(8), th.Pal.AccentSoft)
											is := gtx.Dp(18)
											off := op.Offset(image.Pt((s-is)/2, (s-is)/2)).Push(gtx.Ops)
											gtx.Constraints = layout.Exact(image.Pt(is, is))
											ic.Plus.Layout(gtx, th.Pal.Accent)
											off.Pop()
											return D{Size: image.Pt(s, s)}
										}),
										layout.Rigid(Gap(12)),
										layout.Rigid(Lbl(th, 15.5, th.Pal.Text, font.Medium, "New chat").Layout),
									)
								})
							})
					})
				})
			}),
			// Search.
			layout.Rigid(func(gtx C) D {
				return layout.Inset{Left: 10, Right: 10, Top: 10, Bottom: 6}.Layout(gtx, func(gtx C) D {
					return layout.Stack{Alignment: layout.E}.Layout(gtx,
						layout.Stacked(func(gtx C) D {
							gtx.Constraints.Min.X = gtx.Constraints.Max.X
							return Field(gtx, th, &d.search, "Search chats", ic.Search)
						}),
						layout.Stacked(func(gtx C) D {
							if d.search.Len() == 0 {
								return D{}
							}
							return IconButtonSized(gtx, &d.clearQuery, ic.Close, th.Pal.TextMuted, 16, 40)
						}),
					)
				})
			}),
			// History.
			layout.Flexed(1, func(gtx C) D { return d.history(gtx, u, convs, current) }),
			// Footer: status card, theme toggle, settings.
			layout.Rigid(func(gtx C) D {
				return layout.Inset{Left: 10, Right: 6, Top: 8, Bottom: 10}.Layout(gtx, func(gtx C) D {
					return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
						layout.Flexed(1, func(gtx C) D { return d.statusCard(gtx, u) }),
						layout.Rigid(Gap(4)),
						layout.Rigid(func(gtx C) D {
							icon := ic.Sun
							if !th.Pal.Dark {
								icon = ic.Moon
							}
							return IconButtonSized(gtx, &d.themeBtn, icon, th.Pal.TextMuted, 21, 42)
						}),
						layout.Rigid(func(gtx C) D { return IconButtonSized(gtx, &d.settings, ic.Settings, th.Pal.TextMuted, 21, 42) }),
					)
				})
			}),
		)
	})
}

// dateGroup buckets a timestamp relative to now.
func dateGroup(t, now time.Time) string {
	y1, m1, d1 := now.Date()
	today := time.Date(y1, m1, d1, 0, 0, 0, 0, now.Location())
	switch {
	case !t.Before(today):
		return "Today"
	case !t.Before(today.AddDate(0, 0, -1)):
		return "Yesterday"
	case !t.Before(today.AddDate(0, 0, -7)):
		return "Previous 7 days"
	case !t.Before(today.AddDate(0, 0, -30)):
		return "Previous 30 days"
	case t.Year() == now.Year():
		return t.Format("January")
	}
	return t.Format("January 2006")
}

func (d *Drawer) history(gtx C, u *UI, convs []app.Conversation, current string) D {
	th := u.th
	type item struct {
		header string
		conv   *app.Conversation
	}
	var items []item
	last := ""
	now := time.Now()
	for i := range convs {
		g := dateGroup(convs[i].UpdatedAt, now)
		if g != last {
			items = append(items, item{header: g})
			last = g
		}
		items = append(items, item{conv: &convs[i]})
	}
	if len(items) == 0 {
		msg := "Your conversations will appear here."
		if d.search.Len() > 0 {
			msg = "No chats match “" + strings.TrimSpace(d.search.Text()) + "”."
		}
		return layout.Inset{Left: 20, Right: 20, Top: 20}.Layout(gtx, Lbl(th, 13.5, th.Pal.TextFaint, font.Normal, msg).Layout)
	}
	d.list.Axis = layout.Vertical
	return material.List(th.Theme, &d.list).Layout(gtx, len(items), func(gtx C, i int) D {
		it := items[i]
		if it.conv == nil {
			return layout.Inset{Left: 20, Top: 18, Bottom: 6}.Layout(gtx, Lbl(th, 12.5, th.Pal.TextFaint, font.SemiBold, it.header).Layout)
		}
		c := *it.conv
		row := d.rows[c.ID]
		if row == nil {
			row = &convRow{}
			d.rows[c.ID] = row
		}
		if row.open.Clicked(gtx) {
			u.report(u.a.OpenConversation(c.ID))
			d.Close()
		}
		if row.more.Clicked(gtx) || row.lp.Update(gtx) {
			u.actions.Show(c)
		}
		selected := c.ID == current
		return layout.Inset{Left: 10, Right: 10}.Layout(gtx, func(gtx C) D {
			return withLongPress(gtx, &row.lp, func(gtx C) D { return d.rowBody(gtx, th, row, c, selected) })
		})
	})
}

func (d *Drawer) rowBody(gtx C, th *Theme, row *convRow, c app.Conversation, selected bool) D {
	return layout.Stack{Alignment: layout.E}.Layout(gtx,
		layout.Stacked(func(gtx C) D {
			return row.open.Layout(gtx, func(gtx C) D {
				return layout.Background{}.Layout(gtx,
					func(gtx C) D {
						sz := gtx.Constraints.Min
						r := gtx.Dp(10)
						if selected {
							fillRRect(gtx, sz, r, th.Pal.Elevated)
							bar := image.Pt(gtx.Dp(3), sz.Y-gtx.Dp(16))
							off := op.Offset(image.Pt(0, gtx.Dp(8))).Push(gtx.Ops)
							fillRRect(gtx, bar, bar.X/2, th.Pal.Accent)
							off.Pop()
						}
						pressOverlay(gtx, &row.open, sz, r)
						return D{Size: sz}
					},
					func(gtx C) D {
						gtx.Constraints.Min.X = gtx.Constraints.Max.X
						return layout.Inset{Left: 12, Right: 44, Top: 11, Bottom: 11}.Layout(gtx, func(gtx C) D {
							l := Lbl(th, 15, th.Pal.Text, font.Normal, c.Title)
							l.MaxLines = 1
							l.Truncator = "…"
							l.WrapPolicy = text.WrapGraphemes
							return l.Layout(gtx)
						})
					})
			})
		}),
		layout.Stacked(func(gtx C) D {
			return IconButtonSized(gtx, &row.more, ic.More, th.Pal.TextFaint, 18, 40)
		}),
	)
}

// statusCard is the Mac's "● Ready / Model Library ›" button.
func (d *Drawer) statusCard(gtx C, u *UI) D {
	th := u.th
	st := u.a.Runtime.Status()
	title, dot := "Ready", th.Pal.TextFaint
	if len(u.a.Models.Installed()) == 0 {
		title = "No models yet"
	}
	if spec, ok := app.FindSpec(st.ModelID); ok {
		switch st.Phase {
		case app.PhaseLoading:
			title, dot = "Loading "+spec.Params+"…", th.Pal.Warning
		case app.PhaseGenerating:
			title, dot = spec.Name, th.Pal.Accent
		case app.PhaseReady:
			title, dot = spec.Name, th.Pal.Success
		case app.PhaseError:
			title, dot = "Load failed", th.Pal.Danger
		}
	}
	if u.a.Downloads.AnyActive() {
		title, dot = "Downloading…", th.Pal.Accent
	}
	return d.status.Layout(gtx, func(gtx C) D {
		return layout.Background{}.Layout(gtx,
			func(gtx C) D {
				sz := gtx.Constraints.Min
				r := gtx.Dp(12)
				fillRRect(gtx, sz, r, withAlpha(th.Pal.Glass, 0xf0))
				strokeRRect(gtx, sz, r, float32(gtx.Dp(1)), th.Pal.BorderStrong)
				pressOverlay(gtx, &d.status, sz, r)
				return D{Size: sz}
			},
			func(gtx C) D {
				gtx.Constraints.Min.X = gtx.Constraints.Max.X
				return layout.Inset{Left: 12, Right: 8, Top: 9, Bottom: 9}.Layout(gtx, func(gtx C) D {
					return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
						layout.Rigid(func(gtx C) D { return Dot(gtx, dot, 8) }),
						layout.Rigid(Gap(10)),
						layout.Flexed(1, func(gtx C) D {
							return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
								layout.Rigid(func(gtx C) D {
									l := Lbl(th, 14, th.Pal.Text, font.Medium, title)
									l.MaxLines = 1
									l.Truncator = "…"
									return l.Layout(gtx)
								}),
								layout.Rigid(Lbl(th, 12, th.Pal.TextFaint, font.Normal, "Model Library").Layout),
							)
						}),
						layout.Rigid(func(gtx C) D {
							s := gtx.Dp(18)
							gtx.Constraints = layout.Exact(image.Pt(s, s))
							return ic.ChevronRight.Layout(gtx, th.Pal.TextFaint)
						}),
					)
				})
			})
	})
}
