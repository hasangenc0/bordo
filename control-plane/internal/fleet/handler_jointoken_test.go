package fleet

import (
	"context"
	"database/sql"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	_ "github.com/mattn/go-sqlite3"
)

func newHandlerTestDeps(t *testing.T) (*Store, http.Handler) {
	t.Helper()
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })
	for _, q := range []string{
		`CREATE TABLE regions (
			id TEXT PRIMARY KEY, name TEXT NOT NULL UNIQUE,
			status TEXT NOT NULL DEFAULT 'pending', kubeconfig TEXT NOT NULL DEFAULT '',
			created_at DATETIME NOT NULL DEFAULT (datetime('now')),
			updated_at DATETIME NOT NULL DEFAULT (datetime('now')))`,
		`CREATE TABLE fleet_join_tokens (
			id TEXT PRIMARY KEY, region_name TEXT NOT NULL, token_hash TEXT NOT NULL,
			expires_at DATETIME NOT NULL, used_at DATETIME, revoked INTEGER NOT NULL DEFAULT 0,
			created_at DATETIME NOT NULL DEFAULT (datetime('now')))`,
	} {
		if _, err := db.Exec(q); err != nil {
			t.Fatalf("schema: %v", err)
		}
	}
	s := New(db)
	return s, NewHandler(s, slog.Default())
}

func TestMintJoinTokenHandler(t *testing.T) {
	s, h := newHandlerTestDeps(t)
	ctx := context.Background()
	if _, err := s.AddRegion(ctx, "us-east", ""); err != nil {
		t.Fatalf("add region: %v", err)
	}

	// Unknown region → 404.
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/nope/join-tokens", nil))
	if rr.Code != http.StatusNotFound {
		t.Fatalf("unknown region: code=%d", rr.Code)
	}

	// Known region → 201 with a one-time token that redeems for that region.
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/us-east/join-tokens", nil))
	if rr.Code != http.StatusCreated {
		t.Fatalf("mint: code=%d body=%s", rr.Code, rr.Body.String())
	}
	var resp struct {
		ID, Token, Region string
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Token == "" || resp.Region != "us-east" {
		t.Fatalf("unexpected mint response: %+v", resp)
	}

	region, err := s.RedeemJoinToken(ctx, resp.Token)
	if err != nil || region != "us-east" {
		t.Fatalf("redeem minted token: region=%q err=%v", region, err)
	}
}
