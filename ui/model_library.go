package ui

import (
	"fmt"
	"image"
	"image/color"
	"strings"

	"gioui.org/font"
	"gioui.org/layout"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"

	"github.com/smolworks/smolchat-android/app"
)

type modelCard struct {
	primary, cancel, pause, del widget.Clickable
}

type libTab struct {
	label string
	match func(m app.ModelSpec, installed bool, fit app.Fit) bool
}

var libTabs = []libTab{
	{"All", func(app.ModelSpec, bool, app.Fit) bool { return true }},
	{"Recommended", func(m app.ModelSpec, _ bool, f app.Fit) bool { return m.Supported && !m.Advanced && f <= app.FitOK }},
	{"Downloaded", func(_ app.ModelSpec, inst bool, _ app.Fit) bool { return inst }},
	{"Text & Chat", func(m app.ModelSpec, _ bool, _ app.Fit) bool { return m.Category == app.CategoryText }},
	{"Vision", func(m app.ModelSpec, _ bool, _ app.Fit) bool { return m.Category == app.CategoryVision }},
	{"Documents", func(m app.ModelSpec, _ bool, _ app.Fit) bool { return m.Category == app.CategoryDocument }},
	{"Retrieval", func(m app.ModelSpec, _ bool, _ app.Fit) bool { return m.Category == app.CategoryEmbedding }},
}

func categoryLabel(c app.Category) string {
	switch c {
	case app.CategoryVision:
		return "Vision"
	case app.CategoryDocument:
		return "Documents"
	case app.CategoryEmbedding:
		return "Retrieval"
	}
	return "Text & Chat"
}

// sourceLabel shortens the publisher: "Official (HuggingFaceTB)",
// "Community (bartowski)", "ggml-org".
func sourceLabel(m app.ModelSpec) string {
	name, _, _ := strings.Cut(m.Publisher, " (")
	switch {
	case strings.Contains(m.Publisher, "(official)"):
		return "Official (" + name + ")"
	case strings.Contains(m.Publisher, "community"):
		return "Community (" + name + ")"
	}
	return name
}

// recommendedRAM is the phone RAM tier a model needs (~55% usable by apps).
func recommendedRAM(m app.ModelSpec, nCtx int) string {
	need := float64(m.EstimateRuntimeBytes(nCtx)) / 0.55 / (1 << 30)
	for _, gb := range []float64{2, 3, 4, 6, 8, 12, 16} {
		if need <= gb {
			return fmt.Sprintf("%.0f GB+ phone recommended", gb)
		}
	}
	return "16 GB+ phone recommended"
}

// Library is the model management screen.
type Library struct {
	close   widget.Clickable
	tabs    [7]widget.Clickable
	tab     int
	tabList layout.List
	list    widget.List
	cards   map[string]*modelCard
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
	if l.close.Clicked(gtx) {
		u.pop(0)
	}
	for i := range l.tabs {
		if l.tabs[i].Clicked(gtx) {
			l.tab = i
			l.list.Position = layout.Position{}
		}
	}
	nCtx := a.Settings.Get().ContextSize
	dev := a.Device()
	var specs []app.ModelSpec
	for _, m := range app.Catalog {
		if libTabs[l.tab].match(m, a.Models.IsInstalled(m.ID), app.AssessFit(m, dev, nCtx)) {
			specs = append(specs, m)
		}
	}

	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx C) D {
			return sheetHeader(gtx, u, &l.close, "Model Library",
				"Download once, run locally, chat offline. Models come from Hugging Face and are stored on this phone.",
				func(gtx C) D { return deviceLine(gtx, u, dev) })
		}),
		layout.Rigid(func(gtx C) D { return l.tabBar(gtx, th) }),
		layout.Rigid(func(gtx C) D { return Divider(gtx, th.Pal.Border) }),
		layout.Flexed(1, func(gtx C) D {
			l.list.Axis = layout.Vertical
			n := len(specs)
			return material.List(th.Theme, &l.list).Layout(gtx, n+1, func(gtx C, i int) D {
				return layout.Inset{Left: 20, Right: 20}.Layout(gtx, func(gtx C) D {
					gtx.Constraints.Max.X = min(gtx.Constraints.Max.X, gtx.Dp(720))
					if i == n {
						return layout.Inset{Top: 14, Bottom: 28}.Layout(gtx, func(gtx C) D {
							msg := "Downloads are verified with SHA-256 and can be resumed if interrupted."
							if n == 0 {
								msg = "Nothing here yet. " + msg
							}
							return Lbl(th, 12, th.Pal.TextFaint, font.Normal, msg).Layout(gtx)
						})
					}
					return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
						layout.Rigid(func(gtx C) D { return l.row(gtx, u, specs[i], dev, nCtx) }),
						layout.Rigid(func(gtx C) D { return Divider(gtx, th.Pal.Border) }),
					)
				})
			})
		}),
	)
}

