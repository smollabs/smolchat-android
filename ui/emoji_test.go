package ui

import (
	"image"
	"image/png"
	"os"
	"testing"
	"time"

	"gioui.org/gpu/headless"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
)

// TestEmojiRendering draws emoji in a label, an editor and Markdown and
// checks that pixels were painted where the emoji are.
func TestEmojiRendering(t *testing.T) {
	sz := image.Pt(600, 300)
	w, err := headless.NewWindow(sz.X, sz.Y)
	if err != nil {
		t.Skip(err)
	}
	defer w.Release()
	th := NewTheme()
	var ed widget.Editor
	ed.SetText("👍🎉")
	var md Markdown
	md.Set(th, "**Done** ✅ 👨‍👩‍👧")
	var ops op.Ops
	gtx := layout.Context{Ops: &ops, Now: time.Now(), Metric: unit.Metric{PxPerDp: 2, PxPerSp: 2}, Constraints: layout.Exact(sz)}
	layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(Lbl(th, 20, th.Pal.Text, 400, "😀😂❤️").Layout),
		layout.Rigid(func(gtx C) D { return material.Editor(th.Theme, &ed, "").Layout(gtx) }),
		layout.Rigid(func(gtx C) D { return md.Layout(gtx, th) }),
	)
	if err := w.Frame(&ops); err != nil {
		t.Fatal(err)
	}
	img := image.NewRGBA(image.Rectangle{Max: sz})
	w.Screenshot(img)
	if out := os.Getenv("SMOLCHAT_SHOTS"); out != "" {
		f, _ := os.Create(out + "/emoji.png")
		png.Encode(f, img)
		f.Close()
	}
	// Each row must contain painted (non-transparent) pixels.
	for _, band := range [][2]int{{0, 45}, {45, 85}, {85, 220}} {
		lit := 0
		for y := band[0]; y < band[1]; y++ {
			for x := 0; x < 300; x++ {
				if img.RGBAAt(x, y).A > 0 {
					lit++
				}
			}
		}
		if lit < 200 {
			t.Errorf("rows %v: only %d painted pixels; emoji not rendered", band, lit)
		}
	}
}
