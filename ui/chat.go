package ui

import (
	"fmt"
	"image"
	"io"
	"strings"
	"time"

	"gioui.org/f32"
	"gioui.org/font"
	"gioui.org/io/clipboard"
	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/text"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"

	"github.com/smolworks/smolchat-android/app"
)

type msgState struct {
	md       Markdown
	copy     widget.Clickable
	copiedAt time.Time
	lp       longPress // opens the message menu
}

// quick-action chips (from the Mac app): they prefill the composer.
var quickActions = []struct {
	label, prompt string
	icon          func() *widget.Icon
}{
	{"Explain", "Explain in simple terms: ", func() *widget.Icon { return ic.Lightbulb }},
	{"Brainstorm", "Brainstorm ideas for ", func() *widget.Icon { return ic.Compass }},
	{"Write", "Help me write ", func() *widget.Icon { return ic.Edit }},
	{"Code", "Write code that ", func() *widget.Icon { return ic.Code }},
}

// ChatScreen is the conversation view with its composer.
type ChatScreen struct {
	menu, newChat, modelPill widget.Clickable
	list                     widget.List
	chipList                 layout.List
	editor                   widget.Editor
	send                     widget.Clickable
	regen                    widget.Clickable
	toBottom                 widget.Clickable
	getStarted               widget.Clickable
	browse                   widget.Clickable
	chips                    [4]widget.Clickable
	msgs                     map[int64]*msgState
	convID                   string
	focusComposer            bool
	lastCount                int
	welcomeIn                Anim
	started                  bool
	composerLP               longPress // opens the paste/copy bar
	edit                     editBar
}

func (c *ChatScreen) state(id int64) *msgState {
	if c.msgs == nil {
		c.msgs = map[int64]*msgState{}
	}
	s := c.msgs[id]
	if s == nil {
		s = &msgState{}
		c.msgs[id] = s
	}
	return s
}

// usePrompt prefills the composer (quick-action chips).
func (c *ChatScreen) usePrompt(p string) {
	c.editor.SetText(p)
	c.editor.SetCaret(c.editor.Len(), c.editor.Len())
	c.focusComposer = true
}

func greeting(now time.Time) string {
	switch h := now.Hour(); {
	case h < 5:
		return "Burning the midnight oil?"
	case h < 12:
		return "Good morning."
	case h < 18:
		return "Good afternoon."
	}
	return "Good evening."
}

func (c *ChatScreen) Layout(gtx C, u *UI) D {
	a := u.a
	view := a.Chat()
	if view.Conv.ID != c.convID || !c.started { // switched conversations: drop cached widgets
		c.started = true
		c.convID = view.Conv.ID
		c.msgs = nil
		c.list.Position = layout.Position{}
		c.list.ScrollToEnd = true
		c.lastCount = 0
		c.welcomeIn.Set(0)
		c.welcomeIn.Target = 1
		c.welcomeIn.Rate = 7
	}
	installed := a.Models.Installed()

	if c.menu.Clicked(gtx) {
		u.drawer.Open()
	}
	if c.newChat.Clicked(gtx) {
		a.NewChat()
		c.focusComposer = true
	}
	if c.modelPill.Clicked(gtx) {
		u.sheet.Show()
	}
	if c.getStarted.Clicked(gtx) {
		rec := app.Recommend(a.Device(), a.Settings.Get().ContextSize)
		a.StartDownload(rec.ID)
		u.navigate(screenLibrary)
	}
	if c.browse.Clicked(gtx) {
		u.navigate(screenLibrary)
	}
	for i := range c.chips {
		if c.chips[i].Clicked(gtx) {
			c.usePrompt(quickActions[i].prompt)
		}
	}
	if c.regen.Clicked(gtx) {
		a.Regenerate()
	}
	if c.toBottom.Clicked(gtx) {
		c.list.ScrollTo(len(view.Messages))
		c.list.ScrollToEnd = true
	}
	for {
		ev, ok := c.editor.Update(gtx)
		if !ok {
			break
		}
		if _, ok := ev.(widget.SubmitEvent); ok {
			c.submit(u, c.editor.Text(), installed)
		}
	}
	if c.send.Clicked(gtx) {
		if view.Streaming {
			a.StopGeneration()
		} else {
			c.submit(u, c.editor.Text(), installed)
		}
	}
	if c.focusComposer {
		c.focusComposer = false
		gtx.Execute(key.FocusCmd{Tag: &c.editor})
	}

	empty := len(view.Messages) == 0
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx C) D { return c.header(gtx, u, view) }),
		layout.Flexed(1, func(gtx C) D {
			if empty {
				return c.welcome(gtx, u, installed)
			}
			return c.messages(gtx, u, view)
		}),
		layout.Rigid(func(gtx C) D {
			if c.edit.open {
				return c.edit.Layout(gtx, u.th, &c.editor)
			}
			if !empty || len(installed) == 0 {
				return D{}
			}
			return c.chipRow(gtx, u.th)
		}),
		layout.Rigid(func(gtx C) D { return c.composer(gtx, u, view, installed) }),
		layout.Rigid(func(gtx C) D {
			if empty {
				return layout.Spacer{Height: 10}.Layout(gtx)
			}
			return layout.Inset{Left: 16, Right: 16, Bottom: 8}.Layout(gtx, func(gtx C) D {
				gtx.Constraints.Min.X = gtx.Constraints.Max.X
				l := Lbl(u.th, 11, u.th.Pal.TextFaint, font.Normal, "SmolChat runs entirely on this phone. Small models can make mistakes — check important information.")
				l.Alignment = text.Middle
				return l.Layout(gtx)
			})
		}),
	)
}

