package server

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hasangenc0/bordo/control-plane/internal/config"
	"github.com/hasangenc0/bordo/control-plane/internal/store"
	_ "github.com/mattn/go-sqlite3"
)

func newConsoleTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	t.Setenv("BORDO_GITHUB_CLIENT_ID", "")
	cfg := &config.Config{}
	cfg.Server.Port = 7401
	cfg.Server.AuthToken = "internal-token"
	cfg.Store.Path = filepath.Join(t.TempDir(), "bordod.db")
	db, err := store.Open(cfg.Store)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if err := store.Migrate(db); err != nil {
		t.Fatal(err)
	}
	s := New(cfg, slog.New(slog.NewTextHandler(io.Discard, nil)), db)
	ts := httptest.NewServer(s.router)
	t.Cleanup(ts.Close)
	return ts
}

func call(t *testing.T, ts *httptest.Server, method, path, ctype, body string) (int, map[string]any) {
	t.Helper()
	req, _ := http.NewRequest(method, ts.URL+path, strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer internal-token")
	if ctype != "" {
		req.Header.Set("Content-Type", ctype)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	var out map[string]any
	_ = json.NewDecoder(res.Body).Decode(&out)
	return res.StatusCode, out
}

func TestConsoleSetupFlow(t *testing.T) {
	ts := newConsoleTestServer(t)

	code, out := call(t, ts, "GET", "/v1/setup/status", "", "")
	if code != 200 || out["configured"] != false || out["github_connected"] != false || out["github_device_ready"] != false {
		t.Fatalf("initial status = %d %v", code, out)
	}

	// A cross-site form post must not be able to complete setup.
	if code, _ = call(t, ts, "POST", "/v1/setup", "application/x-www-form-urlencoded", "deepseek_api_key=evil"); code != http.StatusUnsupportedMediaType {
		t.Fatalf("form post = %d, want 415", code)
	}
	if code, _ = call(t, ts, "POST", "/v1/setup", "application/json", `{"deepseek_api_key":""}`); code != http.StatusBadRequest {
		t.Fatalf("empty key = %d, want 400", code)
	}

	code, out = call(t, ts, "POST", "/v1/setup", "application/json", `{"deepseek_api_key":"sk-test","base_url":"http://localhost:7401"}`)
	if code != 200 {
		t.Fatalf("setup = %d %v", code, out)
	}
	if tok, _ := out["admin_token"].(string); tok != "" {
		t.Fatalf("console setup must not return the admin token, got %q", tok)
	}

	code, out = call(t, ts, "GET", "/v1/setup/status", "", "")
	if code != 200 || out["configured"] != true {
		t.Fatalf("status after setup = %d %v", code, out)
	}
	// Second run is refused.
	if code, _ = call(t, ts, "POST", "/v1/setup", "application/json", `{"deepseek_api_key":"sk-other"}`); code != http.StatusConflict {
		t.Fatalf("second setup = %d, want 409", code)
	}
	// The internal token (what the console proxy sends) still works afterwards.
	code, out = call(t, ts, "GET", "/v1/runtime-config", "", "")
	if code != 200 || out["deepseek_api_key"] != "sk-test" {
		t.Fatalf("runtime-config = %d %v", code, out)
	}
}

func TestGithubDeviceStartNeedsClientID(t *testing.T) {
	ts := newConsoleTestServer(t)
	if code, _ := call(t, ts, "POST", "/v1/github/device/start", "application/json", `{}`); code != http.StatusPreconditionFailed {
		t.Fatalf("no client id = %d, want 412", code)
	}
	if code, _ := call(t, ts, "POST", "/v1/github/device/start", "text/plain", `{}`); code != http.StatusUnsupportedMediaType {
		t.Fatalf("wrong content type = %d, want 415", code)
	}
	code, out := call(t, ts, "GET", "/v1/github/device/status", "", "")
	if code != 200 || out["status"] != "idle" {
		t.Fatalf("device status = %d %v", code, out)
	}
	if code, _ = call(t, ts, "GET", "/v1/setup/status", "", ""); code != 200 {
		t.Fatalf("status = %d", code)
	}
}

// The legacy /github routes store GitHub credentials and used to expose the stored token
// without any authentication.
func TestGithubRoutesRequireToken(t *testing.T) {
	ts := newConsoleTestServer(t)
	for _, path := range []string{"/github/token", "/github/status", "/github/app/setup", "/github/app/install", "/github/app/callback?code=x"} {
		res, err := http.Get(ts.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()
		if res.StatusCode != http.StatusUnauthorized {
			t.Errorf("GET %s without a token = %d, want 401", path, res.StatusCode)
		}
	}
	// With the token the route is reachable again (404 for the removed token route).
	if code, _ := call(t, ts, "GET", "/github/token", "", ""); code != http.StatusNotFound && code != http.StatusMethodNotAllowed {
		t.Errorf("/github/token with token = %d, want it removed", code)
	}
	if code, _ := call(t, ts, "GET", "/github/app/setup", "", ""); code != http.StatusOK {
		t.Errorf("/github/app/setup with token = %d, want 200", code)
	}
}

func TestBearerEquals(t *testing.T) {
	for _, c := range []struct {
		header, token string
		want          bool
	}{
		{"Bearer abc", "abc", true},
		{"Bearer abcd", "abc", false},
		{"Bearer ab", "abc", false},
		{"abc", "abc", false},
		{"", "abc", false},
	} {
		if got := bearerEquals(c.header, c.token); got != c.want {
			t.Errorf("bearerEquals(%q, %q) = %v, want %v", c.header, c.token, got, c.want)
		}
	}
}
