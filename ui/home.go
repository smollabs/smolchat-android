package ui

import (
	"fmt"

	"gioui.org/f32"
	"image"
	"time"

	"gioui.org/font"
	"gioui.org/io/event"
	"gioui.org/io/key"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"

	"github.com/smolworks/smolchat-android/app"
)

type screen int

const (
	screenChat screen = iota
	screenLibrary
	screenSettings
)

// UI is the root of the interface.
type UI struct {
	th      *Theme
	a       *app.App
	initErr error
	splash  Splash
	showing bool // splash still visible

	screen     screen
	transition Anim // 0 → 1 as a new screen slides in
	insets     layout.Inset

	drawer   Drawer
	chat     ChatScreen
	lib      Library
	settings SettingsScreen
	dialog   Dialog
	sheet    ModelSheet
	actions  ConvActions
	snack    Snackbar
}

// New creates the UI. The app core is attached later with SetApp so the
// splash can render while it initialises.
func New() *UI {
	u := &UI{th: NewTheme(), showing: true}
	u.transition.Set(1)
	u.transition.Rate = 18
	return u
}

// SetApp attaches the initialised core (or the error that prevented it).
func (u *UI) SetApp(a *app.App, err error) {
	u.a, u.initErr = a, err
}

func (u *UI) report(err error) {
	if err != nil && u.a != nil {
		u.a.Notify(err.Error(), true)
	}
}

func (u *UI) navigate(s screen) {
	if s == u.screen {
		return
	}
	if u.screen == screenSettings {
		u.settings.Leave(u.a)
	}
	u.screen = s
	u.transition.Set(0)
	u.transition.Target = 1
	if s == screenLibrary {
		u.a.RefreshDevice()
		u.a.CheckOnline()
	}
}

func (u *UI) canGoBack() bool {
	return u.dialog.open || u.sheet.open || u.actions.open || u.drawer.IsOpen() || u.screen != screenChat
}

// back handles the Android back button; it reports whether it consumed it.
func (u *UI) back() bool {
	switch {
	case u.dialog.open:
		u.dialog.Close()
	case u.sheet.open:
		u.sheet.open = false
	case u.actions.open:
		u.actions.open = false
	case u.drawer.IsOpen():
		u.drawer.Close()
	case u.screen != screenChat:
		u.navigate(screenChat)
	default:
		return false
	}
	return true
}

// Layout draws a frame. insets are the window's system insets in Dp.
func (u *UI) Layout(gtx C, top, bottom unit.Dp) D {
	paint.Fill(gtx.Ops, u.th.Pal.Bg)
	if u.a != nil && u.initErr == nil {
		u.insets = layout.Inset{Top: top, Bottom: bottom}
		// Back key: only claim it when there is something to dismiss, so at
		// the root Android's default (leave the app) still applies.
		if u.canGoBack() {
			for {
				ev, ok := gtx.Event(key.Filter{Name: key.NameBack})
				if !ok {
					break
				}
				if e, ok := ev.(key.Event); ok && e.State == key.Press {
					u.back()
				}
			}
		}
		u.main(gtx)
	} else if u.initErr != nil {
		u.fatal(gtx)
	}
	if u.showing {
		if u.a != nil || u.initErr != nil {
			u.splash.Finish(gtx.Now)
		}
		u.showing = u.splash.Layout(gtx, u.th)
	}
	return D{Size: gtx.Constraints.Max}
}

func (u *UI) main(gtx C) {
	// Screens: the content sits above the keyboard/navigation inset.
	func() {
		cgtx := gtx
		cgtx.Constraints.Max.Y -= gtx.Dp(u.insets.Bottom)
		cgtx.Constraints.Min = cgtx.Constraints.Max
		p := u.transition.Step(gtx)
		if p < 1 {
			dx := int(float32(gtx.Dp(28)) * (1 - float32(easeOut(float64(p)))))
			defer op.Offset(image.Pt(dx, 0)).Push(gtx.Ops).Pop()
			defer paint.PushOpacity(gtx.Ops, 0.25+0.75*p).Pop()
		}
		switch u.screen {
		case screenLibrary:
			u.lib.Layout(cgtx, u)
		case screenSettings:
			u.settings.Layout(cgtx, u)
		default:
			u.chat.Layout(cgtx, u)
		}
	}()
	u.drawer.Layout(gtx, u)
	u.sheet.Layout(gtx, u)
	u.actions.Layout(gtx, u)
	u.dialog.Layout(gtx, u)
	u.snack.Layout(gtx, u)
}

