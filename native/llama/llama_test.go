package llama

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/smolworks/smolchat-android/app"
)

func TestValidPrefix(t *testing.T) {
	b := []byte("héllo")
	// Split the é (0xC3 0xA9) across two "tokens".
	first := append([]byte{}, b[:2]...)
	if got := validPrefix(&first); got != "h" || len(first) != 1 {
		t.Fatalf("got %q, pending %v", got, first)
	}
	first = append(first, b[2:]...)
	if got := validPrefix(&first); got != "éllo" || len(first) != 0 {
		t.Fatalf("got %q, pending %v", got, first)
	}
}

// TestGenerate runs real inference. Set SMOLCHAT_TEST_MODEL to a GGUF path.
func TestGenerate(t *testing.T) {
	path := os.Getenv("SMOLCHAT_TEST_MODEL")
	if path == "" {
		t.Skip("SMOLCHAT_TEST_MODEL not set")
	}
	e := New()
	var lastProgress float32
	if err := e.LoadModel(context.Background(), app.ModelConfig{ID: "t", Path: path, ContextSize: 1024}, func(p float32) { lastProgress = p }); err != nil {
		t.Fatal(err)
	}
	defer e.UnloadModel()
	if lastProgress != 1 {
		t.Errorf("final progress %v", lastProgress)
	}
	msgs := []app.ChatMessage{{Role: "user", Content: "What is the capital of France? Answer in one word."}}
	opts := app.GenerationOptions{MaxTokens: 32, Temperature: 0}
	reply, stats := collect(t, e, msgs, opts)
	t.Logf("reply %q stats %+v", reply, stats)
	if !strings.Contains(strings.ToLower(reply), "paris") {
		t.Errorf("unexpected reply %q", reply)
	}

	// Second turn reuses the KV cache prefix.
	msgs = append(msgs, app.ChatMessage{Role: "assistant", Content: reply}, app.ChatMessage{Role: "user", Content: "And of Italy?"})
	reply2, stats2 := collect(t, e, msgs, opts)
	t.Logf("reply2 %q stats %+v", reply2, stats2)

	// Cancellation stops promptly.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ch, err := e.Generate(ctx, []app.ChatMessage{{Role: "user", Content: "Write a long story about a robot."}}, app.GenerationOptions{MaxTokens: 2000, Temperature: 0.7, TopK: 40, TopP: 0.95})
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	var final app.Token
	for tok := range ch {
		if tok.Done {
			final = tok
			break
		}
		n++
		if n == 5 {
			cancel()
		}
	}
	if final.Stats.StopReason != "cancelled" {
		t.Errorf("stop reason %q after cancel", final.Stats.StopReason)
	}
	// Engine must be usable again right away.
	if _, _ = collect(t, e, msgs[:1], opts); !e.IsLoaded() {
		t.Fatal("unloaded")
	}
}

func collect(t *testing.T, e *Engine, msgs []app.ChatMessage, opts app.GenerationOptions) (string, app.GenStats) {
	t.Helper()
	ch, err := e.Generate(context.Background(), msgs, opts)
	if err != nil {
		t.Fatal(err)
	}
	var sb strings.Builder
	timeout := time.After(2 * time.Minute)
	for {
		select {
		case tok, ok := <-ch:
			if !ok {
				t.Fatal("channel closed without Done")
			}
			if tok.Done {
				if tok.Err != nil {
					t.Fatal(tok.Err)
				}
				return sb.String(), tok.Stats
			}
			sb.WriteString(tok.Text)
		case <-timeout:
			t.Fatal("timeout")
		}
	}
}
