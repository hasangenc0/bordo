package github

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// BuildWorkflowFile is the workflow Bordo expects in repos it builds.
const BuildWorkflowFile = "bordo-build.yml"

func ghDo(ctx context.Context, token, method, apiURL string, body any, out any) (int, error) {
	var rd *bytes.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return 0, err
		}
		rd = bytes.NewReader(b)
	} else {
		rd = bytes.NewReader(nil)
	}
	req, err := http.NewRequestWithContext(ctx, method, apiURL, rd)
	if err != nil {
		return 0, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := (&http.Client{Timeout: 20 * time.Second}).Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if out != nil && resp.StatusCode < 300 {
		if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
			return resp.StatusCode, err
		}
	}
	return resp.StatusCode, nil
}

// RepoHead returns the repo's default branch and the SHA of its latest commit.
func RepoHead(ctx context.Context, token, owner, repo string) (branch, sha string, err error) {
	var info struct {
		DefaultBranch string `json:"default_branch"`
	}
	code, err := ghDo(ctx, token, http.MethodGet, fmt.Sprintf("%s/repos/%s/%s", githubAPIBase, owner, repo), nil, &info)
	if err != nil {
		return "", "", err
	}
	if code == http.StatusNotFound {
		return "", "", fmt.Errorf("repository %s/%s not found, or the connected GitHub account cannot see it", owner, repo)
	}
	if code >= 300 || info.DefaultBranch == "" {
		return "", "", fmt.Errorf("reading %s/%s: HTTP %d", owner, repo, code)
	}
	var commit struct {
		SHA string `json:"sha"`
	}
	code, err = ghDo(ctx, token, http.MethodGet,
		fmt.Sprintf("%s/repos/%s/%s/commits/%s", githubAPIBase, owner, repo, url.PathEscape(info.DefaultBranch)), nil, &commit)
	if err != nil {
		return "", "", err
	}
	if code >= 300 || commit.SHA == "" {
		return "", "", fmt.Errorf("reading latest commit of %s/%s (%s): HTTP %d", owner, repo, info.DefaultBranch, code)
	}
	return info.DefaultBranch, commit.SHA, nil
}

// RepoHasFile reports whether path exists in the repo at ref.
func RepoHasFile(ctx context.Context, token, owner, repo, path, ref string) (bool, error) {
	u := fmt.Sprintf("%s/repos/%s/%s/contents/%s?ref=%s", githubAPIBase, owner, repo, path, url.QueryEscape(ref))
	code, err := ghDo(ctx, token, http.MethodGet, u, nil, nil)
	if err != nil {
		return false, err
	}
	switch {
	case code == http.StatusOK:
		return true, nil
	case code == http.StatusNotFound:
		return false, nil
	}
	return false, fmt.Errorf("checking %s in %s/%s: HTTP %d", path, owner, repo, code)
}

// DispatchWorkflow starts a workflow_dispatch run of file on ref.
func DispatchWorkflow(ctx context.Context, token, owner, repo, file, ref string) error {
	u := fmt.Sprintf("%s/repos/%s/%s/actions/workflows/%s/dispatches", githubAPIBase, owner, repo, file)
	code, err := ghDo(ctx, token, http.MethodPost, u, map[string]string{"ref": ref}, nil)
	if err != nil {
		return err
	}
	if code >= 300 {
		return fmt.Errorf("dispatching %s on %s: HTTP %d", file, ref, code)
	}
	return nil
}

// FindRun returns the newest workflow run for commit sha, or nil if there is none yet.
func FindRun(ctx context.Context, token, owner, repo, sha string) (*WorkflowRun, error) {
	var res struct {
		WorkflowRuns []WorkflowRun `json:"workflow_runs"`
	}
	// Scoped to the Bordo workflow: an unrelated workflow (CI, lint) sharing the commit must
	// not be mistaken for the image build.
	u := fmt.Sprintf("%s/repos/%s/%s/actions/workflows/%s/runs?head_sha=%s&per_page=1", githubAPIBase, owner, repo, BuildWorkflowFile, url.QueryEscape(sha))
	code, err := ghDo(ctx, token, http.MethodGet, u, nil, &res)
	if err != nil {
		return nil, err
	}
	if code >= 300 {
		return nil, fmt.Errorf("listing workflow runs: HTTP %d", code)
	}
	if len(res.WorkflowRuns) == 0 {
		return nil, nil
	}
	return &res.WorkflowRuns[0], nil
}

// AwaitRunForCommit waits for the Bordo build run of commit sha to finish. A push to
// the default branch normally starts it already; if no run shows up within grace
// (e.g. the default branch is not the one the workflow listens on) it dispatches the
// workflow on branch itself. logf, if set, receives progress lines.
func AwaitRunForCommit(ctx context.Context, token, owner, repo, sha, branch string, grace, poll time.Duration, logf func(string)) (*WorkflowRun, error) {
	if logf == nil {
		logf = func(string) {}
	}
	start := time.Now()
	lastLog := start
	dispatched := false
	for {
		run, err := FindRun(ctx, token, owner, repo, sha)
		if err != nil {
			return nil, err
		}
		if run != nil && run.Status == "completed" {
			return run, nil
		}
		if run == nil && !dispatched && time.Since(start) >= grace {
			logf(fmt.Sprintf("no build run for %s yet — starting %s on %s", sha[:8], BuildWorkflowFile, branch))
			if err := DispatchWorkflow(ctx, token, owner, repo, BuildWorkflowFile, branch); err != nil {
				return nil, err
			}
			dispatched = true
		}
		if time.Since(lastLog) >= time.Minute {
			lastLog = time.Now()
			if run == nil {
				logf(fmt.Sprintf("still waiting for a build run for %s (%s)", sha[:8], time.Since(start).Round(time.Second)))
			} else {
				logf(fmt.Sprintf("workflow run is %s (%s)", run.Status, time.Since(start).Round(time.Second)))
			}
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(poll):
		}
	}
}

// NormalizeRepoURL accepts https://github.com/o/r[.git] or git@github.com:o/r[.git]
// and returns the canonical https URL plus owner and repo. Anything else (other hosts,
// extra path segments, odd characters) is rejected.
func NormalizeRepoURL(raw string) (canonical, owner, repo string, err error) {
	s := strings.TrimSpace(raw)
	s = strings.TrimSuffix(strings.TrimSuffix(s, "/"), ".git")
	switch {
	case strings.HasPrefix(s, "https://github.com/"):
		s = strings.TrimPrefix(s, "https://github.com/")
	case strings.HasPrefix(s, "git@github.com:"):
		s = strings.TrimPrefix(s, "git@github.com:")
	default:
		return "", "", "", fmt.Errorf("only github.com repositories are supported (use https://github.com/<owner>/<repo>)")
	}
	parts := strings.Split(s, "/")
	if len(parts) != 2 || !validRepoPart(parts[0]) || !validRepoPart(parts[1]) {
		return "", "", "", fmt.Errorf("expected https://github.com/<owner>/<repo>, got %q", raw)
	}
	return "https://github.com/" + parts[0] + "/" + parts[1], parts[0], parts[1], nil
}

func validRepoPart(s string) bool {
	if s == "" || s == "." || s == ".." || len(s) > 100 {
		return false
	}
	for _, r := range s {
		ok := r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_' || r == '.'
		if !ok {
			return false
		}
	}
	return true
}
