package autodeploy

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/hasangenc0/bordo/control-plane/internal/approval"
	"github.com/hasangenc0/bordo/control-plane/internal/store"
	_ "github.com/mattn/go-sqlite3"
)

type harness struct {
	db       *sql.DB
	store    *Store
	approval approval.ApprovalStore
	head     string
	builds   int32
	buildErr error
	released []string
	w        *Watcher
}

func newHarness(t *testing.T, requireApproval bool) *harness {
	t.Helper()
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	if err := store.Migrate(db); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	db.Exec(`INSERT INTO projects (id, name) VALUES ('p1', 'city-events-api')`)

	h := &harness{db: db, store: NewStore(db), approval: approval.NewStore(db), head: "aaaa1111"}
	h.w = NewWatcher(h.store, Deps{
		Head: func(context.Context, string) (string, error) { return h.head, nil },
		Build: func(context.Context, string, string) (string, error) {
			atomic.AddInt32(&h.builds, 1)
			return "ghcr.io/o/city-events-api:" + h.head, h.buildErr
		},
		Release: func(_ context.Context, _, image, region, _ string) (string, error) {
			h.released = append(h.released, image+"@"+region)
			return "rel1", nil
		},
		Approvals: h.approval,
	}, 0, nil)
	if err := h.store.Set(context.Background(), Config{ProjectID: "p1", Region: "eu", RequireApproval: requireApproval}, "aaaa1111"); err != nil {
		t.Fatal(err)
	}
	return h
}

func (h *harness) tick() { h.w.Tick(context.Background()); h.w.Wait() }

func (h *harness) cfg(t *testing.T) *Config {
	t.Helper()
	c, err := h.store.Get(context.Background(), "p1")
	if err != nil || c == nil {
		t.Fatalf("get: %v %v", c, err)
	}
	return c
}

func TestUnchangedHeadDoesNothing(t *testing.T) {
	h := newHarness(t, false)
	h.tick()
	if h.builds != 0 || len(h.released) != 0 {
		t.Fatalf("enabling must not redeploy what is already live (builds=%d released=%v)", h.builds, h.released)
	}
}

func TestMergeBuildsAndReleasesWithoutApproval(t *testing.T) {
	h := newHarness(t, false)
	h.head = "bbbb2222"
	h.tick()
	if len(h.released) != 1 || h.released[0] != "ghcr.io/o/city-events-api:bbbb2222@eu" {
		t.Fatalf("released = %v", h.released)
	}
	if c := h.cfg(t); c.LastSHA != "bbbb2222" || !strings.Contains(c.LastResult, "deployed") {
		t.Fatalf("config = %+v", c)
	}
	h.tick() // the same commit must not deploy twice
	if h.builds != 1 || len(h.released) != 1 {
		t.Fatalf("second tick redeployed: builds=%d released=%v", h.builds, h.released)
	}
}

func TestApprovalGatesTheRelease(t *testing.T) {
	h := newHarness(t, true)
	h.head = "bbbb2222"
	h.tick()
	if len(h.released) != 0 {
		t.Fatal("must not release before approval")
	}
	c := h.cfg(t)
	if c.PendingApproval == "" || c.PendingImage == "" {
		t.Fatalf("expected a pending approval, got %+v", c)
	}
	h.tick() // still pending: no new build, no release
	if h.builds != 1 || len(h.released) != 0 {
		t.Fatalf("pending must wait: builds=%d released=%v", h.builds, h.released)
	}

	if err := h.approval.ResolveApproval(context.Background(), c.PendingApproval, "approved"); err != nil {
		t.Fatal(err)
	}
	h.tick()
	if len(h.released) != 1 {
		t.Fatalf("approved deploy not released: %v", h.released)
	}
	if got := h.cfg(t); got.PendingApproval != "" || got.LastSHA != "bbbb2222" {
		t.Fatalf("state after approval: %+v", got)
	}
}

func TestRejectedApprovalSkipsThatCommitOnly(t *testing.T) {
	h := newHarness(t, true)
	h.head = "bbbb2222"
	h.tick()
	_ = h.approval.ResolveApproval(context.Background(), h.cfg(t).PendingApproval, "rejected")
	h.tick()
	if len(h.released) != 0 || h.cfg(t).LastSHA != "bbbb2222" {
		t.Fatalf("rejected commit must be skipped: released=%v cfg=%+v", h.released, h.cfg(t))
	}
	h.tick()
	if h.builds != 1 {
		t.Fatalf("a rejected commit must not be rebuilt every tick (builds=%d)", h.builds)
	}
	h.head = "cccc3333" // a new push is considered again
	h.tick()
	if h.builds != 2 {
		t.Fatalf("a new commit must be built (builds=%d)", h.builds)
	}
}

func TestBuildFailureIsNotRetriedUntilANewPush(t *testing.T) {
	h := newHarness(t, false)
	h.head = "bbbb2222"
	h.buildErr = errors.New("compile error")
	h.tick()
	h.tick()
	if h.builds != 1 || len(h.released) != 0 {
		t.Fatalf("builds=%d released=%v", h.builds, h.released)
	}
	if c := h.cfg(t); !strings.Contains(c.LastResult, "build failed") {
		t.Fatalf("last result = %q", c.LastResult)
	}
}
