package ui

import (
	"image"
	"io"
	"strings"

	"gioui.org/font"
	"gioui.org/io/clipboard"
	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/widget"
	"gioui.org/widget/material"
	"golang.org/x/exp/shiny/materialdesign/icons"

	"github.com/smolworks/smolchat-android/app"
)

var (
	icSelectAll = mustIcon(icons.ContentSelectAll)
	icShare     = mustIcon(icons.SocialShare)
	icPaste     = mustIcon(icons.ContentContentPaste)
	icCut       = mustIcon(icons.ContentContentCut)
)

func copyText(gtx C, s string) {
	gtx.Execute(clipboard.WriteCmd{Type: "application/text", Data: io.NopCloser(strings.NewReader(s))})
}

// ---- long-press menu on a message ------------------------------------------

// MessageActions is the sheet shown when a message is long-pressed.
type MessageActions struct {
	BottomSheet
	msg                           app.Message
	last                          bool
	copy, sel, share, regen, edit widget.Clickable
}

func (m *MessageActions) Show(msg app.Message, last bool) {
	m.msg, m.last, m.open = msg, last, true
}

func (m *MessageActions) Layout(gtx C, u *UI) {
	th, a := u.th, u.a
	text := m.msg.Content
	if m.copy.Clicked(gtx) {
		m.open = false
		copyText(gtx, text)
		a.Notify("Copied to clipboard", false)
	}
	if m.sel.Clicked(gtx) {
		m.open = false
		u.sel.Open(text)
		u.navigate(screenSelect)
	}
	if m.share.Clicked(gtx) {
		m.open = false
		if app.CanShare() {
			go app.ShareText(text)
		} else {
			copyText(gtx, text)
			a.Notify("Copied — sharing isn't available on this device", false)
		}
	}
	if m.regen.Clicked(gtx) {
		m.open = false
		a.Regenerate()
	}
	if m.edit.Clicked(gtx) {
		m.open = false
		u.chat.usePrompt(text)
	}
	assistant := m.msg.Role == "assistant"
	m.layout(gtx, u, func(gtx C) D {
		rows := []layout.FlexChild{
			layout.Rigid(func(gtx C) D {
				return layout.Inset{Left: 24, Right: 24, Bottom: 10}.Layout(gtx, func(gtx C) D {
					gtx.Constraints.Min.X = gtx.Constraints.Max.X
					l := Lbl(th, 13.5, th.Pal.TextMuted, font.Normal, strings.Join(strings.Fields(text), " "))
					l.MaxLines = 2
					l.Truncator = "…"
					return l.Layout(gtx)
				})
			}),
			layout.Rigid(func(gtx C) D { return sheetRow(gtx, th, &m.copy, ic.Copy, "Copy", "", false, false) }),
			layout.Rigid(func(gtx C) D {
				return sheetRow(gtx, th, &m.sel, icSelectAll, "Select text", "Pick part of the message to copy", false, false)
			}),
			layout.Rigid(func(gtx C) D { return sheetRow(gtx, th, &m.share, icShare, "Share", "", false, false) }),
		}
		if assistant && m.last {
			rows = append(rows, layout.Rigid(func(gtx C) D {
				return sheetRow(gtx, th, &m.regen, ic.Refresh, "Regenerate", "", false, false)
			}))
		}
		if !assistant {
			rows = append(rows, layout.Rigid(func(gtx C) D {
				return sheetRow(gtx, th, &m.edit, ic.Edit, "Edit & resend", "Put this message back in the composer", false, false)
			}))
		}
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx, rows...)
	})
}

// ---- full-screen text selection -----------------------------------------------

// SelectView shows one message as touch-selectable text.
type SelectView struct {
	close, copyBtn, selAll widget.Clickable
	txt                    SelText
	list                   widget.List
}

// Open prepares the view for text.
func (s *SelectView) Open(text string) {
	s.txt.SetText(text)
	s.txt.ClearSelection()
	s.list.Position = layout.Position{}
}

