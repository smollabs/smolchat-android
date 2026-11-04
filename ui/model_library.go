package ui

import (
	"fmt"
	"image"
	"image/color"
	"strings"

	"gioui.org/font"
	"gioui.org/layout"
	"gioui.org/widget"
	"gioui.org/widget/material"

	"github.com/smolworks/smolchat-android/app"
)

type modelCard struct {
	primary, cancel, pause, del widget.Clickable
}

// Library is the model management screen.
type Library struct {
	back  widget.Clickable
	tabs  [4]widget.Clickable
	cat   app.Category
	list  widget.List
	cards map[string]*modelCard
}

func (l *Library) card(id string) *modelCard {
	if l.cards == nil {
		l.cards = map[string]*modelCard{}
	}
	c := l.cards[id]
	if c == nil {
		c = &modelCard{}
		l.cards[id] = c
	}
	return c
}

func (l *Library) Layout(gtx C, u *UI) D {
	th, a := u.th, u.a
	if l.cat == "" {
		l.cat = app.CategoryText
	}
	if l.back.Clicked(gtx) {
		u.navigate(screenChat)
	}
	for i := range l.tabs {
		if l.tabs[i].Clicked(gtx) {
			l.cat = app.Categories[i]
			l.list.Position = layout.Position{}
		}
	}

	settings := a.Settings.Get()
	dev := a.Device()
	var specs []app.ModelSpec
	for _, m := range app.Catalog {
		if m.Category == l.cat {
			specs = append(specs, m)
		}
	}

	// Rows: device card, tabs, then cards with an "Advanced" divider.
	type row struct {
		kind string
		spec app.ModelSpec
	}
	rows := []row{{kind: "device"}, {kind: "tabs"}}
	if l.cat != app.CategoryText {
		rows = append(rows, row{kind: "note"})
	}
	advancedShown := false
	for _, m := range specs {
		if m.Advanced && !advancedShown && l.cat == app.CategoryText {
			rows = append(rows, row{kind: "advanced"})
			advancedShown = true
		}
		rows = append(rows, row{kind: "model", spec: m})
	}
	rows = append(rows, row{kind: "footer"})

	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx C) D { return topBar(gtx, u, &l.back, "Model Library") }),
		layout.Flexed(1, func(gtx C) D {
			l.list.Axis = layout.Vertical
			return material.List(th.Theme, &l.list).Layout(gtx, len(rows), func(gtx C, i int) D {
				r := rows[i]
				return layout.Inset{Left: 16, Right: 16, Bottom: 12}.Layout(gtx, func(gtx C) D {
					gtx.Constraints.Max.X = min(gtx.Constraints.Max.X, gtx.Dp(720))
					switch r.kind {
					case "device":
						return deviceCard(gtx, u, dev)
					case "tabs":
						return l.tabBar(gtx, th)
					case "note":
						return infoNote(gtx, th, categoryNote(l.cat))
					case "advanced":
						return Section(gtx, th, "ADVANCED · LARGE DOWNLOADS")
					case "footer":
						return layout.Inset{Top: 4, Bottom: 24}.Layout(gtx, func(gtx C) D {
							return centered(Lbl(th, 12, th.Pal.TextFaint, font.Normal,
								"Models download from Hugging Face over HTTPS and are verified with SHA-256. Licenses belong to their publishers.")).Layout(gtx)
						})
					}
					return l.modelCard(gtx, u, r.spec, dev, settings.ContextSize)
				})
			})
		}),
	)
}

func categoryNote(c app.Category) string {
	switch c {
	case app.CategoryVision:
		return "SmolVLM models have verified GGUF builds, but image input isn't available in this version of SmolChat yet, so they can't be downloaded."
	case app.CategoryDocument:
		return "SmolDocling converts document images to structured text. It has no official GGUF build and needs an image workflow SmolChat doesn't have yet."
	case app.CategoryEmbedding:
		return "ColSmol models produce multi-vector document embeddings. There's no llama.cpp runtime for them, so on-device retrieval isn't available yet."
	}
	return ""
}

