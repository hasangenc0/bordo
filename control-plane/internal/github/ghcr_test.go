package github

import (
	"context"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func fakeGHCR(t *testing.T, status int, claims string) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if u, p, ok := r.BasicAuth(); !ok || u != "owner" || p != "tok" {
			t.Errorf("basic auth = %q %q %v", u, p, ok)
		}
		if got := r.URL.Query().Get("scope"); got != "repository:owner/app:pull" {
			t.Errorf("scope = %q", got)
		}
		w.WriteHeader(status)
		jwt := "h." + base64.RawURLEncoding.EncodeToString([]byte(claims)) + ".s"
		_, _ = w.Write([]byte(`{"token":"` + jwt + `"}`))
	}))
	old := ghcrBase
	ghcrBase = srv.URL
	t.Cleanup(func() { ghcrBase = old; srv.Close() })
}

func TestVerifyGHCRPull(t *testing.T) {
	ctx := context.Background()

	fakeGHCR(t, 200, `{"access":[{"type":"repository","name":"owner/app","actions":["pull"]}]}`)
	if got := VerifyGHCRPull(ctx, "owner", "tok", "owner/app"); got != "" {
		t.Errorf("a pull grant must verify, got %q", got)
	}

	fakeGHCR(t, 200, `{"access":[{"type":"repository","name":"owner/app","actions":[]}]}`)
	if got := VerifyGHCRPull(ctx, "owner", "tok", "owner/app"); !strings.Contains(got, "grants no pull access") {
		t.Errorf("an empty grant must be reported, got %q", got)
	}

	fakeGHCR(t, 403, `{}`)
	if got := VerifyGHCRPull(ctx, "owner", "tok", "owner/app"); !strings.Contains(got, "rejected the credential") {
		t.Errorf("a 403 must be reported, got %q", got)
	}

	// A reply we cannot interpret must not raise a false alarm.
	fakeGHCR(t, 200, `not json`)
	if got := VerifyGHCRPull(ctx, "owner", "tok", "owner/app"); got != "" {
		t.Errorf("an unreadable grant is inconclusive, got %q", got)
	}
}

func TestInspectToken(t *testing.T) {
	for _, tc := range []struct {
		status int
		scopes string
		valid  bool
	}{{200, "repo, write:packages", true}, {401, "", false}} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if tc.scopes != "" {
				w.Header().Set("X-OAuth-Scopes", tc.scopes)
			}
			w.WriteHeader(tc.status)
			_, _ = w.Write([]byte(`{"login":"me"}`))
		}))
		old := githubAPIBase
		githubAPIBase = srv.URL
		got := InspectToken(context.Background(), "gho_abcdefghij")
		githubAPIBase = old
		srv.Close()
		if got.Valid != tc.valid || got.Status != tc.status || got.Scopes != tc.scopes || got.Hint != "gho_abcd" {
			t.Errorf("status %d: got %+v", tc.status, got)
		}
	}
}
