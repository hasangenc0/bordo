package buildorchestrator

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/bordo-io/bordo/build/builder"
	"github.com/bordo-io/bordo/build/template"
)

// Executor runs the full build pipeline: scaffold → docker build → log.
type Executor struct {
	store        *Store
	logger       *slog.Logger
	workDir      string // root dir for scaffold output; e.g. ~/.bordo/builds
	templateRoot string // root dir of golden-path templates; e.g. <repo>/templates
	registryURL  string // optional registry prefix for image refs
	db           *sql.DB
}

// NewExecutor creates an Executor. templateRoot and registryURL are read from
// BORDO_TEMPLATE_ROOT and BORDO_REGISTRY env vars if the passed values are empty.
func NewExecutor(store *Store, db *sql.DB, logger *slog.Logger, workDir, templateRoot, registryURL string) *Executor {
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
	}
}

// Execute runs the build pipeline for the given build ID asynchronously.
// It returns immediately; the build result is persisted to the DB.
func (e *Executor) Execute(buildID string) {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
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

	scaffoldDir := filepath.Join(e.workDir, buildID, "src")

	// Step 1: scaffold from template if a template root is configured.
	if e.templateRoot != "" {
		templateName := e.resolveTemplate(ctx, b.ProjectID)
		if templateName != "" {
			_ = e.store.AppendLog(ctx, buildID, fmt.Sprintf("[bordo] scaffolding template %q", templateName))
			eng := template.NewEngine(e.templateRoot)
			vars := template.Vars{
				ProjectName:  b.ImageName,
				GroupId:      "io.bordo.apps",
				BordoVersion: "0.1.0",
			}
			if expandErr := eng.Expand(templateName, vars, scaffoldDir); expandErr != nil {
				_ = e.store.AppendLog(ctx, buildID, fmt.Sprintf("[bordo] scaffold failed: %v", expandErr))
				// Not fatal: fall through and try to build whatever is in scaffoldDir.
			} else {
				_ = e.store.AppendLog(ctx, buildID, "[bordo] scaffold complete")
			}
		}
	}

	// If no scaffoldDir was created (no template root or scaffold failed), nothing to build.
	if _, statErr := os.Stat(filepath.Join(scaffoldDir, "Dockerfile")); os.IsNotExist(statErr) {
		_ = e.store.AppendLog(ctx, buildID, "[bordo] no Dockerfile found — skipping docker build (set BORDO_TEMPLATE_ROOT)")
		return e.store.UpdateStatus(ctx, buildID, "success", "", "")
	}

	// Step 2: docker buildx build.
	_ = e.store.AppendLog(ctx, buildID, "[bordo] starting docker build")
	bldr := builder.NewLocalBuilder()
	logCh, err := bldr.Build(ctx, builder.BuildOptions{
		ProjectID:   b.ProjectID,
		ContextPath: scaffoldDir,
		ImageName:   b.ImageName,
		Tag:         b.ImageTag,
		RegistryURL: e.registryURL,
	})
	if err != nil {
		msg := fmt.Sprintf("docker build start failed: %v", err)
		_ = e.store.AppendLog(ctx, buildID, "[error] "+msg)
		return e.store.UpdateStatus(ctx, buildID, "failed", "", msg)
	}

	var lastLine string
	for line := range logCh {
		_ = e.store.AppendLog(ctx, buildID, line.Text)
		lastLine = line.Text
	}

	// Detect failure from the last log line written by LocalBuilder.
	if len(lastLine) >= 7 && lastLine[:7] == "[error]" {
		return e.store.UpdateStatus(ctx, buildID, "failed", "", lastLine)
	}

	imageRef := b.ImageRef(e.registryURL)
	_ = e.store.AppendLog(ctx, buildID, fmt.Sprintf("[bordo] build complete: %s", imageRef))
	return e.store.UpdateStatus(ctx, buildID, "success", imageRef, "")
}

// resolveTemplate looks up the project's template name from the DB.
// Returns "" if unavailable or the table doesn't have the column yet.
func (e *Executor) resolveTemplate(ctx context.Context, projectID string) string {
	var templateName string
	row := e.db.QueryRowContext(ctx, `SELECT template FROM projects WHERE id = ?`, projectID)
	if err := row.Scan(&templateName); err != nil {
		return ""
	}
	return templateName
}
