// Package ui implements SmolChat's Gio interface.
package ui

import (
	"image/color"

	"gioui.org/font"
	"gioui.org/font/opentype"
	"gioui.org/text"
	"gioui.org/unit"
	"gioui.org/widget/material"

	"github.com/smolworks/smolchat-android/assets"
)

// Palette holds the colour tokens of one appearance (light or dark) with one
// accent. The values mirror SmolChat for Mac (ui/styles/theme.qml).
type Palette struct {
	Dark bool

	Frame, Bg, Glass, Popup                color.NRGBA // drawer, main panel, glass cards, sheets
	Surface, Surface2, Elevated            color.NRGBA // chips, fields, selected rows
	Border, BorderStrong                   color.NRGBA
	Text, TextMuted, TextFaint             color.NRGBA
	Accent, AccentHi, AccentSoft, AccentLo color.NRGBA
	OnAccent, Primary, OnPrimary           color.NRGBA
	Danger, DangerBg, Success, Warning     color.NRGBA
	UserBubble, CodeBg, CodeHeader         color.NRGBA
	Scrim, Pressed                         color.NRGBA
	Flame, FlameLight                      color.NRGBA
	GlowStrength                           float32

	// Syntax colours for code blocks.
	SynKeyword, SynString, SynNumber, SynComment, SynFunc, SynType, SynText, InlineCode color.NRGBA
}

func rgb(c uint32) color.NRGBA {
	return color.NRGBA{R: uint8(c >> 16), G: uint8(c >> 8), B: uint8(c), A: 0xff}
}

// argb parses 0xAARRGGBB, as Qt writes "#AARRGGBB".
func argb(c uint32) color.NRGBA {
	return color.NRGBA{A: uint8(c >> 24), R: uint8(c >> 16), G: uint8(c >> 8), B: uint8(c)}
}

func withAlpha(c color.NRGBA, a uint8) color.NRGBA {
	c.A = uint8(uint16(c.A) * uint16(a) / 255)
	return c
}

func mix(a, b color.NRGBA, t float32) color.NRGBA {
	l := func(x, y uint8) uint8 { return uint8(float32(x) + (float32(y)-float32(x))*t) }
	return color.NRGBA{R: l(a.R, b.R), G: l(a.G, b.G), B: l(a.B, b.B), A: l(a.A, b.A)}
}

// Accent is one user-selectable accent colour.
type Accent struct {
	Key, Label  string
	Dark, Light color.NRGBA
}

// Accents in display order.
var Accents = []Accent{
	{"blue", "Blue", rgb(0x5b9dff), rgb(0x0a66e4)},
	{"violet", "Violet", rgb(0xad66f5), rgb(0x7b3fe0)},
	{"green", "Green", rgb(0x4ade80), rgb(0x15803d)},
	{"orange", "Orange", rgb(0xfb923c), rgb(0xea580c)},
	{"pink", "Pink", rgb(0xf472b6), rgb(0xdb2777)},
	{"graphite", "Graphite", rgb(0xa1a1aa), rgb(0x52525b)},
}

func accentFor(key string) Accent {
	for _, a := range Accents {
		if a.Key == key {
			return a
		}
	}
	return Accents[1]
}