func (c *ChatScreen) submit(u *UI, txt string, installed []app.ModelSpec) {
	txt = strings.TrimSpace(txt)
	if txt == "" {
		return
	}
	if len(installed) == 0 {
		u.a.Notify("Download a model first — it only takes a minute.", false)
		u.navigate(screenLibrary)
		return
	}
	c.editor.SetText("")
	c.list.ScrollToEnd = true
	c.list.Position.BeforeEnd = false
	u.a.Send(txt)
}

// modelLabel is the pill text: "Auto", or the manually chosen model.
func modelLabel(conv app.Conversation) string {
	if conv.SelectionMode == app.SelectManual {
		if spec, ok := app.FindSpec(conv.ModelID); ok {
			return spec.Name
		}
	}
	return "Auto"
}

// glassPill draws the Mac's glass pill (fill and hairline edge).
func glassPill(gtx C, th *Theme, clk *widget.Clickable, w layout.Widget) D {
	return clk.Layout(gtx, func(gtx C) D {
		return layout.Background{}.Layout(gtx,
			func(gtx C) D {
				sz := gtx.Constraints.Min
				r := sz.Y / 2
				fillRRect(gtx, sz, r, withAlpha(th.Pal.Glass, 0xe6))
				strokeRRect(gtx, sz, r, float32(gtx.Dp(1)), th.Pal.BorderStrong)
				pressOverlay(gtx, clk, sz, r)
				return D{Size: sz}
			}, w)
	})
}

func (c *ChatScreen) header(gtx C, u *UI, view app.ChatView) D {
	th := u.th
	label := modelLabel(view.Conv)
	return layout.Inset{Top: u.insets.Top}.Layout(gtx, func(gtx C) D {
		return layout.Inset{Left: 4, Right: 4, Top: 6, Bottom: 6}.Layout(gtx, func(gtx C) D {
			return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
				layout.Rigid(func(gtx C) D { return IconButton(gtx, &c.menu, ic.Menu, th.Pal.TextMuted) }),
				layout.Rigid(Gap(4)),
				layout.Rigid(func(gtx C) D {
					return glassPill(gtx, th, &c.modelPill, func(gtx C) D {
						return layout.Inset{Left: 12, Right: 10, Top: 8, Bottom: 8}.Layout(gtx, func(gtx C) D {
							return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
								layout.Rigid(func(gtx C) D { return SparkleIcon(gtx, 15, th.Pal.TextMuted) }),
								layout.Rigid(Gap(7)),
								layout.Rigid(func(gtx C) D {
									gtx.Constraints.Max.X = gtx.Dp(180)
									l := Lbl(th, 14, th.Pal.Text, font.Medium, label)
									l.MaxLines = 1
									l.Truncator = "…"
									return l.Layout(gtx)
								}),
								layout.Rigid(Gap(4)),
								layout.Rigid(func(gtx C) D {
									s := gtx.Dp(18)
									gtx.Constraints = layout.Exact(image.Pt(s, s))
									return ic.ChevronDown.Layout(gtx, th.Pal.TextFaint)
								}),
							)
						})
					})
				}),
				layout.Flexed(1, func(gtx C) D { return D{Size: image.Pt(gtx.Constraints.Min.X, 0)} }),
				layout.Rigid(func(gtx C) D { return IconButton(gtx, &c.newChat, ic.NewChat, th.Pal.TextMuted) }),
			)
		})
	})
}

