package ui

import (
	"fmt"
	"image"
	"image/color"
	"strings"

	"gioui.org/font"
	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/widget"
	"gioui.org/widget/material"

	"github.com/smolworks/smolchat-android/app"
)

// segment is one option of a segmented control.
type segment struct {
	label string
	clk   widget.Clickable
}

func segs(labels ...string) []*segment {
	out := make([]*segment, len(labels))
	for i, l := range labels {
		out[i] = &segment{label: l}
	}
	return out
}

// segmented is the Mac's text-pill picker: options in a row, the selected
// one sitting on a soft filled pill.
func segmented(gtx C, th *Theme, sg []*segment, selected int) D {
	var children []layout.FlexChild
	for i, s := range sg {
		i, s := i, s
		children = append(children, layout.Rigid(func(gtx C) D {
			return s.clk.Layout(gtx, func(gtx C) D {
				return layout.Background{}.Layout(gtx,
					func(gtx C) D {
						sz := gtx.Constraints.Min
						if i == selected {
							fillRRect(gtx, sz, gtx.Dp(9), th.Pal.Elevated)
							strokeRRect(gtx, sz, gtx.Dp(9), float32(gtx.Dp(1)), th.Pal.Border)
						}
						pressOverlay(gtx, &s.clk, sz, gtx.Dp(9))
						return D{Size: sz}
					},
					func(gtx C) D {
						return layout.Inset{Left: 14, Right: 14, Top: 8, Bottom: 8}.Layout(gtx, func(gtx C) D {
							col := th.Pal.TextMuted
							if i == selected {
								col = th.Pal.Text
							}
							return Lbl(th, 14, col, font.Medium, s.label).Layout(gtx)
						})
					})
			})
		}))
	}
	return layout.Flex{Alignment: layout.Middle}.Layout(gtx, children...)
}

// SettingsScreen edits preferences.
type SettingsScreen struct {
	close      widget.Clickable
	list       widget.List
	themes     [3]widget.Clickable
	accents    [6]widget.Clickable
	pref       []*segment
	length     []*segment
	ctx        []*segment
	thinking   Toggle
	temp       widget.Float
	sys        widget.Editor
	sysReset   widget.Clickable
	library    widget.Clickable
	clearChats widget.Clickable
	loaded     bool
	lastTemp   float32
	sysLoaded  string
}

var (
	themeKeys  = []string{"system", "light", "dark"}
	prefKeys   = []string{"speed", "balanced", "quality"}
	lengthOpts = []int{512, 1024, 2048, 4096}
	ctxOpts    = []int{2048, 4096, 8192}
)

