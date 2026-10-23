package buildorchestrator

import (
	"context"
	"database/sql"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hasangenc0/bordo/control-plane/internal/config"
	"github.com/hasangenc0/bordo/control-plane/internal/store"
)

func testDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := store.Open(config.StoreConfig{Driver: "sqlite", Path: filepath.Join(t.TempDir(), "t.db")})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if err := store.Migrate(db); err != nil {
		t.Fatal(err)
	}
	return db
}

func addProject(t *testing.T, db *sql.DB, name, tmpl, repo string) string {
	t.Helper()
	id := "p-" + name
	if _, err := db.Exec(`INSERT INTO projects (id, name, template, git_repo_url, status, created_at, updated_at)
		VALUES (?, ?, ?, ?, 'pending', datetime('now'), datetime('now'))`, id, name, tmpl, repo); err != nil {
		t.Fatal(err)
	}
	return id
}

// An imported project (repo, no template) must take the read-only path: no scaffold
// is generated and nothing is pushed, even when templates are available.
func TestImportedProjectIsNeverScaffolded(t *testing.T) {
	db := testDB(t)
	work := t.TempDir()
	tmplRoot := t.TempDir()
	if err := os.MkdirAll(filepath.Join(tmplRoot, "x"), 0o755); err != nil {
		t.Fatal(err)
	}
	bs := New(db)
	ex := NewExecutor(bs, db, slog.New(slog.NewTextHandler(io.Discard, nil)), work, tmplRoot, "", nil)

	pid := addProject(t, db, "hello-api", "", "https://github.com/acme/hello-api")
	b, err := bs.Create(context.Background(), pid, "hello-api", "latest")
	if err != nil {
		t.Fatal(err)
	}
	if err := ex.run(context.Background(), b.ID); err != nil {
		t.Fatal(err)
	}

	got, _ := bs.Get(context.Background(), b.ID)
	if got.Status != "failed" || !strings.Contains(got.ErrorMessage, "GitHub is not connected") {
		t.Errorf("status=%q err=%q, want failed with a connect-GitHub message", got.Status, got.ErrorMessage)
	}
	if _, err := os.Stat(filepath.Join(work, b.ID, "src")); !os.IsNotExist(err) {
		t.Error("an imported project must not be scaffolded")
	}
	logs, _ := bs.GetLogs(context.Background(), b.ID)
	if strings.Contains(strings.Join(logs, "\n"), "scaffold") {
		t.Errorf("logs mention scaffolding: %v", logs)
	}
}

func TestEnableBuildsRequiresARepo(t *testing.T) {
	db := testDB(t)
	ex := NewExecutor(New(db), db, slog.New(slog.NewTextHandler(io.Discard, nil)), t.TempDir(), "", "", nil)
	pid := addProject(t, db, "templated", "go-web-service", "")
	if _, _, err := ex.EnableBuilds(context.Background(), pid); err == nil || !strings.Contains(err.Error(), "no GitHub repository") {
		t.Errorf("got %v, want no-repository error", err)
	}
}

func TestCreateProjectRepo(t *testing.T) {
	db := testDB(t)
	ex := NewExecutor(New(db), db, slog.New(slog.NewTextHandler(io.Discard, nil)), t.TempDir(), "", "", nil)
	ctx := context.Background()

	// A project that already has a repo is returned as-is, without touching GitHub.
	linked := addProject(t, db, "linked", "", "https://github.com/acme/linked")
	url, created, err := ex.CreateProjectRepo(ctx, linked)
	if err != nil || created || url != "https://github.com/acme/linked" {
		t.Errorf("linked: url=%q created=%v err=%v", url, created, err)
	}

	// Without GitHub connected the error says how to fix it.
	fresh := addProject(t, db, "fresh", "go-web-service", "")
	if _, _, err := ex.CreateProjectRepo(ctx, fresh); err == nil || !strings.Contains(err.Error(), "bordo github login") {
		t.Errorf("got %v, want a connect-GitHub error", err)
	}

	if _, _, err := ex.CreateProjectRepo(ctx, "nope"); err == nil || !strings.Contains(err.Error(), "not found") {
		t.Errorf("got %v, want project not found", err)
	}
}
