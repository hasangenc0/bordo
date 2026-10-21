package release

import (
	"context"
	"database/sql"
	"strings"
	"testing"

	_ "github.com/mattn/go-sqlite3"
)

func TestDesiredState(t *testing.T) {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	defer db.Close()
	if _, err := db.Exec(`CREATE TABLE bordo_releases (
		id TEXT, project_id TEXT, image_tag TEXT, region TEXT,
		strategy TEXT, status TEXT, updated_at DATETIME)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO bordo_releases
		(id, project_id, image_tag, region, strategy, status, updated_at) VALUES
		('1','projA','ghcr.io/x/a:old','us-east','rolling','deployed','2026-01-01T00:00:00Z'),
		('2','projA','ghcr.io/x/a:new','us-east','rolling','deployed','2026-02-01T00:00:00Z'),
		('3','projB','ghcr.io/x/b:v1','us-east','rolling','reconciling','2026-01-15T00:00:00Z'),
		('4','projC','ghcr.io/x/c:v1','eu-west','rolling','deployed','2026-01-01T00:00:00Z'),
		('5','projD','ghcr.io/x/d:v1','us-east','rolling','failed','2026-01-01T00:00:00Z')`); err != nil {
		t.Fatal(err)
	}

	ds, err := DesiredState(context.Background(), db, "us-east")
	if err != nil {
		t.Fatalf("desired state: %v", err)
	}

	// latest active release for each us-east project is included
	if !strings.Contains(ds, "a:new") {
		t.Error("expected latest projA release (a:new)")
	}
	if !strings.Contains(ds, "b:v1") {
		t.Error("expected projB release (b:v1)")
	}
	// superseded, other-region, and non-active releases are excluded
	if strings.Contains(ds, "a:old") {
		t.Error("superseded release a:old should not be present")
	}
	if strings.Contains(ds, "c:v1") {
		t.Error("other region (eu-west) should not be present")
	}
	if strings.Contains(ds, "d:v1") {
		t.Error("failed release should not be present")
	}

	// empty region yields empty desired state, not an error
	empty, err := DesiredState(context.Background(), db, "nowhere")
	if err != nil {
		t.Fatal(err)
	}
	if empty != "" {
		t.Errorf("expected empty desired state, got %q", empty)
	}
}