func indexOf[T comparable](opts []T, v T) int {
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

const tempMax = 1.5

func (s *SettingsScreen) Layout(gtx C, u *UI) D {
	th, a := u.th, u.a
	st := a.Settings.Get()
	if !s.loaded {
		s.loaded = true
		s.pref = segs("Speed", "Balanced", "Quality")
		s.length = segs("512", "1K", "2K", "4K")
		s.ctx = segs("2K", "4K", "8K")
		s.temp.Value = st.Temperature / tempMax
		s.lastTemp = s.temp.Value
		s.sys.SetText(st.SystemPrompt)
		s.sysLoaded = st.SystemPrompt
	}
	if s.close.Clicked(gtx) {
		u.pop(0)
	}
	for i := range s.themes {
		if s.themes[i].Clicked(gtx) {
			k := themeKeys[i]
			a.ApplySettings(func(st *app.Settings) { st.Theme = k })
		}
	}
	for i := range s.accents {
		if s.accents[i].Clicked(gtx) {
			k := Accents[i].Key
			a.ApplySettings(func(st *app.Settings) { st.Accent = k })
		}
	}
	for i, sg := range s.pref {
		if sg.clk.Clicked(gtx) {
			k := prefKeys[i]
			a.ApplySettings(func(st *app.Settings) { st.AutoPreference = k })
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
		v := float32(int(s.temp.Value*tempMax*20+0.5)) / 20
		a.ApplySettings(func(st *app.Settings) { st.Temperature = v })
	}
	if s.sysReset.Clicked(gtx) {
		s.sys.SetText(app.DefaultSettings().SystemPrompt)
	}
	if s.library.Clicked(gtx) {
		u.navigate(screenLibrary)
	}
	if s.clearChats.Clicked(gtx) {
		u.dialog.Confirm("Delete all chats?", "Every conversation on this phone will be permanently removed.", "Delete all", true,
			func(string) { a.DeleteAllConversations(); a.Notify("All chats deleted", false) })
	}
	dev := a.Device()
	acc := accentFor(st.Accent)

	sections := []layout.Widget{
		func(gtx C) D { return Section(gtx, th, "APPEARANCE") },
		func(gtx C) D { return s.themeCards(gtx, th, st.Theme, acc) },
		Gap(18),
		func(gtx C) D {
			return settingRow(gtx, th, "Accent colour", acc.Label, func(gtx C) D { return s.accentDots(gtx, th, st.Accent) })
		},

		func(gtx C) D { return Section(gtx, th, "MODEL SELECTION") },
		func(gtx C) D {
			return stacked(gtx, th, "Automatic mode prefers", "How Auto balances response speed against answer quality.",
				func(gtx C) D { return segmented(gtx, th, s.pref, indexOf(prefKeys, st.AutoPreference)) })
		},
		Gap(16),
		func(gtx C) D {
			return settingRow(gtx, th, "SmolLM3 thinking", "Let SmolLM3 reason step by step before answering. Slower, better on hard problems.",
				func(gtx C) D {
					d, changed := s.thinking.Layout(gtx, th, st.Thinking)
					if changed {
						on := !st.Thinking
						a.ApplySettings(func(st *app.Settings) { st.Thinking = on })
					}
					return d
				})
		},

		func(gtx C) D { return Section(gtx, th, "GENERATION") },
		func(gtx C) D {
			v := float32(int(s.temp.Value*tempMax*20+0.5)) / 20
			return stacked(gtx, th, fmt.Sprintf("Temperature  %.1f", v), "Lower is more focused, higher is more creative.", func(gtx C) D {
				sl := material.Slider(th.Theme, &s.temp)
				sl.Color = th.Pal.Accent
				return sl.Layout(gtx)
			})
		},
		Gap(12),
		func(gtx C) D {
			return stacked(gtx, th, "Maximum response length", "", func(gtx C) D {
				return segmented(gtx, th, s.length, indexOf(lengthOpts, st.MaxTokens))
			})
		},
		Gap(16),
		func(gtx C) D {
			return stacked(gtx, th, "Context window", "Larger windows remember more of the chat but use more memory.", func(gtx C) D {
				return segmented(gtx, th, s.ctx, indexOf(ctxOpts, st.ContextSize))
			})
		},
		Gap(16),
		func(gtx C) D {
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
				layout.Rigid(func(gtx C) D {
					return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
						layout.Flexed(1, Lbl(th, 15, th.Pal.Text, font.Normal, "System prompt").Layout),
						layout.Rigid(func(gtx C) D { return Button{Kind: BtnGhost, Text: "Reset", Small: true}.Layout(gtx, th, &s.sysReset) }),
					)
				}),
				layout.Rigid(Gap(6)),
				layout.Rigid(func(gtx C) D {
					gtx.Constraints.Min.Y = gtx.Dp(96)
					return Field(gtx, th, &s.sys, "How should SmolChat behave?", nil)
				}),
			)
		},

		func(gtx C) D { return Section(gtx, th, "STORAGE & PRIVACY") },
		func(gtx C) D {
			return stacked(gtx, th, "Models folder", a.Paths.Models, nil)
		},
		Gap(12),
		func(gtx C) D {
			return settingRow(gtx, th, "Manage models",
				fmt.Sprintf("%s downloaded · %s free", app.HumanBytes(a.Models.InstalledBytes()), app.HumanBytes(dev.FreeStorage)),
				func(gtx C) D {
					return Button{Kind: BtnSecondary, Text: "Model Library", Icon: ic.Library, Small: true}.Layout(gtx, th, &s.library)
				})
		},
		Gap(14),
		func(gtx C) D {
			return hint(gtx, th, "SmolChat has no telemetry and no cloud services. Chats are stored only in a local database on this phone, and the internet is used only to download models from Hugging Face.")
		},
		Gap(14),
		func(gtx C) D {
			return Button{Kind: BtnDanger, Text: "Delete all chats", Icon: ic.Delete, Small: true}.Layout(gtx, th, &s.clearChats)
		},

		func(gtx C) D { return Section(gtx, th, "ABOUT") },
		func(gtx C) D {
			parts := []string{dev.OS}
			if dev.Model != "" {
				parts = append(parts, dev.Model)
			}
			parts = append(parts, fmt.Sprintf("%.1f GB memory", float64(dev.TotalRAM)/(1<<30)), app.HumanBytes(dev.FreeStorage)+" free", "llama.cpp on CPU")
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
				layout.Rigid(Lbl(th, 14, th.Pal.Text, font.Normal, "SmolChat "+app.AppVersion+" · "+app.OrgName).Layout),
				layout.Rigid(Gap(4)),
				layout.Rigid(Lbl(th, 13, th.Pal.TextMuted, font.Normal, strings.Join(parts, " · ")).Layout),
				layout.Rigid(Gap(10)),
				layout.Rigid(func(gtx C) D {
					return hint(gtx, th, "Smol models by Hugging Face and the community, under their own licenses. Inter and JetBrains Mono under the SIL Open Font License.")
				}),
			)
		},
		Gap(36),
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx C) D { return sheetHeader(gtx, u, &s.close, "Settings", "", nil) }),
		layout.Rigid(func(gtx C) D { return Divider(gtx, th.Pal.Border) }),
		layout.Flexed(1, func(gtx C) D {
			s.list.Axis = layout.Vertical
			return material.List(th.Theme, &s.list).Layout(gtx, len(sections), func(gtx C, i int) D {
				return layout.Inset{Left: 20, Right: 20}.Layout(gtx, func(gtx C) D {
					gtx.Constraints.Max.X = min(gtx.Constraints.Max.X, gtx.Dp(720))
					gtx.Constraints.Min.X = gtx.Constraints.Max.X
					return sections[i](gtx)
				})
			})
		}),
	)
}