func (c *ChatScreen) welcome(gtx C, u *UI, installed []app.ModelSpec) D {
	th := u.th
	in := c.welcomeIn.Step(gtx)
	return layout.Center.Layout(gtx, func(gtx C) D {
		return layout.Inset{Left: 20, Right: 20}.Layout(gtx, func(gtx C) D {
			gtx.Constraints.Max.X = min(gtx.Constraints.Max.X, gtx.Dp(460))
			defer paint.PushOpacity(gtx.Ops, in).Pop()
			defer op.Offset(image.Pt(0, int(float32(gtx.Dp(14))*(1-in)))).Push(gtx.Ops).Pop()
			mw := unit.Dp(min(300, float32(gtx.Constraints.Max.X)/gtx.Metric.PxPerDp-20))
			reserve := float32(110) // greeting + title
			if len(installed) == 0 {
				mw = min(mw, 200) // leave room for the download card
				reserve += 250
			}
			// Shrink the mascot when the keyboard leaves little room; hide it
			// when it would be too small to read.
			availH := float32(gtx.Constraints.Max.Y)/gtx.Metric.PxPerDp - reserve
			mw = min(mw, unit.Dp(availH*720/479))
			showMascot := mw >= 90
			return layout.Flex{Axis: layout.Vertical, Alignment: layout.Middle}.Layout(gtx,
				layout.Rigid(func(gtx C) D {
					if !showMascot {
						return D{}
					}
					return layout.Inset{Bottom: 14}.Layout(gtx, func(gtx C) D { return Mascot(gtx, mw) })
				}),
				layout.Rigid(centered(Lbl(th, 14.5, th.Pal.TextMuted, font.Normal, greeting(time.Now()))).Layout),
				layout.Rigid(Gap(4)),
				layout.Rigid(centered(Lbl(th, 25, th.Pal.Text, font.SemiBold, "What shall we explore today?")).Layout),
				layout.Rigid(func(gtx C) D {
					if len(installed) > 0 {
						return D{}
					}
					return layout.Inset{Top: 22}.Layout(gtx, func(gtx C) D { return c.firstRun(gtx, u) })
				}),
			)
		})
	})
}

func (c *ChatScreen) firstRun(gtx C, u *UI) D {
	th, a := u.th, u.a
	rec := app.Recommend(a.Device(), a.Settings.Get().ContextSize)
	pr := a.Downloads.Progress(rec)
	return GlassCard(gtx, th, func(gtx C) D {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(Lbl(th, 16, th.Pal.Text, font.SemiBold, "Download your first model").Layout),
			layout.Rigid(Gap(5)),
			layout.Rigid(func(gtx C) D {
				return hint(gtx, th, fmt.Sprintf("%s is recommended for this phone. Download once, then chat anywhere — no internet needed.", rec.Name))
			}),
			layout.Rigid(Gap(14)),
			layout.Rigid(func(gtx C) D {
				switch {
				case pr.Active():
					return downloadProgress(gtx, th, pr)
				case pr.Status == app.DLPaused:
					return Button{Kind: BtnPrimary, Icon: ic.Play, Wide: true,
						Text: fmt.Sprintf("Resume · %s of %s", app.HumanBytes(pr.Downloaded), app.HumanBytes(pr.Total))}.Layout(gtx, th, &c.getStarted)
				case pr.Status == app.DLFailed:
					return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
						layout.Rigid(Lbl(th, 12.5, th.Pal.Danger, font.Medium, pr.Err).Layout),
						layout.Rigid(Gap(10)),
						layout.Rigid(func(gtx C) D {
							return Button{Kind: BtnPrimary, Text: "Try again", Icon: ic.Refresh, Wide: true}.Layout(gtx, th, &c.getStarted)
						}),
					)
				}
				return Button{Kind: BtnPrimary, Text: "Download · " + app.HumanBytes(rec.DownloadSize()), Icon: ic.Download, Wide: true}.Layout(gtx, th, &c.getStarted)
			}),
			layout.Rigid(Gap(6)),
			layout.Rigid(func(gtx C) D {
				return Button{Kind: BtnGhost, Text: "Browse the Model Library", Wide: true, Small: true}.Layout(gtx, th, &c.browse)
			}),
		)
	})
}

