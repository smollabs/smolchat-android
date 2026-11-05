// Command smolchat is SmolChat for Android (and desktop, for development):
// a private, offline chat app for Smol language models, built with Gio and
// llama.cpp.
package main

import (
	"image/color"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"sync/atomic"

	gioapp "gioui.org/app"
	_ "gioui.org/app/permission/foreground" // foreground service for downloads
	"gioui.org/io/event"
	"gioui.org/layout"
	"gioui.org/op"

	"github.com/smolworks/smolchat-android/app"
	"github.com/smolworks/smolchat-android/native/llama"
	"github.com/smolworks/smolchat-android/ui"
)

type initResult struct {
	a   *app.App
	err error
}

var (
	core atomic.Pointer[app.App]
	win  atomic.Pointer[gioapp.Window] // current window (Android may recreate it)
)

func invalidate() {
	if w := win.Load(); w != nil {
		w.Invalidate()
	}
}

func main() {
	go runWindow()
	// Events replaces app.Main so lifecycle events reach us.
	gioapp.Events(func(e event.Event) bool {
		a := core.Load()
		if a == nil {
			return true
		}
		switch e.(type) {
		case gioapp.BackgroundEvent:
			a.OnBackground()
		case gioapp.ForegroundEvent:
			a.OnForeground()
		}
		return true
	})
}

func dataDir() (string, error) {
	if d := os.Getenv("SMOLCHAT_DATA"); d != "" {
		return d, nil
	}
	dir, err := gioapp.DataDir() // Context.getFilesDir() on Android
	if err != nil {
		return "", err
	}
	if runtime.GOOS != "android" {
		dir = filepath.Join(dir, "SmolChat")
	}
	return dir, nil
}

// keepAlive holds an Android foreground service while downloads run, so
// they survive the app being backgrounded.
func keepAlive() func(bool) {
	var mu sync.Mutex
	var stop func()
	return func(on bool) {
		mu.Lock()
		defer mu.Unlock()
		switch {
		case on && stop == nil:
			s, err := gioapp.StartForeground(gioapp.ForegroundOptions{Title: "SmolChat", Text: "Downloading models…"})
			if err != nil {
				log.Printf("foreground service: %v", err)
				return
			}
			stop = s
		case !on && stop != nil:
			stop()
			stop = nil
		}
	}
}

func newWindow() *gioapp.Window {
	w := new(gioapp.Window)
	w.Option(
		gioapp.Title("SmolChat"),
		gioapp.Size(410, 860),
		gioapp.MinSize(340, 560),
		gioapp.StatusColor(ui.Background),
		gioapp.NavigationColor(ui.Background),
	)
	win.Store(w)
	return w
}

func runWindow() {
	w := newWindow()
	u := ui.New()
	u.OnBarColor = func(c color.NRGBA) { // keep system bars matching the theme
		if w := win.Load(); w != nil {
			w.Option(gioapp.StatusColor(c), gioapp.NavigationColor(c))
		}
	}

	initDone := make(chan initResult, 1)
	go func() {
		dir, err := dataDir()
		if err != nil {
			initDone <- initResult{err: err}
			invalidate()
			return
		}
		a, err := app.New(app.Options{
			DataDir:    dir,
			Engine:     llama.New(),
			Invalidate: invalidate,
			KeepAlive:  keepAlive(),
		})
		initDone <- initResult{a, err}
		invalidate()
	}()

	var ops op.Ops
	for {
		switch e := w.Event().(type) {
		case gioapp.DestroyEvent:
			if a := core.Load(); a != nil {
				a.CheckpointStreaming()
				if runtime.GOOS != "android" {
					a.Close()
				}
			}
			if runtime.GOOS != "android" {
				os.Exit(0)
			}
			// Android: the process (and any foreground download) may live on;
			// a relaunch creates a new window on the same core.
			w = newWindow()
		default:
			platformEvent(e)
		case gioapp.FrameEvent:
			select {
			case r := <-initDone:
				if r.err != nil {
					log.Printf("init: %v", r.err)
				}
				core.Store(r.a)
				u.SetApp(r.a, r.err)
			default:
			}
			// Not app.NewContext: it would also apply e.Insets, and the UI
			// handles insets itself so overlays can extend edge to edge.
			ops.Reset()
			gtx := layout.Context{Ops: &ops, Now: e.Now, Source: e.Source, Metric: e.Metric, Constraints: layout.Exact(e.Size)}
			u.Layout(gtx, e.Insets.Top, e.Insets.Bottom)
			e.Frame(gtx.Ops)
		}
	}
}
