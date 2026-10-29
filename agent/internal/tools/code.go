package tools

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	maxReadBytes   = 200 * 1024
	maxFileBytes   = 2 * 1024 * 1024
	maxListEntries = 500
	maxSearchHits  = 100
	maxDiffBytes   = 100 * 1024
)

var skipDirs = map[string]bool{".git": true, "node_modules": true, "target": true, "dist": true}

var safeIDRe = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)

// CodeTools lets the agent edit a project's source in a git workspace.
// Changes always land on a working branch; the default branch is never pushed.
type CodeTools struct {
	cp   *CPClient
	root string

	// Injection points for tests.
	remoteURL func(host, owner, repo string) string
	apiBase   string
	tokenFn   func(ctx context.Context) (string, error)

	mu    sync.Mutex
	locks map[string]*sync.Mutex
}

// NewCodeTools creates the code tools. root defaults to $BORDO_WORKSPACE or ~/.bordo/workspaces.
func NewCodeTools(cp *CPClient, root string) *CodeTools {
	if root == "" {
		root = os.Getenv("BORDO_WORKSPACE")
	}
	if root == "" {
		home, _ := os.UserHomeDir()
		root = filepath.Join(home, ".bordo", "workspaces")
	}
	c := &CodeTools{
		cp:      cp,
		root:    root,
		apiBase: "https://api.github.com",
		locks:   map[string]*sync.Mutex{},
		remoteURL: func(host, owner, repo string) string {
			return fmt.Sprintf("https://%s/%s/%s.git", host, owner, repo)
		},
	}
	c.tokenFn = c.fetchToken
	return c
}

func (c *CodeTools) fetchToken(ctx context.Context) (string, error) {
	raw, err := c.cp.Get(ctx, "/v1/github/token")
	if err != nil {
		return "", fmt.Errorf("GitHub is not connected (run 'bordo github login'): %w", err)
	}
	var out struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(raw, &out); err != nil || out.Token == "" {
		return "", errors.New("GitHub is not connected (run 'bordo github login')")
	}
	return out.Token, nil
}

// ── project resolution ───────────────────────────────────────────────────────

type codeProject struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	GitRepoURL string `json:"git_repo_url"`
}

func (c *CodeTools) resolve(ctx context.Context, ref string) (*codeProject, error) {
	if ref == "" {
		return nil, errors.New("project is required")
	}
	raw, err := c.cp.Get(ctx, "/v1/projects")
	if err != nil {
		return nil, err
	}
	var list struct {
		Projects []codeProject `json:"projects"`
	}
	if err := json.Unmarshal(raw, &list); err != nil {
		return nil, fmt.Errorf("decode projects: %w", err)
	}
	for i := range list.Projects {
		if list.Projects[i].ID == ref {
			return &list.Projects[i], nil
		}
	}
	for i := range list.Projects {
		if list.Projects[i].Name == ref {
			return &list.Projects[i], nil
		}
	}
	return nil, fmt.Errorf("project %q not found", ref)
}

func (c *CodeTools) lock(id string) func() {
	c.mu.Lock()
	l, ok := c.locks[id]
	if !ok {
		l = &sync.Mutex{}
		c.locks[id] = l
	}
	c.mu.Unlock()
	l.Lock()
	return l.Unlock
}

// ws is a resolved, locked workspace for one project.
type ws struct {
	c       *CodeTools
	proj    *codeProject
	dir     string
	host    string
	owner   string
	repo    string
	token   string
	scrubFn func(string) string
}

// open resolves the project, locks it, and fetches the GitHub token.
// The returned release func must be called.
func (c *CodeTools) open(ctx context.Context, ref string) (*ws, func(), error) {
	p, err := c.resolve(ctx, ref)
	if err != nil {
		return nil, nil, err
	}
	if !safeIDRe.MatchString(p.ID) || p.ID == "." || p.ID == ".." {
		return nil, nil, fmt.Errorf("unsafe project id %q", p.ID)
	}
	if p.GitRepoURL == "" {
		return nil, nil, fmt.Errorf("project %q has no git repository", p.Name)
	}
	host, owner, repo, err := ParseRepoURL(p.GitRepoURL)
	if err != nil {
		return nil, nil, err
	}
	token, err := c.tokenFn(ctx)
	if err != nil {
		return nil, nil, err
	}
	unlock := c.lock(p.ID)
	w := &ws{
		c: c, proj: p, dir: filepath.Join(c.root, p.ID),
		host: host, owner: owner, repo: repo, token: token,
	}
	w.scrubFn = func(s string) string { return ScrubToken(s, token) }
	return w, unlock, nil
}

