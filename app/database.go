package app

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

// Conversation is a chat thread.
type Conversation struct {
	ID            string
	Title         string
	CreatedAt     time.Time
	UpdatedAt     time.Time
	ModelID       string // manual model, or last auto-selected model
	SelectionMode SelectionMode
}

// Message is one persisted chat message.
type Message struct {
	ID             int64
	ConversationID string
	Role           string // "user" | "assistant"
	Content        string
	ModelID        string
	CreatedAt      time.Time
	// Generation metadata (assistant messages).
	Tokens       int
	TokensPerSec float64
	DurationMS   int64
	StopReason   string
	RouteReason  string
}

// Store persists conversations in SQLite.
type Store struct {
	db *sql.DB
}

const schema = `
CREATE TABLE IF NOT EXISTS conversations (
	id             TEXT PRIMARY KEY,
	title          TEXT NOT NULL,
	created_at     INTEGER NOT NULL,
	updated_at     INTEGER NOT NULL,
	model_id       TEXT NOT NULL DEFAULT '',
	selection_mode TEXT NOT NULL DEFAULT 'auto'
);
CREATE TABLE IF NOT EXISTS messages (
	id              INTEGER PRIMARY KEY AUTOINCREMENT,
	conversation_id TEXT NOT NULL REFERENCES conversations(id) ON DELETE CASCADE,
	role            TEXT NOT NULL,
	content         TEXT NOT NULL,
	model_id        TEXT NOT NULL DEFAULT '',
	created_at      INTEGER NOT NULL,
	tokens          INTEGER NOT NULL DEFAULT 0,
	tokens_per_sec  REAL NOT NULL DEFAULT 0,
	duration_ms     INTEGER NOT NULL DEFAULT 0,
	stop_reason     TEXT NOT NULL DEFAULT '',
	route_reason    TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS messages_conv ON messages(conversation_id, id);
CREATE INDEX IF NOT EXISTS conversations_updated ON conversations(updated_at DESC);
PRAGMA user_version = 1;
`

// OpenStore opens (and migrates) the database at path.
func OpenStore(path string) (*Store, error) {
	db, err := sql.Open("sqlite3", "file:"+path+"?_foreign_keys=on&_journal_mode=WAL&_busy_timeout=5000")
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1) // SQLite: one writer; avoids SQLITE_BUSY between goroutines
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate database: %w", err)
	}
	return &Store{db: db}, nil
}

func (s *Store) Close() error { return s.db.Close() }

func newID() string {
	return fmt.Sprintf("c%x", time.Now().UnixNano())
}

func ms(t time.Time) int64 { return t.UnixMilli() }

// CreateConversation inserts a new conversation.
func (s *Store) CreateConversation(title string, mode SelectionMode, modelID string) (Conversation, error) {
	now := time.Now()
	c := Conversation{ID: newID(), Title: title, CreatedAt: now, UpdatedAt: now, SelectionMode: mode, ModelID: modelID}
	_, err := s.db.Exec(`INSERT INTO conversations(id,title,created_at,updated_at,model_id,selection_mode) VALUES(?,?,?,?,?,?)`,
		c.ID, c.Title, ms(now), ms(now), c.ModelID, string(c.SelectionMode))
	return c, err
}

func scanConversations(rows *sql.Rows) ([]Conversation, error) {
	defer rows.Close()
	var out []Conversation
	for rows.Next() {
		var c Conversation
		var created, updated int64
		var mode string
		if err := rows.Scan(&c.ID, &c.Title, &created, &updated, &c.ModelID, &mode); err != nil {
			return nil, err
		}
		c.CreatedAt, c.UpdatedAt = time.UnixMilli(created), time.UnixMilli(updated)
		c.SelectionMode = SelectionMode(mode)
		out = append(out, c)
	}
	return out, rows.Err()
}

const convCols = `id,title,created_at,updated_at,model_id,selection_mode`

// ListConversations returns conversations, most recently updated first.
func (s *Store) ListConversations() ([]Conversation, error) {
	rows, err := s.db.Query(`SELECT ` + convCols + ` FROM conversations ORDER BY updated_at DESC`)
	if err != nil {
		return nil, err
	}
	return scanConversations(rows)
}

