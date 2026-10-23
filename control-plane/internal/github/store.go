// Package github manages GitHub App credentials and installation state.
package github

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sync"
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

	refreshMu sync.Mutex
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

// UserSession is a stored GitHub user login. RefreshToken and ClientID are set when the
// token expires and can be renewed without the user logging in again.
type UserSession struct {
	Token        string
	RefreshToken string
	ClientID     string
	ExpiresAt    time.Time // zero = does not expire
}

func expiryFrom(expiresIn int) time.Time {
	if expiresIn <= 0 {
		return time.Time{}
	}
	return time.Now().UTC().Add(time.Duration(expiresIn) * time.Second)
}

// ExpiryFrom converts GitHub's expires_in seconds into an absolute time (zero if none).
func ExpiryFrom(expiresIn int) time.Time { return expiryFrom(expiresIn) }

// SaveUserToken persists a GitHub user access token that does not renew itself.
func (s *Store) SaveUserToken(ctx context.Context, token, _ string, expiresAt time.Time) error {
	return s.SaveUserSession(ctx, UserSession{Token: token, ExpiresAt: expiresAt})
}

// SaveUserSession persists a user login, including its refresh token when it has one.
func (s *Store) SaveUserSession(ctx context.Context, u UserSession) error {
	if err := s.init(ctx); err != nil {
		return err
	}
	if err := s.setEncrypted(ctx, "user_token", u.Token); err != nil {
		return err
	}
	refresh := ""
	if u.RefreshToken != "" {
		var err error
		if refresh, err = secrets.EncryptAES256GCM(s.key, u.RefreshToken); err != nil {
			return err
		}
	}
	if err := s.set(ctx, "user_refresh_token", refresh); err != nil {
		return err
	}
	if err := s.set(ctx, "user_client_id", u.ClientID); err != nil {
		return err
	}
	exp := ""
	if !u.ExpiresAt.IsZero() {
		exp = u.ExpiresAt.UTC().Format(time.RFC3339)
	}
	return s.set(ctx, "user_token_expires", exp)
}

// refreshSkew renews a token this long before it actually expires.
const refreshSkew = 5 * time.Minute

// ErrUserTokenExpired means the stored login expired and could not be renewed.
var ErrUserTokenExpired = errors.New("GitHub login expired and could not be renewed: run 'bordo github login' again")

// LoadUserToken returns the stored user token, transparently renewing it with the refresh
// token when it is expired or about to expire, so a login lasts until the refresh token
// does (months) instead of the access token (hours). Returns "" if not set.
func (s *Store) LoadUserToken(ctx context.Context) (string, time.Time, error) {
	if err := s.init(ctx); err != nil {
		return "", time.Time{}, err
	}
	token, err := s.getDecrypted(ctx, "user_token")
	if err != nil || token == "" {
		return "", time.Time{}, err
	}
	expiresAt := s.userExpiry(ctx)
	if expiresAt.IsZero() || time.Until(expiresAt) > refreshSkew {
		return token, expiresAt, nil
	}

	s.refreshMu.Lock()
	defer s.refreshMu.Unlock()
	// Another caller may have renewed it while we waited for the lock.
	if t, e := s.getDecrypted(ctx, "user_token"); e == nil && t != "" {
		if exp := s.userExpiry(ctx); exp.IsZero() || time.Until(exp) > refreshSkew {
			return t, exp, nil
		}
	}
	refresh, _ := s.getDecrypted(ctx, "user_refresh_token")
	clientID, _ := s.get(ctx, "user_client_id")
	if refresh == "" || clientID == "" {
		return "", expiresAt, ErrUserTokenExpired
	}
	secret := ""
	if app, aerr := s.LoadApp(ctx); aerr == nil && app != nil && app.ClientID == clientID {
		secret = app.ClientSecret
	}
	nt, rerr := RefreshOAuthToken(clientID, secret, refresh)
	if rerr != nil {
		return "", expiresAt, fmt.Errorf("%w (%v)", ErrUserTokenExpired, rerr)
	}
	next := nt.RefreshToken
	if next == "" {
		next = refresh
	}
	if err := s.SaveUserSession(ctx, UserSession{Token: nt.AccessToken, RefreshToken: next, ClientID: clientID, ExpiresAt: expiryFrom(nt.ExpiresIn)}); err != nil {
		return "", expiresAt, err
	}
	return nt.AccessToken, expiryFrom(nt.ExpiresIn), nil
}

func (s *Store) userExpiry(ctx context.Context) time.Time {
	str, _ := s.get(ctx, "user_token_expires")
	if str == "" {
		return time.Time{}
	}
	t, _ := time.Parse(time.RFC3339, str)
	if t.Year() <= 1 {
		return time.Time{}
	}
	return t
}