func (s *SelectView) Layout(gtx C, u *UI) D {
	th, a := u.th, u.a
	if s.close.Clicked(gtx) {
		u.pop(0)
	}
	if s.selAll.Clicked(gtx) {
		s.txt.SelectAll()
	}
	hasSel := s.txt.HasSelection()
	if s.copyBtn.Clicked(gtx) {
		txt := s.txt.Selected()
		if txt == "" {
			txt = s.txt.src
		}
		copyText(gtx, txt)
		a.Notify("Copied to clipboard", false)
	}
	copyLabel := "Copy all"
	if hasSel {
		copyLabel = "Copy selection"
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx C) D {
			return sheetHeader(gtx, u, &s.close, "Select text", "Press and hold a word, then drag (or move the handles) to select.", nil)
		}),
		layout.Rigid(func(gtx C) D {
			return layout.Inset{Left: 20, Right: 20, Bottom: 12}.Layout(gtx, func(gtx C) D {
				return layout.Flex{}.Layout(gtx,
					layout.Rigid(func(gtx C) D {
						return Button{Kind: BtnSecondary, Text: "Select all", Icon: icSelectAll, Small: true}.Layout(gtx, th, &s.selAll)
					}),
					layout.Rigid(Gap(8)),
					layout.Rigid(func(gtx C) D {
						return Button{Kind: BtnPrimary, Text: copyLabel, Icon: ic.Copy, Small: true}.Layout(gtx, th, &s.copyBtn)
					}),
				)
			})
		}),
		layout.Rigid(func(gtx C) D { return Divider(gtx, th.Pal.Border) }),
		layout.Flexed(1, func(gtx C) D {
			s.list.Axis = layout.Vertical
			return material.List(th.Theme, &s.list).Layout(gtx, 1, func(gtx C, _ int) D {
				// 28dp from the edge keeps clear of Android's back-gesture zone.
				return layout.Inset{Left: 28, Right: 24, Top: 18, Bottom: 24}.Layout(gtx, func(gtx C) D {
					return s.txt.Layout(gtx, th, 16.5, th.Pal.Text, withAlpha(th.Pal.Accent, 0x66))
				})
			})
		}),
	)
}

// ---- composer edit bar ------------------------------------------------------------

// editBar is the Paste / Select all / Cut / Copy strip shown when the
// composer is long-pressed (Gio has no native text context menu).
type editBar struct {
	open                     bool
	paste, all, cut, copy, x widget.Clickable
}

func (b *editBar) Layout(gtx C, th *Theme, ed *widget.Editor) D {
	if !b.open {
		return D{}
	}
	hasSel := ed.SelectionLen() > 0
	if b.paste.Clicked(gtx) {
		gtx.Execute(clipboard.ReadCmd{Tag: ed}) // the editor inserts the text
		gtx.Execute(key.FocusCmd{Tag: ed})
		b.open = false
	}
	if b.all.Clicked(gtx) {
		ed.SetCaret(0, ed.Len())
		gtx.Execute(key.FocusCmd{Tag: ed})
	}
	if b.copy.Clicked(gtx) && hasSel {
		copyText(gtx, ed.SelectedText())
		b.open = false
	}
	if b.cut.Clicked(gtx) && hasSel {
		copyText(gtx, ed.SelectedText())
		ed.Insert("")
		b.open = false
	}
	if b.x.Clicked(gtx) {
		b.open = false
	}
	item := func(clk *widget.Clickable, icon *widget.Icon, label string) layout.FlexChild {
		return layout.Rigid(func(gtx C) D {
			return clk.Layout(gtx, func(gtx C) D {
				return layout.Background{}.Layout(gtx,
					func(gtx C) D {
						pressOverlay(gtx, clk, gtx.Constraints.Min, gtx.Dp(10))
						return D{Size: gtx.Constraints.Min}
					},
					func(gtx C) D {
						return layout.Inset{Left: 10, Right: 10, Top: 8, Bottom: 8}.Layout(gtx, func(gtx C) D {
							return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
								layout.Rigid(func(gtx C) D {
									s := gtx.Dp(16)
									gtx.Constraints = layout.Exact(image.Pt(s, s))
									return icon.Layout(gtx, th.Pal.Text)
								}),
								layout.Rigid(Gap(6)),
								layout.Rigid(Lbl(th, 13.5, th.Pal.Text, font.Medium, label).Layout),
							)
						})
					})
			})
		})
	}
	items := []layout.FlexChild{item(&b.paste, icPaste, "Paste"), item(&b.all, icSelectAll, "Select all")}
	if hasSel {
		items = append(items, item(&b.cut, icCut, "Cut"), item(&b.copy, ic.Copy, "Copy"))
	}
	items = append(items, layout.Rigid(func(gtx C) D { return IconButtonSized(gtx, &b.x, ic.Close, th.Pal.TextFaint, 16, 34) }))
	return layout.Inset{Bottom: 8}.Layout(gtx, func(gtx C) D {
		return layout.Center.Layout(gtx, func(gtx C) D {
			return Surface{Bg: th.Pal.Popup, Border: th.Pal.BorderStrong, Radius: 14}.Layout(gtx, func(gtx C) D {
				return layout.UniformInset(3).Layout(gtx, func(gtx C) D {
					return layout.Flex{Alignment: layout.Middle}.Layout(gtx, items...)
				})
			})
		})
	})
}
