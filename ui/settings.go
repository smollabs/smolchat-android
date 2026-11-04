package ui

import (
	"fmt"
	"image"
	"strings"

	"gioui.org/font"
	"gioui.org/layout"
	"gioui.org/widget"
	"gioui.org/widget/material"

	"github.com/smolworks/smolchat-android/app"
)

// segment is one option of a segmented control.
type segment struct {
	label string
	clk   widget.Clickable
}

func segmented(gtx C, th *Theme, segs []*segment, selected int) D {
	var children []layout.FlexChild
	for i, s := range segs {
		i, s := i, s
		children = append(children, layout.Flexed(1, func(gtx C) D {
			return s.clk.Layout(gtx, func(gtx C) D {
				return layout.Background{}.Layout(gtx,
					func(gtx C) D {
						sz := gtx.Constraints.Min
						if i == selected {
							fillRRect(gtx, sz, gtx.Dp(9), th.Pal.Elevated)
						}
						return D{Size: sz}
					},
					func(gtx C) D {
						gtx.Constraints.Min.X = gtx.Constraints.Max.X
						return layout.Inset{Top: 9, Bottom: 9}.Layout(gtx, func(gtx C) D {
							col := th.Pal.TextMuted
							if i == selected {
								col = th.Pal.Text
							}
							return centered(Lbl(th, 13.5, col, font.SemiBold, s.label)).Layout(gtx)
						})
					})
			})
		}))
	}
	return Surface{Bg: th.Pal.Bg, Border: th.Pal.Border, Radius: 12}.Layout(gtx, func(gtx C) D {
		return layout.UniformInset(3).Layout(gtx, func(gtx C) D { return layout.Flex{}.Layout(gtx, children...) })
	})
}

func segs(labels ...string) []*segment {
	out := make([]*segment, len(labels))
	for i, l := range labels {
		out[i] = &segment{label: l}
	}
	return out
}

// SettingsScreen edits preferences.
type SettingsScreen struct {
	back      widget.Clickable
	list      widget.List
	mode      []*segment
	length    []*segment
	ctx       []*segment
	temp      widget.Float
	sys       widget.Editor
	sysReset  widget.Clickable
	clear     widget.Clickable
	library   widget.Clickable
	models    map[string]*widget.Clickable
	loaded    bool
	lastTemp  float32
	sysLoaded string
}

var (
	lengthOpts = []int{256, 768, 2048}
	ctxOpts    = []int{1024, 2048, 4096}
)

func indexOf(opts []int, v int) int {
	for i, o := range opts {
		if o == v {
			return i
		}
	}
	return -1
}

// Leave saves edits that are applied lazily (the system prompt).
func (s *SettingsScreen) Leave(a *app.App) {
	if !s.loaded {
		return
	}
	if txt := strings.TrimSpace(s.sys.Text()); txt != s.sysLoaded {
		a.ApplySettings(func(st *app.Settings) { st.SystemPrompt = txt })
		s.sysLoaded = txt
	}
}

