// Package settings provides an encrypted key-value store for platform secrets
// and configuration entered via the setup wizard or CLI.
package settings

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"log/slog"
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
	log *slog.Logger
}

// New opens the settings store. dbPath is used to locate the .secret_key file
// (placed in the same directory as the database).
//
// An optional *slog.Logger may be passed; it is used to surface decrypt
// failures and secret-key regeneration as loud, actionable warnings. When
// omitted (or nil), slog.Default() is used so existing callers keep working.
func New(db *sql.DB, dbPath string, logger ...*slog.Logger) (*Store, error) {
	log := slog.Default()
	if len(logger) > 0 && logger[0] != nil {
		log = logger[0]
	}
	key, err := loadOrGenerateKey(dbPath, log)
	if err != nil {
		return nil, fmt.Errorf("settings key: %w", err)
	}
	return &Store{db: db, key: key, log: log}, nil
}

// logger returns the store's logger, falling back to slog.Default() when a
// Store was constructed without one (e.g. via a zero value in tests).
func (s *Store) logger() *slog.Logger {
	if s.log != nil {
		return s.log
	}
	return slog.Default()
}

// loadOrGenerateKey resolves the encryption key with the following priority:
//  1. BORDO_SECRET_KEY env var
//  2. .secret_key file in the same directory as the database
//  3. Auto-generate and persist to .secret_key
func loadOrGenerateKey(dbPath string, log *slog.Logger) ([32]byte, error) {
	if log == nil {
		log = slog.Default()
	}
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

	// The .secret_key file lives next to the SQLite database. It MUST reside on
	// the SAME persistent data volume as the DB: it is the only thing that can
	// decrypt the settings stored in that DB. If this file is lost or
	// regenerated (e.g. a container volume reset while the DB persists, or the
	// DB and key landing on different mounts), every previously-stored setting
	// becomes permanently undecryptable.
	keyPath := filepath.Join(filepath.Dir(dbPath), ".secret_key")
	if data, err := os.ReadFile(keyPath); err == nil {
		raw, err := hex.DecodeString(strings.TrimSpace(string(data)))
		if err == nil && len(raw) == 32 {
			copy(key[:], raw)
			return key, nil
		}
	}

	// No usable key found — generate a new one and persist it. If this happens
	// on an EXISTING install (rather than a fresh one), any settings previously
	// encrypted with the old key just became unreadable, so warn loudly.
	log.Warn("settings: no encryption key found — generating a NEW .secret_key; "+
		"if this is an existing install, previously-stored settings (DeepSeek key, "+
		"admin token, etc.) can no longer be decrypted and must be re-set. "+
		"Ensure .secret_key lives on the persistent data volume alongside the database.",
		"key_path", keyPath)
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
//
// A real decrypt/db error (anything other than the key simply not existing) is
// no longer swallowed silently: it is logged as a WARNING so the "configured
// but reads back empty" limbo is visible and actionable in bordod logs.
func (s *Store) GetOrDefault(key, def string) string {
	v, ok, err := s.Get(key)
	if err != nil && err != sql.ErrNoRows {
		s.logger().Warn(fmt.Sprintf(
			"settings: value for %q exists but could not be decrypted — the "+
				"encryption key may have changed (.secret_key regenerated). "+
				"Re-set it via 'bordo config set %s ...'.", key, key),
			"error", err)
		return def
	}
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