// sheetHeader is the title block of full-screen sheets (Library, Settings).
func sheetHeader(gtx C, u *UI, close *widget.Clickable, title, sub string, extra layout.Widget) D {
	th := u.th
	return layout.Inset{Top: u.insets.Top}.Layout(gtx, func(gtx C) D {
		return layout.Inset{Left: 20, Right: 8, Top: 10, Bottom: 10}.Layout(gtx, func(gtx C) D {
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
				layout.Rigid(func(gtx C) D {
					return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
						layout.Flexed(1, Lbl(th, 23, th.Pal.Text, font.SemiBold, title).Layout),
						layout.Rigid(func(gtx C) D { return IconButton(gtx, close, ic.Close, th.Pal.TextMuted) }),
					)
				}),
				layout.Rigid(func(gtx C) D {
					if sub == "" {
						return D{}
					}
					return layout.Inset{Top: 2, Right: 12}.Layout(gtx, func(gtx C) D { return hint(gtx, th, sub) })
				}),
				layout.Rigid(func(gtx C) D {
					if extra == nil {
						return D{}
					}
					return layout.Inset{Top: 8, Right: 12}.Layout(gtx, extra)
				}),
			)
		})
	})
}

// deviceLine is "Android 15 · Pixel 8 · 7.6 GB memory · 52 GB free · 8 cores".
func deviceLine(gtx C, u *UI, d app.DeviceInfo) D {
	th := u.th
	parts := []string{d.OS}
	if d.Model != "" {
		parts = append(parts, d.Model)
	}
	if d.TotalRAM > 0 {
		parts = append(parts, fmt.Sprintf("%.1f GB memory", float64(d.TotalRAM)/(1<<30)))
	}
	parts = append(parts, app.HumanBytes(d.FreeStorage)+" free", fmt.Sprintf("%d cores", d.CPUCores))
	online := u.a.Online()
	return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
		layout.Rigid(func(gtx C) D {
			s := gtx.Dp(15)
			gtx.Constraints = layout.Exact(image.Pt(s, s))
			return ic.Memory.Layout(gtx, th.Pal.TextFaint)
		}),
		layout.Rigid(Gap(7)),
		layout.Flexed(1, func(gtx C) D {
			l := Lbl(th, 12.5, th.Pal.TextFaint, font.Normal, strings.Join(parts, " · "))
			return l.Layout(gtx)
		}),
		layout.Rigid(func(gtx C) D {
			if online != 0 {
				return D{}
			}
			return layout.Inset{Left: 8}.Layout(gtx, func(gtx C) D {
				return Chip(gtx, th, "Offline", th.Pal.Warning, withAlpha(th.Pal.Warning, 0x22))
			})
		}),
	)
}