// chipRow shows the quick-action pills above the composer.
func (c *ChatScreen) chipRow(gtx C, th *Theme) D {
	return layout.Inset{Bottom: 10}.Layout(gtx, func(gtx C) D {
		return layout.Center.Layout(gtx, func(gtx C) D {
			c.chipList.Axis = layout.Horizontal
			return c.chipList.Layout(gtx, len(quickActions), func(gtx C, i int) D {
				left, right := unit.Dp(3), unit.Dp(3)
				if i == 0 {
					left = 12
				}
				if i == len(quickActions)-1 {
					right = 12
				}
				return layout.Inset{Left: left, Right: right}.Layout(gtx, func(gtx C) D {
					return glassPill(gtx, th, &c.chips[i], func(gtx C) D {
						return layout.Inset{Left: 11, Right: 9, Top: 7, Bottom: 7}.Layout(gtx, func(gtx C) D {
							return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
								layout.Rigid(Lbl(th, 13, th.Pal.Text, font.Medium, quickActions[i].label).Layout),
								layout.Rigid(Gap(4)),
								layout.Rigid(func(gtx C) D {
									s := gtx.Dp(13)
									gtx.Constraints = layout.Exact(image.Pt(s, s))
									return quickActions[i].icon().Layout(gtx, th.Pal.TextFaint)
								}),
							)
						})
					})
				})
			})
		})
	})
}

func (c *ChatScreen) messages(gtx C, u *UI, view app.ChatView) D {
	th := u.th
	if n := len(view.Messages); n != c.lastCount {
		if n > c.lastCount {
			c.list.ScrollToEnd = true
		}
		c.lastCount = n
	}
	c.list.Axis = layout.Vertical
	c.list.ScrollToEnd = true
	return layout.Stack{Alignment: layout.S}.Layout(gtx,
		layout.Expanded(func(gtx C) D {
			return material.List(th.Theme, &c.list).Layout(gtx, len(view.Messages)+1, func(gtx C, i int) D {
				if i == len(view.Messages) {
					return D{Size: image.Pt(0, gtx.Dp(8))}
				}
				m := view.Messages[i]
				last := i == len(view.Messages)-1
				top := unit.Dp(10)
				if m.Role == "assistant" {
					top = 18
				}
				st := c.state(m.ID)
				if st.lp.Update(gtx) && !(last && view.Streaming) {
					u.msgMenu.Show(m, last)
				}
				return layout.Inset{Left: 16, Right: 16, Top: top, Bottom: 4}.Layout(gtx, func(gtx C) D {
					gtx.Constraints.Max.X = min(gtx.Constraints.Max.X, gtx.Dp(760))
					return withLongPress(gtx, &st.lp, func(gtx C) D {
						if m.Role == "user" {
							return c.userBubble(gtx, th, m)
						}
						return c.assistant(gtx, u, view, m, last)
					})
				})
			})
		}),
		layout.Stacked(func(gtx C) D {
			if !c.list.Position.BeforeEnd {
				return D{}
			}
			return layout.Inset{Bottom: 8}.Layout(gtx, func(gtx C) D {
				return glassPill(gtx, th, &c.toBottom, func(gtx C) D {
					return layout.UniformInset(7).Layout(gtx, func(gtx C) D {
						s := gtx.Dp(20)
						gtx.Constraints = layout.Exact(image.Pt(s, s))
						return ic.ChevronDown.Layout(gtx, th.Pal.TextMuted)
					})
				})
			})
		}),
	)
}

