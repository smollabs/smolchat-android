package ui

import (
	"fmt"
	"image"
	"io"
	"strings"
	"time"

	"gioui.org/font"
	"gioui.org/io/clipboard"
	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
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
}

// ChatScreen is the conversation view with its composer.
type ChatScreen struct {
	menu, newChat, title widget.Clickable
	list                 widget.List
	editor               widget.Editor
	send                 widget.Clickable
	regen                widget.Clickable
	toBottom             widget.Clickable
	modelChip            widget.Clickable
	getStarted           widget.Clickable
	browse               widget.Clickable
	suggest              [4]widget.Clickable
	msgs                 map[int64]*msgState
	convID               string
	focusComposer        bool
	lastCount            int
}

var suggestions = [4][2]string{
	{"Explain something", "Explain how black holes form, simply"},
	{"Write", "Write a short poem about the monsoon"},
	{"Plan", "Help me plan a relaxing weekend at home"},
	{"Code", "Write a Python function that checks if a word is a palindrome"},
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

func (c *ChatScreen) Layout(gtx C, u *UI) D {
	a := u.a
	view := a.Chat()
	if view.Conv.ID != c.convID { // switched conversations: drop cached widgets
		c.convID = view.Conv.ID
		c.msgs = nil
		c.list.Position = layout.Position{BeforeEnd: false}
		c.list.ScrollToEnd = true
		c.lastCount = 0
	}
	installed := a.Models.Installed()

	// Header actions.
	if c.menu.Clicked(gtx) {
		u.drawer.Open()
	}
	if c.newChat.Clicked(gtx) {
		a.NewChat()
		c.focusComposer = true
	}
	if c.title.Clicked(gtx) || c.modelChip.Clicked(gtx) {
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
	for i := range c.suggest {
		if c.suggest[i].Clicked(gtx) {
			c.submit(u, suggestions[i][1], installed)
		}
	}
	if c.regen.Clicked(gtx) {
		a.Regenerate()
	}
	if c.toBottom.Clicked(gtx) {
		c.list.ScrollTo(len(view.Messages))
		c.list.ScrollToEnd = true
	}

	// Composer events.
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

	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx C) D { return c.header(gtx, u, view) }),
		layout.Flexed(1, func(gtx C) D {
			if len(view.Messages) == 0 {
				return c.welcome(gtx, u, installed)
			}
			return c.messages(gtx, u, view)
		}),
		layout.Rigid(func(gtx C) D { return c.composer(gtx, u, view, installed) }),
	)
}

func (c *ChatScreen) submit(u *UI, text string, installed []app.ModelSpec) {
	text = strings.TrimSpace(text)
	if text == "" {
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
	u.a.Send(text)
}

// modelLabel describes the current selection, e.g. "Auto · SmolLM2 360M".
func modelLabel(a *app.App, conv app.Conversation) (mode, model string) {
	if conv.SelectionMode == app.SelectManual {
		if spec, ok := app.FindSpec(conv.ModelID); ok {
			return "Manual", spec.Name
		}
		return "Manual", "No model"
	}
	st := a.Runtime.Status()
	if spec, ok := app.FindSpec(st.ModelID); ok && st.Phase != app.PhaseIdle {
		return "Auto", spec.Name
	}
	if spec, ok := app.FindSpec(conv.ModelID); ok && a.Models.IsInstalled(spec.ID) {
		return "Auto", spec.Name
	}
	return "Auto", "Best model per message"
}

func (c *ChatScreen) header(gtx C, u *UI, view app.ChatView) D {
	th := u.th
	mode, model := modelLabel(u.a, view.Conv)
	return layout.Inset{Top: u.insets.Top}.Layout(gtx, func(gtx C) D {
		return layout.Inset{Left: 4, Right: 4, Top: 4, Bottom: 4}.Layout(gtx, func(gtx C) D {
			return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
				layout.Rigid(func(gtx C) D { return IconButton(gtx, &c.menu, ic.Menu, th.Pal.Text) }),
				layout.Flexed(1, func(gtx C) D {
					return layout.Center.Layout(gtx, func(gtx C) D {
						return c.title.Layout(gtx, func(gtx C) D {
							return layout.Inset{Left: 10, Right: 6, Top: 4, Bottom: 4}.Layout(gtx, func(gtx C) D {
								return layout.Flex{Axis: layout.Vertical, Alignment: layout.Middle}.Layout(gtx,
									layout.Rigid(func(gtx C) D {
										return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
											layout.Rigid(Lbl(th, 17, th.Pal.Text, font.SemiBold, "SmolChat").Layout),
											layout.Rigid(func(gtx C) D {
												s := gtx.Dp(20)
												gtx.Constraints = layout.Exact(image.Pt(s, s))
												return ic.ChevronDown.Layout(gtx, th.Pal.TextMuted)
											}),
										)
									}),
									layout.Rigid(func(gtx C) D {
										l := Lbl(th, 12, th.Pal.TextMuted, font.Medium, mode+" · "+model)
										l.MaxLines = 1
										return l.Layout(gtx)
									}),
								)
							})
						})
					})
				}),
				layout.Rigid(func(gtx C) D { return IconButton(gtx, &c.newChat, ic.NewChat, th.Pal.Text) }),
			)
		})
	})
}

