package github

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// apiBaseForTest swaps githubAPIBase and returns the previous value.
func apiBaseForTest(v string) string {
	old := githubAPIBase
	githubAPIBase = v
	return old
}

// TestGetScopedInstallationTokenSendsPermissions verifies the scoped-token
// request carries the least-privilege permissions body.
func TestGetScopedInstallationTokenSendsPermissions(t *testing.T) {
	var gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/access_tokens") {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		_ = json.NewEncoder(w).Encode(map[string]string{"token": "scoped-tok"})
	}))
	defer srv.Close()

	// Point the API base at the test server.
	old := apiBaseForTest(srv.URL)
	defer apiBaseForTest(old)

	tok, err := GetScopedInstallationToken("jwt", 123, map[string]string{"packages": "read"})
	if err != nil {
		t.Fatalf("scoped token: %v", err)
	}
	if tok != "scoped-tok" {
		t.Fatalf("token = %q", tok)
	}
	var parsed struct {
		Permissions map[string]string `json:"permissions"`
	}
	if err := json.Unmarshal([]byte(gotBody), &parsed); err != nil {
		t.Fatalf("request body not JSON: %q", gotBody)
	}
	if parsed.Permissions["packages"] != "read" {
		t.Fatalf("permissions = %+v, want packages:read", parsed.Permissions)
	}
}

func TestPullTokenProblem(t *testing.T) {
	if PullTokenProblem("ghu_abc") == "" {
		t.Error("GitHub App user tokens must be flagged")
	}
	if PullTokenProblem("github_pat_abc") == "" {
		t.Error("fine-grained PATs must be flagged")
	}
	for scopes, wantProblem := range map[string]bool{
		"repo, workflow, write:packages":                true,
		"repo, workflow, write:packages, read:packages": false,
	} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("X-OAuth-Scopes", scopes)
			_, _ = w.Write([]byte(`{"login":"u"}`))
		}))
		old := githubAPIBase
		githubAPIBase = srv.URL
		got := PullTokenProblem("gho_x") != ""
		githubAPIBase = old
		srv.Close()
		if got != wantProblem {
			t.Errorf("scopes %q: problem=%v, want %v", scopes, got, wantProblem)
		}
	}
}
