// Package registry implements the Bordo project catalog and registry.
package registry

import (
	"context"
	"time"
)

// Project is a software project managed by Bordo.
type Project struct {
	ID         string
	Name       string
	Template   string
	GitRepoURL string
	Status     string
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

// Store is the interface for project persistence.
type Store interface {
	CreateProject(ctx context.Context, p *Project) error
	GetProject(ctx context.Context, id string) (*Project, error)
	GetProjectByName(ctx context.Context, name string) (*Project, error)
	ListProjects(ctx context.Context) ([]*Project, error)
	DeleteProject(ctx context.Context, id string) error
}
