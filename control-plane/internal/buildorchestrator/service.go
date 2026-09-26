// Package buildorchestrator implements the control-plane build service.
package buildorchestrator

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Build is a build record in the control-plane DB.
type Build struct {
	ID           string
	ProjectID    string
	Status       string
	ImageName    string
	ImageTag     string
	ImageDigest  string
	ErrorMessage string
	StartedAt    *time.Time
	CompletedAt  *time.Time
	CreatedAt    time.Time
}

// Store manages build records.
type Store struct {
	db *sql.DB
}

// New creates a build Store.
func New(db *sql.DB) *Store {
	return &Store{db: db}
}

// Create inserts a new queued build record.
func (s *Store) Create(ctx context.Context, projectID, imageName, imageTag string) (*Build, error) {
	b := &Build{
		ID:        uuid.NewString(),
		ProjectID: projectID,
		Status:    "queued",
		ImageName: imageName,
		ImageTag:  imageTag,
		CreatedAt: time.Now().UTC(),
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO builds (id, project_id, status, image_name, image_tag, created_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		b.ID, b.ProjectID, b.Status, b.ImageName, b.ImageTag, b.CreatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("create build: %w", err)
	}
	return b, nil
}

// UpdateStatus sets the build status and optional completion fields.
func (s *Store) UpdateStatus(ctx context.Context, id, status, imageDigest, errMsg string) error {
	now := time.Now().UTC()
	_, err := s.db.ExecContext(ctx,
		`UPDATE builds SET status=?, image_digest=?, error_message=?, completed_at=? WHERE id=?`,
		status, imageDigest, errMsg, now, id,
	)
	return err
}

// SetRunning marks the build as running with a start time.
func (s *Store) SetRunning(ctx context.Context, id string) error {
	now := time.Now().UTC()
	_, err := s.db.ExecContext(ctx,
		`UPDATE builds SET status='running', started_at=? WHERE id=?`, now, id,
	)
	return err
}

// Get returns a build by ID.
func (s *Store) Get(ctx context.Context, id string) (*Build, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, project_id, status, image_name, image_tag, image_digest, error_message,
		        started_at, completed_at, created_at
		 FROM builds WHERE id = ?`, id)
	return scanBuild(row)
}

// ListByProject returns all builds for a project, newest first.
func (s *Store) ListByProject(ctx context.Context, projectID string) ([]*Build, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, project_id, status, image_name, image_tag, image_digest, error_message,
		        started_at, completed_at, created_at
		 FROM builds WHERE project_id = ? ORDER BY created_at DESC`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var builds []*Build
	for rows.Next() {
		b, err := scanBuildRow(rows)
		if err != nil {
			return nil, err
		}
		builds = append(builds, b)
	}
	return builds, rows.Err()
}

// AppendLog appends a log line to the build.
func (s *Store) AppendLog(ctx context.Context, buildID, line string) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO build_logs (build_id, line) VALUES (?, ?)`, buildID, line,
	)
	return err
}

// GetLogs returns all log lines for a build.
func (s *Store) GetLogs(ctx context.Context, buildID string) ([]string, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT line FROM build_logs WHERE build_id = ? ORDER BY id`, buildID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var lines []string
	for rows.Next() {
		var line string
		if err := rows.Scan(&line); err != nil {
			return nil, err
		}
		lines = append(lines, line)
	}
	return lines, rows.Err()
}

func scanBuild(row *sql.Row) (*Build, error) {
	b := &Build{}
	var startedAt, completedAt sql.NullTime
	err := row.Scan(&b.ID, &b.ProjectID, &b.Status, &b.ImageName, &b.ImageTag,
		&b.ImageDigest, &b.ErrorMessage, &startedAt, &completedAt, &b.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("build not found")
	}
	if err != nil {
		return nil, err
	}
	if startedAt.Valid {
		b.StartedAt = &startedAt.Time
	}
	if completedAt.Valid {
		b.CompletedAt = &completedAt.Time
	}
	return b, nil
}

func scanBuildRow(rows *sql.Rows) (*Build, error) {
	b := &Build{}
	var startedAt, completedAt sql.NullTime
	err := rows.Scan(&b.ID, &b.ProjectID, &b.Status, &b.ImageName, &b.ImageTag,
		&b.ImageDigest, &b.ErrorMessage, &startedAt, &completedAt, &b.CreatedAt)
	if err != nil {
		return nil, err
	}
	if startedAt.Valid {
		b.StartedAt = &startedAt.Time
	}
	if completedAt.Valid {
		b.CompletedAt = &completedAt.Time
	}
	return b, nil
}

// ImageRef returns the full image reference for pushing.
func (b *Build) ImageRef(registryURL string) string {
	ref := b.ImageName + ":" + b.ImageTag
	if registryURL != "" && !strings.HasPrefix(b.ImageName, registryURL) {
		ref = registryURL + "/" + ref
	}
	return ref
}