// NewPalette builds the palette for an appearance and accent key.
func NewPalette(dark bool, accent string) Palette {
	ac := accentFor(accent)
	white, black := rgb(0xffffff), rgb(0x000000)
	if dark {
		brand := ac.Dark
		return Palette{
			Dark:  true,
			Frame: rgb(0x04030b), Bg: rgb(0x0a0817), Glass: rgb(0x130f22), Popup: rgb(0x110d1e),
			Surface: argb(0x14ffffff), Surface2: rgb(0x130f22), Elevated: argb(0x22ffffff),
			Border: argb(0x17ffffff), BorderStrong: argb(0x2effffff),
			Text: rgb(0xececf1), TextMuted: rgb(0xb4b4bb), TextFaint: rgb(0x85858e),
			Accent: brand, AccentHi: mix(brand, white, 0.25), AccentSoft: withAlpha(brand, 41), AccentLo: mix(brand, black, 0.35),
			OnAccent: white, Primary: rgb(0xececf1), OnPrimary: rgb(0x0f0f12),
			Danger: rgb(0xf87171), DangerBg: rgb(0x2a1717), Success: rgb(0x4ade80), Warning: rgb(0xfbbf24),
			UserBubble: rgb(0x1c1533), CodeBg: rgb(0x07050f), CodeHeader: rgb(0x120e20),
			Scrim: argb(0xa6000000), Pressed: argb(0x14ffffff),
			Flame: rgb(0xf57a12), FlameLight: rgb(0xfde38a), GlowStrength: 0.40,
			SynKeyword: rgb(0xc4a1ff), SynString: rgb(0xa8d991), SynNumber: rgb(0xf5a97f), SynComment: rgb(0x6d6d78),
			SynFunc: rgb(0x8ab4ff), SynType: rgb(0xf3cf7a), SynText: rgb(0xd6d6de), InlineCode: rgb(0xf2b48a),
		}
	}
	brand := ac.Light
	return Palette{
		Frame: rgb(0xeeebf5), Bg: rgb(0xfcfbff), Glass: rgb(0xffffff), Popup: rgb(0xffffff),
		Surface: argb(0x0b000000), Surface2: rgb(0xffffff), Elevated: argb(0x14000000),
		Border: argb(0x12000000), BorderStrong: argb(0x1f000000),
		Text: rgb(0x0f0f12), TextMuted: rgb(0x55555d), TextFaint: rgb(0x8e8e96),
		Accent: brand, AccentHi: brand, AccentSoft: withAlpha(brand, 26), AccentLo: mix(brand, black, 0.3),
		OnAccent: white, Primary: rgb(0x0f0f12), OnPrimary: white,
		Danger: rgb(0xdc2626), DangerBg: rgb(0xfdeeee), Success: rgb(0x16a34a), Warning: rgb(0xb45309),
		UserBubble: rgb(0xf0eafc), CodeBg: rgb(0xf7f5fc), CodeHeader: rgb(0xefebf8),
		Scrim: argb(0x4d000000), Pressed: argb(0x0f000000),
		Flame: rgb(0xf57a12), FlameLight: rgb(0xfde38a), GlowStrength: 0.30,
		SynKeyword: rgb(0x7c3aed), SynString: rgb(0x15803d), SynNumber: rgb(0xc2410c), SynComment: rgb(0x8e8e96),
		SynFunc: rgb(0x1d4ed8), SynType: rgb(0xa16207), SynText: rgb(0x24242a), InlineCode: rgb(0xc2410c),
	}
}

// pal is the active palette (the UI is single-window; Theme.Apply sets it).
var pal = NewPalette(true, "violet")

// Background is the initial window colour (the splash), also used for the
// system bars until the theme is known.
var Background = rgb(0x04030b)

// Theme bundles the material theme with SmolChat's fonts and palette.
type Theme struct {
	*material.Theme
	Pal  Palette
	Mono font.Font
}

// Apply switches the palette.
func (th *Theme) Apply(p Palette) {
	if th.Pal == p {
		return
	}
	th.Pal, pal = p, p
	clear(glowCache) // textures are per colour
	th.Palette = material.Palette{Bg: p.Bg, Fg: p.Text, ContrastBg: p.Accent, ContrastFg: p.OnAccent}
}

const (
	interFace = "Inter"
	monoFace  = "JetBrains Mono"
	emojiFace = "Noto Emoji"

	// Typeface lists used for text: emoji resolve to the bundled font
	// before any system fallback.
	uiFace   font.Typeface = interFace + ", " + emojiFace
	codeFace font.Typeface = monoFace + ", " + emojiFace
)

func loadFaces() []font.FontFace {
	var faces []font.FontFace
	add := func(file string, f font.Font) {
		b, err := assets.Fonts.ReadFile("fonts/" + file)
		if err != nil {
			return
		}
		face, err := opentype.Parse(b)
		if err != nil {
			return
		}
		faces = append(faces, font.FontFace{Font: f, Face: face})
	}
	add("Inter-400.ttf", font.Font{Typeface: interFace, Weight: font.Normal})
	add("Inter-500.ttf", font.Font{Typeface: interFace, Weight: font.Medium})
	add("Inter-600.ttf", font.Font{Typeface: interFace, Weight: font.SemiBold})
	add("Inter-700.ttf", font.Font{Typeface: interFace, Weight: font.Bold})
	add("Inter-400-italic.ttf", font.Font{Typeface: interFace, Style: font.Italic})
	add("Inter-600-italic.ttf", font.Font{Typeface: interFace, Style: font.Italic, Weight: font.SemiBold})
	add("JetBrainsMono-400.ttf", font.Font{Typeface: monoFace})
	// Outline emoji: render in every widget (editor, rich text, labels).
	// Android's own emoji font is COLRv1, which Gio cannot draw.
	add("NotoEmoji-400.ttf", font.Font{Typeface: emojiFace})
	// Other scripts and emoji fall back to the platform's system fonts.
	return faces
}

// NewTheme builds the theme with the default (dark) palette.
func NewTheme() *Theme {
	th := material.NewTheme()
	th.Shaper = text.NewShaper(text.WithCollection(loadFaces()))
	th.Face = uiFace
	th.TextSize = unit.Sp(15)
	th.FingerSize = 44
	loadArt(nil)
	t := &Theme{Theme: th, Mono: font.Font{Typeface: codeFace}}
	t.Pal = Palette{} // force Apply
	t.Apply(pal)
	return t
}
