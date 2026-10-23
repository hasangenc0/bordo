package github

import (
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

func testStore(t *testing.T) *Store {
	t.Helper()
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { db.Close() })
	return NewStore(db, [32]byte{1})
}

func TestLoadUserTokenRefreshesExpiredToken(t *testing.T) {
	var form url.Values
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		form = r.PostForm
		_, _ = w.Write([]byte(`{"access_token":"new-at","refresh_token":"new-rt","expires_in":28800}`))
	}))
	defer srv.Close()
	old := oauthTokenURL
	oauthTokenURL = srv.URL
	defer func() { oauthTokenURL = old }()

	s := testStore(t)
	ctx := context.Background()
	if err := s.SaveUserSession(ctx, UserSession{Token: "old-at", RefreshToken: "old-rt", ClientID: "cid", ExpiresAt: time.Now().Add(-time.Minute)}); err != nil {
		t.Fatal(err)
	}
	tok, exp, err := s.LoadUserToken(ctx)
	if err != nil || tok != "new-at" {
		t.Fatalf("token = %q, err = %v", tok, err)
	}
	if form.Get("grant_type") != "refresh_token" || form.Get("refresh_token") != "old-rt" || form.Get("client_id") != "cid" {
		t.Fatalf("refresh request = %v", form)
	}
	if time.Until(exp) < 7*time.Hour {
		t.Fatalf("expiry = %v", exp)
	}
	// The rotated refresh token and new access token are persisted.
	tok2, _, _ := s.LoadUserToken(ctx)
	if tok2 != "new-at" {
		t.Fatalf("stored token = %q", tok2)
	}
	if rt, _ := s.getDecrypted(ctx, "user_refresh_token"); rt != "new-rt" {
		t.Fatalf("refresh token = %q", rt)
	}
}

func TestLoadUserTokenExpiredWithoutRefreshToken(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	_ = s.SaveUserSession(ctx, UserSession{Token: "at", ExpiresAt: time.Now().Add(-time.Hour)})
	tok, _, err := s.LoadUserToken(ctx)
	if tok != "" || err != ErrUserTokenExpired {
		t.Fatalf("tok=%q err=%v", tok, err)
	}
}

func TestLoadUserTokenNonExpiringIsUntouched(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	_ = s.SaveUserToken(ctx, "pat", "", time.Time{})
	tok, exp, err := s.LoadUserToken(ctx)
	if tok != "pat" || err != nil || !exp.IsZero() {
		t.Fatalf("tok=%q exp=%v err=%v", tok, exp, err)
	}
}