func (s *SettingsScreen) Layout(gtx C, u *UI) D {
	th, a := u.th, u.a
	st := a.Settings.Get()
	if !s.loaded {
		s.loaded = true
		s.mode = segs("Auto", "Manual")
		s.length = segs("Short", "Medium", "Long")
		s.ctx = segs("1K", "2K", "4K")
		s.models = map[string]*widget.Clickable{}
		s.temp.Value = st.Temperature / 1.5
		s.lastTemp = s.temp.Value
		s.sys.SetText(st.SystemPrompt)
		s.sysLoaded = st.SystemPrompt
	}
	if s.back.Clicked(gtx) {
		s.Leave(a)
		u.navigate(screenChat)
	}
	installed := a.Models.Installed()
	if s.mode[0].clk.Clicked(gtx) {
		a.SetSelection(app.SelectAuto, "")
	}
	if s.mode[1].clk.Clicked(gtx) && st.SelectionMode != app.SelectManual {
		id := st.ManualModelID
		if id == "" && len(installed) > 0 {
			id = installed[0].ID
		}
		if id == "" {
			a.Notify("Download a model to choose one manually", false)
		} else {
			a.SetSelection(app.SelectManual, id)
		}
	}
	for _, m := range installed {
		clk := s.models[m.ID]
		if clk == nil {
			clk = &widget.Clickable{}
			s.models[m.ID] = clk
		}
		if clk.Clicked(gtx) {
			a.SetSelection(app.SelectManual, m.ID)
		}
	}
	for i, sg := range s.length {
		if sg.clk.Clicked(gtx) {
			v := lengthOpts[i]
			a.ApplySettings(func(st *app.Settings) { st.MaxTokens = v })
		}
	}
	for i, sg := range s.ctx {
		if sg.clk.Clicked(gtx) {
			v := ctxOpts[i]
			a.ApplySettings(func(st *app.Settings) { st.ContextSize = v })
		}
	}
	s.temp.Update(gtx)
	if !s.temp.Dragging() && s.temp.Value != s.lastTemp { // save when released
		s.lastTemp = s.temp.Value
		v := float32(int(s.temp.Value*1.5*20+0.5)) / 20
		a.ApplySettings(func(st *app.Settings) { st.Temperature = v })
	}
	if s.sysReset.Clicked(gtx) {
		s.sys.SetText(app.DefaultSettings().SystemPrompt)
	}
	if s.clear.Clicked(gtx) {
		u.dialog.Confirm("Delete all chats?", "Every conversation on this device will be permanently removed.", "Delete all", true,
			func(string) { a.DeleteAllConversations(); a.Notify("All chats deleted", false) })
	}
	if s.library.Clicked(gtx) {
		u.navigate(screenLibrary)
	}

	modeSel := 0
	if st.SelectionMode == app.SelectManual {
		modeSel = 1
	}
	dev := a.Device()

	sections := []layout.Widget{
		func(gtx C) D { return Section(gtx, th, "MODEL SELECTION") },
		func(gtx C) D {
			return card(gtx, th, func(gtx C) D {
				return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
					layout.Rigid(func(gtx C) D { return segmented(gtx, th, s.mode, modeSel) }),
					layout.Rigid(Gap(10)),
					layout.Rigid(func(gtx C) D {
						txt := "Auto picks the best downloaded model for each message — small models for quick questions, larger ones for complex requests — based on this phone's memory."
						if modeSel == 1 {
							txt = "Manual keeps the model you pick for the whole conversation."
						}
						return hint(gtx, th, txt)
					}),
					layout.Rigid(func(gtx C) D {
						if modeSel != 1 {
							return D{}
						}
						if len(installed) == 0 {
							return layout.Inset{Top: 10}.Layout(gtx, func(gtx C) D {
								return Button{Kind: BtnSecondary, Text: "Open Model Library", Wide: true}.Layout(gtx, th, &s.library)
							})
						}
						var rows []layout.FlexChild
						for _, m := range installed {
							m := m
							rows = append(rows, layout.Rigid(func(gtx C) D {
								return radioRow(gtx, th, s.models[m.ID], m.Name, fmt.Sprintf("%s · %s", m.Quant, app.HumanBytes(m.DownloadSize())), st.ManualModelID == m.ID)
							}))
						}
						return layout.Inset{Top: 8}.Layout(gtx, func(gtx C) D { return layout.Flex{Axis: layout.Vertical}.Layout(gtx, rows...) })
					}),
				)
			})
		},
		func(gtx C) D { return Section(gtx, th, "GENERATION") },
		func(gtx C) D {
			return card(gtx, th, func(gtx C) D {
				return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
					layout.Rigid(func(gtx C) D { return settingLabel(gtx, th, "Response length", "") }),
					layout.Rigid(func(gtx C) D { return segmented(gtx, th, s.length, indexOf(lengthOpts, st.MaxTokens)) }),
					layout.Rigid(Gap(16)),
					layout.Rigid(func(gtx C) D {
						return settingLabel(gtx, th, "Context window", "Longer memory of the chat uses more RAM.")
					}),
					layout.Rigid(func(gtx C) D { return segmented(gtx, th, s.ctx, indexOf(ctxOpts, st.ContextSize)) }),
					layout.Rigid(Gap(16)),
					layout.Rigid(func(gtx C) D {
						v := float32(int(s.temp.Value*1.5*20+0.5)) / 20
						return settingLabel(gtx, th, fmt.Sprintf("Creativity · %.2f", v), "Lower is more focused, higher is more varied.")
					}),
					layout.Rigid(func(gtx C) D {
						sl := material.Slider(th.Theme, &s.temp)
						sl.Color = th.Pal.Accent
						return sl.Layout(gtx)
					}),
					layout.Rigid(Gap(12)),
					layout.Rigid(func(gtx C) D {
						return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
							layout.Flexed(1, func(gtx C) D { return settingLabel(gtx, th, "System prompt", "") }),
							layout.Rigid(func(gtx C) D { return Button{Kind: BtnGhost, Text: "Reset", Small: true}.Layout(gtx, th, &s.sysReset) }),
						)
					}),
					layout.Rigid(func(gtx C) D { return Field(gtx, th, &s.sys, "How should SmolChat behave?", nil) }),
				)
			})
		},
		func(gtx C) D { return Section(gtx, th, "STORAGE") },
		func(gtx C) D {
			convs, _ := a.Conversations()
			return card(gtx, th, func(gtx C) D {
				return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
					layout.Rigid(func(gtx C) D { return kv(gtx, th, "Downloaded models", app.HumanBytes(a.Models.InstalledBytes())) }),
					layout.Rigid(func(gtx C) D { return kv(gtx, th, "Free space", app.HumanBytes(dev.FreeStorage)) }),
					layout.Rigid(func(gtx C) D { return kv(gtx, th, "Conversations", fmt.Sprint(len(convs))) }),
					layout.Rigid(Gap(10)),
					layout.Rigid(func(gtx C) D {
						return Button{Kind: BtnDanger, Text: "Delete all chats", Icon: ic.Delete, Wide: true}.Layout(gtx, th, &s.clear)
					}),
				)
			})
		},
		func(gtx C) D { return Section(gtx, th, "PRIVACY") },
		func(gtx C) D {
			return card(gtx, th, func(gtx C) D {
				return layout.Flex{}.Layout(gtx,
					layout.Rigid(func(gtx C) D {
						sz := gtx.Dp(20)
						gtx.Constraints = layout.Exact(image.Pt(sz, sz))
						return ic.Shield.Layout(gtx, th.Pal.Success)
					}),
					layout.Rigid(Gap(12)),
					layout.Flexed(1, func(gtx C) D {
						return hint(gtx, th, "Everything runs on this device. Your messages are never sent anywhere. SmolChat only uses the internet to download models you choose from Hugging Face.")
					}),
				)
			})
		},
		func(gtx C) D { return Section(gtx, th, "ABOUT") },
		func(gtx C) D {
			return card(gtx, th, func(gtx C) D {
				return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
					layout.Rigid(func(gtx C) D { return kv(gtx, th, "SmolChat", app.AppVersion) }),
					layout.Rigid(func(gtx C) D { return kv(gtx, th, "Made by", app.OrgName) }),
					layout.Rigid(func(gtx C) D { return kv(gtx, th, "Inference", "llama.cpp (CPU)") }),
					layout.Rigid(func(gtx C) D { return kv(gtx, th, "Device", strings.TrimSpace(dev.Model+" "+dev.OS)) }),
					layout.Rigid(Gap(8)),
					layout.Rigid(func(gtx C) D {
						return hint(gtx, th, "Smol models by Hugging Face (HuggingFaceTB) and the community, under their own licenses. Inter and JetBrains Mono fonts under the SIL Open Font License.")
					}),
				)
			})
		},
		Gap(32),
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx C) D { return topBar(gtx, u, &s.back, "Settings") }),
		layout.Flexed(1, func(gtx C) D {
			s.list.Axis = layout.Vertical
			return material.List(th.Theme, &s.list).Layout(gtx, len(sections), func(gtx C, i int) D {
				return layout.Inset{Left: 16, Right: 16}.Layout(gtx, func(gtx C) D {
					gtx.Constraints.Max.X = min(gtx.Constraints.Max.X, gtx.Dp(720))
					return sections[i](gtx)
				})
			})
		}),
	)
}

