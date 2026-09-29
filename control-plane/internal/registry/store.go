// Package registry implements the Bordo project catalog and registry.
package registry

import (
	"context"
	"time"
)

// Project is a software project managed by Bordo.
type Project struct {
	ID         string    `json:"id"`
	Name       string    `json:"name"`
	Template   string    `json:"template"`
	GitRepoURL string    `json:"git_repo_url"`
	Status     string    `json:"status"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// ProjectWithStatus embeds Project and adds the latest release info.
type ProjectWithStatus struct {
	*Project
	ReleaseStatus string `json:"release_status"`
	ReleaseRegion string `json:"release_region"`
}

// EnvVar is a project environment variable.
type EnvVar struct {
	Name   string `json:"name"`
	Value  string `json:"value"`
	Secret bool   `json:"secret"`
}

// Store is the interface for project persistence.
type Store interface {
	CreateProject(ctx context.Context, p *Project) error
	GetProject(ctx context.Context, id string) (*Project, error)
	GetProjectByName(ctx context.Context, name string) (*Project, error)
	ListProjects(ctx context.Context) ([]*Project, error)
	DeleteProject(ctx context.Context, id string) error

	// Release status helpers.
	GetLatestReleaseInfo(ctx context.Context, projectID string) (status, region string, found bool, err error)

	// Env var helpers.
	GetEnvVars(ctx context.Context, projectID string) ([]*EnvVar, error)
	SetEnvVars(ctx context.Context, projectID string, vars []*EnvVar) error
}
