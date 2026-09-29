// Package approval manages deployment approvals in Bordo.
package approval

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// Approval is a pending or resolved deployment gate.
type Approval struct {
	ID         string     `json:"id"`
	Kind       string     `json:"kind"`
	Target     string     `json:"target"`
	Payload    string     `json:"payload"`
	Status     string     `json:"status"`
	CreatedAt  time.Time  `json:"created_at"`
	ResolvedAt *time.Time `json:"resolved_at,omitempty"`
}

// ApprovalStore is the persistence interface for approvals.
type ApprovalStore interface {
	CreateApproval(ctx context.Context, a *Approval) error
	GetApproval(ctx context.Context, id string) (*Approval, error)
	ListPendingApprovals(ctx context.Context) ([]*Approval, error)
	ResolveApproval(ctx context.Context, id, status string) error
}

type sqliteApprovalStore struct {
	db *sql.DB
}

// NewStore returns an ApprovalStore backed by the given *sql.DB.
func NewStore(db *sql.DB) ApprovalStore {
	return &sqliteApprovalStore{db: db}
}

func (s *sqliteApprovalStore) CreateApproval(ctx context.Context, a *Approval) error {
	a.ID = uuid.NewString()
	a.CreatedAt = time.Now().UTC()
	if a.Status == "" {
		a.Status = "pending"
	}
	if a.Payload == "" {
		a.Payload = "{}"
	}

	_, err := s.db.ExecContext(ctx,
		`INSERT INTO bordo_approvals (id, kind, target, payload, status, created_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		a.ID, a.Kind, a.Target, a.Payload, a.Status, a.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("create approval: %w", err)
	}
	return nil
}

func (s *sqliteApprovalStore) GetApproval(ctx context.Context, id string) (*Approval, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, kind, target, payload, status, created_at, resolved_at
		 FROM bordo_approvals WHERE id = ?`, id)
	return scanApproval(row)
}

func (s *sqliteApprovalStore) ListPendingApprovals(ctx context.Context) ([]*Approval, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, kind, target, payload, status, created_at, resolved_at
		 FROM bordo_approvals WHERE status = 'pending' ORDER BY created_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("list pending approvals: %w", err)
	}
	defer rows.Close()

	var approvals []*Approval
	for rows.Next() {
		a, err := scanApprovalRow(rows)
		if err != nil {
			return nil, err
		}
		approvals = append(approvals, a)
	}
	return approvals, rows.Err()
}

func (s *sqliteApprovalStore) ResolveApproval(ctx context.Context, id, status string) error {
	now := time.Now().UTC()
	res, err := s.db.ExecContext(ctx,
		`UPDATE bordo_approvals SET status = ?, resolved_at = ? WHERE id = ?`,
		status, now, id,
	)
	if err != nil {
		return fmt.Errorf("resolve approval: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("approval not found")
	}
	return nil
}

func scanApproval(row *sql.Row) (*Approval, error) {
	a := &Approval{}
	var resolvedAt sql.NullTime
	err := row.Scan(&a.ID, &a.Kind, &a.Target, &a.Payload, &a.Status, &a.CreatedAt, &resolvedAt)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("approval not found")
	}
	if err != nil {
		return nil, fmt.Errorf("scan approval: %w", err)
	}
	if resolvedAt.Valid {
		a.ResolvedAt = &resolvedAt.Time
	}
	return a, nil
}

func scanApprovalRow(rows *sql.Rows) (*Approval, error) {
	a := &Approval{}
	var resolvedAt sql.NullTime
	if err := rows.Scan(&a.ID, &a.Kind, &a.Target, &a.Payload, &a.Status, &a.CreatedAt, &resolvedAt); err != nil {
		return nil, fmt.Errorf("scan approval: %w", err)
	}
	if resolvedAt.Valid {
		a.ResolvedAt = &resolvedAt.Time
	}
	return a, nil
}