// ParseRepoURL extracts host, owner and repo from https, ssh and scp-style git URLs.
func ParseRepoURL(u string) (host, owner, repo string, err error) {
	s := strings.TrimSpace(u)
	s = strings.TrimSuffix(s, "/")
	s = strings.TrimSuffix(s, ".git")
	var path string
	switch {
	case strings.HasPrefix(s, "git@"):
		rest := strings.TrimPrefix(s, "git@")
		i := strings.Index(rest, ":")
		if i < 0 {
			return "", "", "", fmt.Errorf("unrecognised repo url %q", u)
		}
		host, path = rest[:i], rest[i+1:]
	case strings.Contains(s, "://"):
		rest := s[strings.Index(s, "://")+3:]
		if at := strings.Index(rest, "@"); at >= 0 && at < strings.Index(rest, "/") {
			rest = rest[at+1:]
		}
		i := strings.Index(rest, "/")
		if i < 0 {
			return "", "", "", fmt.Errorf("unrecognised repo url %q", u)
		}
		host, path = rest[:i], rest[i+1:]
	default:
		i := strings.Index(s, "/")
		if i < 0 {
			return "", "", "", fmt.Errorf("unrecognised repo url %q", u)
		}
		host, path = s[:i], s[i+1:]
	}
	parts := strings.Split(path, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" || host == "" {
		return "", "", "", fmt.Errorf("unrecognised repo url %q", u)
	}
	return host, parts[0], parts[1], nil
}

// ScrubToken removes the token (and its basic-auth encoding) from s.
func ScrubToken(s, token string) string {
	if token == "" {
		return s
	}
	enc := base64.StdEncoding.EncodeToString([]byte("x-access-token:" + token))
	s = strings.ReplaceAll(s, enc, "***")
	return strings.ReplaceAll(s, token, "***")
}

// ── git runner ───────────────────────────────────────────────────────────────

type gitErr struct {
	code int
	msg  string
}

func (e *gitErr) Error() string { return e.msg }

func (w *ws) git(ctx context.Context, args ...string) (string, error) {
	return w.gitIn(ctx, w.dir, args...)
}

func (w *ws) gitIn(ctx context.Context, dir string, args ...string) (string, error) {
	full := append([]string{"-c", "core.hooksPath=/dev/null", "-c", "core.fsmonitor=false"}, args...)
	cmd := exec.CommandContext(ctx, "git", full...)
	cmd.Dir = dir
	env := append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	if w.token != "" {
		hdr := "Authorization: Basic " + base64.StdEncoding.EncodeToString([]byte("x-access-token:"+w.token))
		env = append(env, "GIT_CONFIG_COUNT=1", "GIT_CONFIG_KEY_0=http.extraHeader", "GIT_CONFIG_VALUE_0="+hdr)
	}
	cmd.Env = env
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(w.scrubFn(stderr.String()))
		if msg == "" {
			msg = w.scrubFn(err.Error())
		}
		code := -1
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			code = ee.ExitCode()
		}
		return stdout.String(), &gitErr{code: code, msg: fmt.Sprintf("git %s: %s", args[0], msg)}
	}
	return stdout.String(), nil
}

func (w *ws) cloned() bool {
	_, err := os.Stat(filepath.Join(w.dir, ".git"))
	return err == nil
}

func (w *ws) requireCloned() error {
	if !w.cloned() {
		return errors.New("no workspace for this project: call code_checkout first")
	}
	return nil
}

func (w *ws) currentBranch(ctx context.Context) (string, error) {
	out, err := w.git(ctx, "rev-parse", "--abbrev-ref", "HEAD")
	return strings.TrimSpace(out), err
}

func (w *ws) defaultBranch(ctx context.Context) (string, error) {
	out, err := w.git(ctx, "symbolic-ref", "--short", "refs/remotes/origin/HEAD")
	if err == nil {
		return strings.TrimPrefix(strings.TrimSpace(out), "origin/"), nil
	}
	for _, b := range []string{"main", "master"} {
		if _, err := w.git(ctx, "rev-parse", "--verify", "-q", "refs/remotes/origin/"+b); err == nil {
			return b, nil
		}
	}
	return "", errors.New("cannot determine default branch")
}

