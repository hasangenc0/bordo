package chatstore

import (
	"context"
	"database/sql"
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
