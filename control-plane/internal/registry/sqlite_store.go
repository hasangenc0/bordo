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
