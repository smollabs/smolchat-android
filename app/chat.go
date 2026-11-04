package app

import (
	"context"
	"errors"
	"strings"
	"time"
	"unicode/utf8"
)

// ChatView is the UI's snapshot of the open conversation.
type ChatView struct {
	Conv      Conversation // ID == "" for a new, unsaved chat
	Messages  []Message
	Streaming bool
	// RouteNote explains the automatic choice for the in-flight reply.
	RouteNote string
	// Live statistics of the reply being streamed.
	StreamStart  time.Time // first token
	StreamTokens int
}

// chatState is the mutable conversation state guarded by App.mu.
type chatState struct {
	view   ChatView
	cancel context.CancelFunc
	// lastSaved tracks the streaming checkpoint.
	lastSaved time.Time
}

func titleFrom(text string) string {
	t := strings.Join(strings.Fields(text), " ")
	if utf8.RuneCountInString(t) > 48 {
		r := []rune(t)
		t = strings.TrimSpace(string(r[:48])) + "…"
	}
	if t == "" {
		t = "New chat"
	}
	return t
}

// NewChat opens an empty conversation using the default selection settings.
func (a *App) NewChat() {
	s := a.Settings.Get()
	a.mu.Lock()
	if a.chat.view.Streaming {
		a.mu.Unlock()
		a.StopGeneration()
		a.mu.Lock()
	}
	a.chat = chatState{view: ChatView{Conv: Conversation{Title: "New chat", SelectionMode: s.SelectionMode, ModelID: s.ManualModelID}}}
	a.mu.Unlock()
	a.invalidate()
}

// OpenConversation loads a saved conversation.
func (a *App) OpenConversation(id string) error {
	c, err := a.Store.GetConversation(id)
	if err != nil {
		return err
	}
	msgs, err := a.Store.Messages(id)
	if err != nil {
		return err
	}
	a.StopGeneration()
	a.mu.Lock()
	a.chat = chatState{view: ChatView{Conv: c, Messages: msgs}}
	a.mu.Unlock()
	a.invalidate()
	return nil
}

// Chat returns a snapshot of the open conversation.
func (a *App) Chat() ChatView {
	a.mu.Lock()
	defer a.mu.Unlock()
	v := a.chat.view
	v.Messages = append([]Message(nil), v.Messages...)
	return v
}

// SetSelection changes how the current conversation picks its model and
// remembers it as the default for new chats.
func (a *App) SetSelection(mode SelectionMode, modelID string) {
	if mode == SelectAuto {
		modelID = ""
	}
	a.Settings.Update(func(s *Settings) { s.SelectionMode, s.ManualModelID = mode, modelID })
	a.mu.Lock()
	a.chat.view.Conv.SelectionMode, a.chat.view.Conv.ModelID = mode, modelID
	id := a.chat.view.Conv.ID
	a.mu.Unlock()
	if id != "" {
		a.report(a.Store.SetConversationModel(id, mode, modelID))
	}
	if mode == SelectManual && modelID != "" {
		a.Prewarm(modelID)
	}
	a.invalidate()
}

// Send posts a user message and starts streaming the reply.
func (a *App) Send(text string) {
	text = strings.TrimSpace(text)
	if text == "" {
		return
	}
	a.mu.Lock()
	if a.chat.view.Streaming {
		a.mu.Unlock()
		return
	}
	conv := a.chat.view.Conv
	a.mu.Unlock()

	if conv.ID == "" {
		c, err := a.Store.CreateConversation(titleFrom(text), conv.SelectionMode, conv.ModelID)
		if err != nil {
			a.report(err)
			return
		}
		conv = c
	}
	user := Message{ConversationID: conv.ID, Role: "user", Content: text}
	if err := a.Store.AddMessage(&user); err != nil {
		a.report(err)
		return
	}
	a.mu.Lock()
	a.chat.view.Conv = conv
	a.chat.view.Messages = append(a.chat.view.Messages, user)
	a.mu.Unlock()
	a.refreshConversations()
	a.generate()
}

