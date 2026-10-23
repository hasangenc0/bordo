package release

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hasangenc0/bordo/control-plane/internal/config"
	"github.com/hasangenc0/bordo/control-plane/internal/store"
)

func testHandler(t *testing.T) (http.Handler, *sql.DB) {
	t.Helper()
	db, err := store.Open(config.StoreConfig{Driver: "sqlite", Path: filepath.Join(t.TempDir(), "t.db")})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if err := store.Migrate(db); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO projects (id, name, template, git_repo_url, status, created_at, updated_at)
		VALUES ('p-uuid', 'city-events-api', 'go-web-service', '', 'pending', datetime('now'), datetime('now'))`); err != nil {
		t.Fatal(err)
	}
	return NewHandler(db), db
}

func do(h http.Handler, method, path, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	return w
}

// The agent deploys by project name and then asks about the deploy using whichever id it was
// shown. Both must land on the project's real id and resolve to the release.
func TestDeployByNameIsListedByIDAndGroupIDResolves(t *testing.T) {
	h, _ := testHandler(t)

	w := do(h, "POST", "/", `{"project_id":"city-events-api","image_tag":"ghcr.io/o/r:abc","region":"eu-central","expose":"nodeport"}`)
	if w.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", w.Code, w.Body)
	}
	var rel Release
	_ = json.Unmarshal(w.Body.Bytes(), &rel)
	if rel.ProjectID != "p-uuid" {
		t.Errorf("project_id = %q, want the project's UUID (the name was stored)", rel.ProjectID)
	}

	for _, q := range []string{"p-uuid", "city-events-api"} {
		var out struct{ Releases []Release }
		_ = json.Unmarshal(do(h, "GET", "/?project_id="+q, "").Body.Bytes(), &out)
		if len(out.Releases) != 1 {
			t.Errorf("list by %q: %d releases, want 1", q, len(out.Releases))
		}
	}

	if w := do(h, "GET", "/"+rel.ID, ""); w.Code != http.StatusOK {
		t.Errorf("get by release id: %d", w.Code)
	}
	if w := do(h, "GET", "/"+rel.ReleaseGroupID, ""); w.Code != http.StatusOK || !strings.Contains(w.Body.String(), rel.ID) {
		t.Errorf("get by group id: %d %s", w.Code, w.Body)
	}
	if w := do(h, "GET", "/"+rel.ReleaseGroupID+"/logs", ""); w.Code != http.StatusOK {
		t.Errorf("logs by group id: %d %s", w.Code, w.Body)
	}
}

func TestDeployUnknownProjectIs404(t *testing.T) {
	h, _ := testHandler(t)
	w := do(h, "POST", "/", `{"project_id":"ghost","image_tag":"x:1","region":"eu"}`)
	if w.Code != http.StatusNotFound {
		t.Errorf("got %d, want 404 (a release for a project that does not exist is orphaned)", w.Code)
	}
}
