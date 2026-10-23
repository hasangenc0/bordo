package buildorchestrator

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/hasangenc0/bordo/build/builder"
	"github.com/hasangenc0/bordo/build/template"
	githubpkg "github.com/hasangenc0/bordo/control-plane/internal/github"
)

// noBuilderMessage is the actionable error surfaced when no local container
// builder is available. On standard installs the bordod host has no docker CLI
// by design — builds are meant to run in GitHub Actions.
const noBuilderMessage = "No container builder available. Bordo builds images in GitHub Actions — connect GitHub with 'bordo github setup', then re-trigger the build. (Local Docker builds require the docker CLI on the bordod host, which is intentionally absent on standard installs.)"

// dockerAvailable reports whether a local container builder CLI is on PATH.
// It mirrors build/internal/container's binary resolution: prefer "docker",
// fall back to "nerdctl" (Rancher Desktop).
func dockerAvailable() bool {
	if _, err := exec.LookPath("docker"); err == nil {
		return true
	}
	if _, err := exec.LookPath("nerdctl"); err == nil {
		return true
	}
	return false
}

// Executor runs the full build pipeline: scaffold → build → log.
type Executor struct {
	store        *Store
	logger       *slog.Logger
	workDir      string
	templateRoot string
	registryURL  string
	db           *sql.DB
	ghStore      *githubpkg.Store // nil if GitHub App not configured
}

// NewExecutor creates an Executor. templateRoot and registryURL are read from
// BORDO_TEMPLATE_ROOT and BORDO_REGISTRY env vars if the passed values are empty.
// ghStore may be nil; when non-nil the executor uses GitHub Actions for builds.
func NewExecutor(store *Store, db *sql.DB, logger *slog.Logger, workDir, templateRoot, registryURL string, ghStore *githubpkg.Store) *Executor {
	if templateRoot == "" {
		templateRoot = os.Getenv("BORDO_TEMPLATE_ROOT")
	}
	if registryURL == "" {
		registryURL = os.Getenv("BORDO_REGISTRY")
	}
	if workDir == "" {
		home, _ := os.UserHomeDir()
		workDir = filepath.Join(home, ".bordo", "builds")
	}
	return &Executor{
		store:        store,
		logger:       logger,
		workDir:      workDir,
		templateRoot: templateRoot,
		registryURL:  registryURL,
		db:           db,
		ghStore:      ghStore,
	}
}

// Execute runs the build pipeline for the given build ID asynchronously.
func (e *Executor) Execute(buildID string) {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
		defer cancel()
		if err := e.run(ctx, buildID); err != nil {
			e.logger.Error("build failed", "id", buildID, "err", err)
		}
	}()
}

