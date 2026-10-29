// Package autodeploy deploys a project when its repository's default branch moves.
//
// GitHub Actions already builds an image on every push (the Bordo build workflow). What was
// missing is anything on the Bordo side that notices: nothing turned a merge into a build record,
// a release and a rollout. The watcher polls the default branch of each enabled project (polling
// rather than webhooks, so bordod does not have to be reachable from GitHub), builds the new
// commit, and releases it, optionally behind an approval.
package autodeploy

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/hasangenc0/bordo/control-plane/internal/approval"
)

// Config is one project's auto-deploy setting and progress.
type Config struct {
	ProjectID       string `json:"project_id"`
	Region          string `json:"region"`
	Expose          string `json:"expose"`
	RequireApproval bool   `json:"require_approval"`
	// LastSHA is the newest commit already handled (deployed, rejected or failed to build).
	LastSHA string `json:"last_sha"`
	// Pending* describe a built image waiting for approval.
	PendingSHA      string `json:"pending_sha,omitempty"`
	PendingImage    string `json:"pending_image,omitempty"`
	PendingApproval string `json:"pending_approval,omitempty"`
	LastResult      string `json:"last_result,omitempty"`
}

// Store persists Config.
type Store struct{ db *sql.DB }

func NewStore(db *sql.DB) *Store { return &Store{db: db} }

const cols = `project_id, region, expose, require_approval, last_sha, pending_sha, pending_image, pending_approval, last_result`

func scan(row interface{ Scan(...any) error }) (*Config, error) {
	var c Config
	var req int
	if err := row.Scan(&c.ProjectID, &c.Region, &c.Expose, &req, &c.LastSHA, &c.PendingSHA, &c.PendingImage, &c.PendingApproval, &c.LastResult); err != nil {
		return nil, err
	}
	c.RequireApproval = req != 0
	return &c, nil
}

// Get returns the project's config, or nil when auto-deploy is not enabled.
func (s *Store) Get(ctx context.Context, projectID string) (*Config, error) {
	c, err := scan(s.db.QueryRowContext(ctx, `SELECT `+cols+` FROM bordo_autodeploy WHERE project_id = ?`, projectID))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return c, err
}