func (u *UI) fatal(gtx C) {
	th := u.th
	layout.Center.Layout(gtx, func(gtx C) D {
		return layout.UniformInset(24).Layout(gtx, func(gtx C) D {
			return layout.Flex{Axis: layout.Vertical, Alignment: layout.Middle}.Layout(gtx,
				layout.Rigid(func(gtx C) D { return Logo(gtx, 52, false) }),
				layout.Rigid(Gap(18)),
				layout.Rigid(Lbl(th, 18, th.Pal.Text, font.SemiBold, "SmolChat couldn't start").Layout),
				layout.Rigid(Gap(8)),
				layout.Rigid(centered(Lbl(th, 14, th.Pal.TextMuted, font.Normal, u.initErr.Error())).Layout),
			)
		})
	})
}

// ---- overlay plumbing -------------------------------------------------------

// overlay draws a scrim with progress p and blocks input underneath.
func overlay(gtx C, th *Theme, p float32, scrim *widget.Clickable) {
	full := gtx.Constraints.Max
	defer paint.PushOpacity(gtx.Ops, p).Pop()
	scrim.Layout(gtx, func(gtx C) D {
		paint.FillShape(gtx.Ops, th.Pal.Scrim, clip.Rect{Max: full}.Op())
		return D{Size: full}
	})
}

// blockInput stops pointer events from reaching widgets below a panel.
func blockInput(gtx C, sz image.Point, tag event.Tag) {
	defer clip.Rect{Max: sz}.Push(gtx.Ops).Pop()
	event.Op(gtx.Ops, tag)
	for {
		if _, ok := gtx.Event(pointer.Filter{Target: tag, Kinds: pointer.Press | pointer.Release}); !ok {
			break
		}
	}
}

// BottomSheet slides content up from the bottom.
type BottomSheet struct {
	open  bool
	anim  Anim
	scrim widget.Clickable
}

func (b *BottomSheet) layout(gtx C, u *UI, content layout.Widget) {
	if b.scrim.Clicked(gtx) {
		b.open = false
	}
	b.anim.Target = 0
	if b.open {
		b.anim.Target = 1
	}
	p := b.anim.Step(gtx)
	if p == 0 {
		return
	}
	overlay(gtx, u.th, p, &b.scrim)
	full := gtx.Constraints.Max
	m := op.Record(gtx.Ops)
	cgtx := gtx
	cgtx.Constraints.Min = image.Point{}
	cgtx.Constraints.Max.Y = full.Y * 80 / 100
	maxW := min(full.X, gtx.Dp(600))
	cgtx.Constraints.Max.X = maxW
	cgtx.Constraints.Min.X = maxW
	d := layout.Inset{Bottom: u.insets.Bottom}.Layout(cgtx, func(gtx C) D {
		return layout.Inset{Top: 10, Bottom: 16}.Layout(gtx, func(gtx C) D {
			return layout.Flex{Axis: layout.Vertical, Alignment: layout.Middle}.Layout(gtx,
				layout.Rigid(func(gtx C) D { // grab handle
					sz := image.Pt(gtx.Dp(36), gtx.Dp(4))
					fillRRect(gtx, sz, sz.Y/2, u.th.Pal.BorderStrong)
					return D{Size: sz}
				}),
				layout.Rigid(Gap(10)),
				layout.Rigid(content),
			)
		})
	})
	call := m.Stop()
	y := full.Y - int(float32(d.Size.Y)*float32(easeOut(float64(p))))
	x := (full.X - maxW) / 2
	defer op.Offset(image.Pt(x, y)).Push(gtx.Ops).Pop()
	r := gtx.Dp(22)
	sz := image.Pt(maxW, d.Size.Y+r)
	paint.FillShape(gtx.Ops, u.th.Pal.Surface, clip.RRect{Rect: image.Rectangle{Max: sz}, NW: r, NE: r}.Op(gtx.Ops))
	blockInput(gtx, sz, b)
	call.Add(gtx.Ops)
}

