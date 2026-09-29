package registry

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

type sqliteStore struct {
	db *sql.DB
}

// New returns a Store backed by the given *sql.DB.
func New(db *sql.DB) Store {
	return &sqliteStore{db: db}
}

func (s *sqliteStore) CreateProject(ctx context.Context, p *Project) error {
	p.ID = uuid.NewString()
	now := time.Now().UTC()
	p.CreatedAt = now
	p.UpdatedAt = now
	if p.Status == "" {
		p.Status = "pending"
	}

	_, err := s.db.ExecContext(ctx,
		`INSERT INTO projects (id, name, template, git_repo_url, status, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		p.ID, p.Name, p.Template, p.GitRepoURL, p.Status, p.CreatedAt, p.UpdatedAt,
	)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			return fmt.Errorf("project name %q already exists", p.Name)
		}
		return fmt.Errorf("create project: %w", err)
	}
	return nil
}

func (s *sqliteStore) GetProject(ctx context.Context, id string) (*Project, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, name, template, git_repo_url, status, created_at, updated_at
		 FROM projects WHERE id = ?`, id)
	return scanProject(row)
}

func (s *sqliteStore) GetProjectByName(ctx context.Context, name string) (*Project, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, name, template, git_repo_url, status, created_at, updated_at
		 FROM projects WHERE name = ?`, name)
	return scanProject(row)
}

func (s *sqliteStore) ListProjects(ctx context.Context) ([]*Project, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, name, template, git_repo_url, status, created_at, updated_at
		 FROM projects ORDER BY created_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("list projects: %w", err)
	}
	defer rows.Close()

	var projects []*Project
	for rows.Next() {
		p := &Project{}
		if err := rows.Scan(&p.ID, &p.Name, &p.Template, &p.GitRepoURL, &p.Status, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan project: %w", err)
		}
		projects = append(projects, p)
	}
	return projects, rows.Err()
}

func (s *sqliteStore) DeleteProject(ctx context.Context, id string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM projects WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete project: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *sqliteStore) GetLatestReleaseInfo(ctx context.Context, projectID string) (status, region string, found bool, err error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT status, region FROM bordo_releases
		 WHERE project_id = ?
		 ORDER BY created_at DESC LIMIT 1`, projectID)
	err = row.Scan(&status, &region)
	if err == sql.ErrNoRows {
		return "", "", false, nil
	}
	if err != nil {
		return "", "", false, fmt.Errorf("get latest release info: %w", err)
	}
	return status, region, true, nil
}

func (s *sqliteStore) GetEnvVars(ctx context.Context, projectID string) ([]*EnvVar, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT name, value, secret FROM bordo_project_env
		 WHERE project_id = ? ORDER BY name`, projectID)
	if err != nil {
		return nil, fmt.Errorf("get env vars: %w", err)
	}
	defer rows.Close()

	var vars []*EnvVar
	for rows.Next() {
		var v EnvVar
		var secretInt int
		if err := rows.Scan(&v.Name, &v.Value, &secretInt); err != nil {
			return nil, fmt.Errorf("scan env var: %w", err)
		}
		v.Secret = secretInt != 0
		vars = append(vars, &v)
	}
	return vars, rows.Err()
}

func (s *sqliteStore) SetEnvVars(ctx context.Context, projectID string, vars []*EnvVar) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	// Replace all env vars for this project.
	if _, err := tx.ExecContext(ctx,
		`DELETE FROM bordo_project_env WHERE project_id = ?`, projectID); err != nil {
		return fmt.Errorf("delete env vars: %w", err)
	}

	now := time.Now().UTC()
	for _, v := range vars {
		secretInt := 0
		if v.Secret {
			secretInt = 1
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO bordo_project_env (id, project_id, name, value, secret, created_at, updated_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?)`,
			uuid.NewString(), projectID, v.Name, v.Value, secretInt, now, now,
		); err != nil {
			return fmt.Errorf("insert env var %q: %w", v.Name, err)
		}
	}

	return tx.Commit()
}

func scanProject(row *sql.Row) (*Project, error) {
	p := &Project{}
	err := row.Scan(&p.ID, &p.Name, &p.Template, &p.GitRepoURL, &p.Status, &p.CreatedAt, &p.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("scan project: %w", err)
	}
	return p, nil
}