// Set enables (or reconfigures) auto-deploy. baseline is the commit to treat as already
// handled, so enabling it does not redeploy code that is already live.
func (s *Store) Set(ctx context.Context, c Config, baseline string) error {
	req := 0
	if c.RequireApproval {
		req = 1
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO bordo_autodeploy (project_id, region, expose, require_approval, last_sha, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(project_id) DO UPDATE SET
			region = excluded.region, expose = excluded.expose,
			require_approval = excluded.require_approval, updated_at = excluded.updated_at`,
		c.ProjectID, c.Region, c.Expose, req, baseline, time.Now().UTC())
	return err
}

// Disable turns auto-deploy off for the project.
func (s *Store) Disable(ctx context.Context, projectID string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM bordo_autodeploy WHERE project_id = ?`, projectID)
	return err
}

func (s *Store) list(ctx context.Context) ([]*Config, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+cols+` FROM bordo_autodeploy`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Config
	for rows.Next() {
		c, err := scan(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (s *Store) save(ctx context.Context, c *Config) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE bordo_autodeploy SET last_sha = ?, pending_sha = ?, pending_image = ?, pending_approval = ?,
			last_result = ?, updated_at = ? WHERE project_id = ?`,
		c.LastSHA, c.PendingSHA, c.PendingImage, c.PendingApproval, c.LastResult, time.Now().UTC(), c.ProjectID)
	return err
}

func (s *Store) projectName(ctx context.Context, id string) string {
	var name string
	_ = s.db.QueryRowContext(ctx, `SELECT name FROM projects WHERE id = ?`, id).Scan(&name)
	if name == "" {
		return id
	}
	return name
}

// Deps are the watcher's collaborators; functions so tests need no GitHub or cluster.
type Deps struct {
	// Head returns the repository's current default-branch commit.
	Head func(ctx context.Context, projectID string) (sha string, err error)
	// Build builds the current default-branch commit and returns the image reference.
	Build func(ctx context.Context, projectID, projectName string) (image string, err error)
	// Release records a rollout of image and returns the release id.
	Release func(ctx context.Context, projectID, image, region, expose string) (releaseID string, err error)
	// Approvals is where an approval card is raised for projects that require one.
	Approvals approval.ApprovalStore
}

// Watcher polls enabled projects and drives them through build, approval and release.
type Watcher struct {
	store    *Store
	deps     Deps
	interval time.Duration
	logf     func(format string, args ...any)

	mu       sync.Mutex
	inflight map[string]bool
	wg       sync.WaitGroup
}

// NewWatcher builds a Watcher. logf may be nil.
func NewWatcher(store *Store, deps Deps, interval time.Duration, logf func(string, ...any)) *Watcher {
	if interval <= 0 {
		interval = time.Minute
	}
	if logf == nil {
		logf = func(string, ...any) {}
	}
	return &Watcher{store: store, deps: deps, interval: interval, logf: logf, inflight: map[string]bool{}}
}

// Run ticks until ctx is cancelled.
func (w *Watcher) Run(ctx context.Context) {
	t := time.NewTicker(w.interval)
	defer t.Stop()
	for {
		w.Tick(ctx)
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

// Tick checks every enabled project once. A project with work in progress is skipped, so a long
// build is never started twice. Work runs in the background; Wait blocks until it is done.
func (w *Watcher) Tick(ctx context.Context) {
	cfgs, err := w.store.list(ctx) // fully read before any other query: the DB has one connection
	if err != nil {
		w.logf("autodeploy: list: %v", err)
		return
	}
	for _, c := range cfgs {
		w.mu.Lock()
		busy := w.inflight[c.ProjectID]
		if !busy {
			w.inflight[c.ProjectID] = true
		}
		w.mu.Unlock()
		if busy {
			continue
		}
		w.wg.Add(1)
		go func(c *Config) {
			defer w.wg.Done()
			defer func() {
				w.mu.Lock()
				delete(w.inflight, c.ProjectID)
				w.mu.Unlock()
			}()
			w.process(ctx, c)
		}(c)
	}
}

// Wait blocks until all in-progress work finishes (for tests and shutdown).
func (w *Watcher) Wait() { w.wg.Wait() }

func (w *Watcher) process(ctx context.Context, c *Config) {
	if c.PendingApproval != "" {
		w.resolvePending(ctx, c)
		return
	}
	head, err := w.deps.Head(ctx, c.ProjectID)
	if err != nil {
		w.logf("autodeploy %s: reading the repository: %v", c.ProjectID, err)
		return // transient (network, token); try again next tick
	}
	if head == "" || head == c.LastSHA {
		return
	}
	name := w.store.projectName(ctx, c.ProjectID)
	w.logf("autodeploy %s: %s moved to %.8s, building", name, "default branch", head)
	image, err := w.deps.Build(ctx, c.ProjectID, name)
	if err != nil {
		// Remember the commit so a broken build is not retried every minute; a new push retries.
		c.LastSHA = head
		c.LastResult = fmt.Sprintf("%.8s: build failed: %v", head, err)
		_ = w.store.save(ctx, c)
		return
	}
	if !c.RequireApproval {
		w.release(ctx, c, head, image)
		return
	}
	payload, _ := json.Marshal(map[string]string{
		"project_id": c.ProjectID, "project": name, "image": image, "sha": head, "region": c.Region,
	})
	a := &approval.Approval{Kind: "deploy", Target: fmt.Sprintf("%s → %s (%.8s)", name, c.Region, head), Payload: string(payload)}
	if err := w.deps.Approvals.CreateApproval(ctx, a); err != nil {
		c.LastResult = fmt.Sprintf("%.8s: built, but could not request approval: %v", head, err)
		_ = w.store.save(ctx, c)
		return
	}
	c.PendingSHA, c.PendingImage, c.PendingApproval = head, image, a.ID
	c.LastResult = fmt.Sprintf("%.8s: built, waiting for approval", head)
	_ = w.store.save(ctx, c)
}

func (w *Watcher) resolvePending(ctx context.Context, c *Config) {
	a, err := w.deps.Approvals.GetApproval(ctx, c.PendingApproval)
	if err != nil {
		w.logf("autodeploy %s: approval %s: %v", c.ProjectID, c.PendingApproval, err)
		return
	}
	switch a.Status {
	case "pending":
		return
	case "approved":
		sha, image := c.PendingSHA, c.PendingImage
		c.PendingSHA, c.PendingImage, c.PendingApproval = "", "", ""
		w.release(ctx, c, sha, image)
	default: // rejected or anything else: do not deploy this commit
		c.LastSHA = c.PendingSHA
		c.LastResult = fmt.Sprintf("%.8s: deploy %s", c.PendingSHA, a.Status)
		c.PendingSHA, c.PendingImage, c.PendingApproval = "", "", ""
		_ = w.store.save(ctx, c)
	}
}

func (w *Watcher) release(ctx context.Context, c *Config, sha, image string) {
	id, err := w.deps.Release(ctx, c.ProjectID, image, c.Region, c.Expose)
	c.LastSHA = sha
	if err != nil {
		c.LastResult = fmt.Sprintf("%.8s: built, but the release failed: %v", sha, err)
	} else {
		c.LastResult = fmt.Sprintf("%.8s: deployed (release %s)", sha, id)
	}
	_ = w.store.save(ctx, c)
}