// sheetRow is a tappable row inside a sheet.
func sheetRow(gtx C, th *Theme, clk *widget.Clickable, icon *widget.Icon, title, sub string, selected, danger bool) D {
	return layout.Inset{Left: 12, Right: 12}.Layout(gtx, func(gtx C) D {
		return clk.Layout(gtx, func(gtx C) D {
			return layout.Background{}.Layout(gtx,
				func(gtx C) D {
					sz := gtx.Constraints.Min
					if selected {
						fillRRect(gtx, sz, gtx.Dp(14), th.Pal.AccentSoft)
					}
					pressOverlay(gtx, clk, sz, gtx.Dp(14))
					return D{Size: sz}
				},
				func(gtx C) D {
					gtx.Constraints.Min.X = gtx.Constraints.Max.X
					return layout.Inset{Left: 12, Right: 12, Top: 12, Bottom: 12}.Layout(gtx, func(gtx C) D {
						fg, iconCol := th.Pal.Text, th.Pal.TextMuted
						if danger {
							fg, iconCol = th.Pal.Danger, th.Pal.Danger
						}
						return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
							layout.Rigid(func(gtx C) D {
								if icon == nil {
									return D{}
								}
								return layout.Inset{Right: 14}.Layout(gtx, func(gtx C) D {
									s := gtx.Dp(22)
									gtx.Constraints = layout.Exact(image.Pt(s, s))
									return icon.Layout(gtx, iconCol)
								})
							}),
							layout.Flexed(1, func(gtx C) D {
								return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
									layout.Rigid(Lbl(th, 15, fg, font.Medium, title).Layout),
									layout.Rigid(func(gtx C) D {
										if sub == "" {
											return D{}
										}
										l := Lbl(th, 12.5, th.Pal.TextMuted, font.Normal, sub)
										l.MaxLines = 2
										return layout.Inset{Top: 2}.Layout(gtx, l.Layout)
									}),
								)
							}),
							layout.Rigid(func(gtx C) D {
								if !selected {
									return D{}
								}
								s := gtx.Dp(20)
								gtx.Constraints = layout.Exact(image.Pt(s, s))
								return ic.Check.Layout(gtx, th.Pal.AccentHi)
							}),
						)
					})
				})
		})
	})
}

// ---- model picker -----------------------------------------------------------

// ModelSheet chooses between automatic and manual model selection.
type ModelSheet struct {
	BottomSheet
	auto    widget.Clickable
	library widget.Clickable
	rows    map[string]*widget.Clickable
	list    widget.List
}

func (s *ModelSheet) Show() { s.open = true }

func (s *ModelSheet) Layout(gtx C, u *UI) {
	th, a := u.th, u.a
	installed := a.Models.Installed()
	conv := a.Chat().Conv
	if s.rows == nil {
		s.rows = map[string]*widget.Clickable{}
	}
	if s.auto.Clicked(gtx) {
		a.SetSelection(app.SelectAuto, "")
		s.open = false
	}
	if s.library.Clicked(gtx) {
		s.open = false
		u.navigate(screenLibrary)
	}
	for _, m := range installed {
		if s.rows[m.ID] == nil {
			s.rows[m.ID] = &widget.Clickable{}
		}
		if s.rows[m.ID].Clicked(gtx) {
			a.SetSelection(app.SelectManual, m.ID)
			s.open = false
		}
	}
	dev := a.Device()
	nCtx := a.Settings.Get().ContextSize
	loaded := a.Runtime.Status().ModelID
	s.layout(gtx, u, func(gtx C) D {
		items := []layout.Widget{
			func(gtx C) D {
				return layout.Inset{Left: 24, Right: 24, Bottom: 8}.Layout(gtx, func(gtx C) D {
					gtx.Constraints.Min.X = gtx.Constraints.Max.X
					return Lbl(th, 17, th.Pal.Text, font.SemiBold, "Model").Layout(gtx)
				})
			},
			func(gtx C) D {
				return sheetRow(gtx, th, &s.auto, ic.Bolt, "Auto",
					"Picks the best downloaded model for each message", conv.SelectionMode != app.SelectManual, false)
			},
		}
		if len(installed) > 0 {
			items = append(items, func(gtx C) D {
				return layout.Inset{Left: 24, Top: 12, Bottom: 4}.Layout(gtx, Lbl(th, 12, th.Pal.TextFaint, font.SemiBold, "DOWNLOADED").Layout)
			})
		}
		for _, m := range installed {
			m := m
			items = append(items, func(gtx C) D {
				sub := fmt.Sprintf("%s · %s", m.Params, app.AssessFit(m, dev, nCtx).Label())
				if m.ID == loaded {
					sub += " · loaded"
				}
				return sheetRow(gtx, th, s.rows[m.ID], ic.Memory, m.Name, sub,
					conv.SelectionMode == app.SelectManual && conv.ModelID == m.ID, false)
			})
		}
		items = append(items, func(gtx C) D {
			return layout.Inset{Top: 8}.Layout(gtx, func(gtx C) D {
				sub := "Download more Smol models"
				if len(installed) == 0 {
					sub = "No models yet — download one to start chatting"
				}
				return sheetRow(gtx, th, &s.library, ic.Library, "Model Library", sub, false, false)
			})
		})
		s.list.Axis = layout.Vertical
		return s.list.Layout(gtx, len(items), func(gtx C, i int) D { return items[i](gtx) })
	})
}

