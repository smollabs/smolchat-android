package llama

import (
	"context"
	"os"
	"testing"

	"github.com/smolworks/smolchat-android/app"
)

func TestPromptFormat(t *testing.T) {
	path := os.Getenv("SMOLCHAT_TEST_MODEL")
	if path == "" {
		t.Skip("SMOLCHAT_TEST_MODEL not set")
	}
	e := New()
	if err := e.LoadModel(context.Background(), app.ModelConfig{Path: path, ContextSize: 1024}, nil); err != nil {
		t.Fatal(err)
	}
	defer e.UnloadModel()
	toks, err := e.tokenizeChat([]app.ChatMessage{{Role: "system", Content: "You are helpful."}, {Role: "user", Content: "hi"}})
	if err != nil {
		t.Fatal(err)
	}
	var out []byte
	buf := make([]byte, 64)
	for _, tk := range toks {
		n := int(smolPiece(e, tk, buf))
		out = append(out, buf[:n]...)
	}
	t.Logf("%d tokens: %q", len(toks), out)
}
