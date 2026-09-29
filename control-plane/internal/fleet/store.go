// Package fleet manages Bordo regions and clusters.
package fleet

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Region represents a geographic region with a k3s cluster.
type Region struct {
	ID         string
	Name       string
	Status     string
	Kubeconfig string
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

// Cluster is a k3s cluster within a region.
type Cluster struct {
	ID         string
	RegionID   string
	Name       string
	NodeCount  int
	K8sVersion string
	Status     string
	LastSeen   *time.Time
	CreatedAt  time.Time
}

var ErrNotFound = fmt.Errorf("region not found")

// Store persists fleet state.
type Store struct {
	db *sql.DB
}

// New creates a fleet.Store backed by db.
func New(db *sql.DB) *Store {
	return &Store{db: db}
}

// AddRegion registers a new region.
func (s *Store) AddRegion(ctx context.Context, name, kubeconfig string) (*Region, error) {
	r := &Region{
		ID:         uuid.NewString(),
		Name:       name,
		Status:     "pending",
		Kubeconfig: kubeconfig,
		CreatedAt:  time.Now().UTC(),
		UpdatedAt:  time.Now().UTC(),
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO regions (id, name, status, kubeconfig, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		r.ID, r.Name, r.Status, r.Kubeconfig, r.CreatedAt, r.UpdatedAt,
	)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			return nil, fmt.Errorf("region %q already exists", name)
		}
		return nil, fmt.Errorf("add region: %w", err)
	}
	return r, nil
}

// GetRegion retrieves a region by ID.
func (s *Store) GetRegion(ctx context.Context, id string) (*Region, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, name, status, kubeconfig, created_at, updated_at FROM regions WHERE id = ?`, id)
	return scanRegion(row)
}

// GetRegionByName retrieves a region by name.
func (s *Store) GetRegionByName(ctx context.Context, name string) (*Region, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, name, status, kubeconfig, created_at, updated_at FROM regions WHERE name = ?`, name)
	return scanRegion(row)
}

// ListRegions returns all regions.
func (s *Store) ListRegions(ctx context.Context) ([]*Region, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, name, status, kubeconfig, created_at, updated_at FROM regions ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var regions []*Region
	for rows.Next() {
		r := &Region{}
		if err := rows.Scan(&r.ID, &r.Name, &r.Status, &r.Kubeconfig, &r.CreatedAt, &r.UpdatedAt); err != nil {
			return nil, err
		}
		regions = append(regions, r)
	}
	return regions, rows.Err()
}

// UpdateRegionStatus updates the health status of a region by ID.
func (s *Store) UpdateRegionStatus(ctx context.Context, id, status string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE regions SET status = ?, updated_at = ? WHERE id = ?`,
		status, time.Now().UTC(), id,
	)
	return err
}

// UpdateRegionStatusByName updates the health status of a region by name.
func (s *Store) UpdateRegionStatusByName(ctx context.Context, name, status string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE regions SET status = ?, updated_at = ? WHERE name = ?`,
		status, time.Now().UTC(), name,
	)
	return err
}

// UpdateKubeconfig sets the kubeconfig and status for a region identified by name.
func (s *Store) UpdateKubeconfig(ctx context.Context, name, kubeconfig, status string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE regions SET kubeconfig = ?, status = ?, updated_at = ? WHERE name = ?`,
		kubeconfig, status, time.Now().UTC(), name,
	)
	return err
}

// RemoveRegion deletes a region and its clusters.
func (s *Store) RemoveRegion(ctx context.Context, id string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM regions WHERE id = ?`, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func scanRegion(row *sql.Row) (*Region, error) {
	r := &Region{}
	err := row.Scan(&r.ID, &r.Name, &r.Status, &r.Kubeconfig, &r.CreatedAt, &r.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return r, nil
}