func card(gtx C, th *Theme, w layout.Widget) D {
	return Surface{Bg: th.Pal.Surface, Border: th.Pal.Border, Radius: 18}.Layout(gtx, func(gtx C) D {
		gtx.Constraints.Min.X = gtx.Constraints.Max.X
		return layout.UniformInset(16).Layout(gtx, w)
	})
}

func hint(gtx C, th *Theme, s string) D {
	l := Lbl(th, 13, th.Pal.TextMuted, font.Normal, s)
	l.LineHeightScale = 1.35
	return l.Layout(gtx)
}

func settingLabel(gtx C, th *Theme, title, sub string) D {
	return layout.Inset{Bottom: 8}.Layout(gtx, func(gtx C) D {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(Lbl(th, 14.5, th.Pal.Text, font.Medium, title).Layout),
			layout.Rigid(func(gtx C) D {
				if sub == "" {
					return D{}
				}
				return layout.Inset{Top: 2}.Layout(gtx, Lbl(th, 12, th.Pal.TextFaint, font.Normal, sub).Layout)
			}),
		)
	})
}

func kv(gtx C, th *Theme, k, v string) D {
	return layout.Inset{Top: 6, Bottom: 6}.Layout(gtx, func(gtx C) D {
		return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
			layout.Flexed(1, Lbl(th, 14, th.Pal.TextMuted, font.Normal, k).Layout),
			layout.Rigid(Lbl(th, 14, th.Pal.Text, font.Medium, v).Layout),
		)
	})
}

func radioRow(gtx C, th *Theme, clk *widget.Clickable, title, sub string, on bool) D {
	return clk.Layout(gtx, func(gtx C) D {
		return layout.Background{}.Layout(gtx,
			func(gtx C) D {
				sz := gtx.Constraints.Min
				if on {
					fillRRect(gtx, sz, gtx.Dp(12), th.Pal.AccentSoft)
				}
				pressOverlay(gtx, clk, sz, gtx.Dp(12))
				return D{Size: sz}
			},
			func(gtx C) D {
				gtx.Constraints.Min.X = gtx.Constraints.Max.X
				return layout.Inset{Left: 12, Right: 12, Top: 10, Bottom: 10}.Layout(gtx, func(gtx C) D {
					return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
						layout.Flexed(1, func(gtx C) D {
							return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
								layout.Rigid(Lbl(th, 14.5, th.Pal.Text, font.Medium, title).Layout),
								layout.Rigid(func(gtx C) D {
									if sub == "" {
										return D{}
									}
									l := Lbl(th, 12, th.Pal.TextMuted, font.Normal, sub)
									l.MaxLines = 2
									return l.Layout(gtx)
								}),
							)
						}),
						layout.Rigid(func(gtx C) D {
							if !on {
								return D{}
							}
							sz := gtx.Dp(20)
							gtx.Constraints = layout.Exact(image.Pt(sz, sz))
							return ic.Check.Layout(gtx, th.Pal.AccentHi)
						}),
					)
				})
			})
	})
}
