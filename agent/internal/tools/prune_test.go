package tools

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestPruneWorkspaces(t *testing.T) {
	c, _ := setup(t)
	if _, err := call(t, c, "code_checkout", map[string]any{"project": "my-app"}); err != nil {
		t.Fatal(err)
	}
	live := filepath.Join(c.root, "p-1")
	orphan := filepath.Join(c.root, "p-deleted")
	if err := os.MkdirAll(filepath.Join(orphan, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	// A fresh checkout of a live project stays; the deleted project's goes.
	if removed := c.PruneWorkspaces(ctx, 30*24*time.Hour); len(removed) != 1 {
		t.Fatalf("removed = %v, want only the deleted project", removed)
	}
	if _, err := os.Stat(orphan); !os.IsNotExist(err) {
		t.Error("checkout of a deleted project survived")
	}
	if _, err := os.Stat(live); err != nil {
		t.Fatal("checkout of a live project was removed")
	}

	// Idle but with an uncommitted edit: kept, the edit exists nowhere else.
	_ = os.WriteFile(filepath.Join(live, "README.md"), []byte("# edited\n"), 0o644)
	if removed := c.PruneWorkspaces(ctx, -time.Second); len(removed) != 0 {
		t.Fatalf("a checkout with uncommitted work was pruned: %v", removed)
	}

	// Idle and committed but not pushed: kept.
	run(t, live, "-c", "user.name=t", "-c", "user.email=t@t", "commit", "-am", "local only")
	if removed := c.PruneWorkspaces(ctx, -time.Second); len(removed) != 0 {
		t.Fatalf("a checkout with unpushed commits was pruned: %v", removed)
	}

	// Idle and fully pushed: removed.
	run(t, live, "push", "origin", "HEAD")
	run(t, live, "fetch", "origin")
	if removed := c.PruneWorkspaces(ctx, -time.Second); len(removed) != 1 {
		t.Fatalf("an idle clean checkout must be pruned, removed = %v", removed)
	}
}

func TestPruneWorkspacesKeepsEverythingWhenBordodIsDown(t *testing.T) {
	c, _ := setup(t)
	if _, err := call(t, c, "code_checkout", map[string]any{"project": "my-app"}); err != nil {
		t.Fatal(err)
	}
	c.cp = NewCPClient("http://127.0.0.1:1", "")
	_ = os.WriteFile(filepath.Join(c.root, "p-1", "dirty"), []byte("x"), 0o644)
	if removed := c.PruneWorkspaces(context.Background(), time.Hour); len(removed) != 0 {
		t.Fatalf("nothing may be pruned as orphaned when the project list is unavailable: %v", removed)
	}
}