func (c *ChatScreen) welcome(gtx C, u *UI, installed []app.ModelSpec) D {
	th := u.th
	return layout.Center.Layout(gtx, func(gtx C) D {
		return layout.Inset{Left: 20, Right: 20}.Layout(gtx, func(gtx C) D {
			gtx.Constraints.Max.X = min(gtx.Constraints.Max.X, gtx.Dp(460))
			return layout.Flex{Axis: layout.Vertical, Alignment: layout.Middle}.Layout(gtx,
				layout.Rigid(func(gtx C) D { return Logo(gtx, 52, false) }),
				layout.Rigid(Gap(18)),
				layout.Rigid(centered(Lbl(th, 23, th.Pal.Text, font.SemiBold, "What can I help with?")).Layout),
				layout.Rigid(Gap(6)),
				layout.Rigid(func(gtx C) D {
					return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
						layout.Rigid(func(gtx C) D {
							s := gtx.Dp(13)
							gtx.Constraints = layout.Exact(image.Pt(s, s))
							return ic.Lock.Layout(gtx, th.Pal.TextFaint)
						}),
						layout.Rigid(Gap(5)),
						layout.Rigid(Lbl(th, 13, th.Pal.TextFaint, font.Normal, "Private and offline — runs entirely on this phone").Layout),
					)
				}),
				layout.Rigid(Gap(28)),
				layout.Rigid(func(gtx C) D {
					if len(installed) == 0 {
						return c.firstRun(gtx, u)
					}
					return c.suggestionGrid(gtx, th)
				}),
			)
		})
	})
}

func (c *ChatScreen) firstRun(gtx C, u *UI) D {
	th, a := u.th, u.a
	d := a.Device()
	rec := app.Recommend(d, a.Settings.Get().ContextSize)
	pr := a.Downloads.Progress(rec)
	return Surface{Bg: th.Pal.Surface, Border: th.Pal.Border, Radius: 20}.Layout(gtx, func(gtx C) D {
		gtx.Constraints.Min.X = gtx.Constraints.Max.X
		return layout.UniformInset(20).Layout(gtx, func(gtx C) D {
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
				layout.Rigid(Lbl(th, 16.5, th.Pal.Text, font.SemiBold, "Download your first model").Layout),
				layout.Rigid(Gap(6)),
				layout.Rigid(Lbl(th, 14, th.Pal.TextMuted, font.Normal,
					"SmolChat ships without model weights. Download one once, then chat anywhere — no internet needed.").Layout),
				layout.Rigid(Gap(16)),
				layout.Rigid(func(gtx C) D {
					return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
						layout.Rigid(func(gtx C) D { return familyBadge(gtx, th, rec) }),
						layout.Rigid(Gap(12)),
						layout.Flexed(1, func(gtx C) D {
							return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
								layout.Rigid(Lbl(th, 14.5, th.Pal.Text, font.SemiBold, rec.Name).Layout),
								layout.Rigid(Lbl(th, 12.5, th.Pal.TextMuted, font.Normal,
									fmt.Sprintf("Recommended for this phone · %s", app.HumanBytes(rec.DownloadSize()))).Layout),
							)
						}),
					)
				}),
				layout.Rigid(Gap(16)),
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
					return Button{Kind: BtnPrimary, Text: "Download " + app.HumanBytes(rec.DownloadSize()), Icon: ic.Download, Wide: true}.Layout(gtx, th, &c.getStarted)
				}),
				layout.Rigid(Gap(8)),
				layout.Rigid(func(gtx C) D {
					return Button{Kind: BtnGhost, Text: "Browse all models", Wide: true}.Layout(gtx, th, &c.browse)
				}),
			)
		})
	})
}

