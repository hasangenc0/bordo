// Package github manages GitHub App credentials and installation state.
package github

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/hasangenc0/bordo/control-plane/internal/secrets"
)

// AppCredentials holds the GitHub App registration details.
type AppCredentials struct {
	AppID         int64
	PrivateKeyPEM string
	WebhookSecret string
	ClientID      string
	ClientSecret  string
	AppSlug       string
}

// Store persists GitHub App config in a single SQLite key-value table.
type Store struct {
	db  *sql.DB
	key [32]byte
}

// NewStore creates a Store backed by db.
func NewStore(db *sql.DB, key [32]byte) *Store {
	return &Store{db: db, key: key}
}

func (s *Store) init(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS github_config (
			key   TEXT PRIMARY KEY,
			value TEXT NOT NULL
		)`)
	return err
}

func (s *Store) set(ctx context.Context, key, value string) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO github_config (key, value) VALUES (?, ?)
		 ON CONFLICT(key) DO UPDATE SET value = excluded.value`,
		key, value)
	return err
}

func (s *Store) get(ctx context.Context, key string) (string, error) {
	var value string
	err := s.db.QueryRowContext(ctx, `SELECT value FROM github_config WHERE key = ?`, key).Scan(&value)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return value, err
}

func (s *Store) setEncrypted(ctx context.Context, key, plaintext string) error {
	enc, err := secrets.EncryptAES256GCM(s.key, plaintext)
	if err != nil {
		return fmt.Errorf("encrypt %s: %w", key, err)
	}
	return s.set(ctx, key, enc)
}

func (s *Store) getDecrypted(ctx context.Context, key string) (string, error) {
	enc, err := s.get(ctx, key)
	if err != nil || enc == "" {
		return "", err
	}
	plain, err := secrets.DecryptAES256GCM(s.key, enc)
	if err != nil {
		return "", fmt.Errorf("decrypt %s: %w", key, err)
	}
	return plain, nil
}

// SaveApp persists the GitHub App registration credentials.
func (s *Store) SaveApp(ctx context.Context, creds AppCredentials) error {
	if err := s.init(ctx); err != nil {
		return err
	}
	if err := s.set(ctx, "app_id", fmt.Sprintf("%d", creds.AppID)); err != nil {
		return err
	}
	if err := s.set(ctx, "app_slug", creds.AppSlug); err != nil {
		return err
	}
	if err := s.set(ctx, "client_id", creds.ClientID); err != nil {
		return err
	}
	if err := s.setEncrypted(ctx, "client_secret", creds.ClientSecret); err != nil {
		return err
	}
	if err := s.setEncrypted(ctx, "private_key_pem", creds.PrivateKeyPEM); err != nil {
		return err
	}
	if err := s.setEncrypted(ctx, "webhook_secret", creds.WebhookSecret); err != nil {
		return err
	}
	return nil
}

// LoadApp retrieves the GitHub App credentials. Returns nil, nil if not configured.
func (s *Store) LoadApp(ctx context.Context) (*AppCredentials, error) {
	if err := s.init(ctx); err != nil {
		return nil, err
	}
	appIDStr, err := s.get(ctx, "app_id")
	if err != nil || appIDStr == "" {
		return nil, err
	}
	var appID int64
	fmt.Sscanf(appIDStr, "%d", &appID)

	slug, _ := s.get(ctx, "app_slug")
	clientID, _ := s.get(ctx, "client_id")
	clientSecret, _ := s.getDecrypted(ctx, "client_secret")
	privateKey, _ := s.getDecrypted(ctx, "private_key_pem")
	webhookSecret, _ := s.getDecrypted(ctx, "webhook_secret")

	return &AppCredentials{
		AppID:         appID,
		AppSlug:       slug,
		ClientID:      clientID,
		ClientSecret:  clientSecret,
		PrivateKeyPEM: privateKey,
		WebhookSecret: webhookSecret,
	}, nil
}

// SaveInstallationID persists the GitHub App installation ID.
func (s *Store) SaveInstallationID(ctx context.Context, installID int64, targetType, targetLogin string) error {
	if err := s.init(ctx); err != nil {
		return err
	}
	if err := s.set(ctx, "installation_id", fmt.Sprintf("%d", installID)); err != nil {
		return err
	}
	if err := s.set(ctx, "installation_target_type", targetType); err != nil {
		return err
	}
	return s.set(ctx, "installation_target_login", targetLogin)
}

// LoadInstallationID retrieves the installation ID. Returns 0 if not set.
func (s *Store) LoadInstallationID(ctx context.Context) (int64, string, string, error) {
	if err := s.init(ctx); err != nil {
		return 0, "", "", err
	}
	idStr, err := s.get(ctx, "installation_id")
	if err != nil || idStr == "" {
		return 0, "", "", err
	}
	var id int64
	fmt.Sscanf(idStr, "%d", &id)
	targetType, _ := s.get(ctx, "installation_target_type")
	targetLogin, _ := s.get(ctx, "installation_target_login")
	return id, targetType, targetLogin, nil
}

// SaveUserToken persists a GitHub OAuth user access token.
func (s *Store) SaveUserToken(ctx context.Context, token, _ string, expiresAt time.Time) error {
	if err := s.init(ctx); err != nil {
		return err
	}
	if err := s.setEncrypted(ctx, "user_token", token); err != nil {
		return err
	}
	return s.set(ctx, "user_token_expires", expiresAt.UTC().Format(time.RFC3339))
}

// LoadUserToken retrieves the stored user OAuth token. Returns "", zero time if not set.
func (s *Store) LoadUserToken(ctx context.Context) (string, time.Time, error) {
	if err := s.init(ctx); err != nil {
		return "", time.Time{}, err
	}
	token, err := s.getDecrypted(ctx, "user_token")
	if err != nil || token == "" {
		return "", time.Time{}, err
	}
	expiresStr, _ := s.get(ctx, "user_token_expires")
	var expiresAt time.Time
	if expiresStr != "" {
		expiresAt, _ = time.Parse(time.RFC3339, expiresStr)
	}
	return token, expiresAt, nil
}
