package ui

import (
	"encoding/json"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"

	"gioui.org/gpu/headless"
	"gioui.org/io/input"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"

	"github.com/smolworks/smolchat-android/app"
	"github.com/smolworks/smolchat-android/native/llama"
)

// TestScreens renders every screen headlessly at phone resolution, which
// catches layout panics. With SMOLCHAT_SHOTS=<dir> it saves PNGs there; with
// SMOLCHAT_TEST_MODEL=<SmolLM2-135M gguf> it also runs a real conversation.
func TestScreens(t *testing.T) {
	out := os.Getenv("SMOLCHAT_SHOTS")
	const scale = 2.625 // a typical 420dpi phone
	sz := image.Pt(1080, 2340)
	win, err := headless.NewWindow(sz.X, sz.Y)
	if err != nil {
		t.Skipf("no GPU for headless rendering: %v", err)
	}
	defer win.Release()

	dir := t.TempDir()
	if model := os.Getenv("SMOLCHAT_TEST_MODEL"); model != "" {
		installModel(t, dir, model)
	}
	u := New()
	var router input.Router
	now := time.Now()
	bottom := unit.Dp(18)
	frame := func(name string, d time.Duration) {
		t.Helper()
		var ops op.Ops
		// Run a few frames so animations settle.
		for step := time.Duration(0); step <= d; step += 16 * time.Millisecond {
			ops.Reset()
			now = now.Add(16 * time.Millisecond)
			gtx := layout.Context{
				Ops: &ops, Now: now, Source: router.Source(),
				Metric:      unit.Metric{PxPerDp: scale, PxPerSp: scale},
				Constraints: layout.Exact(sz),
			}
			u.Layout(gtx, 28, bottom)
			router.Frame(&ops)
		}
		if err := win.Frame(&ops); err != nil {
			t.Fatal(err)
		}
		if out == "" {
			return
		}
		img := image.NewRGBA(image.Rectangle{Max: sz})
		if err := win.Screenshot(img); err != nil {
			t.Fatal(err)
		}
		f, err := os.Create(filepath.Join(out, name+".png"))
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		png.Encode(f, img)
	}

	frame("01a-splash", 500*time.Millisecond)
	frame("01b-splash", 1300*time.Millisecond)

	a, err := app.New(app.Options{DataDir: dir, Engine: llama.New()})
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	u.SetApp(a, nil)
	frame("02-welcome", 1500*time.Millisecond)
	if u.showing {
		t.Fatal("splash did not finish")
	}

	if len(a.Models.Installed()) > 0 {
		u.chat.editor.SetText("Hello! What can you do?")
		frame("03-composer", 50*time.Millisecond)
		u.chat.submit(u, u.chat.editor.Text(), a.Models.Installed())
		waitIdle(t, a)
		a.Send("Write a short Python function that adds two numbers, and explain it in a markdown list.")
		waitIdle(t, a)
		frame("04-conversation", 500*time.Millisecond)
		view := a.Chat()
		if len(view.Messages) != 4 || view.Messages[3].Content == "" || view.Messages[3].StopReason == "error" {
			t.Fatalf("conversation did not complete: %+v", view.Messages)
		}
		t.Logf("reply: %q (%.1f tok/s, model %s)", view.Messages[3].Content, view.Messages[3].TokensPerSec, view.Messages[3].ModelID)
	} else {
		// Without a model: persist a sample conversation to exercise markdown.
		c, _ := a.Store.CreateConversation("Markdown sample", app.SelectAuto, "")
		a.Store.AddMessage(&app.Message{ConversationID: c.ID, Role: "user", Content: "Show me some markdown"})
		a.Store.AddMessage(&app.Message{ConversationID: c.ID, Role: "assistant", ModelID: "smollm2-360m-instruct-q8", TokensPerSec: 31.2,
			Content: "## Heading\nSome **bold**, *italic* and `code`.\n\n1. First\n2. Second\n   - nested\n\n> A quote\n\n```go\nfunc add(a, b int) int {\n\treturn a + b // sum\n}\n```\n\n| A | B |\n|---|---|\n| 1 | 2 |"})
		a.OpenConversation(c.ID)
		frame("04-conversation", 500*time.Millisecond)
	}

	bottom = 336 // soft keyboard open
	frame("04b-keyboard", 100*time.Millisecond)
	bottom = 18

	if msgs := a.Chat().Messages; len(msgs) > 1 {
		u.msgMenu.Show(msgs[1], false)
		frame("15-message-menu", 600*time.Millisecond)
		u.back()
		frame("_", 500*time.Millisecond)
		u.sel.Open(msgs[1].Content)
		u.navigate(screenSelect)
		u.sel.txt.start, u.sel.txt.end = 0, 40
		frame("16-select-text", 600*time.Millisecond)
		u.back()
		frame("_", 600*time.Millisecond)
	}
	u.chat.edit.open = true
	u.chat.editor.SetText("draft message")
	u.chat.editor.SetCaret(0, 5)
	frame("17-edit-bar", 200*time.Millisecond)
	u.chat.edit.open = false
	u.chat.editor.SetText("")

	u.drawer.Open()
	frame("05-drawer", 600*time.Millisecond)
	u.drawer.Close()
	frame("_", 600*time.Millisecond)

	u.sheet.Show()
	frame("06-model-sheet", 600*time.Millisecond)
	u.sheet.open = false
	frame("_", 600*time.Millisecond)

	u.navigate(screenLibrary)
	frame("07-library", 600*time.Millisecond)
	u.lib.tab = 4 // Vision
	frame("08-library-vision", 300*time.Millisecond)
	u.lib.tab = 0

	u.navigate(screenSettings)
	frame("09-settings", 600*time.Millisecond)

	// Light theme with a blue accent, then dark orange (as in the Mac shots).
	a.ApplySettings(func(s *app.Settings) { s.Theme, s.Accent = "light", "blue" })
	frame("11-settings-light", 300*time.Millisecond)
	u.navigate(screenChat)
	a.NewChat()
	frame("12-welcome-light", 900*time.Millisecond)
	a.ApplySettings(func(s *app.Settings) { s.Theme, s.Accent = "dark", "orange" })
	frame("13-welcome-orange", 300*time.Millisecond)
	a.ApplySettings(func(s *app.Settings) { s.Theme, s.Accent = "dark", "violet" })
	u.drawer.Open()
	frame("14-drawer", 600*time.Millisecond)
	u.drawer.Close()
	frame("_", 600*time.Millisecond)
	u.navigate(screenSettings)

	u.navigate(screenChat)
	u.dialog.Confirm("Delete chat?", "“Markdown sample” will be permanently deleted.", "Delete", true, nil)
	frame("10-dialog", 600*time.Millisecond)
	if !u.back() || u.dialog.open {
		t.Fatal("back did not close the dialog")
	}
	frame("_", 400*time.Millisecond)
}

func waitIdle(t *testing.T, a *app.App) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Minute)
	time.Sleep(50 * time.Millisecond)
	for a.Chat().Streaming {
		if time.Now().After(deadline) {
			t.Fatal("generation timed out")
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// installModel registers a local GGUF as the SmolLM2 135M catalog entry.
func installModel(t *testing.T, dir, path string) {
	spec, _ := app.FindSpec("smollm2-135m-instruct-q8")
	p, err := app.NewPaths(dir)
	if err != nil {
		t.Fatal(err)
	}
	md := filepath.Join(p.Models, spec.ID)
	os.MkdirAll(md, 0o755)
	abs, _ := filepath.Abs(path)
	if err := os.Symlink(abs, filepath.Join(md, spec.Weights().Name)); err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(app.InstalledModel{ID: spec.ID, Repo: spec.Repo, Revision: spec.Revision, Files: spec.Files, Verified: true})
	os.WriteFile(filepath.Join(p.Meta, "installed-"+spec.ID+".json"), b, 0o644)
}
