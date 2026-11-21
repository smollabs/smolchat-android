package tests

import (
    "encoding/json"
    "os"
    "path/filepath"
    "testing"
    "time"

    "github.com/smollabs/smolchat-android/app"
)

func newChatTestApp(t *testing.T) *app.App {
    t.Helper()

    root := t.TempDir()
    paths, err := app.NewPaths(root)
    if err != nil {
        t.Fatal(err)
    }

    model := spec(t, "smollm2-135m-instruct-q8")
    modelDir := filepath.Join(paths.Models, model.ID)

    if err := os.MkdirAll(modelDir, 0755); err != nil {
        t.Fatal(err)
    }

    if err := os.WriteFile(filepath.Join(modelDir, "test.gguf"), nil, 0644); err != nil {
        t.Fatal(err)
    }

    manifest := app.InstalledModel{
        ID: model.ID,
        Files: []app.ModelFile{
            {Name: "test.gguf", Size: 0},
        },
    }

    data, err := json.Marshal(manifest)
    if err != nil {
        t.Fatal(err)
    }

    if err := os.WriteFile(filepath.Join(paths.Meta, "installed-"+model.ID+".json"), data, 0644); err != nil {
        t.Fatal(err)
    }

    a, err := app.New(app.Options{
        DataDir: root,
        Engine:  &fakeEngine{},
    })
    if err != nil {
        t.Fatal(err)
    }

    t.Cleanup(func() {
        a.Close()
    })

    return a
}

func TestChatSendSavesMessages(t *testing.T) {
    a := newChatTestApp(t)

    a.Send("Hello")

    deadline := time.Now().Add(5 * time.Second)
    for a.Chat().Streaming && time.Now().Before(deadline) {
        time.Sleep(10 * time.Millisecond)
    }

    chat := a.Chat()
    if chat.Streaming {
        t.Fatal("generation did not finish")
    }

    if chat.Conv.ID == "" {
        t.Fatal("Send did not create a conversation")
    }

    if len(chat.Messages) != 2 {
        t.Fatalf("got %d messages, want 2", len(chat.Messages))
    }

    if chat.Messages[0].Role != "user" || chat.Messages[0].Content != "Hello" {
        t.Errorf("unexpected user message: %+v", chat.Messages[0])
    }

    if chat.Messages[1].Role != "assistant" || chat.Messages[1].Content != "hi" {
        t.Errorf("unexpected assistant message: %+v", chat.Messages[1])
    }

    saved, err := a.Store.Messages(chat.Conv.ID)
    if err != nil {
        t.Fatal(err)
    }
    if len(saved) != 2 {
        t.Fatalf("database contains %d messages, want 2", len(saved))
    }
}

func TestChatRegenerateReplacesAssistantReply(t *testing.T) {
    a := newChatTestApp(t)

    a.Send("Hello")

    deadline := time.Now().Add(5 * time.Second)
    for a.Chat().Streaming && time.Now().Before(deadline) {
        time.Sleep(10 * time.Millisecond)
    }

    before := a.Chat()
    if before.Streaming || len(before.Messages) != 2 {
        t.Fatalf("initial generation did not finish: %+v", before)
    }

    a.Regenerate()

    deadline = time.Now().Add(5 * time.Second)
    for a.Chat().Streaming && time.Now().Before(deadline) {
        time.Sleep(10 * time.Millisecond)
    }

    after := a.Chat()
    if after.Streaming {
        t.Fatal("regeneration did not finish")
    }
    if len(after.Messages) != 2 {
        t.Fatalf("got %d messages after regeneration, want 2", len(after.Messages))
    }
    if after.Messages[0].Role != "user" || after.Messages[1].Role != "assistant" {
        t.Fatalf("unexpected messages after regeneration: %+v", after.Messages)
    }
    if after.Messages[1].Content != "hi" {
        t.Errorf("got regenerated reply %q, want %q", after.Messages[1].Content, "hi")
    }
}