func (c *ChatScreen) suggestionGrid(gtx C, th *Theme) D {
	cell := func(i int) layout.FlexChild {
		return layout.Flexed(1, func(gtx C) D {
			clk := &c.suggest[i]
			return clk.Layout(gtx, func(gtx C) D {
				return Surface{Bg: th.Pal.Surface, Border: th.Pal.Border, Radius: 16}.Layout(gtx, func(gtx C) D {
					gtx.Constraints.Min.X = gtx.Constraints.Max.X
					gtx.Constraints.Min.Y = gtx.Dp(76)
					return layout.Stack{}.Layout(gtx,
						layout.Stacked(func(gtx C) D {
							return layout.UniformInset(14).Layout(gtx, func(gtx C) D {
								return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
									layout.Rigid(Lbl(th, 13.5, th.Pal.Text, font.SemiBold, suggestions[i][0]).Layout),
									layout.Rigid(Gap(3)),
									layout.Rigid(func(gtx C) D {
										l := Lbl(th, 12.5, th.Pal.TextMuted, font.Normal, suggestions[i][1])
										l.MaxLines = 2
										l.Truncator = "…"
										return l.Layout(gtx)
									}),
								)
							})
						}),
						layout.Expanded(func(gtx C) D {
							pressOverlay(gtx, clk, gtx.Constraints.Min, gtx.Dp(16))
							return D{Size: gtx.Constraints.Min}
						}),
					)
				})
			})
		})
	}
	row := func(i int) layout.FlexChild {
		return layout.Rigid(func(gtx C) D {
			return layout.Flex{}.Layout(gtx, cell(i), layout.Rigid(Gap(10)), cell(i+1))
		})
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx, row(0), layout.Rigid(Gap(10)), row(2))
}

func (c *ChatScreen) messages(gtx C, u *UI, view app.ChatView) D {
	th := u.th
	if n := len(view.Messages); n != c.lastCount {
		if n > c.lastCount { // a new message: follow it
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
					return D{Size: image.Pt(0, gtx.Dp(12))}
				}
				m := view.Messages[i]
				last := i == len(view.Messages)-1
				return layout.Inset{Left: 16, Right: 16, Top: 10, Bottom: 6}.Layout(gtx, func(gtx C) D {
					gtx.Constraints.Max.X = min(gtx.Constraints.Max.X, gtx.Dp(760))
					if m.Role == "user" {
						return c.userBubble(gtx, th, m)
					}
					return c.assistant(gtx, u, view, m, last)
				})
			})
		}),
		layout.Stacked(func(gtx C) D {
			if !c.list.Position.BeforeEnd {
				return D{}
			}
			return layout.Inset{Bottom: 10}.Layout(gtx, func(gtx C) D {
				return c.toBottom.Layout(gtx, func(gtx C) D {
					return Surface{Bg: th.Pal.Elevated, Border: th.Pal.BorderStrong, Radius: 18}.Layout(gtx, func(gtx C) D {
						return layout.UniformInset(7).Layout(gtx, func(gtx C) D {
							s := gtx.Dp(22)
							gtx.Constraints = layout.Exact(image.Pt(s, s))
							return ic.ChevronDown.Layout(gtx, th.Pal.Text)
						})
					})
				})
			})
		}),
	)
}