func (w *ws) dirty(ctx context.Context) (bool, error) {
	out, err := w.git(ctx, "status", "--porcelain")
	return strings.TrimSpace(out) != "", err
}

// ── path safety ──────────────────────────────────────────────────────────────

// SafeRel validates a user-supplied relative path and returns it cleaned.
// An empty path means the repo root (".").
func SafeRel(p string) (string, error) {
	if strings.ContainsRune(p, 0) {
		return "", errors.New("invalid path")
	}
	if p == "" {
		return ".", nil
	}
	if filepath.IsAbs(p) || strings.HasPrefix(p, "/") || strings.HasPrefix(p, `\`) {
		return "", errors.New("path must be relative to the repository root")
	}
	clean := filepath.ToSlash(filepath.Clean(p))
	if clean == ".." || strings.HasPrefix(clean, "../") {
		return "", errors.New("path escapes the repository")
	}
	for _, part := range strings.Split(clean, "/") {
		if strings.EqualFold(part, ".git") {
			return "", errors.New("access to .git is not allowed")
		}
	}
	return clean, nil
}

// IsWorkflowPath reports whether a cleaned relative path is under .github/workflows.
func IsWorkflowPath(rel string) bool {
	l := strings.ToLower(rel)
	return l == ".github/workflows" || strings.HasPrefix(l, ".github/workflows/")
}

var errWorkflow = errors.New("the agent cannot modify .github/workflows (CI is a privilege-escalation surface and the GitHub token may lack the workflow scope); ask the user to edit it manually")

// resolvePath returns the absolute path for rel and verifies symlinks do not
// lead outside the workspace.
func (w *ws) resolvePath(rel string) (string, error) {
	abs := filepath.Join(w.dir, filepath.FromSlash(rel))
	realRoot, err := filepath.EvalSymlinks(w.dir)
	if err != nil {
		return "", err
	}
	// Resolve the deepest existing ancestor.
	probe := abs
	for {
		real, err := filepath.EvalSymlinks(probe)
		if err == nil {
			if real != realRoot && !strings.HasPrefix(real, realRoot+string(filepath.Separator)) {
				return "", errors.New("path resolves outside the repository")
			}
			break
		}
		if !errors.Is(err, os.ErrNotExist) {
			return "", err
		}
		next := filepath.Dir(probe)
		if next == probe {
			break
		}
		probe = next
	}
	return abs, nil
}

func (w *ws) prepPath(p string, write bool) (rel, abs string, err error) {
	rel, err = SafeRel(p)
	if err != nil {
		return "", "", err
	}
	if write && IsWorkflowPath(rel) {
		return "", "", errWorkflow
	}
	abs, err = w.resolvePath(rel)
	return rel, abs, err
}

// ── tool plumbing ────────────────────────────────────────────────────────────

func str(params map[string]any, k string) string {
	s, _ := params[k].(string)
	return s
}

func intParam(params map[string]any, k string) int {
	switch v := params[k].(type) {
	case float64:
		return int(v)
	case int:
		return v
	case string:
		n, _ := strconv.Atoi(v)
		return n
	}
	return 0
}

func boolParam(params map[string]any, k string) bool {
	b, _ := params[k].(bool)
	return b
}

func prop(typ, desc string) map[string]any {
	return map[string]any{"type": typ, "description": desc}
}

func (c *CodeTools) tool(name, desc string, props map[string]any, required []string,
	fn func(ctx context.Context, w *ws, params map[string]any) (any, error)) *Tool {
	props["project"] = prop("string", "Project name or UUID")
	return &Tool{
		Name:        name,
		Description: desc,
		Schema:      map[string]any{"type": "object", "properties": props, "required": append([]string{"project"}, required...)},
		CallFn: func(ctx context.Context, params map[string]any) (any, error) {
			w, release, err := c.open(ctx, str(params, "project"))
			if err != nil {
				return nil, err
			}
			defer release()
			res, err := fn(ctx, w, params)
			if err != nil {
				return nil, errors.New(w.scrubFn(err.Error()))
			}
			return res, nil
		},
	}
}

// Tools returns all code tools.
func (c *CodeTools) Tools() []*Tool {
	return []*Tool{
		c.tool("code_checkout",
			"Prepare the project's source for editing: clones the repo if needed, fetches, and switches to a working branch (default bordo/change-<timestamp>). Call this first before any other code_* tool. Resumes the current working branch if one exists.",
			map[string]any{"branch": prop("string", "Working branch name (optional)")}, nil, c.checkout),
		c.tool("code_list_files",
			"List files in the project workspace (skips .git, node_modules, target, dist; max 500 entries).",
			map[string]any{"path": prop("string", "Subdirectory, relative to repo root (optional)")}, nil, c.listFiles),
		c.tool("code_read_file",
			"Read a text file (max ~200KB). offset is the 1-based first line; limit is a line count. Set numbered=true for line numbers.",
			map[string]any{
				"path":     prop("string", "File path relative to repo root"),
				"offset":   prop("integer", "1-based start line (optional)"),
				"limit":    prop("integer", "Max lines (optional)"),
				"numbered": prop("boolean", "Prefix lines with numbers"),
			}, []string{"path"}, c.readFile),
		c.tool("code_write_file",
			"Create or overwrite a file with the given content. Cannot write under .github/workflows.",
			map[string]any{
				"path":    prop("string", "File path relative to repo root"),
				"content": prop("string", "Full file content"),
			}, []string{"path", "content"}, c.writeFile),
		c.tool("code_edit_file",
			"Replace old_string with new_string in a file. old_string must match exactly once unless replace_all is true. Cannot edit under .github/workflows.",
			map[string]any{
				"path":        prop("string", "File path relative to repo root"),
				"old_string":  prop("string", "Exact text to replace"),
				"new_string":  prop("string", "Replacement text"),
				"replace_all": prop("boolean", "Replace every occurrence"),
			}, []string{"path", "old_string", "new_string"}, c.editFile),
		c.tool("code_search",
			"Search the workspace with an extended regex (git grep). Max 100 hits.",
			map[string]any{
				"pattern": prop("string", "Extended regular expression"),
				"path":    prop("string", "Limit to a subdirectory (optional)"),
			}, []string{"pattern"}, c.search),
		c.tool("code_diff",
			"Show uncommitted changes (files with additions/deletions plus a unified diff) on the working branch.",
			map[string]any{}, nil, c.diff),
		c.tool("code_commit_push",
			"Commit all changes and push the working branch to origin. Refuses on the default branch.",
			map[string]any{"message": prop("string", "Commit message")}, []string{"message"}, c.commitPush),
		c.tool("code_open_pr",
			"Open a GitHub pull request from the pushed working branch into the default branch.",
			map[string]any{
				"title": prop("string", "PR title"),
				"body":  prop("string", "PR description (optional)"),
			}, []string{"title"}, c.openPR),
		c.mergeTool(),
	}
}

// mergeTool merges a pull request. It changes the default branch (and so triggers builds), so it
// is gated: the user sees an approval card naming the PR and the agent waits for their decision.
func (c *CodeTools) mergeTool() *Tool {
	t := c.tool("code_merge_pr",
		"Merge an open pull request into the default branch. The user must approve first (an approval card appears and you wait for it). Squash by default. Merging pushes to the default branch, which starts a build; afterwards trigger a build if none started and wait for it.",
		map[string]any{
			"number": prop("integer", "Pull request number"),
			"method": map[string]any{"type": "string", "enum": []string{"squash", "merge", "rebase"}, "description": "Merge method (default squash)"},
		}, []string{"number"}, c.mergePR)
	t.ApprovalSummary = func(params map[string]any) string {
		method := str(params, "method")
		if method == "" {
			method = "squash"
		}
		return fmt.Sprintf("Merge PR #%d of %s (%s) into the default branch", intParam(params, "number"), str(params, "project"), method)
	}
	return t
}

func (c *CodeTools) mergePR(ctx context.Context, w *ws, params map[string]any) (any, error) {
	n := intParam(params, "number")
	if n <= 0 {
		return nil, errors.New("number is required")
	}
	method := str(params, "method")
	if method == "" {
		method = "squash"
	}
	if method != "squash" && method != "merge" && method != "rebase" {
		return nil, errors.New("method must be squash, merge or rebase")
	}
	prURL := fmt.Sprintf("%s/repos/%s/%s/pulls/%d", c.apiBase, w.owner, w.repo, n)
	var pr struct {
		State  string `json:"state"`
		Merged bool   `json:"merged"`
		Draft  bool   `json:"draft"`
		Title  string `json:"title"`
		Base   struct {
			Ref string `json:"ref"`
		} `json:"base"`
		Head struct {
			SHA string `json:"sha"`
		} `json:"head"`
	}
	if err := c.githubJSON(ctx, w.token, http.MethodGet, prURL, nil, &pr); err != nil {
		return nil, err
	}
	switch {
	case pr.Merged:
		return nil, fmt.Errorf("PR #%d is already merged", n)
	case pr.State != "open":
		return nil, fmt.Errorf("PR #%d is %s, not open", n, pr.State)
	case pr.Draft:
		return nil, fmt.Errorf("PR #%d is a draft; mark it ready for review first", n)
	}
	var out struct {
		SHA     string `json:"sha"`
		Merged  bool   `json:"merged"`
		Message string `json:"message"`
	}
	// Pinning sha makes GitHub refuse the merge if the branch moved after we looked at it.
	body := map[string]string{"merge_method": method, "sha": pr.Head.SHA}
	if err := c.githubJSON(ctx, w.token, http.MethodPut, prURL+"/merge", body, &out); err != nil {
		return nil, err
	}
	return map[string]any{"merged": out.Merged, "merge_commit": out.SHA, "pr": n, "title": pr.Title, "into": pr.Base.Ref, "method": method,
		"next": "a build starts from the push to the default branch; check build_list for it, or build_trigger if none appears"}, nil
}

// githubJSON performs an authenticated GitHub API call and decodes the JSON response.
func (c *CodeTools) githubJSON(ctx context.Context, token, method, url string, body, out any) error {
	var rdr io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rdr = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, url, rdr)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.github+json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := (&http.Client{Timeout: 30 * time.Second}).Do(req)
	if err != nil {
		return fmt.Errorf("github: %w", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode >= 300 {
		return fmt.Errorf("github %d: %s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}
	return json.Unmarshal(raw, out)
}

// ── tool implementations ─────────────────────────────────────────────────────

func (c *CodeTools) checkout(ctx context.Context, w *ws, params map[string]any) (any, error) {
	if err := os.MkdirAll(c.root, 0o755); err != nil {
		return nil, err
	}
	if !w.cloned() {
		url := c.remoteURL(w.host, w.owner, w.repo)
		if _, err := w.gitIn(ctx, c.root, "clone", url, w.dir); err != nil {
			_ = os.RemoveAll(w.dir)
			return nil, err
		}
	}
	if _, err := w.git(ctx, "fetch", "origin", "--prune"); err != nil {
		return nil, err
	}
	base, err := w.defaultBranch(ctx)
	if err != nil {
		// A freshly created repo has no commits and so no default branch to check out.
		return nil, errors.New("the repository has no commits yet, so there is nothing to check out. For a template project run build_trigger first: the first build pushes the template scaffold. Then call code_checkout again")
	}
	cur, _ := w.currentBranch(ctx)

	want := str(params, "branch")
	if want == "" {
		if cur != base && strings.HasPrefix(cur, "bordo/") {
			want = cur
		} else {
			want = fmt.Sprintf("bordo/change-%d", time.Now().Unix())
		}
	}
	if want == base {
		return nil, fmt.Errorf("refusing to work directly on the default branch %q", base)
	}
	if _, err := w.git(ctx, "check-ref-format", "--branch", want); err != nil {
		return nil, fmt.Errorf("invalid branch name %q", want)
	}
	if cur != want {
		dirty, err := w.dirty(ctx)
		if err != nil {
			return nil, err
		}
		if dirty {
			return nil, fmt.Errorf("workspace has uncommitted changes on %q; commit/push them or continue on that branch", cur)
		}
		switch {
		case w.refExists(ctx, "refs/heads/"+want):
			_, err = w.git(ctx, "checkout", want)
		case w.refExists(ctx, "refs/remotes/origin/"+want):
			_, err = w.git(ctx, "checkout", "-B", want, "origin/"+want)
		default:
			_, err = w.git(ctx, "checkout", "-B", want, "origin/"+base)
		}
		if err != nil {
			return nil, err
		}
	}
	files, _ := w.git(ctx, "ls-files")
	n := 0
	if t := strings.TrimSpace(files); t != "" {
		n = strings.Count(t, "\n") + 1
	}
	top := []string{}
	if entries, err := os.ReadDir(w.dir); err == nil {
		for _, e := range entries {
			if e.Name() == ".git" {
				continue
			}
			name := e.Name()
			if e.IsDir() {
				name += "/"
			}
			top = append(top, name)
		}
	}
	return map[string]any{"branch": want, "base": base, "path_count": n, "top_level": top}, nil
}

func (w *ws) refExists(ctx context.Context, ref string) bool {
	_, err := w.git(ctx, "rev-parse", "--verify", "-q", ref)
	return err == nil
}

func (c *CodeTools) listFiles(ctx context.Context, w *ws, params map[string]any) (any, error) {
	if err := w.requireCloned(); err != nil {
		return nil, err
	}
	rel, abs, err := w.prepPath(str(params, "path"), false)
	if err != nil {
		return nil, err
	}
	var out []string
	truncated := false
	walkErr := filepath.WalkDir(abs, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if p == abs {
			return nil
		}
		if d.IsDir() && skipDirs[d.Name()] {
			return filepath.SkipDir
		}
		r, _ := filepath.Rel(w.dir, p)
		r = filepath.ToSlash(r)
		if d.IsDir() {
			r += "/"
		}
		if len(out) >= maxListEntries {
			truncated = true
			return filepath.SkipAll
		}
		out = append(out, r)
		return nil
	})
	if walkErr != nil {
		return nil, walkErr
	}
	sort.Strings(out)
	return map[string]any{"path": rel, "files": out, "truncated": truncated}, nil
}

func (c *CodeTools) readFile(ctx context.Context, w *ws, params map[string]any) (any, error) {
	if err := w.requireCloned(); err != nil {
		return nil, err
	}
	rel, abs, err := w.prepPath(str(params, "path"), false)
	if err != nil {
		return nil, err
	}
	st, err := os.Stat(abs)
	if err != nil {
		return nil, fmt.Errorf("cannot read %s", rel)
	}
	if st.IsDir() {
		return nil, fmt.Errorf("%s is a directory; use code_list_files", rel)
	}
	if st.Size() > maxFileBytes {
		return nil, fmt.Errorf("%s is too large (%d bytes)", rel, st.Size())
	}
	data, err := os.ReadFile(abs)
	if err != nil {
		return nil, err
	}
	head := data
	if len(head) > 8000 {
		head = head[:8000]
	}
	if bytes.IndexByte(head, 0) >= 0 {
		return nil, fmt.Errorf("%s looks like a binary file", rel)
	}
	lines := strings.Split(string(data), "\n")
	total := len(lines)
	start := intParam(params, "offset")
	if start < 1 {
		start = 1
	}
	if start > total {
		return map[string]any{"path": rel, "content": "", "start_line": start, "total_lines": total}, nil
	}
	end := total
	if lim := intParam(params, "limit"); lim > 0 && start-1+lim < end {
		end = start - 1 + lim
	}
	sel := lines[start-1 : end]
	if boolParam(params, "numbered") {
		for i := range sel {
			sel[i] = fmt.Sprintf("%d\t%s", start+i, sel[i])
		}
	}
	content := strings.Join(sel, "\n")
	truncated := false
	if len(content) > maxReadBytes {
		content = content[:maxReadBytes]
		truncated = true
	}
	return map[string]any{"path": rel, "content": content, "start_line": start, "total_lines": total, "truncated": truncated}, nil
}

func (c *CodeTools) writeFile(ctx context.Context, w *ws, params map[string]any) (any, error) {
	if err := w.requireCloned(); err != nil {
		return nil, err
	}
	rel, abs, err := w.prepPath(str(params, "path"), true)
	if err != nil {
		return nil, err
	}
	if rel == "." {
		return nil, errors.New("path is required")
	}
	content, ok := params["content"].(string)
	if !ok {
		return nil, errors.New("content is required")
	}
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		return nil, err
	}
	// Re-check after creating directories (a symlinked parent could not have existed before).
	if _, err := w.resolvePath(rel); err != nil {
		return nil, err
	}
	if err := os.WriteFile(abs, []byte(content), 0o644); err != nil {
		return nil, err
	}
	return map[string]any{"path": rel, "bytes": len(content)}, nil
}

// ReplaceUnique replaces old with new in s. Unless all is set, old must match exactly once.
func ReplaceUnique(s, old, new string, all bool) (string, int, error) {
	if old == "" {
		return "", 0, errors.New("old_string must not be empty")
	}
	if old == new {
		return "", 0, errors.New("old_string and new_string are identical")
	}
	n := strings.Count(s, old)
	switch {
	case n == 0:
		return "", 0, errors.New("old_string not found")
	case n > 1 && !all:
		return "", 0, fmt.Errorf("old_string matches %d times; add context to make it unique or set replace_all", n)
	}
	if all {
		return strings.ReplaceAll(s, old, new), n, nil
	}
	return strings.Replace(s, old, new, 1), 1, nil
}

func (c *CodeTools) editFile(ctx context.Context, w *ws, params map[string]any) (any, error) {
	if err := w.requireCloned(); err != nil {
		return nil, err
	}
	rel, abs, err := w.prepPath(str(params, "path"), true)
	if err != nil {
		return nil, err
	}
	st, err := os.Stat(abs)
	if err != nil {
		return nil, fmt.Errorf("%s does not exist", rel)
	}
	if st.IsDir() || st.Size() > maxFileBytes {
		return nil, fmt.Errorf("%s is not an editable file", rel)
	}
	data, err := os.ReadFile(abs)
	if err != nil {
		return nil, err
	}
	out, n, err := ReplaceUnique(string(data), str(params, "old_string"), str(params, "new_string"), boolParam(params, "replace_all"))
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(abs, []byte(out), st.Mode().Perm()); err != nil {
		return nil, err
	}
	return map[string]any{"path": rel, "replacements": n}, nil
}

func (c *CodeTools) search(ctx context.Context, w *ws, params map[string]any) (any, error) {
	if err := w.requireCloned(); err != nil {
		return nil, err
	}
	pat := str(params, "pattern")
	if pat == "" {
		return nil, errors.New("pattern is required")
	}
	rel, _, err := w.prepPath(str(params, "path"), false)
	if err != nil {
		return nil, err
	}
	out, err := w.git(ctx, "grep", "-n", "-I", "-E", "--untracked", "-e", pat, "--", rel)
	if err != nil {
		var ge *gitErr
		if errors.As(err, &ge) && ge.code == 1 && strings.TrimSpace(out) == "" {
			// git grep exits 1 when nothing matches.
			return map[string]any{"matches": []string{}, "truncated": false}, nil
		}
		return nil, err
	}
	hits := strings.Split(strings.TrimRight(out, "\n"), "\n")
	truncated := false
	if len(hits) > maxSearchHits {
		hits, truncated = hits[:maxSearchHits], true
	}
	return map[string]any{"matches": hits, "truncated": truncated}, nil
}

func (c *CodeTools) diff(ctx context.Context, w *ws, params map[string]any) (any, error) {
	if err := w.requireCloned(); err != nil {
		return nil, err
	}
	branch, _ := w.currentBranch(ctx)
	base, _ := w.defaultBranch(ctx)
	// Intent-to-add so untracked files show up in the diff.
	if _, err := w.git(ctx, "add", "-N", "."); err != nil {
		return nil, err
	}
	status, err := w.git(ctx, "diff", "HEAD", "--no-renames", "--name-status")
	if err != nil {
		return nil, err
	}
	numstat, err := w.git(ctx, "diff", "HEAD", "--no-renames", "--numstat")
	if err != nil {
		return nil, err
	}
	diff, err := w.git(ctx, "diff", "HEAD", "--no-renames")
	if err != nil {
		return nil, err
	}
	files := ParseDiffFiles(status, numstat)
	truncated := false
	if len(diff) > maxDiffBytes {
		diff, truncated = diff[:maxDiffBytes]+"\n… diff truncated …\n", true
	}
	return map[string]any{"branch": branch, "base": base, "files": files, "diff": diff, "truncated": truncated}, nil
}

// DiffFile is one changed file in code_diff output.
type DiffFile struct {
	Path      string `json:"path"`
	Status    string `json:"status"`
	Additions int    `json:"additions"`
	Deletions int    `json:"deletions"`
}

// ParseDiffFiles merges `git diff --name-status` and `--numstat` output.
func ParseDiffFiles(nameStatus, numstat string) []DiffFile {
	files := []DiffFile{}
	idx := map[string]int{}
	for _, l := range strings.Split(strings.TrimSpace(nameStatus), "\n") {
		f := strings.SplitN(l, "\t", 2)
		if len(f) != 2 {
			continue
		}
		st := map[string]string{"A": "added", "M": "modified", "D": "deleted"}[f[0][:1]]
		if st == "" {
			st = "modified"
		}
		idx[f[1]] = len(files)
		files = append(files, DiffFile{Path: f[1], Status: st})
	}
	for _, l := range strings.Split(strings.TrimSpace(numstat), "\n") {
		f := strings.SplitN(l, "\t", 3)
		if len(f) != 3 {
			continue
		}
		if i, ok := idx[f[2]]; ok {
			files[i].Additions, _ = strconv.Atoi(f[0])
			files[i].Deletions, _ = strconv.Atoi(f[1])
		}
	}
	return files
}

func (c *CodeTools) commitPush(ctx context.Context, w *ws, params map[string]any) (any, error) {
	if err := w.requireCloned(); err != nil {
		return nil, err
	}
	msg := strings.TrimSpace(str(params, "message"))
	if msg == "" {
		return nil, errors.New("message is required")
	}
	branch, err := w.currentBranch(ctx)
	if err != nil {
		return nil, err
	}
	base, err := w.defaultBranch(ctx)
	if err != nil {
		return nil, err
	}
	if branch == base || branch == "HEAD" {
		return nil, fmt.Errorf("refusing to push from %q; run code_checkout to create a working branch", branch)
	}
	if _, err := w.git(ctx, "add", "-A"); err != nil {
		return nil, err
	}
	names, err := w.git(ctx, "diff", "--cached", "--name-only")
	if err != nil {
		return nil, err
	}
	for _, n := range strings.Split(strings.TrimSpace(names), "\n") {
		if n != "" && IsWorkflowPath(strings.ToLower(n)) {
			_, _ = w.git(ctx, "reset", "-q")
			return nil, errWorkflow
		}
	}
	if strings.TrimSpace(names) == "" {
		return nil, errors.New("nothing to commit")
	}
	if _, err := w.git(ctx, "-c", "user.name=Bordo Agent", "-c", "user.email=agent@bordo.local", "commit", "-m", msg); err != nil {
		return nil, err
	}
	sha, err := w.git(ctx, "rev-parse", "HEAD")
	if err != nil {
		return nil, err
	}
	if _, err := w.git(ctx, "push", "-u", "origin", branch); err != nil {
		return nil, fmt.Errorf("committed locally but push failed: %w", err)
	}
	cmp := ""
	if strings.Contains(w.host, "github") {
		cmp = fmt.Sprintf("https://%s/%s/%s/compare/%s...%s", w.host, w.owner, w.repo, base, branch)
	}
	return map[string]any{"branch": branch, "commit": strings.TrimSpace(sha), "pushed": true, "compare_url": cmp}, nil
}

func (c *CodeTools) openPR(ctx context.Context, w *ws, params map[string]any) (any, error) {
	if err := w.requireCloned(); err != nil {
		return nil, err
	}
	title := strings.TrimSpace(str(params, "title"))
	if title == "" {
		return nil, errors.New("title is required")
	}
	branch, err := w.currentBranch(ctx)
	if err != nil {
		return nil, err
	}
	base, err := w.defaultBranch(ctx)
	if err != nil {
		return nil, err
	}
	if branch == base {
		return nil, errors.New("current branch is the default branch; nothing to open a PR from")
	}
	body, _ := json.Marshal(map[string]string{"title": title, "head": branch, "base": base, "body": str(params, "body")})
	url := fmt.Sprintf("%s/repos/%s/%s/pulls", c.apiBase, w.owner, w.repo)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+w.token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("Content-Type", "application/json")
	resp, err := (&http.Client{Timeout: 30 * time.Second}).Do(req)
	if err != nil {
		return nil, fmt.Errorf("github: %w", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("github %d: %s (is the branch pushed? run code_commit_push first)", resp.StatusCode, strings.TrimSpace(string(raw)))
	}
	var out struct {
		HTMLURL string `json:"html_url"`
		Number  int    `json:"number"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("decode github response: %w", err)
	}
	return map[string]any{"url": out.HTMLURL, "number": out.Number}, nil
}