// ---- conversation actions ---------------------------------------------------

// ConvActions offers rename/delete for a conversation.
type ConvActions struct {
	BottomSheet
	conv   app.Conversation
	rename widget.Clickable
	del    widget.Clickable
}

func (c *ConvActions) Show(conv app.Conversation) { c.conv, c.open = conv, true }

func (c *ConvActions) Layout(gtx C, u *UI) {
	th, a := u.th, u.a
	conv := c.conv
	if c.rename.Clicked(gtx) {
		c.open = false
		u.dialog.Prompt("Rename chat", conv.Title, "Save", func(s string) { a.RenameConversation(conv.ID, s) })
	}
	if c.del.Clicked(gtx) {
		c.open = false
		u.dialog.Confirm("Delete chat?", "“"+conv.Title+"” will be permanently deleted.", "Delete", true,
			func(string) { a.DeleteConversation(conv.ID) })
	}
	c.layout(gtx, u, func(gtx C) D {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(func(gtx C) D {
				return layout.Inset{Left: 24, Right: 24, Bottom: 8}.Layout(gtx, func(gtx C) D {
					gtx.Constraints.Min.X = gtx.Constraints.Max.X
					l := Lbl(th, 16, th.Pal.Text, font.SemiBold, conv.Title)
					l.MaxLines = 1
					l.Truncator = "…"
					return l.Layout(gtx)
				})
			}),
			layout.Rigid(func(gtx C) D { return sheetRow(gtx, th, &c.rename, ic.Edit, "Rename", "", false, false) }),
			layout.Rigid(func(gtx C) D { return sheetRow(gtx, th, &c.del, ic.Delete, "Delete", "", false, true) }),
		)
	})
}

// ---- dialog -------------------------------------------------------------------

// Dialog is a centred modal for confirmations and short text input.
type Dialog struct {
	open        bool
	anim        Anim
	title, body string
	confirmText string
	danger      bool
	withInput   bool
	input       widget.Editor
	onConfirm   func(string)
	ok, cancel  widget.Clickable
	scrim       widget.Clickable
	focus       bool
}

func (d *Dialog) Confirm(title, body, confirm string, danger bool, f func(string)) {
	*d = Dialog{open: true, anim: d.anim, title: title, body: body, confirmText: confirm, danger: danger, onConfirm: f}
}

func (d *Dialog) Prompt(title, initial, confirm string, f func(string)) {
	*d = Dialog{open: true, anim: d.anim, title: title, confirmText: confirm, withInput: true, onConfirm: f, focus: true}
	d.input.SingleLine = true
	d.input.Submit = true
	d.input.SetText(initial)
	d.input.SetCaret(d.input.Len(), 0)
}

func (d *Dialog) Close() { d.open = false }

