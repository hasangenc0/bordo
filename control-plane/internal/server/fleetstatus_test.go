package server

import (
	"context"
	"database/sql"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/hasangenc0/bordo/control-plane/internal/fleet"
	"github.com/hasangenc0/bordo/control-plane/internal/store"
	_ "github.com/mattn/go-sqlite3"
)

func newStatusTestServer(t *testing.T) (*Server, *sql.DB) {
	t.Helper()
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	if err := store.Migrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return &Server{db: db, logger: slog.New(slog.NewTextHandler(io.Discard, nil))}, db
}

func statusOf(t *testing.T, db *sql.DB, id string) string {
	t.Helper()
	var s string
	if err := db.QueryRow(`SELECT status FROM bordo_releases WHERE id = ?`, id).Scan(&s); err != nil {
		t.Fatalf("status of %s: %v", id, err)
	}
	return s
}

func postStatus(s *Server, token, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/fleet/status", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	s.handleFleetStatus(w, req)
	return w
}

func TestHandleFleetStatusAdvancesReleases(t *testing.T) {
	s, db := newStatusTestServer(t)
	ctx := context.Background()

	db.Exec(`INSERT INTO regions (id, name, status) VALUES ('reg1','eu-central','pending'), ('reg2','other','pending')`)
	token, err := fleet.New(db).IssueClusterToken(ctx, "eu-central")
	if err != nil {
		t.Fatalf("issue cluster token: %v", err)
	}
	db.Exec(`INSERT INTO bordo_releases (id, project_id, image_tag, region, status) VALUES
		('r1','p1','img:1','eu-central','reconciling'),
		('r2','p2','img:2','other','reconciling')`)

	w := postStatus(s, token, `{"applied_release_ids":["r1","r2"]}`)
	if w.Code != http.StatusOK {
		t.Fatalf("status code = %d, body=%s", w.Code, w.Body.String())
	}

	// r1 is in the reporting region and was reconciling -> deployed.
	if got := statusOf(t, db, "r1"); got != "deployed" {
		t.Errorf("r1 status = %q, want deployed", got)
	}
	// r2 belongs to another region; the cluster token must not touch it.
	if got := statusOf(t, db, "r2"); got != "reconciling" {
		t.Errorf("r2 status = %q, want reconciling (region-scoped)", got)
	}

	// The report is also a heartbeat: region becomes healthy with a pull time
	// and a reconciler node-health row.
	var pull sql.NullTime
	var regStatus string
	db.QueryRow(`SELECT status, last_pull_at FROM regions WHERE name='eu-central'`).Scan(&regStatus, &pull)
	if regStatus != "healthy" || !pull.Valid {
		t.Errorf("region after report: status=%q last_pull_valid=%v", regStatus, pull.Valid)
	}
	var ready int
	if err := db.QueryRow(`SELECT ready FROM bordo_node_health WHERE region_name='eu-central' AND node_name='reconciler'`).Scan(&ready); err != nil {
		t.Errorf("reconciler heartbeat row missing: %v", err)
	} else if ready != 1 {
		t.Errorf("reconciler ready = %d, want 1", ready)
	}
}

func TestHandleFleetStatusReportsFailure(t *testing.T) {
	s, db := newStatusTestServer(t)
	ctx := context.Background()

	db.Exec(`INSERT INTO regions (id, name, status) VALUES ('reg1','eu-central','pending')`)
	token, err := fleet.New(db).IssueClusterToken(ctx, "eu-central")
	if err != nil {
		t.Fatal(err)
	}
	db.Exec(`INSERT INTO bordo_releases (id, project_id, image_tag, region, status) VALUES
		('r1','p1','img:1','eu-central','reconciling')`)

	w := postStatus(s, token, `{"applied_release_ids":["r1"],"error":"kubectl apply: boom"}`)
	if w.Code != http.StatusOK {
		t.Fatalf("status code = %d", w.Code)
	}
	if got := statusOf(t, db, "r1"); got != "failed" {
		t.Errorf("r1 status = %q, want failed", got)
	}
	var logText string
	db.QueryRow(`SELECT log FROM bordo_releases WHERE id='r1'`).Scan(&logText)
	if !strings.Contains(logText, "boom") {
		t.Errorf("release log = %q, want the apply error", logText)
	}
}

func TestHandleFleetStatusRejectsBadToken(t *testing.T) {
	s, _ := newStatusTestServer(t)
	w := postStatus(s, "nonsense", `{"applied_release_ids":["r1"]}`)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status code = %d, want 401", w.Code)
	}
}

// "deployed" with an empty log left the agent (and the user) guessing; the log must say what happened.
func TestHandleFleetStatusWritesALogOnSuccess(t *testing.T) {
	s, db := newStatusTestServer(t)
	ctx := context.Background()
	db.Exec(`INSERT INTO regions (id, name, status) VALUES ('reg1','eu-central','pending')`)
	token, _ := fleet.New(db).IssueClusterToken(ctx, "eu-central")
	db.Exec(`INSERT INTO bordo_releases (id, project_id, image_tag, region, status) VALUES ('r1','p1','img:1','eu-central','reconciling')`)

	postStatus(s, token, `{"applied_release_ids":["r1"]}`)

	var log string
	db.QueryRow(`SELECT log FROM bordo_releases WHERE id = 'r1'`).Scan(&log)
	if statusOf(t, db, "r1") != "deployed" || !strings.Contains(log, "applied the manifests") {
		t.Errorf("status=%q log=%q", statusOf(t, db, "r1"), log)
	}
}