func infoNote(gtx C, th *Theme, s string) D {
	return Surface{Bg: th.Pal.Surface, Border: th.Pal.Border, Radius: 14}.Layout(gtx, func(gtx C) D {
		gtx.Constraints.Min.X = gtx.Constraints.Max.X
		return layout.UniformInset(14).Layout(gtx, func(gtx C) D {
			return layout.Flex{}.Layout(gtx,
				layout.Rigid(func(gtx C) D {
					sz := gtx.Dp(18)
					gtx.Constraints = layout.Exact(image.Pt(sz, sz))
					return ic.Info.Layout(gtx, th.Pal.TextMuted)
				}),
				layout.Rigid(Gap(10)),
				layout.Flexed(1, func(gtx C) D {
					l := Lbl(th, 13.5, th.Pal.TextMuted, font.Normal, s)
					l.LineHeightScale = 1.35
					return l.Layout(gtx)
				}),
			)
		})
	})
}

// topBar is the back-arrow header used by secondary screens.
func topBar(gtx C, u *UI, back *widget.Clickable, title string) D {
	th := u.th
	return layout.Inset{Top: u.insets.Top}.Layout(gtx, func(gtx C) D {
		return layout.Inset{Left: 4, Right: 8, Top: 4, Bottom: 8}.Layout(gtx, func(gtx C) D {
			return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
				layout.Rigid(func(gtx C) D { return IconButton(gtx, back, ic.Back, th.Pal.Text) }),
				layout.Rigid(Gap(6)),
				layout.Flexed(1, Lbl(th, 19, th.Pal.Text, font.SemiBold, title).Layout),
			)
		})
	})
}

func deviceCard(gtx C, u *UI, d app.DeviceInfo) D {
	th := u.th
	online := u.a.Online()
	stat := func(icon *widget.Icon, label, value string) layout.FlexChild {
		return layout.Flexed(1, func(gtx C) D {
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
				layout.Rigid(func(gtx C) D {
					return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
						layout.Rigid(func(gtx C) D {
							s := gtx.Dp(14)
							gtx.Constraints = layout.Exact(image.Pt(s, s))
							return icon.Layout(gtx, th.Pal.TextFaint)
						}),
						layout.Rigid(Gap(5)),
						layout.Rigid(Lbl(th, 11.5, th.Pal.TextFaint, font.Medium, label).Layout),
					)
				}),
				layout.Rigid(Gap(3)),
				layout.Rigid(Lbl(th, 14.5, th.Pal.Text, font.SemiBold, value).Layout),
			)
		})
	}
	ram := "—"
	if d.TotalRAM > 0 {
		ram = fmt.Sprintf("%.1f GB", float64(d.TotalRAM)/(1<<30))
	}
	cpu := fmt.Sprintf("%d cores", d.CPUCores)
	title := "This device"
	if d.Model != "" {
		title = d.Model
	}
	sub := d.OS + " · " + d.Arch
	if d.Thermal != app.ThermalUnknown {
		sub += " · " + d.Thermal.String()
	}
	return Surface{Bg: th.Pal.Surface, Border: th.Pal.Border, Radius: 18}.Layout(gtx, func(gtx C) D {
		gtx.Constraints.Min.X = gtx.Constraints.Max.X
		return layout.UniformInset(16).Layout(gtx, func(gtx C) D {
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
				layout.Rigid(func(gtx C) D {
					return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
						layout.Flexed(1, func(gtx C) D {
							return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
								layout.Rigid(Lbl(th, 15, th.Pal.Text, font.SemiBold, title).Layout),
								layout.Rigid(Lbl(th, 12, th.Pal.TextMuted, font.Normal, sub).Layout),
							)
						}),
						layout.Rigid(func(gtx C) D {
							switch online {
							case 0:
								return Chip(gtx, th, "Offline", th.Pal.Warning, withAlpha(th.Pal.Warning, 0x22))
							case 1:
								return Chip(gtx, th, "Online", th.Pal.Success, withAlpha(th.Pal.Success, 0x1e))
							}
							return D{}
						}),
					)
				}),
				layout.Rigid(Gap(14)),
				layout.Rigid(func(gtx C) D {
					return layout.Flex{}.Layout(gtx,
						stat(ic.Memory, "RAM", ram),
						stat(ic.Storage, "FREE", app.HumanBytes(d.FreeStorage)),
						stat(ic.Phone, "CPU", cpu),
					)
				}),
				layout.Rigid(func(gtx C) D {
					if online != 0 {
						return D{}
					}
					return layout.Inset{Top: 12}.Layout(gtx, Lbl(th, 12.5, th.Pal.TextMuted, font.Normal,
						"You're offline. Downloaded models keep working; connect to download new ones.").Layout)
				}),
			)
		})
	})
}