// SearchConversations matches titles and message text (case-insensitive).
func (s *Store) SearchConversations(q string) ([]Conversation, error) {
	q = strings.TrimSpace(q)
	if q == "" {
		return s.ListConversations()
	}
	pat := "%" + escapeLike(q) + "%"
	rows, err := s.db.Query(`SELECT `+convCols+` FROM conversations c WHERE title LIKE ? ESCAPE '\'
		OR EXISTS (SELECT 1 FROM messages m WHERE m.conversation_id = c.id AND m.content LIKE ? ESCAPE '\')
		ORDER BY updated_at DESC`, pat, pat)
	if err != nil {
		return nil, err
	}
	return scanConversations(rows)
}

func escapeLike(s string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(s)
}

// GetConversation loads one conversation.
func (s *Store) GetConversation(id string) (Conversation, error) {
	rows, err := s.db.Query(`SELECT `+convCols+` FROM conversations WHERE id = ?`, id)
	if err != nil {
		return Conversation{}, err
	}
	cs, err := scanConversations(rows)
	if err != nil {
		return Conversation{}, err
	}
	if len(cs) == 0 {
		return Conversation{}, sql.ErrNoRows
	}
	return cs[0], nil
}

// RenameConversation sets a new title.
func (s *Store) RenameConversation(id, title string) error {
	title = strings.TrimSpace(title)
	if title == "" {
		return errors.New("title cannot be empty")
	}
	_, err := s.db.Exec(`UPDATE conversations SET title = ? WHERE id = ?`, title, id)
	return err
}

// SetConversationModel records the selection mode and model.
func (s *Store) SetConversationModel(id string, mode SelectionMode, modelID string) error {
	_, err := s.db.Exec(`UPDATE conversations SET selection_mode = ?, model_id = ? WHERE id = ?`, string(mode), modelID, id)
	return err
}

// DeleteConversation removes a conversation and its messages.
func (s *Store) DeleteConversation(id string) error {
	_, err := s.db.Exec(`DELETE FROM conversations WHERE id = ?`, id)
	return err
}

// DeleteAllConversations wipes the chat history.
func (s *Store) DeleteAllConversations() error {
	_, err := s.db.Exec(`DELETE FROM conversations`)
	return err
}

// AddMessage appends a message and bumps the conversation's updated time.
func (s *Store) AddMessage(m *Message) error {
	if m.CreatedAt.IsZero() {
		m.CreatedAt = time.Now()
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	res, err := tx.Exec(`INSERT INTO messages(conversation_id,role,content,model_id,created_at,tokens,tokens_per_sec,duration_ms,stop_reason,route_reason)
		VALUES(?,?,?,?,?,?,?,?,?,?)`, m.ConversationID, m.Role, m.Content, m.ModelID, ms(m.CreatedAt),
		m.Tokens, m.TokensPerSec, m.DurationMS, m.StopReason, m.RouteReason)
	if err != nil {
		return err
	}
	if m.ID, err = res.LastInsertId(); err != nil {
		return err
	}
	if _, err := tx.Exec(`UPDATE conversations SET updated_at = ? WHERE id = ?`, ms(m.CreatedAt), m.ConversationID); err != nil {
		return err
	}
	return tx.Commit()
}

// UpdateMessage rewrites a message's content and metadata (streaming
// checkpoints and final results).
func (s *Store) UpdateMessage(m Message) error {
	_, err := s.db.Exec(`UPDATE messages SET content=?, model_id=?, tokens=?, tokens_per_sec=?, duration_ms=?, stop_reason=?, route_reason=? WHERE id=?`,
		m.Content, m.ModelID, m.Tokens, m.TokensPerSec, m.DurationMS, m.StopReason, m.RouteReason, m.ID)
	return err
}

// DeleteMessage removes one message (used by regenerate).
func (s *Store) DeleteMessage(id int64) error {
	_, err := s.db.Exec(`DELETE FROM messages WHERE id = ?`, id)
	return err
}

// Messages returns a conversation's messages in order.
func (s *Store) Messages(convID string) ([]Message, error) {
	rows, err := s.db.Query(`SELECT id,conversation_id,role,content,model_id,created_at,tokens,tokens_per_sec,duration_ms,stop_reason,route_reason
		FROM messages WHERE conversation_id = ? ORDER BY id`, convID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Message
	for rows.Next() {
		var m Message
		var created int64
		if err := rows.Scan(&m.ID, &m.ConversationID, &m.Role, &m.Content, &m.ModelID, &created,
			&m.Tokens, &m.TokensPerSec, &m.DurationMS, &m.StopReason, &m.RouteReason); err != nil {
			return nil, err
		}
		m.CreatedAt = time.UnixMilli(created)
		out = append(out, m)
	}
	return out, rows.Err()
}