func (l *Library) tabBar(gtx C, th *Theme) D {
	l.tabList.Axis = layout.Horizontal
	return layout.Inset{Top: 2, Bottom: 10}.Layout(gtx, func(gtx C) D {
		return l.tabList.Layout(gtx, len(libTabs), func(gtx C, i int) D {
			left := unit.Dp(2)
			if i == 0 {
				left = 16
			}
			sel := i == l.tab
			return layout.Inset{Left: left, Right: 2}.Layout(gtx, func(gtx C) D {
				return l.tabs[i].Layout(gtx, func(gtx C) D {
					return layout.Background{}.Layout(gtx,
						func(gtx C) D {
							sz := gtx.Constraints.Min
							if sel {
								fillRRect(gtx, sz, sz.Y/2, th.Pal.Elevated)
							}
							pressOverlay(gtx, &l.tabs[i], sz, sz.Y/2)
							return D{Size: sz}
						},
						func(gtx C) D {
							return layout.Inset{Left: 13, Right: 13, Top: 8, Bottom: 8}.Layout(gtx, func(gtx C) D {
								col := th.Pal.TextMuted
								if sel {
									col = th.Pal.Text
								}
								return Lbl(th, 14, col, font.Medium, libTabs[i].label).Layout(gtx)
							})
						})
				})
			})
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

// badge is the small tinted status label next to a model name.
func badge(gtx C, th *Theme, s string, col color.NRGBA) D {
	return Chip(gtx, th, s, col, withAlpha(col, 0x22))
}

func (l *Library) row(gtx C, u *UI, m app.ModelSpec, dev app.DeviceInfo, nCtx int) D {
	th, a := u.th, u.a
	c := l.card(m.ID)
	pr := a.Downloads.Progress(m)
	installed := pr.Status == app.DLDone
	fit := app.AssessFit(m, dev, nCtx)

	if c.primary.Clicked(gtx) && m.Supported {
		switch pr.Status {
		case app.DLDone:
			a.SetSelection(app.SelectManual, m.ID)
			a.Notify(m.Name+" will be used in this chat", false)
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
			fmt.Sprintf("This frees %s on this phone. You can download it again later.", app.HumanBytes(m.DownloadSize())),
			"Delete", true, func(string) { a.DeleteModel(id); a.Notify(name+" deleted", false) })
	}

	return layout.Inset{Top: 16, Bottom: 16}.Layout(gtx, func(gtx C) D {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			// Name + status badge.
			layout.Rigid(func(gtx C) D {
				var items []layout.Widget
				items = append(items, Lbl(th, 16.5, th.Pal.Text, font.SemiBold, m.Name).Layout)
				switch {
				case !m.Supported:
					items = append(items, func(gtx C) D { return badge(gtx, th, "Coming soon", th.Pal.TextFaint) })
				case installed:
					items = append(items, func(gtx C) D { return badge(gtx, th, "Downloaded", th.Pal.Success) })
				default:
					items = append(items, func(gtx C) D { return badge(gtx, th, fit.Label(), fitColor(th, fit)) })
				}
				return Wrap(gtx, 8, 6, items)
			}),
			layout.Rigid(Gap(6)),
			layout.Rigid(func(gtx C) D {
				lb := Lbl(th, 14, th.Pal.TextMuted, font.Normal, m.Description)
				lb.LineHeightScale = 1.35
				return lb.Layout(gtx)
			}),
			layout.Rigid(Gap(10)),
			layout.Rigid(func(gtx C) D {
				return Wrap(gtx, 6, 6, []layout.Widget{
					func(gtx C) D { return Chip(gtx, th, categoryLabel(m.Category), th.Pal.AccentHi, th.Pal.AccentSoft) },
					func(gtx C) D { return Chip(gtx, th, m.Family+" · "+m.Params, th.Pal.TextMuted, th.Pal.Surface) },
				})
			}),
			layout.Rigid(Gap(10)),
			layout.Rigid(func(gtx C) D {
				var meta []string
				if m.Quant != "" {
					meta = append(meta, m.Quant)
				}
				if m.Supported {
					meta = append(meta, "~"+app.HumanBytes(m.EstimateRuntimeBytes(nCtx))+" of memory while in use", recommendedRAM(m, nCtx))
				}
				return Lbl(th, 12.5, th.Pal.TextFaint, font.Normal, strings.Join(meta, " · ")).Layout(gtx)
			}),
			layout.Rigid(Gap(3)),
			layout.Rigid(Lbl(th, 12.5, th.Pal.TextFaint, font.Normal, sourceLabel(m)+" · "+m.License).Layout),
			layout.Rigid(Gap(14)),
			layout.Rigid(func(gtx C) D { return l.actions(gtx, u, m, c, pr, fit) }),
		)
	})
}

// sizeBlock is "145 MB / download size" (or "on disk").
func sizeBlock(gtx C, th *Theme, m app.ModelSpec, installed bool) D {
	sub := "download size"
	if installed {
		sub = "on disk"
	}
	if len(m.Files) == 0 {
		return D{}
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(Lbl(th, 16, th.Pal.Text, font.SemiBold, app.HumanBytes(m.DownloadSize())).Layout),
		layout.Rigid(Lbl(th, 11.5, th.Pal.TextFaint, font.Normal, sub).Layout),
	)
}

func (l *Library) actions(gtx C, u *UI, m app.ModelSpec, c *modelCard, pr app.DownloadProgress, fit app.Fit) D {
	th := u.th
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
			layout.Rigid(func(gtx C) D { return ProgressBar(gtx, pr.Fraction(), th.Pal.TextFaint, th.Pal.Elevated) }),
			layout.Rigid(Gap(8)),
			layout.Rigid(func(gtx C) D {
				return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
					layout.Flexed(1, Lbl(th, 12.5, th.Pal.TextMuted, font.Normal,
						fmt.Sprintf("Paused · %s of %s", app.HumanBytes(pr.Downloaded), app.HumanBytes(pr.Total))).Layout),
					layout.Rigid(func(gtx C) D { return Button{Kind: BtnGhost, Text: "Discard", Small: true}.Layout(gtx, th, &c.cancel) }),
					layout.Rigid(Gap(6)),
					layout.Rigid(func(gtx C) D {
						return Button{Kind: BtnPrimary, Text: "Resume", Icon: ic.Play, Small: true}.Layout(gtx, th, &c.primary)
					}),
				)
			}),
		)
	case app.DLFailed:
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(Lbl(th, 12.5, th.Pal.Danger, font.Medium, pr.Err).Layout),
			layout.Rigid(Gap(10)),
			layout.Rigid(func(gtx C) D {
				return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
					layout.Flexed(1, func(gtx C) D { return sizeBlock(gtx, th, m, false) }),
					layout.Rigid(func(gtx C) D {
						return Button{Kind: BtnPrimary, Text: "Try again", Icon: ic.Refresh, Small: true}.Layout(gtx, th, &c.primary)
					}),
				)
			}),
		)
	case app.DLDone:
		return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
			layout.Flexed(1, func(gtx C) D { return sizeBlock(gtx, th, m, true) }),
			layout.Rigid(func(gtx C) D {
				return Button{Kind: BtnGhost, Text: "Delete", Icon: ic.Delete, Small: true}.Layout(gtx, th, &c.del)
			}),
			layout.Rigid(Gap(6)),
			layout.Rigid(func(gtx C) D {
				return Button{Kind: BtnSecondary, Text: "Use in this chat", Small: true}.Layout(gtx, th, &c.primary)
			}),
		)
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx C) D {
			if fit != app.FitNo {
				return D{}
			}
			return layout.Inset{Bottom: 10}.Layout(gtx, Lbl(th, 12.5, th.Pal.Warning, font.Normal,
				"This model probably needs more memory than this phone has. It may fail to load or be closed by Android.").Layout)
		}),
		layout.Rigid(func(gtx C) D {
			return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
				layout.Flexed(1, func(gtx C) D { return sizeBlock(gtx, th, m, false) }),
				layout.Rigid(func(gtx C) D {
					return Button{Kind: BtnPrimary, Text: "Download", Icon: ic.Download, Small: true}.Layout(gtx, th, &c.primary)
				}),
			)
		}),
	)
}

func infoNote(gtx C, th *Theme, s string) D {
	return Surface{Bg: th.Pal.Surface, Border: th.Pal.Border, Radius: 12}.Layout(gtx, func(gtx C) D {
		gtx.Constraints.Min.X = gtx.Constraints.Max.X
		return layout.UniformInset(12).Layout(gtx, func(gtx C) D {
			return layout.Flex{}.Layout(gtx,
				layout.Rigid(func(gtx C) D {
					sz := gtx.Dp(17)
					gtx.Constraints = layout.Exact(image.Pt(sz, sz))
					return ic.Info.Layout(gtx, th.Pal.TextFaint)
				}),
				layout.Rigid(Gap(10)),
				layout.Flexed(1, func(gtx C) D {
					l := Lbl(th, 13, th.Pal.TextMuted, font.Normal, s)
					l.LineHeightScale = 1.35
					return l.Layout(gtx)
				}),
			)
		})
	})
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
