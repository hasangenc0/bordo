package fleet

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

// ErrClusterTokenInvalid is returned when a cluster token is missing, malformed,
// does not match, or has been revoked.
var ErrClusterTokenInvalid = errors.New("cluster token invalid")

// IssueClusterToken mints a revocable, region-scoped credential for the
// on-cluster reconciler to pull desired state (ADR-0006 Phase 2). It is NOT
// single-use. Returns the one-time plaintext secret ("<id>.<random>"); only the
// hash is stored.
func (s *Store) IssueClusterToken(ctx context.Context, regionName string) (secret string, err error) {
	if strings.TrimSpace(regionName) == "" {
		return "", fmt.Errorf("region name required")
	}
	id := uuid.NewString()
	raw := make([]byte, 32)
	if _, err = rand.Read(raw); err != nil {
		return "", fmt.Errorf("generating cluster token: %w", err)
	}
	secret = id + "." + hex.EncodeToString(raw)
	if _, err = s.db.ExecContext(ctx,
		`INSERT INTO fleet_cluster_tokens (id, region_name, token_hash, created_at)
		 VALUES (?, ?, ?, ?)`,
		id, regionName, hashToken(secret), time.Now().UTC(),
	); err != nil {
		return "", fmt.Errorf("persist cluster token: %w", err)
	}
	return secret, nil
}

// ValidateClusterToken returns the region a cluster token is scoped to. It
// updates last_seen on success. Any failure yields ErrClusterTokenInvalid so
// the caller can return a uniform 401.
func (s *Store) ValidateClusterToken(ctx context.Context, secret string) (regionName string, err error) {
	parts := strings.SplitN(secret, ".", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", ErrClusterTokenInvalid
	}
	id := parts[0]

	var (
		storedHash string
		revoked    bool
	)
	row := s.db.QueryRowContext(ctx,
		`SELECT region_name, token_hash, revoked FROM fleet_cluster_tokens WHERE id = ?`, id)
	if err = row.Scan(&regionName, &storedHash, &revoked); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", ErrClusterTokenInvalid
		}
		return "", err
	}
	if subtle.ConstantTimeCompare([]byte(hashToken(secret)), []byte(storedHash)) != 1 {
		return "", ErrClusterTokenInvalid
	}
	if revoked {
		return "", ErrClusterTokenInvalid
	}
	_, _ = s.db.ExecContext(ctx,
		`UPDATE fleet_cluster_tokens SET last_seen = ? WHERE id = ?`, time.Now().UTC(), id)
	return regionName, nil
}

// RevokeClusterTokensForRegion revokes every cluster token for a region (e.g.
// when decommissioning or rotating). Idempotent.
func (s *Store) RevokeClusterTokensForRegion(ctx context.Context, regionName string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE fleet_cluster_tokens SET revoked = 1 WHERE region_name = ?`, regionName)
	return err
}