// settingRow puts a title/description on the left and a control on the right.
func settingRow(gtx C, th *Theme, title, sub string, control layout.Widget) D {
	return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
		layout.Flexed(1, func(gtx C) D {
			return layout.Inset{Right: 12}.Layout(gtx, func(gtx C) D { return titleSub(gtx, th, title, sub) })
		}),
		layout.Rigid(control),
	)
}

// stacked puts the control under its title (for wide controls on phones).
func stacked(gtx C, th *Theme, title, sub string, control layout.Widget) D {
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx C) D { return titleSub(gtx, th, title, sub) }),
		layout.Rigid(func(gtx C) D {
			if control == nil {
				return D{}
			}
			return layout.Inset{Top: 8}.Layout(gtx, control)
		}),
	)
}

func titleSub(gtx C, th *Theme, title, sub string) D {
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(Lbl(th, 15, th.Pal.Text, font.Normal, title).Layout),
		layout.Rigid(func(gtx C) D {
			if sub == "" {
				return D{}
			}
			return layout.Inset{Top: 3}.Layout(gtx, func(gtx C) D {
				l := Lbl(th, 12.5, th.Pal.TextFaint, font.Normal, sub)
				l.LineHeightScale = 1.3
				return l.Layout(gtx)
			})
		}),
	)
}

func hint(gtx C, th *Theme, s string) D {
	l := Lbl(th, 13, th.Pal.TextMuted, font.Normal, s)
	l.LineHeightScale = 1.35
	return l.Layout(gtx)
}