func (c *ChatScreen) userBubble(gtx C, th *Theme, m app.Message) D {
	gtx.Constraints.Min.X = gtx.Constraints.Max.X // list items start at 0: give E room to align
	return layout.E.Layout(gtx, func(gtx C) D {
		gtx.Constraints.Max.X = gtx.Constraints.Max.X * 82 / 100
		return Surface{Bg: th.Pal.UserBubble, Radius: 20}.Layout(gtx, func(gtx C) D {
			return layout.Inset{Left: 15, Right: 15, Top: 10, Bottom: 10}.Layout(gtx, func(gtx C) D {
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

	var children []layout.FlexChild
	switch {
	case m.StopReason == "error":
		children = append(children, layout.Rigid(func(gtx C) D { return c.errorCard(gtx, u, m) }))
	case strings.TrimSpace(m.Content) == "" && streaming:
		children = append(children, layout.Rigid(func(gtx C) D { return c.pending(gtx, u, view) }))
	default:
		st.md.Set(th, m.Content)
		children = append(children, layout.Rigid(func(gtx C) D { return st.md.Layout(gtx, th) }))
	}
	if !streaming && m.StopReason != "error" {
		children = append(children, layout.Rigid(func(gtx C) D { return c.actionRow(gtx, u, m, st, last) }))
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
}

// pending shows model loading or the typing indicator before the first token.
func (c *ChatScreen) pending(gtx C, u *UI, view app.ChatView) D {
	th := u.th
	st := u.a.Runtime.Status()
	return layout.Inset{Top: 6, Bottom: 6}.Layout(gtx, func(gtx C) D {
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
		return TypingDots(gtx, th.Pal.TextMuted)
	})
}

func (c *ChatScreen) errorCard(gtx C, u *UI, m app.Message) D {
	th := u.th
	return Surface{Bg: withAlpha(th.Pal.Danger, 0x14), Border: withAlpha(th.Pal.Danger, 0x40), Radius: 14}.Layout(gtx, func(gtx C) D {
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
	meta := ""
	if spec, ok := app.FindSpec(m.ModelID); ok {
		meta = spec.Name
	}
	if m.TokensPerSec > 0 {
		meta += fmt.Sprintf(" · %.1f tok/s", m.TokensPerSec)
	}
	switch m.StopReason {
	case "cancelled":
		meta += " · stopped"
	case "length":
		meta += " · length limit"
	}
	if m.RouteReason != "" {
		meta += " · auto: " + m.RouteReason
	}
	return layout.Inset{Top: 4}.Layout(gtx, func(gtx C) D {
		return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
			layout.Rigid(func(gtx C) D {
				icon := ic.Copy
				if gtx.Now.Sub(st.copiedAt) < 1500*time.Millisecond {
					icon = ic.Done
					gtx.Execute(op.InvalidateCmd{At: st.copiedAt.Add(1500 * time.Millisecond)})
				}
				return IconButtonSized(gtx, &st.copy, icon, th.Pal.TextMuted, 17, 36)
			}),
			layout.Rigid(func(gtx C) D {
				if !last {
					return D{}
				}
				return IconButtonSized(gtx, &c.regen, ic.Refresh, th.Pal.TextMuted, 18, 36)
			}),
			layout.Rigid(Gap(4)),
			layout.Flexed(1, func(gtx C) D {
				l := Lbl(th, 11.5, th.Pal.TextFaint, font.Normal, strings.TrimPrefix(meta, " · "))
				l.MaxLines = 1
				l.Truncator = "…"
				return l.Layout(gtx)
			}),
		)
	})
}

func (c *ChatScreen) composer(gtx C, u *UI, view app.ChatView, installed []app.ModelSpec) D {
	th, a := u.th, u.a
	mode, model := modelLabel(a, view.Conv)
	status, statusCol := "", th.Pal.TextFaint
	st := a.Runtime.Status()
	switch {
	case view.Streaming && st.Phase == app.PhaseLoading:
		status, statusCol = fmt.Sprintf("Loading model %d%%", int(st.Progress*100)), th.Pal.Warning
	case view.Streaming && view.StreamTokens > 1:
		tps := float64(view.StreamTokens) / max(0.001, time.Since(view.StreamStart).Seconds())
		status, statusCol = fmt.Sprintf("Generating · %.0f tok/s", tps), th.Pal.AccentHi
	case view.Streaming:
		status, statusCol = "Thinking…", th.Pal.AccentHi
	case len(installed) == 0:
		status = "No model downloaded"
	default:
		status = "On-device · offline"
	}

	return layout.Inset{Left: 12, Right: 12, Bottom: 10, Top: 4}.Layout(gtx, func(gtx C) D {
		return layout.Center.Layout(gtx, func(gtx C) D {
			gtx.Constraints.Max.X = min(gtx.Constraints.Max.X, gtx.Dp(760))
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
				// Model + status line.
				layout.Rigid(func(gtx C) D {
					return layout.Inset{Left: 4, Right: 6, Bottom: 6}.Layout(gtx, func(gtx C) D {
						return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
							layout.Rigid(func(gtx C) D {
								return c.modelChip.Layout(gtx, func(gtx C) D {
									return Surface{Bg: th.Pal.Surface2, Border: th.Pal.Border, Radius: 14}.Layout(gtx, func(gtx C) D {
										return layout.Inset{Left: 10, Right: 6, Top: 5, Bottom: 5}.Layout(gtx, func(gtx C) D {
											return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
												layout.Rigid(func(gtx C) D {
													s := gtx.Dp(14)
													gtx.Constraints = layout.Exact(image.Pt(s, s))
													icon := ic.Bolt
													if mode == "Manual" {
														icon = ic.Memory
													}
													return icon.Layout(gtx, th.Pal.AccentHi)
												}),
												layout.Rigid(Gap(6)),
												layout.Rigid(func(gtx C) D {
													gtx.Constraints.Max.X = gtx.Dp(190)
													l := Lbl(th, 12.5, th.Pal.Text, font.Medium, mode+" · "+model)
													l.MaxLines = 1
													l.Truncator = "…"
													return l.Layout(gtx)
												}),
												layout.Rigid(func(gtx C) D {
													s := gtx.Dp(16)
													gtx.Constraints = layout.Exact(image.Pt(s, s))
													return ic.ChevronDown.Layout(gtx, th.Pal.TextMuted)
												}),
											)
										})
									})
								})
							}),
							layout.Flexed(1, func(gtx C) D {
								return layout.E.Layout(gtx, func(gtx C) D {
									l := Lbl(th, 12, statusCol, font.Medium, status)
									l.MaxLines = 1
									l.Alignment = text.End
									return l.Layout(gtx)
								})
							}),
						)
					})
				}),
				// Input pill.
				layout.Rigid(func(gtx C) D {
					return Surface{Bg: th.Pal.Surface2, Border: th.Pal.BorderStrong, Radius: 24}.Layout(gtx, func(gtx C) D {
						return layout.Inset{Left: 18, Right: 6, Top: 6, Bottom: 6}.Layout(gtx, func(gtx C) D {
							return layout.Flex{Alignment: layout.End}.Layout(gtx,
								layout.Flexed(1, func(gtx C) D {
									return layout.Inset{Top: 8, Bottom: 8, Right: 8}.Layout(gtx, func(gtx C) D {
										gtx.Constraints.Max.Y = gtx.Dp(150)
										e := material.Editor(th.Theme, &c.editor, "Message SmolChat")
										e.Color, e.HintColor = th.Pal.Text, th.Pal.TextFaint
										e.SelectionColor = withAlpha(th.Pal.Accent, 0x60)
										e.TextSize = 16
										e.LineHeightScale = 1.3
										return e.Layout(gtx)
									})
								}),
								layout.Rigid(func(gtx C) D { return c.sendButton(gtx, th, view) }),
							)
						})
					})
				}),
			)
		})
	})
}

func (c *ChatScreen) sendButton(gtx C, th *Theme, view app.ChatView) D {
	empty := strings.TrimSpace(c.editor.Text()) == ""
	bg, fg := th.Pal.Text, th.Pal.Bg
	icon := ic.Send
	if view.Streaming {
		icon = ic.Stop
	} else if empty {
		bg, fg = th.Pal.Elevated, th.Pal.TextFaint
	}
	return c.send.Layout(gtx, func(gtx C) D {
		s := gtx.Dp(38)
		sz := image.Pt(s, s)
		paint := func() {
			fillRRect(gtx, sz, s/2, bg)
			pressOverlay(gtx, &c.send, sz, s/2)
		}
		paint()
		is := gtx.Dp(20)
		if view.Streaming {
			is = gtx.Dp(18)
		}
		off := (s - is) / 2
		defer op.Offset(image.Pt(off, off)).Push(gtx.Ops).Pop()
		gtx.Constraints = layout.Exact(image.Pt(is, is))
		icon.Layout(gtx, fg)
		return D{Size: sz}
	})
}

// familyBadge is a small rounded tile with the model family initials.
func familyBadge(gtx C, th *Theme, m app.ModelSpec) D {
	s := gtx.Dp(40)
	sz := image.Pt(s, s)
	tint := th.Pal.Accent
	switch m.Family {
	case "SmolLM":
		tint = rgb(0x6fb6ff)
	case "SmolLM3":
		tint = rgb(0xf3a35a)
	case "SmolVLM", "SmolVLM2":
		tint = rgb(0x4fd0b0)
	case "SmolDocling":
		tint = rgb(0xf37aa8)
	case "ColSmol":
		tint = rgb(0xc9c96a)
	}
	fillRRect(gtx, sz, gtx.Dp(12), withAlpha(tint, 0x2c))
	label := m.Params
	defer clip.Rect{Max: sz}.Push(gtx.Ops).Pop()
	gtx.Constraints = layout.Exact(sz)
	layout.Center.Layout(gtx, func(gtx C) D {
		return Lbl(th, unit.Sp(12), tint, font.Bold, label).Layout(gtx)
	})
	return D{Size: sz}
}