// Regenerate replaces the last assistant reply.
func (a *App) Regenerate() {
	a.mu.Lock()
	msgs := a.chat.view.Messages
	if a.chat.view.Streaming || len(msgs) == 0 || msgs[len(msgs)-1].Role != "assistant" {
		a.mu.Unlock()
		return
	}
	last := msgs[len(msgs)-1]
	a.chat.view.Messages = msgs[:len(msgs)-1]
	a.mu.Unlock()
	a.report(a.Store.DeleteMessage(last.ID))
	a.generate()
}

// StopGeneration interrupts the reply in progress.
func (a *App) StopGeneration() {
	a.mu.Lock()
	c := a.chat.cancel
	a.mu.Unlock()
	if c != nil {
		c()
	}
	a.Runtime.Cancel()
}

// history converts the conversation into engine messages.
func history(system string, msgs []Message) []ChatMessage {
	out := make([]ChatMessage, 0, len(msgs)+1)
	if system != "" {
		out = append(out, ChatMessage{Role: "system", Content: system})
	}
	for _, m := range msgs {
		if m.StopReason == "error" || strings.TrimSpace(m.Content) == "" {
			continue
		}
		out = append(out, ChatMessage{Role: m.Role, Content: stripThink(m.Content)})
	}
	return out
}

// stripThink removes <think>…</think> reasoning so it is not fed back.
func stripThink(s string) string {
	for {
		i := strings.Index(s, "<think>")
		if i < 0 {
			return s
		}
		j := strings.Index(s[i:], "</think>")
		if j < 0 {
			return strings.TrimSpace(s[:i])
		}
		s = s[:i] + strings.TrimLeft(s[i+j+len("</think>"):], "\n ")
	}
}

// pickModel resolves the model for the current turn.
func (a *App) pickModel(conv Conversation, query string, turns int) (ModelSpec, string, error) {
	if conv.SelectionMode == SelectManual && conv.ModelID != "" {
		spec, ok := FindSpec(conv.ModelID)
		if ok && a.Models.IsInstalled(spec.ID) {
			return spec, "", nil
		}
		return ModelSpec{}, "", errors.New("the model selected for this chat is no longer downloaded; pick another model or switch to Auto")
	}
	r, err := SelectModel(RouteRequest{Query: query, HistoryTurns: turns}, RouteInput{
		Installed:   a.Models.Installed(),
		Device:      a.Device(),
		ContextSize: a.Settings.Get().ContextSize,
		LoadedID:    a.Runtime.Status().ModelID,
	})
	if err != nil {
		return ModelSpec{}, "", err
	}
	return r.Model, r.Reason, nil
}

func (a *App) generate() {
	settings := a.Settings.Get()
	a.mu.Lock()
	conv := a.chat.view.Conv
	prior := append([]Message(nil), a.chat.view.Messages...)
	ctx, cancel := context.WithCancel(context.Background())
	a.chat.cancel = cancel
	a.chat.view.Streaming = true
	a.chat.view.RouteNote = ""
	a.chat.view.StreamStart, a.chat.view.StreamTokens = time.Time{}, 0
	a.mu.Unlock()
	a.invalidate()

	go func() {
		defer cancel()
		query := ""
		if n := len(prior); n > 0 {
			query = prior[n-1].Content
		}
		reply := Message{ConversationID: conv.ID, Role: "assistant"}
		spec, reason, err := a.pickModel(conv, query, len(prior)/2)
		if err == nil {
			reply.ModelID, reply.RouteReason = spec.ID, reason
		}
		if err := a.Store.AddMessage(&reply); err != nil {
			a.report(err)
		}
		a.mu.Lock()
		a.chat.view.Messages = append(a.chat.view.Messages, reply)
		a.chat.view.RouteNote = reason
		a.chat.lastSaved = time.Now()
		a.mu.Unlock()
		a.invalidate()

		if err == nil {
			if conv.SelectionMode != SelectManual {
				a.report(a.Store.SetConversationModel(conv.ID, SelectAuto, spec.ID))
			}
			err = a.runGeneration(ctx, spec, settings, prior, reply.ID)
		}
		a.finishReply(reply.ID, err)
	}()
}