func (c *ChatScreen) userBubble(gtx C, th *Theme, m app.Message) D {
	gtx.Constraints.Min.X = gtx.Constraints.Max.X // list items start at 0: give E room to align
	return layout.E.Layout(gtx, func(gtx C) D {
		gtx.Constraints.Max.X = gtx.Constraints.Max.X * 84 / 100
		return Surface{Bg: th.Pal.UserBubble, Border: th.Pal.Border, Radius: 20}.Layout(gtx, func(gtx C) D {
			return layout.Inset{Left: 16, Right: 16, Top: 10, Bottom: 10}.Layout(gtx, func(gtx C) D {
				l := Lbl(th, bodySize, th.Pal.Text, font.Normal, m.Content)
				l.LineHeightScale = 1.38
				return l.Layout(gtx)
			})
		})
	})
}

func (c *ChatScreen) assistant(gtx C, u *UI, view app.ChatView, m app.Message, last bool) D {
	th, a := u.th, u.a
	st := c.state(m.ID)
	streaming := last && view.Streaming
	if st.copy.Clicked(gtx) {
		gtx.Execute(clipboard.WriteCmd{Type: "application/text", Data: io.NopCloser(strings.NewReader(m.Content))})
		st.copiedAt = gtx.Now
		a.Notify("Copied to clipboard", false)
	}
	name := "SmolChat"
	if spec, ok := app.FindSpec(m.ModelID); ok {
		name = spec.Name
	}

	children := []layout.FlexChild{
		layout.Rigid(func(gtx C) D { // avatar + model name
			return layout.Inset{Bottom: 8}.Layout(gtx, func(gtx C) D {
				return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
					layout.Rigid(func(gtx C) D { return LogoTile(gtx, 24) }),
					layout.Rigid(Gap(10)),
					layout.Rigid(Lbl(th, 14, th.Pal.Text, font.SemiBold, name).Layout),
				)
			})
		}),
	}
	switch {
	case m.StopReason == "error":
		children = append(children, layout.Rigid(func(gtx C) D { return c.errorCard(gtx, u, m) }))
	case strings.TrimSpace(m.Content) == "" && streaming:
		children = append(children, layout.Rigid(func(gtx C) D { return c.pending(gtx, u) }))
	case strings.TrimSpace(m.Content) == "":
		children = append(children, layout.Rigid(Lbl(th, 14, th.Pal.TextFaint, font.Normal, "Stopped before replying.").Layout))
	default:
		st.md.Set(th, m.Content)
		children = append(children, layout.Rigid(func(gtx C) D { return st.md.Layout(gtx, th) }))
	}
	if streaming && strings.TrimSpace(m.Content) != "" {
		children = append(children, layout.Rigid(func(gtx C) D { return c.liveStats(gtx, th, view) }))
	}
	if !streaming && m.StopReason != "error" {
		children = append(children, layout.Rigid(func(gtx C) D { return c.actionRow(gtx, u, m, st, last) }))
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
}

func (c *ChatScreen) liveStats(gtx C, th *Theme, view app.ChatView) D {
	if view.StreamTokens < 2 {
		return D{}
	}
	tps := float64(view.StreamTokens) / max(0.001, time.Since(view.StreamStart).Seconds())
	return layout.Inset{Top: 8}.Layout(gtx, func(gtx C) D {
		return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
			layout.Rigid(func(gtx C) D { return TypingDots(gtx, th.Pal.Accent) }),
			layout.Rigid(Gap(10)),
			layout.Rigid(Lbl(th, 12, th.Pal.TextFaint, font.Medium, fmt.Sprintf("%.0f tok/s", tps)).Layout),
		)
	})
}

// pending shows model loading or the typing indicator before the first token.
func (c *ChatScreen) pending(gtx C, u *UI) D {
	th := u.th
	st := u.a.Runtime.Status()
	return layout.Inset{Top: 4, Bottom: 6}.Layout(gtx, func(gtx C) D {
		if st.Phase == app.PhaseLoading {
			spec, _ := app.FindSpec(st.ModelID)
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
				layout.Rigid(Lbl(th, 13.5, th.Pal.TextMuted, font.Medium,
					fmt.Sprintf("Loading %s · %d%%", spec.Name, int(st.Progress*100))).Layout),
				layout.Rigid(Gap(8)),
				layout.Rigid(func(gtx C) D {
					gtx.Constraints.Max.X = min(gtx.Constraints.Max.X, gtx.Dp(220))
					return ProgressBar(gtx, st.Progress, th.Pal.Accent, th.Pal.Elevated)
				}),
			)
		}
		return TypingDots(gtx, th.Pal.Accent)
	})
}

