package chatstore

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

type Store struct{ db *sql.DB }

type ChatSession struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type ChatMessage struct {
	ID        int64     `json:"id"`
	SessionID string    `json:"session_id"`
	Role      string    `json:"role"`
	Content   string    `json:"content"`
	CreatedAt time.Time `json:"created_at"`
}

func Open(path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return nil, fmt.Errorf("mkdir: %w", err)
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	if err := migrate(db); err != nil {
		return nil, err
	}
	return &Store{db: db}, nil
}

func migrate(db *sql.DB) error {
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS chat_sessions (
			id         TEXT PRIMARY KEY,
			title      TEXT NOT NULL DEFAULT 'New Chat',
			created_at DATETIME NOT NULL DEFAULT (datetime('now')),
			updated_at DATETIME NOT NULL DEFAULT (datetime('now'))
		);
		CREATE TABLE IF NOT EXISTS chat_messages (
			id         INTEGER PRIMARY KEY AUTOINCREMENT,
			session_id TEXT NOT NULL REFERENCES chat_sessions(id) ON DELETE CASCADE,
			role       TEXT NOT NULL,
			content    TEXT NOT NULL,
			created_at DATETIME NOT NULL DEFAULT (datetime('now'))
		);
		CREATE INDEX IF NOT EXISTS idx_messages_session ON chat_messages(session_id);
	`)
	return err
}

func (s *Store) CreateSession(ctx context.Context, id, title string) error {
	if title == "" {
		title = "New Chat"
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO chat_sessions (id, title) VALUES (?, ?)`, id, title)
	return err
}

func (s *Store) ListSessions(ctx context.Context) ([]ChatSession, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, title, created_at, updated_at FROM chat_sessions ORDER BY updated_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ChatSession
	for rows.Next() {
		var sess ChatSession
		var ca, ua string
		if err := rows.Scan(&sess.ID, &sess.Title, &ca, &ua); err != nil {
			continue
		}
		sess.CreatedAt, _ = time.Parse("2006-01-02T15:04:05Z", ca)
		sess.UpdatedAt, _ = time.Parse("2006-01-02T15:04:05Z", ua)
		out = append(out, sess)
	}
	return out, nil
}

func (s *Store) GetSession(ctx context.Context, id string) (*ChatSession, error) {
	var sess ChatSession
	var ca, ua string
	err := s.db.QueryRowContext(ctx,
		`SELECT id, title, created_at, updated_at FROM chat_sessions WHERE id = ?`, id).
		Scan(&sess.ID, &sess.Title, &ca, &ua)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	sess.CreatedAt, _ = time.Parse("2006-01-02T15:04:05Z", ca)
	sess.UpdatedAt, _ = time.Parse("2006-01-02T15:04:05Z", ua)
	return &sess, nil
}

func (s *Store) DeleteSession(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM chat_sessions WHERE id = ?`, id)
	return err
}

func (s *Store) AppendMessage(ctx context.Context, sessionID, role, content string) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO chat_messages (session_id, role, content) VALUES (?, ?, ?)`,
		sessionID, role, content)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx,
		`UPDATE chat_sessions SET updated_at = datetime('now') WHERE id = ?`, sessionID)
	return err
}

func (s *Store) GetMessages(ctx context.Context, sessionID string) ([]ChatMessage, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, session_id, role, content, created_at FROM chat_messages WHERE session_id = ? ORDER BY id`,
		sessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ChatMessage
	for rows.Next() {
		var m ChatMessage
		var ca string
		if err := rows.Scan(&m.ID, &m.SessionID, &m.Role, &m.Content, &ca); err != nil {
			continue
		}
		m.CreatedAt, _ = time.Parse("2006-01-02T15:04:05Z", ca)
		out = append(out, m)
	}
	return out, nil
}

func (s *Store) UpdateTitle(ctx context.Context, sessionID, title string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE chat_sessions SET title = ?, updated_at = datetime('now') WHERE id = ?`,
		title, sessionID)
	return err
}

// ErrNoSuchMessage means the chat has fewer user messages than the index asked for.
var ErrNoSuchMessage = errors.New("no such message")

// userMessageID is the row id of the chat's n-th (0-based) user message. Counting user messages
// rather than rows lets a client that never sees row ids (tool cards and mid-turn notes are not
// stored) still name a message.
func (s *Store) userMessageID(ctx context.Context, sessionID string, n int) (int64, error) {
	if n < 0 {
		return 0, ErrNoSuchMessage
	}
	var id int64
	err := s.db.QueryRowContext(ctx,
		`SELECT id FROM chat_messages WHERE session_id = ? AND role = 'user' ORDER BY id LIMIT 1 OFFSET ?`,
		sessionID, n).Scan(&id)
	if err == sql.ErrNoRows {
		return 0, ErrNoSuchMessage
	}
	return id, err
}

// TruncateFromUser deletes the chat's n-th user message and everything after it, so that message
// can be retried or replaced by an edit.
func (s *Store) TruncateFromUser(ctx context.Context, sessionID string, n int) error {
	id, err := s.userMessageID(ctx, sessionID, n)
	if err != nil {
		return err
	}
	if _, err := s.db.ExecContext(ctx, `DELETE FROM chat_messages WHERE session_id = ? AND id >= ?`, sessionID, id); err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `UPDATE chat_sessions SET updated_at = datetime('now') WHERE id = ?`, sessionID)
	return err
}

// Fork copies everything before the source chat's n-th user message into a new chat, leaving the
// source untouched. The new chat is where an alternative to that message can be tried.
func (s *Store) Fork(ctx context.Context, srcID, newID string, n int) (*ChatSession, error) {
	src, err := s.GetSession(ctx, srcID)
	if err != nil {
		return nil, err
	}
	if src == nil {
		return nil, ErrNoSuchMessage
	}
	cut, err := s.userMessageID(ctx, srcID, n)
	if err != nil {
		return nil, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `INSERT INTO chat_sessions (id, title) VALUES (?, ?)`, newID, src.Title+" (fork)"); err != nil {
		return nil, err
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO chat_messages (session_id, role, content, created_at)
		 SELECT ?, role, content, created_at FROM chat_messages WHERE session_id = ? AND id < ? ORDER BY id`,
		newID, srcID, cut); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return s.GetSession(ctx, newID)
}
