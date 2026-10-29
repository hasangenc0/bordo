package buildorchestrator

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestBuildWithTheSameImageAsBeforeIsFlagged(t *testing.T) {
	db := testDB(t)
	bs := New(db)
	h := &handler{store: bs}
	p := addProject(t, db, "app", "", "https://github.com/x/app")
	ctx := context.Background()

	first, _ := bs.Create(ctx, p, "app", "latest")
	_ = bs.UpdateStatus(ctx, first.ID, "success", "ghcr.io/x/app:aaa", "")
	if v := h.view(ctx, first); v.Warning != "" {
		t.Fatalf("the first build of an image must not warn: %q", v.Warning)
	}

	time.Sleep(1100 * time.Millisecond) // created_at has one-second resolution
	again, _ := bs.Create(ctx, p, "app", "latest")
	_ = bs.UpdateStatus(ctx, again.ID, "success", "ghcr.io/x/app:aaa", "")
	got, _ := bs.Get(ctx, again.ID)
	if v := h.view(ctx, got); !strings.Contains(v.Warning, "has not moved") || !strings.Contains(v.Warning, first.ID) {
		t.Fatalf("a rebuild of the same commit must warn, got %q", v.Warning)
	}

	time.Sleep(1100 * time.Millisecond)
	next, _ := bs.Create(ctx, p, "app", "latest")
	_ = bs.UpdateStatus(ctx, next.ID, "success", "ghcr.io/x/app:bbb", "")
	got, _ = bs.Get(ctx, next.ID)
	if v := h.view(ctx, got); v.Warning != "" {
		t.Fatalf("a new commit must not warn: %q", v.Warning)
	}
}