func (c *ChatScreen) errorCard(gtx C, u *UI, m app.Message) D {
	th := u.th
	return Surface{Bg: th.Pal.DangerBg, Border: withAlpha(th.Pal.Danger, 0x50), Radius: 14}.Layout(gtx, func(gtx C) D {
		gtx.Constraints.Min.X = gtx.Constraints.Max.X
		return layout.UniformInset(14).Layout(gtx, func(gtx C) D {
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
				layout.Rigid(func(gtx C) D {
					l := Lbl(th, 14, th.Pal.Text, font.Normal, m.Content)
					l.LineHeightScale = 1.35
					return l.Layout(gtx)
				}),
				layout.Rigid(Gap(10)),
				layout.Rigid(func(gtx C) D {
					return layout.Flex{}.Layout(gtx,
						layout.Rigid(func(gtx C) D {
							return Button{Kind: BtnSecondary, Text: "Retry", Icon: ic.Refresh, Small: true}.Layout(gtx, th, &c.regen)
						}),
						layout.Rigid(Gap(8)),
						layout.Rigid(func(gtx C) D {
							return Button{Kind: BtnGhost, Text: "Model Library", Small: true}.Layout(gtx, th, &c.browse)
						}),
					)
				}),
			)
		})
	})
}

func (c *ChatScreen) actionRow(gtx C, u *UI, m app.Message, st *msgState, last bool) D {
	th := u.th
	var parts []string
	if m.TokensPerSec > 0 {
		parts = append(parts, fmt.Sprintf("%.1f tok/s", m.TokensPerSec))
	}
	switch m.StopReason {
	case "cancelled":
		parts = append(parts, "stopped")
	case "length":
		parts = append(parts, "length limit")
	}
	if m.RouteReason != "" {
		parts = append(parts, "auto: "+m.RouteReason)
	}
	return layout.Inset{Top: 4}.Layout(gtx, func(gtx C) D {
		return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
			layout.Rigid(func(gtx C) D {
				icon := ic.Copy
				if gtx.Now.Sub(st.copiedAt) < 1500*time.Millisecond {
					icon = ic.Done
					gtx.Execute(op.InvalidateCmd{At: st.copiedAt.Add(1500 * time.Millisecond)})
				}
				return IconButtonSized(gtx, &st.copy, icon, th.Pal.TextFaint, 17, 36)
			}),
			layout.Rigid(func(gtx C) D {
				if !last {
					return D{}
				}
				return IconButtonSized(gtx, &c.regen, ic.Refresh, th.Pal.TextFaint, 18, 36)
			}),
			layout.Rigid(Gap(4)),
			layout.Flexed(1, func(gtx C) D {
				l := Lbl(th, 11.5, th.Pal.TextFaint, font.Normal, strings.Join(parts, " · "))
				l.MaxLines = 1
				l.Truncator = "…"
				return l.Layout(gtx)
			}),
		)
	})
}

// composer is the glass "Ask anything…" box.
func (c *ChatScreen) composer(gtx C, u *UI, view app.ChatView, installed []app.ModelSpec) D {
	th, a := u.th, u.a
	focused := gtx.Focused(&c.editor)
	st := a.Runtime.Status()
	busy := ""
	switch {
	case view.Streaming && st.Phase == app.PhaseLoading:
		busy = fmt.Sprintf("Loading model · %d%%", int(st.Progress*100))
	case view.Streaming:
		busy = "Generating…"
	}
	if c.composerLP.Update(gtx) {
		c.edit.open = true
		gtx.Execute(key.FocusCmd{Tag: &c.editor})
	}
	hintText := "Ask anything…"
	if len(installed) == 0 {
		hintText = "Download a model to start chatting"
	}
	return layout.Inset{Left: 12, Right: 12, Bottom: 6}.Layout(gtx, func(gtx C) D {
		return layout.Center.Layout(gtx, func(gtx C) D {
			gtx.Constraints.Max.X = min(gtx.Constraints.Max.X, gtx.Dp(760))
			gtx.Constraints.Min.X = gtx.Constraints.Max.X
			return withLongPress(gtx, &c.composerLP, func(gtx C) D { return c.composerBox(gtx, th, view, focused, busy, hintText) })
		})
	})
}

