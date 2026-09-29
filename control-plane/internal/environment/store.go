// Package environment manages project environments in Bordo.
package environment

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// Environment is a named deployment target for a project.
type Environment struct {
	ID         string    `json:"id"`
	ProjectID  string    `json:"project_id"`
	Name       string    `json:"name"`
	Branch     string    `json:"branch"`
	RegionName string    `json:"region_name"`
	Namespace  string    `json:"namespace"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// EnvironmentStore is the persistence interface for environments.
type EnvironmentStore interface {
	CreateEnvironment(ctx context.Context, env *Environment) error
	ListEnvironments(ctx context.Context, projectID string) ([]*Environment, error)
	DeleteEnvironment(ctx context.Context, id string) error
}

type sqliteEnvironmentStore struct {
	db *sql.DB
}

// NewStore returns an EnvironmentStore backed by the given *sql.DB.
func NewStore(db *sql.DB) EnvironmentStore {
	return &sqliteEnvironmentStore{db: db}
}

func (s *sqliteEnvironmentStore) CreateEnvironment(ctx context.Context, env *Environment) error {
	env.ID = uuid.NewString()
	now := time.Now().UTC()
	env.CreatedAt = now
	env.UpdatedAt = now
	if env.Branch == "" {
		env.Branch = "main"
	}

	_, err := s.db.ExecContext(ctx,
		`INSERT INTO bordo_environments (id, project_id, name, branch, region_name, namespace, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		env.ID, env.ProjectID, env.Name, env.Branch, env.RegionName, env.Namespace,
		env.CreatedAt, env.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("create environment: %w", err)
	}
	return nil
}

func (s *sqliteEnvironmentStore) ListEnvironments(ctx context.Context, projectID string) ([]*Environment, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, project_id, name, branch, region_name, namespace, created_at, updated_at
		 FROM bordo_environments WHERE project_id = ? ORDER BY name`, projectID)
	if err != nil {
		return nil, fmt.Errorf("list environments: %w", err)
	}
	defer rows.Close()

	var envs []*Environment
	for rows.Next() {
		e := &Environment{}
		if err := rows.Scan(&e.ID, &e.ProjectID, &e.Name, &e.Branch, &e.RegionName, &e.Namespace,
			&e.CreatedAt, &e.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan environment: %w", err)
		}
		envs = append(envs, e)
	}
	return envs, rows.Err()
}

func (s *sqliteEnvironmentStore) DeleteEnvironment(ctx context.Context, id string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM bordo_environments WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete environment: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("environment not found")
	}
	return nil
}