func (l *Library) tabBar(gtx C, th *Theme) D {
	var children []layout.FlexChild
	for i, cat := range app.Categories {
		i, cat := i, cat
		n := 0
		for _, m := range app.Catalog {
			if m.Category == cat {
				n++
			}
		}
		children = append(children, layout.Flexed(1, func(gtx C) D {
			sel := l.cat == cat
			return l.tabs[i].Layout(gtx, func(gtx C) D {
				return layout.Background{}.Layout(gtx,
					func(gtx C) D {
						sz := gtx.Constraints.Min
						if sel {
							fillRRect(gtx, sz, gtx.Dp(10), th.Pal.Elevated)
						}
						return D{Size: sz}
					},
					func(gtx C) D {
						gtx.Constraints.Min.X = gtx.Constraints.Max.X
						return layout.Inset{Top: 9, Bottom: 9}.Layout(gtx, func(gtx C) D {
							col := th.Pal.TextMuted
							if sel {
								col = th.Pal.Text
							}
							lb := centered(Lbl(th, 13, col, font.SemiBold, cat.Title()))
							lb.MaxLines = 1
							return lb.Layout(gtx)
						})
					})
			})
		}))
	}
	return Surface{Bg: th.Pal.Surface, Border: th.Pal.Border, Radius: 13}.Layout(gtx, func(gtx C) D {
		return layout.UniformInset(3).Layout(gtx, func(gtx C) D {
			return layout.Flex{}.Layout(gtx, children...)
		})
	})
}

func fitColor(th *Theme, f app.Fit) color.NRGBA {
	switch f {
	case app.FitGreat:
		return th.Pal.Success
	case app.FitOK:
		return th.Pal.AccentHi
	case app.FitTight:
		return th.Pal.Warning
	}
	return th.Pal.Danger
}