func (c *ChatScreen) composerBox(gtx C, th *Theme, view app.ChatView, focused bool, busy, hintText string) D {
	return layout.Background{}.Layout(gtx,
		func(gtx C) D {
			sz := gtx.Constraints.Min
			r := gtx.Dp(24)
			fillRRect(gtx, sz, r, withAlpha(th.Pal.Glass, 0xf0))
			edge := th.Pal.BorderStrong
			if focused || view.Streaming {
				edge = withAlpha(th.Pal.Accent, 0xb0)
			}
			strokeRRect(gtx, sz, r, float32(gtx.Dp(1.2)), edge)
			return D{Size: sz}
		},
		func(gtx C) D {
			return layout.Inset{Left: 16, Right: 10, Top: 14, Bottom: 10}.Layout(gtx, func(gtx C) D {
				return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
					layout.Rigid(func(gtx C) D {
						return layout.Flex{}.Layout(gtx,
							layout.Rigid(func(gtx C) D {
								return layout.Inset{Top: 2, Right: 10}.Layout(gtx, func(gtx C) D { return SparkleIcon(gtx, 17, th.Pal.Accent) })
							}),
							layout.Flexed(1, func(gtx C) D {
								gtx.Constraints.Max.Y = gtx.Dp(140)
								e := material.Editor(th.Theme, &c.editor, hintText)
								e.Color, e.HintColor = th.Pal.Text, th.Pal.TextFaint
								e.SelectionColor = withAlpha(th.Pal.Accent, 0x60)
								e.TextSize = 16
								e.LineHeightScale = 1.3
								return e.Layout(gtx)
							}),
						)
					}),
					layout.Rigid(Gap(6)),
					layout.Rigid(func(gtx C) D {
						return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
							layout.Flexed(1, func(gtx C) D {
								if busy == "" {
									return D{Size: image.Pt(gtx.Constraints.Min.X, 0)}
								}
								return layout.Inset{Left: 2}.Layout(gtx, Lbl(th, 12, th.Pal.AccentHi, font.Medium, busy).Layout)
							}),
							layout.Rigid(func(gtx C) D { return c.sendButton(gtx, th, view) }),
						)
					}),
				)
			})
		})
}

func (c *ChatScreen) sendButton(gtx C, th *Theme, view app.ChatView) D {
	enabled := view.Streaming || strings.TrimSpace(c.editor.Text()) != ""
	return c.send.Layout(gtx, func(gtx C) D {
		s := gtx.Dp(36)
		sz := image.Pt(s, s)
		func() {
			defer clip.UniformRRect(image.Rectangle{Max: sz}, s/2).Push(gtx.Ops).Pop()
			top, bot := th.Pal.Elevated, th.Pal.Elevated
			switch {
			case view.Streaming:
				top, bot = th.Pal.Primary, th.Pal.Primary
			case enabled:
				top, bot = mix(th.Pal.Accent, rgb(0xffffff), 0.18), mix(th.Pal.Accent, rgb(0x000000), 0.15)
			}
			paint.LinearGradientOp{Stop1: f32.Pt(0, 0), Color1: top, Stop2: f32.Pt(0, float32(s)), Color2: bot}.Add(gtx.Ops)
			paint.PaintOp{}.Add(gtx.Ops)
		}()
		pressOverlay(gtx, &c.send, sz, s/2)
		icon, fg, is := ic.Send, th.Pal.OnAccent, gtx.Dp(19)
		switch {
		case view.Streaming:
			icon, fg, is = ic.Stop, th.Pal.OnPrimary, gtx.Dp(17)
		case !enabled:
			fg = th.Pal.TextFaint
		}
		off := (s - is) / 2
		defer op.Offset(image.Pt(off, off)).Push(gtx.Ops).Pop()
		gtx.Constraints = layout.Exact(image.Pt(is, is))
		icon.Layout(gtx, fg)
		return D{Size: sz}
	})
}

// GlassCard is the Mac's frosted card surface.
func GlassCard(gtx C, th *Theme, w layout.Widget) D {
	return Surface{Bg: withAlpha(th.Pal.Glass, 0xeb), Border: th.Pal.BorderStrong, Radius: 18}.Layout(gtx, func(gtx C) D {
		gtx.Constraints.Min.X = gtx.Constraints.Max.X
		return layout.UniformInset(18).Layout(gtx, w)
	})
}
