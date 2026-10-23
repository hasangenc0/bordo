package buildorchestrator

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	githubpkg "github.com/hasangenc0/bordo/control-plane/internal/github"
)

const workflowPath = ".github/workflows/" + githubpkg.BuildWorkflowFile

// Tunable for tests: how long to wait for GitHub to start a run on its own before
// dispatching the workflow, and how often to poll afterwards.
var (
	importedRunGrace = 45 * time.Second
	importedPoll     = 15 * time.Second
)

// buildImported builds a project whose code already lives on GitHub. It only reads
// the repo: it finds the latest commit on the default branch and waits for the
// image the repo's own Bordo workflow builds for it. It never pushes.
func (e *Executor) buildImported(ctx context.Context, buildID, repoURL string) error {
	fail := func(msg string) error {
		_ = e.store.AppendLog(ctx, buildID, "[error] "+msg)
		return e.store.UpdateStatus(ctx, buildID, "failed", "", msg)
	}

	owner, repo, err := githubpkg.ParseRepoURL(repoURL)
	if err != nil {
		return fail(fmt.Sprintf("bad repository URL %q: %v", repoURL, err))
	}
	if e.ghStore == nil {
		return fail("GitHub is not connected. Run 'bordo github login', then build again.")
	}
	token, err := e.githubToken(ctx)
	if err != nil || token == "" {
		return fail("GitHub is not connected. Run 'bordo github login', then build again.")
	}

	_ = e.store.AppendLog(ctx, buildID, fmt.Sprintf("[bordo] imported repo github.com/%s/%s — building the latest commit (read-only)", owner, repo))
	branch, sha, err := githubpkg.RepoHead(ctx, token, owner, repo)
	if err != nil {
		return fail(err.Error())
	}
	_ = e.store.AppendLog(ctx, buildID, fmt.Sprintf("[bordo] %s @ %s", branch, sha[:8]))

	hasDockerfile, err := githubpkg.RepoHasFile(ctx, token, owner, repo, "Dockerfile", sha)
	if err != nil {
		return fail(err.Error())
	}
	if !hasDockerfile {
		return fail(fmt.Sprintf("github.com/%s/%s has no Dockerfile at its root. Bordo builds a container image from the root Dockerfile; add one (the agent can do that for you) and build again.", owner, repo))
	}
	hasWorkflow, err := githubpkg.RepoHasFile(ctx, token, owner, repo, workflowPath, sha)
	if err != nil {
		return fail(err.Error())
	}
	if !hasWorkflow {
		return fail(fmt.Sprintf("github.com/%s/%s has no Bordo build workflow (%s). Ask the agent to \"enable builds\" for this project: that adds the workflow with one commit to %s. Bordo does not change your repository without being asked.", owner, repo, workflowPath, branch))
	}

	_ = e.store.AppendLog(ctx, buildID, "[bordo] waiting for GitHub Actions")
	run, err := githubpkg.AwaitRunForCommit(ctx, token, owner, repo, sha, branch, importedRunGrace, importedPoll,
		func(line string) { _ = e.store.AppendLog(ctx, buildID, "[bordo] "+line) })
	if err != nil {
		return fail(fmt.Sprintf("waiting for the build: %v", err))
	}
	// Container registries require lowercase names.
	imageRef := fmt.Sprintf("ghcr.io/%s/%s:%s", strings.ToLower(owner), strings.ToLower(repo), sha)
	_ = e.store.AppendLog(ctx, buildID, fmt.Sprintf("[bordo] workflow %s → %s", run.HTMLURL, run.Conclusion))
	if run.Conclusion != "success" {
		_ = e.store.AppendLog(ctx, buildID, "[error] the build failed; see the workflow run above")
		return e.store.UpdateStatus(ctx, buildID, "failed", imageRef, "GitHub Actions workflow "+run.Conclusion)
	}
	_ = e.store.AppendLog(ctx, buildID, "[bordo] image available: "+imageRef)
	return e.store.UpdateStatus(ctx, buildID, "success", imageRef, "")
}

// EnableBuilds adds the Bordo build workflow to an imported project's repo (one
// commit on the default branch). This changes the user's repository, so it only
// runs when explicitly requested.
func (e *Executor) EnableBuilds(ctx context.Context, projectID string) (repo, path string, err error) {
	repoURL := e.resolveRepoURL(ctx, projectID)
	if repoURL == "" {
		return "", "", errors.New("this project has no GitHub repository")
	}
	owner, name, err := githubpkg.ParseRepoURL(repoURL)
	if err != nil {
		return "", "", err
	}
	if e.ghStore == nil {
		return "", "", errors.New("GitHub is not connected; run 'bordo github login'")
	}
	token, err := e.githubToken(ctx)
	if err != nil || token == "" {
		return "", "", errors.New("GitHub is not connected; run 'bordo github login'")
	}
	if err := githubpkg.EnsureWorkflowFile(ctx, token, owner, name); err != nil {
		return "", "", fmt.Errorf("could not add the workflow to %s/%s: %w (the GitHub token needs write access to the repo and the 'workflow' scope)", owner, name, err)
	}
	return owner + "/" + name, workflowPath, nil
}