func (l *Library) modelCard(gtx C, u *UI, m app.ModelSpec, dev app.DeviceInfo, nCtx int) D {
	th, a := u.th, u.a
	c := l.card(m.ID)
	pr := a.Downloads.Progress(m)
	st := a.Runtime.Status()
	loaded := st.ModelID == m.ID && (st.Phase == app.PhaseReady || st.Phase == app.PhaseGenerating)
	fit := app.AssessFit(m, dev, nCtx)

	if c.primary.Clicked(gtx) && m.Supported {
		switch pr.Status {
		case app.DLDone:
			a.SetSelection(app.SelectManual, m.ID)
			a.Notify(m.Name+" selected for this chat", false)
			u.navigate(screenChat)
		case app.DLNone, app.DLFailed, app.DLPaused:
			a.StartDownload(m.ID)
		}
	}
	if c.pause.Clicked(gtx) {
		a.Downloads.Pause(m.ID)
	}
	if c.cancel.Clicked(gtx) {
		a.Downloads.Cancel(m.ID)
	}
	if c.del.Clicked(gtx) {
		id, name := m.ID, m.Name
		u.dialog.Confirm("Delete "+name+"?",
			fmt.Sprintf("This frees %s. You can download it again later.", app.HumanBytes(m.DownloadSize())),
			"Delete", true, func(string) { a.DeleteModel(id); a.Notify(name+" deleted", false) })
	}

	return Surface{Bg: th.Pal.Surface, Border: th.Pal.Border, Radius: 18}.Layout(gtx, func(gtx C) D {
		gtx.Constraints.Min.X = gtx.Constraints.Max.X
		return layout.UniformInset(16).Layout(gtx, func(gtx C) D {
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
				// Title row.
				layout.Rigid(func(gtx C) D {
					return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
						layout.Rigid(func(gtx C) D { return familyBadge(gtx, th, m) }),
						layout.Rigid(Gap(12)),
						layout.Flexed(1, func(gtx C) D {
							return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
								layout.Rigid(Lbl(th, 15.5, th.Pal.Text, font.SemiBold, m.Name).Layout),
								layout.Rigid(func(gtx C) D {
									parts := []string{m.Family, m.Params + " params"}
									if m.Quant != "" {
										parts = append(parts, m.Quant)
									}
									return Lbl(th, 12, th.Pal.TextMuted, font.Normal, strings.Join(parts, " · ")).Layout(gtx)
								}),
							)
						}),
						layout.Rigid(func(gtx C) D {
							switch {
							case loaded:
								return Chip(gtx, th, "Loaded", th.Pal.Success, withAlpha(th.Pal.Success, 0x1e))
							case pr.Status == app.DLDone:
								return Chip(gtx, th, "Installed", th.Pal.Text, th.Pal.Elevated)
							case !m.Supported:
								return Chip(gtx, th, "Coming soon", th.Pal.TextMuted, th.Pal.Elevated)
							}
							return D{}
						}),
					)
				}),
				layout.Rigid(Gap(12)),
				layout.Rigid(func(gtx C) D {
					lb := Lbl(th, 13.5, th.Pal.TextMuted, font.Normal, m.Description)
					lb.LineHeightScale = 1.35
					return lb.Layout(gtx)
				}),
				layout.Rigid(Gap(10)),
				// Capability chips.
				layout.Rigid(func(gtx C) D {
					var chips []layout.Widget
					for _, cp := range m.Capabilities {
						cp := cp
						chips = append(chips, func(gtx C) D { return Chip(gtx, th, cp, th.Pal.TextMuted, th.Pal.Elevated) })
					}
					return Wrap(gtx, 6, 6, chips)
				}),
				layout.Rigid(Gap(12)),
				// Facts.
				layout.Rigid(func(gtx C) D {
					return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
						layout.Rigid(func(gtx C) D {
							if !m.Supported {
								return Lbl(th, 12, th.Pal.TextFaint, font.Medium, m.License).Layout(gtx)
							}
							return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
								layout.Rigid(func(gtx C) D { return Dot(gtx, fitColor(th, fit), 7) }),
								layout.Rigid(Gap(6)),
								layout.Rigid(Lbl(th, 12, fitColor(th, fit), font.SemiBold, fit.Label()).Layout),
								layout.Rigid(Lbl(th, 12, th.Pal.TextFaint, font.Normal,
									fmt.Sprintf(" · ~%s RAM · %s", app.HumanBytes(m.EstimateRuntimeBytes(nCtx)), m.License)).Layout),
							)
						}),
					)
				}),
				layout.Rigid(Gap(4)),
				layout.Rigid(func(gtx C) D {
					lb := Lbl(th, 11.5, th.Pal.TextFaint, font.Normal, m.Repo+" · "+m.Publisher)
					lb.MaxLines = 2
					lb.Truncator = "…"
					return lb.Layout(gtx)
				}),
				layout.Rigid(Gap(14)),
				layout.Rigid(func(gtx C) D { return l.actions(gtx, u, m, c, pr, fit) }),
			)
		})
	})
}