func (d *Dialog) Layout(gtx C, u *UI) {
	th := u.th
	confirm := false
	if d.ok.Clicked(gtx) {
		confirm = true
	}
	for {
		ev, ok := d.input.Update(gtx)
		if !ok {
			break
		}
		if _, ok := ev.(widget.SubmitEvent); ok {
			confirm = true
		}
	}
	if confirm && d.open {
		d.open = false
		if d.onConfirm != nil {
			d.onConfirm(d.input.Text())
		}
	}
	if d.cancel.Clicked(gtx) || d.scrim.Clicked(gtx) {
		d.open = false
	}
	d.anim.Rate = 20
	d.anim.Target = 0
	if d.open {
		d.anim.Target = 1
	}
	p := d.anim.Step(gtx)
	if p == 0 {
		return
	}
	overlay(gtx, th, p, &d.scrim)
	if d.focus {
		d.focus = false
		gtx.Execute(key.FocusCmd{Tag: &d.input})
	}
	full := gtx.Constraints.Max
	full.Y -= gtx.Dp(u.insets.Bottom)
	m := op.Record(gtx.Ops)
	cgtx := gtx
	w := min(full.X-gtx.Dp(48), gtx.Dp(400))
	cgtx.Constraints = layout.Constraints{Min: image.Pt(w, 0), Max: image.Pt(w, full.Y)}
	dims := Surface{Bg: th.Pal.Surface2, Border: th.Pal.BorderStrong, Radius: 22}.Layout(cgtx, func(gtx C) D {
		blockInput(gtx, gtx.Constraints.Min, d)
		return layout.UniformInset(22).Layout(gtx, func(gtx C) D {
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
				layout.Rigid(Lbl(th, 18, th.Pal.Text, font.SemiBold, d.title).Layout),
				layout.Rigid(func(gtx C) D {
					if d.body == "" {
						return D{}
					}
					return layout.Inset{Top: 8}.Layout(gtx, func(gtx C) D { return hint(gtx, th, d.body) })
				}),
				layout.Rigid(func(gtx C) D {
					if !d.withInput {
						return D{}
					}
					return layout.Inset{Top: 14}.Layout(gtx, func(gtx C) D { return Field(gtx, th, &d.input, "", nil) })
				}),
				layout.Rigid(Gap(20)),
				layout.Rigid(func(gtx C) D {
					return layout.Flex{Spacing: layout.SpaceStart}.Layout(gtx,
						layout.Rigid(func(gtx C) D { return Button{Kind: BtnGhost, Text: "Cancel"}.Layout(gtx, th, &d.cancel) }),
						layout.Rigid(Gap(8)),
						layout.Rigid(func(gtx C) D {
							k := BtnPrimary
							if d.danger {
								k = BtnDanger
							}
							return Button{Kind: k, Text: d.confirmText}.Layout(gtx, th, &d.ok)
						}),
					)
				}),
			)
		})
	})
	call := m.Stop()
	scale := 0.94 + 0.06*float32(easeOut(float64(p)))
	x, y := (full.X-dims.Size.X)/2, (full.Y-dims.Size.Y)/2
	defer op.Offset(image.Pt(x, y)).Push(gtx.Ops).Pop()
	defer op.Affine(scaleAround(dims.Size, scale)).Push(gtx.Ops).Pop()
	defer paint.PushOpacity(gtx.Ops, p).Pop()
	call.Add(gtx.Ops)
}

// ---- snackbar -------------------------------------------------------------------

// Snackbar shows app notices briefly above the composer.
type Snackbar struct {
	cur   app.Notice
	until time.Time
	anim  Anim
}

func (s *Snackbar) Layout(gtx C, u *UI) {
	th := u.th
	if gtx.Now.After(s.until) {
		if n, ok := u.a.TakeNotice(); ok {
			s.cur = n
			s.until = gtx.Now.Add(2800 * time.Millisecond)
		}
	}
	s.anim.Rate = 16
	s.anim.Target = 0
	if gtx.Now.Before(s.until) {
		s.anim.Target = 1
		gtx.Execute(op.InvalidateCmd{At: s.until})
	}
	p := s.anim.Step(gtx)
	if p == 0 {
		return
	}
	full := gtx.Constraints.Max
	m := op.Record(gtx.Ops)
	cgtx := gtx
	w := min(full.X-gtx.Dp(32), gtx.Dp(520))
	cgtx.Constraints = layout.Constraints{Min: image.Pt(w, 0), Max: image.Pt(w, full.Y)}
	dims := Surface{Bg: th.Pal.Elevated, Border: th.Pal.BorderStrong, Radius: 14}.Layout(cgtx, func(gtx C) D {
		return layout.Inset{Left: 16, Right: 16, Top: 12, Bottom: 12}.Layout(gtx, func(gtx C) D {
			return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
				layout.Rigid(func(gtx C) D {
					col := th.Pal.Success
					if s.cur.Error {
						col = th.Pal.Danger
					}
					return Dot(gtx, col, 8)
				}),
				layout.Rigid(Gap(10)),
				layout.Flexed(1, func(gtx C) D {
					l := Lbl(th, 13.5, th.Pal.Text, font.Medium, s.cur.Text)
					l.MaxLines = 3
					return l.Layout(gtx)
				}),
			)
		})
	})
	call := m.Stop()
	bottomGap := gtx.Dp(u.insets.Bottom) + gtx.Dp(130)
	y := full.Y - bottomGap - dims.Size.Y + int(float32(gtx.Dp(16))*(1-p))
	defer op.Offset(image.Pt((full.X-dims.Size.X)/2, y)).Push(gtx.Ops).Pop()
	defer paint.PushOpacity(gtx.Ops, p).Pop()
	call.Add(gtx.Ops)
}

// scaleAround scales about the centre of a box of size sz.
func scaleAround(sz image.Point, s float32) f32.Affine2D {
	c := f32.Pt(float32(sz.X)/2, float32(sz.Y)/2)
	return f32.AffineId().Scale(c, f32.Pt(s, s))
}
