package app

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

const (
	AppName    = "SmolChat"
	AppVersion = "1.0.0"
	OrgName    = "SmolWorks"
)

// Paths groups the on-device storage locations. Root comes from the platform
// (Context.getFilesDir() on Android, via gioui.org/app.DataDir).
type Paths struct {
	Root     string
	Models   string // models/<id>/<file>.gguf
	Meta     string // installed-model manifests and the cached registry
	DB       string // conversation database file
	Settings string // settings.json
	Logs     string
	Tmp      string // partial downloads
}

// NewPaths derives and creates the storage layout under root.
func NewPaths(root string) (Paths, error) {
	p := Paths{
		Root:     root,
		Models:   filepath.Join(root, "models"),
		Meta:     filepath.Join(root, "meta"),
		DB:       filepath.Join(root, "db", "smolchat.db"),
		Settings: filepath.Join(root, "settings.json"),
		Logs:     filepath.Join(root, "logs"),
		Tmp:      filepath.Join(root, "tmp", "downloads"),
	}
	for _, d := range []string{p.Models, p.Meta, filepath.Dir(p.DB), p.Logs, p.Tmp} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return p, fmt.Errorf("create %s: %w", d, err)
		}
	}
	return p, nil
}

// SelectionMode is how the model for a conversation is chosen.
type SelectionMode string

const (
	SelectAuto   SelectionMode = "auto"
	SelectManual SelectionMode = "manual"
)

// Settings are persisted user preferences.
type Settings struct {
	SelectionMode SelectionMode `json:"selection_mode"`
	ManualModelID string        `json:"manual_model_id"`
	ContextSize   int           `json:"context_size"`
	MaxTokens     int           `json:"max_tokens"`
	Temperature   float32       `json:"temperature"`
	SystemPrompt  string        `json:"system_prompt"`
	Threads       int           `json:"threads"` // 0 = automatic

	// Appearance.
	Theme  string `json:"theme"`  // "system", "light" or "dark"
	Accent string `json:"accent"` // blue, violet, green, orange, pink, graphite

	// AutoPreference biases automatic selection: "speed", "balanced", "quality".
	AutoPreference string `json:"auto_preference"`
	// Thinking lets SmolLM3 reason step by step before answering.
	Thinking bool `json:"thinking"`
}

// DefaultSettings returns conservative, phone-friendly defaults.
func DefaultSettings() Settings {
	return Settings{
		SelectionMode:  SelectAuto,
		ContextSize:    2048,
		MaxTokens:      1024,
		Temperature:    0.7,
		SystemPrompt:   "You are SmolChat, a helpful assistant running locally on the user's phone. Answer clearly and concisely.",
		Theme:          "system",
		Accent:         "violet",
		AutoPreference: "balanced",
	}
}

func (s *Settings) normalize() {
	d := DefaultSettings()
	if s.SelectionMode != SelectManual {
		s.SelectionMode = SelectAuto
	}
	if s.ContextSize < 512 || s.ContextSize > 8192 {
		s.ContextSize = d.ContextSize
	}
	if s.MaxTokens < 64 || s.MaxTokens > 4096 {
		s.MaxTokens = d.MaxTokens
	}
	if s.Temperature < 0 || s.Temperature > 2 {
		s.Temperature = d.Temperature
	}
	if s.Threads < 0 || s.Threads > 16 {
		s.Threads = 0
	}
	switch s.Theme {
	case "system", "light", "dark":
	default:
		s.Theme = d.Theme
	}
	switch s.Accent {
	case "blue", "violet", "green", "orange", "pink", "graphite":
	default:
		s.Accent = d.Accent
	}
	switch s.AutoPreference {
	case "speed", "balanced", "quality":
	default:
		s.AutoPreference = d.AutoPreference
	}
}

// SettingsStore loads and atomically saves settings.json.
type SettingsStore struct {
	path string
	mu   sync.Mutex
	cur  Settings
}

// OpenSettings loads settings from path, falling back to defaults.
func OpenSettings(path string) (*SettingsStore, error) {
	st := &SettingsStore{path: path, cur: DefaultSettings()}
	b, err := os.ReadFile(path)
	switch {
	case errors.Is(err, os.ErrNotExist):
		return st, nil
	case err != nil:
		return st, err
	}
	s := DefaultSettings()
	if err := json.Unmarshal(b, &s); err != nil {
		return st, nil // corrupt file: keep defaults, it is rewritten on next save
	}
	s.normalize()
	st.cur = s
	return st, nil
}

// Get returns a copy of the current settings.
func (st *SettingsStore) Get() Settings {
	st.mu.Lock()
	defer st.mu.Unlock()
	return st.cur
}

// Update applies f and persists the result.
func (st *SettingsStore) Update(f func(*Settings)) error {
	st.mu.Lock()
	defer st.mu.Unlock()
	s := st.cur
	f(&s)
	s.normalize()
	st.cur = s
	return writeJSONAtomic(st.path, s)
}

func writeJSONAtomic(path string, v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