func (a *App) runGeneration(ctx context.Context, spec ModelSpec, s Settings, prior []Message, replyID int64) error {
	system := s.SystemPrompt
	if spec.Family == "SmolLM3" {
		system = strings.TrimSpace(system + "\n/no_think") // keep phone replies fast
	}
	cfg := ModelConfig{ID: spec.ID, Path: a.Models.WeightsPath(spec), ContextSize: s.ContextSize, Threads: s.Threads}
	opts := GenerationOptions{
		MaxTokens: s.MaxTokens, Temperature: s.Temperature,
		TopK: 40, TopP: 0.95, MinP: 0.05, RepeatPenalty: 1.1, Seed: uint32(time.Now().UnixNano()),
	}
	var genErr error
	// The callback runs on this generation goroutine, never the UI loop.
	err := a.Runtime.Generate(ctx, cfg, history(system, prior), opts, func(t Token) {
		a.mu.Lock()
		m := a.findMsg(replyID)
		if m == nil {
			a.mu.Unlock()
			return
		}
		if t.Done {
			genErr = t.Err
			m.Tokens = t.Stats.GeneratedTokens
			m.TokensPerSec = t.Stats.TokensPerSecond
			m.DurationMS = t.Stats.Duration.Milliseconds()
			m.StopReason = t.Stats.StopReason
			a.mu.Unlock()
			return
		}
		m.Content += t.Text
		if a.chat.view.StreamTokens == 0 {
			a.chat.view.StreamStart = time.Now()
		}
		a.chat.view.StreamTokens++
		var checkpoint *Message
		if time.Since(a.chat.lastSaved) > 2*time.Second { // crash/kill safety
			a.chat.lastSaved = time.Now()
			snapshot := *m
			checkpoint = &snapshot
		}
		a.mu.Unlock()
		if checkpoint != nil {
			a.report(a.Store.UpdateMessage(*checkpoint))
		}
		a.invalidate()
	})
	if err != nil {
		return err
	}
	return genErr
}

func (a *App) findMsg(id int64) *Message {
	for i := range a.chat.view.Messages {
		if a.chat.view.Messages[i].ID == id {
			return &a.chat.view.Messages[i]
		}
	}
	return nil
}

func (a *App) finishReply(id int64, err error) {
	a.mu.Lock()
	m := a.findMsg(id)
	var final Message
	if m != nil {
		switch {
		case errors.Is(err, context.Canceled):
			m.StopReason = "cancelled"
		case errors.Is(err, ErrNoModel):
			m.StopReason, m.Content = "error", "No model is downloaded yet. Open the Model Library to download one — it only takes a minute, and then SmolChat works offline."
		case err != nil:
			m.StopReason, m.Content = "error", friendlyError(err)
		}
		if m.StopReason == "" {
			m.StopReason = "eos"
		}
		final = *m
	}
	a.chat.view.Streaming = false
	a.chat.cancel = nil
	a.mu.Unlock()
	if m != nil {
		a.report(a.Store.UpdateMessage(final))
	}
	a.refreshConversations()
	a.invalidate()
}

func friendlyError(err error) string {
	msg := err.Error()
	if strings.Contains(msg, "context") && strings.Contains(msg, "memory") {
		return "The model ran out of memory. Try a smaller model or a shorter context in Settings."
	}
	return "Something went wrong: " + msg
}

// CheckpointStreaming persists the partial reply (called when the app goes
// to the background, where Android may kill it without notice).
func (a *App) CheckpointStreaming() {
	a.mu.Lock()
	if !a.chat.view.Streaming || len(a.chat.view.Messages) == 0 {
		a.mu.Unlock()
		return
	}
	m := a.chat.view.Messages[len(a.chat.view.Messages)-1]
	a.mu.Unlock()
	a.report(a.Store.UpdateMessage(m))
}