// themeCards draws the System / Light / Dark previews.
func (s *SettingsScreen) themeCards(gtx C, th *Theme, cur string, acc Accent) D {
	labels := []string{"System", "Light", "Dark"}
	iconsFor := []*widget.Icon{ic.Monitor, ic.Sun, ic.Moon}
	var children []layout.FlexChild
	for i := range labels {
		i := i
		if i > 0 {
			children = append(children, layout.Rigid(Gap(10)))
		}
		children = append(children, layout.Flexed(1, func(gtx C) D {
			sel := themeKeys[i] == cur
			return s.themes[i].Layout(gtx, func(gtx C) D {
				return layout.Flex{Axis: layout.Vertical, Alignment: layout.Middle}.Layout(gtx,
					layout.Rigid(func(gtx C) D {
						w := gtx.Constraints.Max.X
						h := w * 62 / 100
						sz := image.Pt(w, h)
						r := gtx.Dp(10)
						func() {
							defer clip.UniformRRect(image.Rectangle{Max: sz}, r).Push(gtx.Ops).Pop()
							light, dark := NewPalette(false, acc.Key), NewPalette(true, acc.Key)
							switch i {
							case 0: // half light, half dark
								miniWindow(gtx, image.Rect(0, 0, w, h), light, acc.Light)
								func() {
									defer clip.Rect{Min: image.Pt(w*45/100, 0), Max: sz}.Push(gtx.Ops).Pop()
									miniWindow(gtx, image.Rect(0, 0, w, h), dark, acc.Dark)
								}()
							case 1:
								miniWindow(gtx, image.Rect(0, 0, w, h), light, acc.Light)
							default:
								miniWindow(gtx, image.Rect(0, 0, w, h), dark, acc.Dark)
							}
						}()
						edge, ew := th.Pal.BorderStrong, float32(gtx.Dp(1))
						if sel {
							edge, ew = th.Pal.Accent, float32(gtx.Dp(2))
						}
						strokeRRect(gtx, sz, r, ew, edge)
						return D{Size: sz}
					}),
					layout.Rigid(Gap(8)),
					layout.Rigid(func(gtx C) D {
						col, w := th.Pal.TextMuted, font.Normal
						if sel {
							col, w = th.Pal.Text, font.SemiBold
						}
						return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
							layout.Rigid(func(gtx C) D {
								sz := gtx.Dp(15)
								gtx.Constraints = layout.Exact(image.Pt(sz, sz))
								return iconsFor[i].Layout(gtx, col)
							}),
							layout.Rigid(Gap(5)),
							layout.Rigid(Lbl(th, 14, col, w, labels[i]).Layout),
						)
					}),
				)
			})
		}))
	}
	return layout.Flex{}.Layout(gtx, children...)
}

// miniWindow paints a tiny SmolChat window in palette p within r.
func miniWindow(gtx C, r image.Rectangle, p Palette, accent color.NRGBA) {
	w, h := r.Dx(), r.Dy()
	paint.FillShape(gtx.Ops, p.Bg, clip.Rect(r).Op())
	paint.FillShape(gtx.Ops, p.Frame, clip.Rect{Max: image.Pt(w*26/100, h)}.Op())
	bar := func(x, y, bw, bh int, c color.NRGBA) {
		paint.FillShape(gtx.Ops, c, clip.UniformRRect(image.Rect(x, y, x+bw, y+bh), bh/2).Op(gtx.Ops))
	}
	line := withAlpha(p.TextFaint, 0x90)
	bar(w*5/100, h*14/100, w*15/100, h*5/100, line)
	bar(w*5/100, h*26/100, w*12/100, h*5/100, line)
	bar(w*5/100, h*38/100, w*14/100, h*5/100, line)
	bar(w*62/100, h*12/100, w*30/100, h*10/100, p.UserBubble)
	bar(w*34/100, h*34/100, w*50/100, h*5/100, line)
	bar(w*34/100, h*46/100, w*40/100, h*5/100, line)
	// composer with accent send dot
	bar(w*34/100, h*72/100, w*58/100, h*14/100, withAlpha(p.TextFaint, 0x40))
	d := h * 8 / 100
	paint.FillShape(gtx.Ops, accent, clip.Ellipse(image.Rect(w*86/100-d, h*79/100-d/2, w*86/100, h*79/100+d/2)).Op(gtx.Ops))
}

// accentDots is the row of accent swatches.
func (s *SettingsScreen) accentDots(gtx C, th *Theme, cur string) D {
	var children []layout.FlexChild
	for i, ac := range Accents {
		i, ac := i, ac
		children = append(children, layout.Rigid(func(gtx C) D {
			return layout.Inset{Left: 1}.Layout(gtx, func(gtx C) D {
				return s.accents[i].Layout(gtx, func(gtx C) D {
					d, g := gtx.Dp(28), gtx.Dp(4) // swatch and ring margin
					sz := image.Pt(d+2*g, d+2*g)
					col := ac.Light
					if th.Pal.Dark {
						col = ac.Dark
					}
					circle := func(inset int, c color.NRGBA) {
						paint.FillShape(gtx.Ops, c, clip.Ellipse(image.Rect(inset, inset, sz.X-inset, sz.Y-inset)).Op(gtx.Ops))
					}
					if ac.Key == cur { // ring: accent, then a gap in the page colour
						circle(0, col)
						circle(gtx.Dp(2), th.Pal.Bg)
					}
					circle(g, col)
					if ac.Key == cur {
						circle(g+gtx.Dp(9), rgb(0xffffff))
					}
					return D{Size: sz}
				})
			})
		}))
	}
	return layout.Flex{Alignment: layout.Middle}.Layout(gtx, children...)
}
