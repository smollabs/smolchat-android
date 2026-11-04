//go:build ignore

// Renders the SmolChat logo to icon.png (used by gogio -icon).
//
//	go run assets/icons/gen.go
package main

import (
	"image"
	"image/png"
	"log"
	"os"
	"time"

	"gioui.org/gpu/headless"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"

	"github.com/smolworks/smolchat-android/ui"
)

func main() {
	const px = 1024
	w, err := headless.NewWindow(px, px)
	if err != nil {
		log.Fatal(err)
	}
	defer w.Release()
	var ops op.Ops
	gtx := layout.Context{Ops: &ops, Now: time.Now(), Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Exact(image.Pt(px, px))}
	ui.Logo(gtx, px, false)
	if err := w.Frame(&ops); err != nil {
		log.Fatal(err)
	}
	img := image.NewRGBA(image.Rect(0, 0, px, px))
	if err := w.Screenshot(img); err != nil {
		log.Fatal(err)
	}
	f, err := os.Create("assets/icons/icon.png")
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		log.Fatal(err)
	}
}