func (l *Library) actions(gtx C, u *UI, m app.ModelSpec, c *modelCard, pr app.DownloadProgress, fit app.Fit) D {
	th := u.th
	size := app.HumanBytes(m.DownloadSize())
	if !m.Supported {
		return infoNote(gtx, th, m.Unsupported)
	}
	switch pr.Status {
	case app.DLQueued, app.DLDownloading, app.DLVerifying:
		return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
			layout.Flexed(1, func(gtx C) D { return downloadProgress(gtx, th, pr) }),
			layout.Rigid(Gap(8)),
			layout.Rigid(func(gtx C) D {
				if pr.Status == app.DLVerifying {
					return D{}
				}
				return IconButtonSized(gtx, &c.pause, ic.Pause, th.Pal.Text, 20, 40)
			}),
			layout.Rigid(func(gtx C) D { return IconButtonSized(gtx, &c.cancel, ic.Close, th.Pal.TextMuted, 20, 40) }),
		)
	case app.DLPaused:
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(func(gtx C) D { return ProgressBar(gtx, pr.Fraction(), th.Pal.TextMuted, th.Pal.Elevated) }),
			layout.Rigid(Gap(8)),
			layout.Rigid(Lbl(th, 12, th.Pal.TextMuted, font.Normal,
				fmt.Sprintf("Paused · %s of %s", app.HumanBytes(pr.Downloaded), app.HumanBytes(pr.Total))).Layout),
			layout.Rigid(Gap(10)),
			layout.Rigid(func(gtx C) D {
				return layout.Flex{}.Layout(gtx,
					layout.Flexed(1, func(gtx C) D {
						return Button{Kind: BtnPrimary, Text: "Resume", Icon: ic.Play, Wide: true}.Layout(gtx, th, &c.primary)
					}),
					layout.Rigid(Gap(8)),
					layout.Rigid(func(gtx C) D { return Button{Kind: BtnSecondary, Text: "Discard"}.Layout(gtx, th, &c.cancel) }),
				)
			}),
		)
	case app.DLFailed:
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(Lbl(th, 12.5, th.Pal.Danger, font.Medium, pr.Err).Layout),
			layout.Rigid(Gap(10)),
			layout.Rigid(func(gtx C) D {
				return Button{Kind: BtnPrimary, Text: "Try again", Icon: ic.Refresh, Wide: true}.Layout(gtx, th, &c.primary)
			}),
		)
	case app.DLDone:
		return layout.Flex{}.Layout(gtx,
			layout.Flexed(1, func(gtx C) D {
				return Button{Kind: BtnPrimary, Text: "Use this model", Icon: ic.Done, Wide: true}.Layout(gtx, th, &c.primary)
			}),
			layout.Rigid(Gap(8)),
			layout.Rigid(func(gtx C) D { return Button{Kind: BtnDanger, Text: "Delete", Icon: ic.Delete}.Layout(gtx, th, &c.del) }),
		)
	}
	label := "Download · " + size
	kind := BtnPrimary
	if fit == app.FitNo {
		label, kind = "Download anyway · "+size, BtnSecondary
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx C) D {
			if fit != app.FitNo {
				return D{}
			}
			return layout.Inset{Bottom: 10}.Layout(gtx, Lbl(th, 12.5, th.Pal.Warning, font.Normal,
				"This model probably needs more memory than this device has. It may fail to load or be closed by Android.").Layout)
		}),
		layout.Rigid(func(gtx C) D {
			return Button{Kind: kind, Text: label, Icon: ic.Download, Wide: true}.Layout(gtx, th, &c.primary)
		}),
	)
}

func downloadProgress(gtx C, th *Theme, pr app.DownloadProgress) D {
	line := fmt.Sprintf("%s of %s", app.HumanBytes(pr.Downloaded), app.HumanBytes(pr.Total))
	if pr.BytesPerSec > 0 {
		line += fmt.Sprintf(" · %s/s", app.HumanBytes(int64(pr.BytesPerSec)))
		if rem := float64(pr.Total-pr.Downloaded) / pr.BytesPerSec; rem > 0 && rem < 48*3600 {
			line += " · " + eta(rem)
		}
	}
	switch pr.Status {
	case app.DLQueued:
		line = "Starting…"
	case app.DLVerifying:
		line = "Verifying SHA-256…"
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx C) D {
			if pr.Status == app.DLVerifying || pr.Status == app.DLQueued {
				return Indeterminate(gtx, th.Pal.Accent, th.Pal.Elevated)
			}
			return ProgressBar(gtx, pr.Fraction(), th.Pal.Accent, th.Pal.Elevated)
		}),
		layout.Rigid(Gap(8)),
		layout.Rigid(Lbl(th, 12, th.Pal.TextMuted, font.Medium, line).Layout),
	)
}

func eta(sec float64) string {
	switch {
	case sec < 60:
		return fmt.Sprintf("%.0fs left", sec)
	case sec < 3600:
		return fmt.Sprintf("%.0f min left", sec/60)
	}
	return fmt.Sprintf("%.1f h left", sec/3600)
}
