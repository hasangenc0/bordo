package tools

import (
	"context"
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Checkouts live in the agent's workspace volume forever unless something removes them. The
// janitor removes two kinds, both safe to drop because code_checkout clones again:
//   - checkouts of projects that no longer exist in bordod
//   - checkouts nobody has used for a while that hold no uncommitted or unpushed work

// Janitor prunes workspaces every interval until ctx is done.
func (c *CodeTools) Janitor(ctx context.Context, interval, idle time.Duration) {
	prune := func() {
		if removed := c.PruneWorkspaces(ctx, idle); len(removed) > 0 {
			log.Printf("agent: pruned %d workspace(s): %s", len(removed), strings.Join(removed, ", "))
		}
	}
	prune()
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			prune()
		}
	}
}

// PruneWorkspaces removes deleted projects' checkouts and clean checkouts idle for longer than idle,
// and returns the project ids it removed. If bordod cannot be reached it removes nothing for being
// orphaned: a failed lookup must not look like every project was deleted.
func (c *CodeTools) PruneWorkspaces(ctx context.Context, idle time.Duration) []string {
	entries, err := os.ReadDir(c.root)
	if err != nil {
		return nil
	}
	known, listed := c.projectIDs(ctx)
	var removed []string
	for _, e := range entries {
		id := e.Name()
		if !e.IsDir() || !safeIDRe.MatchString(id) || id == "." || id == ".." {
			continue
		}
		dir := filepath.Join(c.root, id)
		unlock := c.lock(id) // never prune under a tool that is using the checkout
		var reason string
		switch {
		case listed && !known[id]:
			reason = "project deleted"
		case time.Since(lastUsed(dir)) > idle && c.clean(ctx, dir):
			reason = "idle"
		}
		if reason != "" {
			if err := os.RemoveAll(dir); err != nil {
				log.Printf("agent: pruning workspace %s (%s): %v", id, reason, err)
			} else {
				removed = append(removed, id+" ("+reason+")")
			}
		}
		unlock()
	}
	return removed
}

func (c *CodeTools) projectIDs(ctx context.Context) (map[string]bool, bool) {
	raw, err := c.cp.Get(ctx, "/v1/projects")
	if err != nil {
		return nil, false
	}
	var list struct {
		Projects []codeProject `json:"projects"`
	}
	if err := json.Unmarshal(raw, &list); err != nil {
		return nil, false
	}
	ids := make(map[string]bool, len(list.Projects))
	for _, p := range list.Projects {
		ids[p.ID] = true
	}
	return ids, true
}

// clean reports whether the checkout has nothing that exists only here: no uncommitted changes and
// no commits missing from the remote. Anything that cannot be checked counts as not clean.
func (c *CodeTools) clean(ctx context.Context, dir string) bool {
	w := &ws{c: c, dir: dir, scrubFn: func(s string) string { return s }}
	if out, err := w.git(ctx, "status", "--porcelain"); err != nil || strings.TrimSpace(out) != "" {
		return false
	}
	out, err := w.git(ctx, "log", "--branches", "--not", "--remotes", "--oneline", "-1")
	return err == nil && strings.TrimSpace(out) == ""
}

// lastUsed is the newest modification time among the paths git and the code tools touch when a
// checkout is used.
func lastUsed(dir string) time.Time {
	var latest time.Time
	for _, p := range []string{dir, filepath.Join(dir, ".git"), filepath.Join(dir, ".git", "index"), filepath.Join(dir, ".git", "HEAD"), filepath.Join(dir, ".git", "FETCH_HEAD")} {
		if st, err := os.Stat(p); err == nil && st.ModTime().After(latest) {
			latest = st.ModTime()
		}
	}
	return latest
}
