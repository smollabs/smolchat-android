package ui

import (
	"testing"

	"github.com/smolworks/smolchat-android/app"
)

func TestNavigationStack(t *testing.T) {
	a, err := app.New(app.Options{DataDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	u := New()
	u.SetApp(a, nil)

	if u.back() {
		t.Fatal("back at the root must be left to Android (exit)")
	}
	u.navigate(screenSettings)
	u.navigate(screenLibrary) // Settings → Model Library
	if u.screen != screenLibrary || len(u.stack) != 2 {
		t.Fatalf("screen %v stack %v", u.screen, u.stack)
	}
	u.drawer.Open()
	if !u.back() || u.drawer.IsOpen() || u.screen != screenLibrary {
		t.Fatal("back should close the drawer first")
	}
	if !u.back() || u.screen != screenSettings {
		t.Fatalf("back should return to Settings, got %v", u.screen)
	}
	if !u.back() || u.screen != screenChat || len(u.stack) != 0 {
		t.Fatalf("back should return to chat, got %v %v", u.screen, u.stack)
	}
	if u.canGoBack() {
		t.Fatal("nothing left to go back to")
	}

	// A completed swipe-back must not leave the revealed page offset.
	u.navigate(screenLibrary)
	u.pageSpring.Set(300)
	u.pop(300)
	if u.pageSpring.V != 0 || u.pageSpring.Target != 0 {
		t.Fatalf("page offset left at %v", u.pageSpring.V)
	}

	// "Use this model" style navigation unwinds to the chat.
	u.navigate(screenSettings)
	u.navigate(screenLibrary)
	u.navigate(screenChat)
	if u.screen != screenChat || len(u.stack) != 0 {
		t.Fatalf("navigate(chat) should unwind, got %v %v", u.screen, u.stack)
	}

	u.msgMenu.Show(app.Message{Role: "user", Content: "hi"}, true)
	if !u.back() || u.msgMenu.open {
		t.Fatal("back should close the message menu")
	}
}
