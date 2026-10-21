package fleet

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	_ "github.com/mattn/go-sqlite3"
)

func newClusterTokenStore(t *testing.T) *Store {
	t.Helper()
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })
	if _, err := db.Exec(`CREATE TABLE fleet_cluster_tokens (
		id TEXT PRIMARY KEY, region_name TEXT NOT NULL, token_hash TEXT NOT NULL,
		revoked INTEGER NOT NULL DEFAULT 0,
		created_at DATETIME NOT NULL DEFAULT (datetime('now')), last_seen DATETIME
	)`); err != nil {
		t.Fatalf("schema: %v", err)
	}
	return New(db)
}

func TestIssueAndValidateClusterToken(t *testing.T) {
	s := newClusterTokenStore(t)
	ctx := context.Background()

	secret, err := s.IssueClusterToken(ctx, "us-east")
	if err != nil {
		t.Fatalf("issue: %v", err)
	}
	region, err := s.ValidateClusterToken(ctx, secret)
	if err != nil {
		t.Fatalf("validate: %v", err)
	}
	if region != "us-east" {
		t.Fatalf("region = %q", region)
	}
	// Unlike join tokens, cluster tokens are reusable.
	if _, err := s.ValidateClusterToken(ctx, secret); err != nil {
		t.Fatalf("second validate: %v", err)
	}
}

func TestValidateClusterTokenRejects(t *testing.T) {
	s := newClusterTokenStore(t)
	ctx := context.Background()

	secret, err := s.IssueClusterToken(ctx, "r")
	if err != nil {
		t.Fatal(err)
	}
	parts := secret // tampered secret: correct id, wrong random
	id := parts[:36]
	if _, err := s.ValidateClusterToken(ctx, id+".wrong"); !errors.Is(err, ErrClusterTokenInvalid) {
		t.Fatalf("tampered err = %v", err)
	}
	for _, bad := range []string{"", "nodot", "unknown.secret"} {
		if _, err := s.ValidateClusterToken(ctx, bad); !errors.Is(err, ErrClusterTokenInvalid) {
			t.Fatalf("validate(%q) err = %v", bad, err)
		}
	}
}

func TestRevokeClusterTokens(t *testing.T) {
	s := newClusterTokenStore(t)
	ctx := context.Background()

	secret, err := s.IssueClusterToken(ctx, "r")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.RevokeClusterTokensForRegion(ctx, "r"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ValidateClusterToken(ctx, secret); !errors.Is(err, ErrClusterTokenInvalid) {
		t.Fatalf("after revoke err = %v, want ErrClusterTokenInvalid", err)
	}
}