func (e *Executor) run(ctx context.Context, buildID string) error {
	b, err := e.store.Get(ctx, buildID)
	if err != nil {
		return fmt.Errorf("get build %s: %w", buildID, err)
	}
	if err := e.store.SetRunning(ctx, buildID); err != nil {
		return fmt.Errorf("set running: %w", err)
	}
	_ = e.store.AppendLog(ctx, buildID, "[bordo] build started")

	var known int
	if err := e.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM projects WHERE id = ?`, b.ProjectID).Scan(&known); err != nil || known == 0 {
		msg := fmt.Sprintf("project %q does not exist (deleted or wrong id); nothing to build", b.ProjectID)
		_ = e.store.AppendLog(ctx, buildID, "[error] "+msg)
		return e.store.UpdateStatus(ctx, buildID, "failed", "", msg)
	}

	// A project that points at an existing repo and has no template is imported: its
	// code already lives on GitHub. It must never be scaffolded into or pushed over.
	if repoURL := e.resolveRepoURL(ctx, b.ProjectID); repoURL != "" && e.resolveTemplate(ctx, b.ProjectID) == "" {
		return e.buildImported(ctx, buildID, repoURL)
	}

	// A template project whose repo already has code (a previous build pushed the scaffold,
	// or the agent has since changed it) builds from that repo's default branch, read-only.
	// Re-scaffolding would force-push the template over those changes.
	if repoURL := e.resolveRepoURL(ctx, b.ProjectID); repoURL != "" && e.repoHasBuildableCode(ctx, buildID, repoURL) {
		return e.buildImported(ctx, buildID, repoURL)
	}

	scaffoldDir := filepath.Join(e.workDir, buildID, "src")

	// Step 1: scaffold from template.
	if e.templateRoot != "" {
		templateName := e.resolveTemplate(ctx, b.ProjectID)
		if templateName != "" {
			_ = e.store.AppendLog(ctx, buildID, fmt.Sprintf("[bordo] scaffolding %q", templateName))
			eng := template.NewEngine(e.templateRoot)
			vars := template.Vars{
				ProjectName:  b.ImageName,
				GroupId:      "io.bordo.apps",
				BordoVersion: "0.1.0",
			}
			if err := eng.Expand(templateName, vars, scaffoldDir); err != nil {
				_ = e.store.AppendLog(ctx, buildID, fmt.Sprintf("[bordo] scaffold failed: %v", err))
			} else {
				_ = e.store.AppendLog(ctx, buildID, "[bordo] scaffold complete")
			}
		}
	}

	if _, statErr := os.Stat(filepath.Join(scaffoldDir, "Dockerfile")); os.IsNotExist(statErr) {
		// "Success" with no image made later deploys ship an empty tag, so this is a failure.
		msg := "nothing to build: the project has no template or repository that provides a Dockerfile"
		_ = e.store.AppendLog(ctx, buildID, "[error] "+msg)
		return e.store.UpdateStatus(ctx, buildID, "failed", "", msg)
	}

	// Step 2: build via GitHub Actions when a GitHub token is configured.
	// Create the repo on first build if the project doesn't have one yet.
	if e.ghStore != nil {
		repoURL := e.resolveRepoURL(ctx, b.ProjectID)
		if repoURL == "" {
			repoURL = e.ensureRepo(ctx, buildID, b)
		}
		if repoURL != "" {
			return e.buildViaGitHubActions(ctx, buildID, b, scaffoldDir, repoURL)
		}
	}

	// Fallback: local Docker build (fails loudly if no builder is present).
	return e.buildLocally(ctx, buildID, b, scaffoldDir)
}

// ensureRepo creates a GitHub repo for the project when none is set yet, using
// the configured GitHub token (device-flow/PAT user token, or App installation).
// Returns the repo URL, or "" if no token is configured or creation failed.
func (e *Executor) ensureRepo(ctx context.Context, buildID string, b *Build) string {
	token, err := e.githubToken(ctx)
	if err != nil || token == "" {
		_ = e.store.AppendLog(ctx, buildID, "[error] "+errGitHubNotConnected)
		return ""
	}
	_ = e.store.AppendLog(ctx, buildID, fmt.Sprintf("[bordo] creating GitHub repo %q", b.ImageName))
	cloneURL, err := githubpkg.CreateRepoForUser(ctx, token, b.ImageName, true)
	if err != nil {
		_ = e.store.AppendLog(ctx, buildID, fmt.Sprintf("[error] creating repo: %v", err))
		return ""
	}
	if _, derr := e.db.ExecContext(ctx, `UPDATE projects SET git_repo_url = ? WHERE id = ?`, cloneURL, b.ProjectID); derr != nil {
		_ = e.store.AppendLog(ctx, buildID, fmt.Sprintf("[warn] could not persist repo URL: %v", derr))
	}
	_ = e.store.AppendLog(ctx, buildID, "[bordo] repo created: "+cloneURL)
	return cloneURL
}

// repoHasBuildableCode reports whether repoURL already has a commit with a root Dockerfile on its
// default branch (whatever that is called). For such repos Bordo adds its build workflow if it
// is missing, since for a project Bordo created it owns that file. Any doubt returns false and
// the caller falls back to scaffolding into an empty repo.
func (e *Executor) repoHasBuildableCode(ctx context.Context, buildID, repoURL string) bool {
	if e.ghStore == nil {
		return false
	}
	owner, name, err := githubpkg.ParseRepoURL(repoURL)
	if err != nil {
		return false
	}
	token, err := e.githubToken(ctx)
	if err != nil || token == "" {
		return false
	}
	branch, sha, err := githubpkg.RepoHead(ctx, token, owner, name)
	if err != nil || sha == "" {
		return false // empty repo: nothing to preserve
	}
	if ok, err := githubpkg.RepoHasFile(ctx, token, owner, name, "Dockerfile", sha); err != nil || !ok {
		return false
	}
	_ = e.store.AppendLog(ctx, buildID, fmt.Sprintf("[bordo] %s/%s already has code on %q — building it as is (not re-scaffolding)", owner, name, branch))
	if ok, err := githubpkg.RepoHasFile(ctx, token, owner, name, workflowPath, sha); err == nil && !ok {
		if err := githubpkg.EnsureWorkflowFile(ctx, token, owner, name); err != nil {
			_ = e.store.AppendLog(ctx, buildID, fmt.Sprintf("[warn] could not add the build workflow: %v", err))
		}
	}
	return true
}

const errGitHubNotConnected = "GitHub is not connected, so no repository can be created. Run 'bordo github login' on a machine that can reach bordod, then try again."

// CreateProjectRepo creates the private GitHub repo for a template project without
// waiting for its first build, and links it to the project. It is idempotent: a
// project that already has a repo returns it with created=false. Imported projects
// already point at a repo, so they never reach the create call.
func (e *Executor) CreateProjectRepo(ctx context.Context, projectID string) (repoURL string, created bool, err error) {
	var name string
	if err := e.db.QueryRowContext(ctx, `SELECT name FROM projects WHERE id = ?`, projectID).Scan(&name); err != nil {
		return "", false, errors.New("project not found")
	}
	if existing := e.resolveRepoURL(ctx, projectID); existing != "" {
		return existing, false, nil
	}
	if e.ghStore == nil {
		return "", false, errors.New(errGitHubNotConnected)
	}
	token, terr := e.githubToken(ctx)
	if terr != nil || token == "" {
		return "", false, errors.New(errGitHubNotConnected)
	}
	cloneURL, err := githubpkg.CreateRepoForUser(ctx, token, name, true)
	if err != nil {
		return "", false, fmt.Errorf("creating GitHub repo %q: %w", name, err)
	}
	if _, err := e.db.ExecContext(ctx, `UPDATE projects SET git_repo_url = ? WHERE id = ?`, cloneURL, projectID); err != nil {
		return "", false, fmt.Errorf("repo %s was created but could not be linked to the project: %w", cloneURL, err)
	}
	return cloneURL, true, nil
}

// buildViaGitHubActions pushes the scaffold to GitHub and waits for the workflow.
func (e *Executor) buildViaGitHubActions(ctx context.Context, buildID string, b *Build, scaffoldDir, repoURL string) error {
	owner, repo, err := githubpkg.ParseRepoURL(repoURL)
	if err != nil {
		_ = e.store.AppendLog(ctx, buildID, fmt.Sprintf("[error] bad repo URL %q: %v", repoURL, err))
		if !dockerAvailable() {
			return e.failNoBuilder(ctx, buildID)
		}
		return e.buildLocally(ctx, buildID, b, scaffoldDir)
	}

	token, err := e.githubToken(ctx)
	if err != nil || token == "" {
		if !dockerAvailable() {
			_ = e.store.AppendLog(ctx, buildID, fmt.Sprintf("[error] no GitHub token: %v", err))
			return e.failNoBuilder(ctx, buildID)
		}
		_ = e.store.AppendLog(ctx, buildID, fmt.Sprintf("[bordo] no GitHub token — falling back to local build: %v", err))
		return e.buildLocally(ctx, buildID, b, scaffoldDir)
	}

	_ = e.store.AppendLog(ctx, buildID, fmt.Sprintf("[bordo] pushing scaffold to github.com/%s/%s", owner, repo))
	commitSHA, err := githubpkg.PushScaffoldToRepo(ctx, token, owner, repo, scaffoldDir)
	if err != nil {
		_ = e.store.AppendLog(ctx, buildID, fmt.Sprintf("[error] git push failed: %v", err))
		return e.store.UpdateStatus(ctx, buildID, "failed", "", err.Error())
	}
	_ = e.store.AppendLog(ctx, buildID, fmt.Sprintf("[bordo] pushed commit %s — waiting for GitHub Actions", commitSHA[:8]))

	run, err := githubpkg.WaitForWorkflowRun(ctx, token, owner, repo, commitSHA)
	if err != nil {
		_ = e.store.AppendLog(ctx, buildID, fmt.Sprintf("[error] waiting for workflow: %v", err))
		return e.store.UpdateStatus(ctx, buildID, "failed", "", err.Error())
	}

	imageRef := fmt.Sprintf("ghcr.io/%s/%s:%s", owner, repo, commitSHA)
	_ = e.store.AppendLog(ctx, buildID, fmt.Sprintf("[bordo] workflow %s → %s", run.HTMLURL, run.Conclusion))

	if run.Conclusion != "success" {
		msg := fmt.Sprintf("GitHub Actions workflow %s", run.Conclusion)
		return e.store.UpdateStatus(ctx, buildID, "failed", imageRef, msg)
	}
	_ = e.store.AppendLog(ctx, buildID, fmt.Sprintf("[bordo] image available: %s", imageRef))
	return e.store.UpdateStatus(ctx, buildID, "success", imageRef, "")
}

// failNoBuilder marks the build failed with the actionable "no builder"
// message and logs it, instead of attempting a doomed local docker build.
func (e *Executor) failNoBuilder(ctx context.Context, buildID string) error {
	_ = e.store.AppendLog(ctx, buildID, "[error] "+noBuilderMessage)
	return e.store.UpdateStatus(ctx, buildID, "failed", "", noBuilderMessage)
}

// buildLocally runs a Docker build on the local daemon (requires Docker socket).
func (e *Executor) buildLocally(ctx context.Context, buildID string, b *Build, scaffoldDir string) error {
	if !dockerAvailable() {
		return e.failNoBuilder(ctx, buildID)
	}
	_ = e.store.AppendLog(ctx, buildID, "[bordo] starting local docker build")
	bldr := builder.NewLocalBuilder()
	logCh, err := bldr.Build(ctx, builder.BuildOptions{
		ProjectID:   b.ProjectID,
		ContextPath: scaffoldDir,
		ImageName:   b.ImageName,
		Tag:         b.ImageTag,
		RegistryURL: e.registryURL,
	})
	if err != nil {
		msg := fmt.Sprintf("docker build failed to start: %v", err)
		_ = e.store.AppendLog(ctx, buildID, "[error] "+msg)
		return e.store.UpdateStatus(ctx, buildID, "failed", "", msg)
	}

	var lastLine string
	for line := range logCh {
		_ = e.store.AppendLog(ctx, buildID, line.Text)
		lastLine = line.Text
	}

	if len(lastLine) >= 7 && lastLine[:7] == "[error]" {
		return e.store.UpdateStatus(ctx, buildID, "failed", "", lastLine)
	}
	imageRef := b.ImageRef(e.registryURL)
	_ = e.store.AppendLog(ctx, buildID, fmt.Sprintf("[bordo] build complete: %s", imageRef))
	return e.store.UpdateStatus(ctx, buildID, "success", imageRef, "")
}

// githubToken returns a fresh installation token for pushing to GitHub.
func (e *Executor) githubToken(ctx context.Context) (string, error) {
	// Prefer a user token (device-flow / PAT): it works without a GitHub App,
	// which is the standard path for self-hosted installs.
	if userToken, _, uerr := e.ghStore.LoadUserToken(ctx); uerr == nil && userToken != "" {
		return userToken, nil
	}
	creds, err := e.ghStore.LoadApp(ctx)
	if err != nil || creds == nil {
		return "", err
	}
	installID, _, _, err := e.ghStore.LoadInstallationID(ctx)
	if err != nil || installID == 0 {
		// Try user token as fallback.
		userToken, _, err2 := e.ghStore.LoadUserToken(ctx)
		return userToken, err2
	}
	jwtToken, err := githubpkg.GenerateJWT(creds.AppID, creds.PrivateKeyPEM)
	if err != nil {
		return "", err
	}
	return githubpkg.GetInstallationToken(jwtToken, installID)
}

// resolveTemplate looks up the project's template name from the DB.
func (e *Executor) resolveTemplate(ctx context.Context, projectID string) string {
	var templateName string
	row := e.db.QueryRowContext(ctx, `SELECT template FROM projects WHERE id = ?`, projectID)
	if err := row.Scan(&templateName); err != nil {
		return ""
	}
	return templateName
}

// resolveRepoURL looks up the project's git_repo_url from the DB.
func (e *Executor) resolveRepoURL(ctx context.Context, projectID string) string {
	var repoURL string
	row := e.db.QueryRowContext(ctx, `SELECT git_repo_url FROM projects WHERE id = ?`, projectID)
	if err := row.Scan(&repoURL); err != nil {
		return ""
	}
	return repoURL
}
