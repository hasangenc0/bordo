package secrets

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"
)

// Store is the secrets persistence interface.
type Store interface {
	Set(ctx context.Context, projectID, key, value string) error
	Get(ctx context.Context, projectID, key string) (string, error)
	List(ctx context.Context, projectID string) ([]Secret, error)
	Delete(ctx context.Context, projectID, key string) error
}

// SQLiteStore stores secrets AES-256-GCM encrypted in SQLite.
type SQLiteStore struct {
	db  *sql.DB
	key [32]byte
}

// NewSQLiteStore creates a SQLiteStore. encKey is the 32-byte AES key.
func NewSQLiteStore(db *sql.DB, encKey [32]byte) *SQLiteStore {
	return &SQLiteStore{db: db, key: encKey}
}

// DeriveKey returns a 32-byte AES key. If BORDO_SECRET_KEY env is set, it is
// SHA-256 hashed. Otherwise a persistent random key is read from (or created at)
// ~/.bordo/secret.key.
func DeriveKey() ([32]byte, error) {
	if v := os.Getenv("BORDO_SECRET_KEY"); v != "" {
		return sha256.Sum256([]byte(v)), nil
	}
	dir := filepath.Join(os.Getenv("HOME"), ".bordo")
	_ = os.MkdirAll(dir, 0700)
	keyFile := filepath.Join(dir, "secret.key")
	data, err := os.ReadFile(keyFile)
	if err == nil && len(data) == 64 {
		// hex-encoded 32 bytes stored as 64 ASCII chars
		var key [32]byte
		for i := range key {
			var b byte
			_, _ = fmt.Sscanf(string(data[i*2:i*2+2]), "%02x", &b)
			key[i] = b
		}
		return key, nil
	}
	// Generate a new random key.
	var raw [32]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return raw, fmt.Errorf("generating secret key: %w", err)
	}
	hex := fmt.Sprintf("%x", raw)
	_ = os.WriteFile(keyFile, []byte(hex), 0600)
	return raw, nil
}

func (s *SQLiteStore) Set(ctx context.Context, projectID, key, value string) error {
	enc, err := EncryptAES256GCM(s.key, value)
	if err != nil {
		return fmt.Errorf("encrypting secret: %w", err)
	}
	now := time.Now().UTC()
	id := uuid.New().String()
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO bordo_secrets (id, project_id, key, encrypted_value, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(project_id, key) DO UPDATE SET
			encrypted_value = excluded.encrypted_value,
			updated_at = excluded.updated_at`,
		id, projectID, key, enc, now, now,
	)
	return err
}

func (s *SQLiteStore) Get(ctx context.Context, projectID, key string) (string, error) {
	var enc string
	err := s.db.QueryRowContext(ctx,
		`SELECT encrypted_value FROM bordo_secrets WHERE project_id = ? AND key = ?`,
		projectID, key,
	).Scan(&enc)
	if err == sql.ErrNoRows {
		return "", fmt.Errorf("secret %q not found", key)
	}
	if err != nil {
		return "", err
	}
	return DecryptAES256GCM(s.key, enc)
}

func (s *SQLiteStore) List(ctx context.Context, projectID string) ([]Secret, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, project_id, key, created_at, updated_at FROM bordo_secrets WHERE project_id = ? ORDER BY key`,
		projectID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Secret
	for rows.Next() {
		var sc Secret
		if err := rows.Scan(&sc.ID, &sc.ProjectID, &sc.Key, &sc.CreatedAt, &sc.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, sc)
	}
	return out, rows.Err()
}

func (s *SQLiteStore) Delete(ctx context.Context, projectID, key string) error {
	res, err := s.db.ExecContext(ctx,
		`DELETE FROM bordo_secrets WHERE project_id = ? AND key = ?`, projectID, key)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("secret %q not found", key)
	}
	return nil
}
