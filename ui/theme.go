// Package ui implements SmolChat's Gio interface.
package ui

import (
	"image/color"

	"gioui.org/font"
	"gioui.org/font/gofont"
	"gioui.org/font/opentype"
	"gioui.org/text"
	"gioui.org/unit"
	"gioui.org/widget/material"

	"github.com/smolworks/smolchat-android/assets"
)

// Palette is SmolChat's dark colour system.
type Palette struct {
	Bg, Surface, Surface2, Elevated, Border, BorderStrong color.NRGBA
	Text, TextMuted, TextFaint                            color.NRGBA
	Accent, AccentHi, AccentSoft, OnAccent                color.NRGBA
	Danger, Success, Warning                              color.NRGBA
	UserBubble, CodeBg, CodeHeader, Scrim, Pressed        color.NRGBA
}

func rgb(c uint32) color.NRGBA {
	return color.NRGBA{R: uint8(c >> 16), G: uint8(c >> 8), B: uint8(c), A: 0xff}
}

func withAlpha(c color.NRGBA, a uint8) color.NRGBA {
	c.A = uint8(uint16(c.A) * uint16(a) / 255)
	return c
}

// Background is the window colour (also used for the system bars).
var Background = rgb(0x0e0e10)

var pal = Palette{
	Bg:           rgb(0x0e0e10),
	Surface:      rgb(0x161619),
	Surface2:     rgb(0x1c1c20),
	Elevated:     rgb(0x232328),
	Border:       rgb(0x26262c),
	BorderStrong: rgb(0x34343b),
	Text:         rgb(0xececf1),
	TextMuted:    rgb(0xa3a3ad),
	TextFaint:    rgb(0x6c6c76),
	Accent:       rgb(0x8b85ff),
	AccentHi:     rgb(0xa9a4ff),
	AccentSoft:   withAlpha(rgb(0x8b85ff), 0x2a),
	OnAccent:     rgb(0x0e0e10),
	Danger:       rgb(0xf47174),
	Success:      rgb(0x4fd08a),
	Warning:      rgb(0xf3c25a),
	UserBubble:   rgb(0x26262c),
	CodeBg:       rgb(0x0a0a0c),
	CodeHeader:   rgb(0x1a1a1f),
	Scrim:        color.NRGBA{A: 0xa0},
	Pressed:      color.NRGBA{R: 0xff, G: 0xff, B: 0xff, A: 0x12},
}

// Theme bundles the material theme with SmolChat's fonts.
type Theme struct {
	*material.Theme
	Pal  Palette
	Mono font.Font
}

const (
	interFace = "Inter"
	monoFace  = "JetBrains Mono"
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
	// Go fonts cover Latin extended, Greek and Cyrillic; the
	// shaper falls back to system fonts (CJK, emoji) on Android.
	return append(faces, gofont.Collection()...)
}

// NewTheme builds the theme. Parsing fonts takes a few milliseconds, so it
// runs before the first frame.
func NewTheme() *Theme {
	th := material.NewTheme()
	th.Shaper = text.NewShaper(text.WithCollection(loadFaces()))
	th.Face = interFace
	th.TextSize = unit.Sp(15)
	th.Palette = material.Palette{Bg: pal.Bg, Fg: pal.Text, ContrastBg: pal.Accent, ContrastFg: pal.OnAccent}
	th.FingerSize = 44
	return &Theme{Theme: th, Pal: pal, Mono: font.Font{Typeface: monoFace}}
}
