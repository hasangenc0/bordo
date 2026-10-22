package server

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/hasangenc0/bordo/agent/internal/tools"
)

// The console reaches the control plane through the agent's /v1 passthrough.
// chi's Mount leaves r.URL.Path untouched, so the handler must not double the prefix.
func TestCPProxyForwardsWithSingleV1Prefix(t *testing.T) {
	var gotPath, gotQuery, gotAuth string
	cp := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotQuery, gotAuth = r.URL.Path, r.URL.RawQuery, r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer cp.Close()

	s := &Server{cpClient: tools.NewCPClient(cp.URL, "internal-token")}
	r := chi.NewRouter()
	r.Mount("/v1", s.handleCPProxy())

	for _, tc := range []struct{ in, wantPath, wantQuery string }{
		{"/v1/projects?include=release_status", "/v1/projects", "include=release_status"},
		{"/v1/regions/us-east/health", "/v1/regions/us-east/health", ""},
	} {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, tc.in, nil)
		req.Header.Set("Authorization", "Bearer browser-supplied")
		r.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s: status %d", tc.in, rec.Code)
		}
		if gotPath != tc.wantPath || gotQuery != tc.wantQuery {
			t.Errorf("%s: upstream got %s?%s, want %s?%s", tc.in, gotPath, gotQuery, tc.wantPath, tc.wantQuery)
		}
		if gotAuth != "Bearer internal-token" {
			t.Errorf("%s: upstream auth = %q, want the agent's own token", tc.in, gotAuth)
		}
	}
}
