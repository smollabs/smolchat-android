package tests

import (
	"path/filepath"
	"testing"

	"github.com/smolworks/smolchat-android/app"
)

func openStore(t *testing.T) *app.Store {
	t.Helper()
	s, err := app.OpenStore(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestConversationLifecycle(t *testing.T) {
	s := openStore(t)
	c, err := s.CreateConversation("Trip to Lisbon", app.SelectAuto, "")
	if err != nil {
		t.Fatal(err)
	}
	u := app.Message{ConversationID: c.ID, Role: "user", Content: "What should I pack?"}
	if err := s.AddMessage(&u); err != nil {
		t.Fatal(err)
	}
	r := app.Message{ConversationID: c.ID, Role: "assistant", Content: "Light layers and comfortable shoes."}
	if err := s.AddMessage(&r); err != nil {
		t.Fatal(err)
	}
	r.Content += " Bring sunscreen."
	r.Tokens, r.TokensPerSec, r.StopReason, r.ModelID = 12, 31.5, "eos", "smollm2-360m-instruct-q8"
	if err := s.UpdateMessage(r); err != nil {
		t.Fatal(err)
	}

	msgs, err := s.Messages(c.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 2 || msgs[0].Role != "user" || msgs[1].Content != "Light layers and comfortable shoes. Bring sunscreen." {
		t.Fatalf("messages: %+v", msgs)
	}
	if msgs[1].Tokens != 12 || msgs[1].TokensPerSec != 31.5 || msgs[1].ModelID != "smollm2-360m-instruct-q8" {
		t.Fatalf("metadata not persisted: %+v", msgs[1])
	}

	if err := s.RenameConversation(c.ID, "Lisbon packing"); err != nil {
		t.Fatal(err)
	}
	if err := s.RenameConversation(c.ID, "   "); err == nil {
		t.Fatal("empty title accepted")
	}
	if err := s.SetConversationModel(c.ID, app.SelectManual, "smollm2-135m-instruct-q8"); err != nil {
		t.Fatal(err)
	}
	got, err := s.GetConversation(c.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Title != "Lisbon packing" || got.SelectionMode != app.SelectManual || got.ModelID != "smollm2-135m-instruct-q8" {
		t.Fatalf("conversation: %+v", got)
	}

	if err := s.DeleteMessage(r.ID); err != nil {
		t.Fatal(err)
	}
	if msgs, _ := s.Messages(c.ID); len(msgs) != 1 {
		t.Fatalf("after delete: %d messages", len(msgs))
	}

	if err := s.DeleteConversation(c.ID); err != nil {
		t.Fatal(err)
	}
	if msgs, _ := s.Messages(c.ID); len(msgs) != 0 {
		t.Fatal("messages not cascaded")
	}
}

func TestSearchAndOrdering(t *testing.T) {
	s := openStore(t)
	a, _ := s.CreateConversation("Go generics", app.SelectAuto, "")
	b, _ := s.CreateConversation("Dinner ideas", app.SelectAuto, "")
	s.AddMessage(&app.Message{ConversationID: b.ID, Role: "user", Content: "Something with 100% chickpeas"})
	s.AddMessage(&app.Message{ConversationID: a.ID, Role: "user", Content: "explain type parameters"})

	all, err := s.ListConversations()
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 2 || all[0].ID != a.ID {
		t.Fatalf("expected most recently updated first, got %+v", all)
	}

	for q, want := range map[string]string{"generics": a.ID, "CHICKPEA": b.ID, "100%": b.ID, "type param": a.ID} {
		res, err := s.SearchConversations(q)
		if err != nil {
			t.Fatal(err)
		}
		if len(res) != 1 || res[0].ID != want {
			t.Errorf("search %q: %+v", q, res)
		}
	}
	if res, _ := s.SearchConversations("_"); len(res) != 0 {
		t.Errorf("LIKE wildcard not escaped: %+v", res)
	}
	if err := s.DeleteAllConversations(); err != nil {
		t.Fatal(err)
	}
	if all, _ := s.ListConversations(); len(all) != 0 {
		t.Fatal("not cleared")
	}
}

func TestPersistsAcrossReopen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "p.db")
	s, err := app.OpenStore(path)
	if err != nil {
		t.Fatal(err)
	}
	c, _ := s.CreateConversation("Kept", app.SelectAuto, "")
	s.AddMessage(&app.Message{ConversationID: c.ID, Role: "user", Content: "hello"})
	s.Close()

	s2, err := app.OpenStore(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s2.Close()
	msgs, err := s2.Messages(c.ID)
	if err != nil || len(msgs) != 1 || msgs[0].Content != "hello" {
		t.Fatalf("reopen: %v %+v", err, msgs)
	}
}
