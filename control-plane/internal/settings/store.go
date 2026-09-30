// Package settings provides an encrypted key-value store for platform secrets
// and configuration entered via the setup wizard or CLI.
package settings

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/hasangenc0/bordo/control-plane/internal/secrets"
)

// Known setting keys.
const (
	KeyAdminToken   = "admin_token"
	KeyDeepSeekKey  = "deepseek_api_key"
	KeyBaseURL      = "base_url"
	KeyLLMModel     = "llm_model"
	KeyLLMURL       = "llm_url"
	KeyRegistryURL  = "registry_url"
)

// Store is an encrypted KV store backed by SQLite.
type Store struct {
	db  *sql.DB
	key [32]byte
}

// New opens the settings store. dbPath is used to locate the .secret_key file
// (placed in the same directory as the database).
func New(db *sql.DB, dbPath string) (*Store, error) {
	key, err := loadOrGenerateKey(dbPath)
	if err != nil {
		return nil, fmt.Errorf("settings key: %w", err)
	}
	return &Store{db: db, key: key}, nil
}

// loadOrGenerateKey resolves the encryption key with the following priority:
//  1. BORDO_SECRET_KEY env var
//  2. .secret_key file in the same directory as the database
//  3. Auto-generate and persist to .secret_key
func loadOrGenerateKey(dbPath string) ([32]byte, error) {
	var key [32]byte

	if v := os.Getenv("BORDO_SECRET_KEY"); v != "" {
		raw, err := hex.DecodeString(v)
		if err == nil && len(raw) >= 32 {
			copy(key[:], raw[:32])
			return key, nil
		}
		// Treat as raw UTF-8 bytes (legacy support).
		b := []byte(v)
		if len(b) < 32 {
			b = append(b, make([]byte, 32-len(b))...)
		}
		copy(key[:], b[:32])
		return key, nil
	}

	keyPath := filepath.Join(filepath.Dir(dbPath), ".secret_key")
	if data, err := os.ReadFile(keyPath); err == nil {
		raw, err := hex.DecodeString(strings.TrimSpace(string(data)))
		if err == nil && len(raw) == 32 {
			copy(key[:], raw)
			return key, nil
		}
	}

	// Generate a new key and persist it.
	if _, err := rand.Read(key[:]); err != nil {
		return key, fmt.Errorf("generating secret key: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(keyPath), 0o700); err != nil {
		return key, fmt.Errorf("creating key dir: %w", err)
	}
	if err := os.WriteFile(keyPath, []byte(hex.EncodeToString(key[:])), 0o600); err != nil {
		return key, fmt.Errorf("persisting secret key to %s: %w", keyPath, err)
	}
	return key, nil
}

// Set encrypts and stores value under key.
func (s *Store) Set(key, value string) error {
	enc, err := secrets.EncryptAES256GCM(s.key, value)
	if err != nil {
		return fmt.Errorf("encrypting %s: %w", key, err)
	}
	_, err = s.db.Exec(
		`INSERT OR REPLACE INTO platform_settings (key, value, updated_at) VALUES (?, ?, CURRENT_TIMESTAMP)`,
		key, enc,
	)
	return err
}

// Get decrypts and returns the value stored under key.
// Returns ("", false, nil) when the key does not exist.
func (s *Store) Get(key string) (string, bool, error) {
	var enc string
	err := s.db.QueryRow(`SELECT value FROM platform_settings WHERE key = ?`, key).Scan(&enc)
	if err == sql.ErrNoRows {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	plain, err := secrets.DecryptAES256GCM(s.key, enc)
	if err != nil {
		return "", false, fmt.Errorf("decrypting %s: %w", key, err)
	}
	return plain, true, nil
}

// GetOrDefault returns the stored value, or def if not present.
func (s *Store) GetOrDefault(key, def string) string {
	v, ok, _ := s.Get(key)
	if !ok || v == "" {
		return def
	}
	return v
}

// Delete removes a key from the store.
func (s *Store) Delete(key string) error {
	_, err := s.db.Exec(`DELETE FROM platform_settings WHERE key = ?`, key)
	return err
}

// IsConfigured returns true if the admin token has been set (setup complete).
func (s *Store) IsConfigured() (bool, error) {
	_, ok, err := s.Get(KeyAdminToken)
	return ok, err
}
