package fleet

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Join-token errors. Callers should map these to appropriate HTTP statuses; the
// node-facing redeem endpoint must NOT leak which specific condition failed
// beyond "unauthorized" to avoid an oracle, but the distinct errors are useful
// for logging and for the admin/mint side.
var (
	ErrTokenInvalid = errors.New("join token invalid")
	ErrTokenExpired = errors.New("join token expired")
	ErrTokenUsed    = errors.New("join token already used")
	ErrTokenRevoked = errors.New("join token revoked")
)

// DefaultJoinTokenTTL is how long a freshly minted token stays redeemable.
const DefaultJoinTokenTTL = 15 * time.Minute

// JoinToken is metadata about a single-use node-join credential. The secret
// itself is never stored — only its SHA-256 hash.
type JoinToken struct {
	ID         string
	RegionName string
	ExpiresAt  time.Time
	UsedAt     *time.Time
	Revoked    bool
	CreatedAt  time.Time
}

// hashToken returns the hex SHA-256 of a token secret. Tokens carry 256 bits of
// entropy, so a plain (unsalted) hash is sufficient to prevent recovery from the
// stored value while allowing a direct lookup.
func hashToken(secret string) string {
	sum := sha256.Sum256([]byte(secret))
	return hex.EncodeToString(sum[:])
}

// MintJoinToken creates a single-use join token for regionName valid for ttl
// (DefaultJoinTokenTTL when ttl <= 0). It returns the token id and the one-time
// plaintext secret in the form "<id>.<random>"; only the hash is persisted, so
// the secret cannot be recovered later.
func (s *Store) MintJoinToken(ctx context.Context, regionName string, ttl time.Duration) (id, secret string, err error) {
	if strings.TrimSpace(regionName) == "" {
		return "", "", fmt.Errorf("region name required")
	}
	if ttl <= 0 {
		ttl = DefaultJoinTokenTTL
	}
	id = uuid.NewString()
	raw := make([]byte, 32)
	if _, err = rand.Read(raw); err != nil {
		return "", "", fmt.Errorf("generating token: %w", err)
	}
	secret = id + "." + hex.EncodeToString(raw)
	now := time.Now().UTC()
	if _, err = s.db.ExecContext(ctx,
		`INSERT INTO fleet_join_tokens (id, region_name, token_hash, expires_at, created_at)
		 VALUES (?, ?, ?, ?, ?)`,
		id, regionName, hashToken(secret), now.Add(ttl), now,
	); err != nil {
		return "", "", fmt.Errorf("persist join token: %w", err)
	}
	return id, secret, nil
}

// RedeemJoinToken validates and atomically consumes a join token. On success it
// returns the region the token was scoped to. It is single-use: a concurrent or
// subsequent redeem of the same token returns ErrTokenUsed. Checks run in a
// fixed order (existence+hash → revoked → used → expired) and the consuming
// UPDATE is guarded so that exactly one caller can ever win.
func (s *Store) RedeemJoinToken(ctx context.Context, secret string) (regionName string, err error) {
	parts := strings.SplitN(secret, ".", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", ErrTokenInvalid
	}
	id := parts[0]

	var (
		storedHash string
		expiresAt  time.Time
		usedAt     sql.NullTime
		revoked    bool
	)
	row := s.db.QueryRowContext(ctx,
		`SELECT region_name, token_hash, expires_at, used_at, revoked
		 FROM fleet_join_tokens WHERE id = ?`, id)
	if err = row.Scan(&regionName, &storedHash, &expiresAt, &usedAt, &revoked); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", ErrTokenInvalid
		}
		return "", err
	}

	// Constant-time comparison of the presented secret's hash against the stored
	// hash, so a timing side-channel can't be used to guess the token.
	if subtle.ConstantTimeCompare([]byte(hashToken(secret)), []byte(storedHash)) != 1 {
		return "", ErrTokenInvalid
	}
	if revoked {
		return "", ErrTokenRevoked
	}
	if usedAt.Valid {
		return "", ErrTokenUsed
	}
	if time.Now().UTC().After(expiresAt) {
		return "", ErrTokenExpired
	}

	// Atomically consume. The WHERE clause is the real single-use guard: if two
	// redeems race, only one UPDATE affects a row; the loser sees 0 rows.
	res, err := s.db.ExecContext(ctx,
		`UPDATE fleet_join_tokens SET used_at = ?
		 WHERE id = ? AND used_at IS NULL AND revoked = 0`,
		time.Now().UTC(), id)
	if err != nil {
		return "", err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return "", ErrTokenUsed
	}
	return regionName, nil
}

// RevokeJoinToken marks a token revoked so it can no longer be redeemed. It is
// idempotent and does not error if the token does not exist.
func (s *Store) RevokeJoinToken(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE fleet_join_tokens SET revoked = 1 WHERE id = ?`, id)
	return err
}

// GetJoinToken returns a token's metadata (never the secret), for status/admin.
func (s *Store) GetJoinToken(ctx context.Context, id string) (*JoinToken, error) {
	var (
		jt     JoinToken
		usedAt sql.NullTime
	)
	row := s.db.QueryRowContext(ctx,
		`SELECT id, region_name, expires_at, used_at, revoked, created_at
		 FROM fleet_join_tokens WHERE id = ?`, id)
	if err := row.Scan(&jt.ID, &jt.RegionName, &jt.ExpiresAt, &usedAt, &jt.Revoked, &jt.CreatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrTokenInvalid
		}
		return nil, err
	}
	if usedAt.Valid {
		jt.UsedAt = &usedAt.Time
	}
	return &jt, nil
}
