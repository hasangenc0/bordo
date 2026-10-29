package autodeploy

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
)

func call(r http.Handler, method, path, body string) (int, map[string]any) {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	var out map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &out)
	return w.Code, out
}

func TestHandlerEnableStatusDisable(t *testing.T) {
	h := newHarness(t, false)
	h.db.Exec(`INSERT INTO regions (id, name, status) VALUES ('r1','eu','healthy')`)
	if err := h.store.Disable(context.Background(), "p1"); err != nil {
		t.Fatal(err)
	}
	// Mounted the way the server mounts it, next to a broader /projects router.
	root := chi.NewRouter()
	root.Mount("/projects", http.NotFoundHandler())
	root.Mount("/projects/{projectID}/autodeploy", NewHandler(h.db, h.store, func(context.Context, string) (string, error) { return "head1111", nil }))

	if _, out := call(root, "GET", "/projects/city-events-api/autodeploy", ""); out["enabled"] != false {
		t.Fatalf("not enabled by default: %v", out)
	}
	if code, _ := call(root, "PUT", "/projects/city-events-api/autodeploy", `{"region":"nowhere"}`); code != http.StatusBadRequest {
		t.Fatalf("unknown region: %d", code)
	}
	code, out := call(root, "PUT", "/projects/city-events-api/autodeploy", `{"region":"eu"}`)
	if code != http.StatusOK {
		t.Fatalf("enable: %d %v", code, out)
	}
	cfg := out["config"].(map[string]any)
	if cfg["require_approval"] != true {
		t.Errorf("approval must be required unless turned off: %v", cfg)
	}
	if cfg["last_sha"] != "head1111" {
		t.Errorf("baseline must be the current head so live code is not redeployed: %v", cfg)
	}
	if code, out = call(root, "PUT", "/projects/p1/autodeploy", `{"region":"eu","require_approval":false}`); code != http.StatusOK || out["config"].(map[string]any)["require_approval"] != false {
		t.Fatalf("explicit opt-out of approval: %d %v", code, out)
	}
	if _, out = call(root, "DELETE", "/projects/p1/autodeploy", ""); out["enabled"] != false {
		t.Fatalf("disable: %v", out)
	}
	if code, _ = call(root, "GET", "/projects/ghost/autodeploy", ""); code != http.StatusNotFound {
		t.Fatalf("unknown project: %d", code)
	}
}
