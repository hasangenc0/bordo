package buildorchestrator

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"
)

func TestListWithoutProjectFilterReturnsRecentBuilds(t *testing.T) {
	db := testDB(t)
	bs := New(db)
	a := addProject(t, db, "a", "", "https://github.com/x/a")
	b := addProject(t, db, "b", "", "https://github.com/x/b")
	for _, p := range []string{a, b} {
		if _, err := bs.Create(context.Background(), p, "img", "latest"); err != nil {
			t.Fatal(err)
		}
	}
	all, err := bs.ListByProject(context.Background(), "")
	if err != nil || len(all) != 2 {
		t.Fatalf("unfiltered list = %d builds, err %v", len(all), err)
	}
}

func TestFailIfRunningOnlyTouchesRunningBuilds(t *testing.T) {
	db := testDB(t)
	bs := New(db)
	p := addProject(t, db, "a", "", "https://github.com/x/a")
	run, _ := bs.Create(context.Background(), p, "img", "latest")
	done, _ := bs.Create(context.Background(), p, "img", "latest")
	_ = bs.SetRunning(context.Background(), run.ID)
	_ = bs.UpdateStatus(context.Background(), done.ID, "success", "ghcr.io/x/a:sha", "")

	for _, id := range []string{run.ID, done.ID} {
		if err := bs.FailIfRunning(context.Background(), id, "timed out"); err != nil {
			t.Fatal(err)
		}
	}
	if got, _ := bs.Get(context.Background(), run.ID); got.Status != "failed" {
		t.Errorf("running build status = %q, want failed", got.Status)
	}
	if got, _ := bs.Get(context.Background(), done.ID); got.Status != "success" || got.ImageDigest == "" {
		t.Errorf("finished build was overwritten: %+v", got)
	}
}

func TestResumeInterruptedFailsStaleBuilds(t *testing.T) {
	db := testDB(t)
	bs := New(db)
	ex := NewExecutor(bs, db, slog.New(slog.NewTextHandler(io.Discard, nil)), t.TempDir(), "", "", nil)
	p := addProject(t, db, "a", "", "https://github.com/x/a")
	old, _ := bs.Create(context.Background(), p, "img", "latest")
	if _, err := db.Exec(`UPDATE builds SET status='running', started_at=? WHERE id=?`, time.Now().Add(-2*time.Hour).UTC(), old.ID); err != nil {
		t.Fatal(err)
	}
	ex.ResumeInterrupted(context.Background())
	if got, _ := bs.Get(context.Background(), old.ID); got.Status != "failed" {
		t.Fatalf("stale running build status = %q, want failed", got.Status)
	}
}
