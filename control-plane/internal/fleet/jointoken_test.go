package fleet

import (
	"context"
	"database/sql"
	"errors"
	"sync"
	"testing"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	// :memory: is per-connection — pin to one so the schema is visible to every query.
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })
	if _, err := db.Exec(`CREATE TABLE fleet_join_tokens (
		id          TEXT PRIMARY KEY,
		region_name TEXT NOT NULL,
		token_hash  TEXT NOT NULL,
		expires_at  DATETIME NOT NULL,
		used_at     DATETIME,
		revoked     INTEGER NOT NULL DEFAULT 0,
		created_at  DATETIME NOT NULL DEFAULT (datetime('now'))
	)`); err != nil {
		t.Fatalf("schema: %v", err)
	}
	return New(db)
}

func TestMintAndRedeem(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	id, secret, err := s.MintJoinToken(ctx, "us-east", time.Minute)
	if err != nil {
		t.Fatalf("mint: %v", err)
	}
	if id == "" || secret == "" {
		t.Fatal("empty id/secret")
	}

	region, err := s.RedeemJoinToken(ctx, secret)
	if err != nil {
		t.Fatalf("redeem: %v", err)
	}
	if region != "us-east" {
		t.Fatalf("region = %q, want us-east", region)
	}
}

func TestRedeemIsSingleUse(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	_, secret, err := s.MintJoinToken(ctx, "r", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.RedeemJoinToken(ctx, secret); err != nil {
		t.Fatalf("first redeem: %v", err)
	}
	if _, err := s.RedeemJoinToken(ctx, secret); !errors.Is(err, ErrTokenUsed) {
		t.Fatalf("second redeem err = %v, want ErrTokenUsed", err)
	}
}

func TestRedeemExpired(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	id, secret, err := s.MintJoinToken(ctx, "r", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	// Force the token into the past.
	if _, err := s.db.ExecContext(ctx,
		`UPDATE fleet_join_tokens SET expires_at = ? WHERE id = ?`,
		time.Now().UTC().Add(-time.Hour), id); err != nil {
		t.Fatal(err)
	}
	if _, err := s.RedeemJoinToken(ctx, secret); !errors.Is(err, ErrTokenExpired) {
		t.Fatalf("redeem err = %v, want ErrTokenExpired", err)
	}
}

func TestRedeemRevoked(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	id, secret, err := s.MintJoinToken(ctx, "r", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.RevokeJoinToken(ctx, id); err != nil {
		t.Fatal(err)
	}
	if _, err := s.RedeemJoinToken(ctx, secret); !errors.Is(err, ErrTokenRevoked) {
		t.Fatalf("redeem err = %v, want ErrTokenRevoked", err)
	}
}

func TestRedeemTamperedSecret(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	id, _, err := s.MintJoinToken(ctx, "r", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	// Correct id, wrong random half.
	if _, err := s.RedeemJoinToken(ctx, id+".deadbeef"); !errors.Is(err, ErrTokenInvalid) {
		t.Fatalf("redeem err = %v, want ErrTokenInvalid", err)
	}
}

func TestRedeemUnknownAndMalformed(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	for _, bad := range []string{"", "nodot", "a.", ".b", "unknown-id.abcdef"} {
		if _, err := s.RedeemJoinToken(ctx, bad); !errors.Is(err, ErrTokenInvalid) {
			t.Fatalf("redeem(%q) err = %v, want ErrTokenInvalid", bad, err)
		}
	}
}

func TestMintRequiresRegion(t *testing.T) {
	s := newTestStore(t)
	if _, _, err := s.MintJoinToken(context.Background(), "  ", time.Minute); err == nil {
		t.Fatal("expected error for empty region")
	}
}

// TestConcurrentRedeemSingleWinner verifies the atomic consume guard: many
// redeems of the same token yield exactly one success.
func TestConcurrentRedeemSingleWinner(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	_, secret, err := s.MintJoinToken(ctx, "r", time.Minute)
	if err != nil {
		t.Fatal(err)
	}

	const n = 20
	var wg sync.WaitGroup
	var mu sync.Mutex
	successes := 0
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			if _, err := s.RedeemJoinToken(ctx, secret); err == nil {
				mu.Lock()
				successes++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	if successes != 1 {
		t.Fatalf("got %d successful redeems, want exactly 1", successes)
	}
}

func TestGetJoinToken(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	id, secret, err := s.MintJoinToken(ctx, "us-west", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	jt, err := s.GetJoinToken(ctx, id)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if jt.RegionName != "us-west" || jt.UsedAt != nil || jt.Revoked {
		t.Fatalf("unexpected token meta: %+v", jt)
	}
	if _, err := s.RedeemJoinToken(ctx, secret); err != nil {
		t.Fatal(err)
	}
	jt, err = s.GetJoinToken(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if jt.UsedAt == nil {
		t.Fatal("expected UsedAt to be set after redeem")
	}
}
