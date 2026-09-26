package rbac

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// Store provides CRUD operations for the RBAC model.
type Store struct {
	db *sql.DB
}

// New creates an rbac.Store backed by db.
func New(db *sql.DB) *Store {
	return &Store{db: db}
}

// CreateOrganization inserts a new organization.
func (s *Store) CreateOrganization(ctx context.Context, name string) (*Organization, error) {
	org := &Organization{
		ID:        uuid.NewString(),
		Name:      name,
		CreatedAt: time.Now().UTC(),
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO organizations (id, name, created_at) VALUES (?, ?, ?)`,
		org.ID, org.Name, org.CreatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("create organization: %w", err)
	}
	return org, nil
}

// CreateUser inserts a new user in an organization.
func (s *Store) CreateUser(ctx context.Context, orgID, email string, role Role) (*User, error) {
	u := &User{
		ID:             uuid.NewString(),
		OrganizationID: orgID,
		Email:          email,
		Role:           role,
		CreatedAt:      time.Now().UTC(),
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO users (id, organization_id, email, role, created_at) VALUES (?, ?, ?, ?, ?)`,
		u.ID, u.OrganizationID, u.Email, string(u.Role), u.CreatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("create user: %w", err)
	}
	return u, nil
}

// IssueToken generates a random bearer token for an org + role.
// Returns the raw token (shown once) and the stored record.
func (s *Store) IssueToken(ctx context.Context, orgID string, role Role, description string, expiresAt *time.Time) (string, *APIToken, error) {
	raw, err := generateToken()
	if err != nil {
		return "", nil, fmt.Errorf("generating token: %w", err)
	}

	sum := sha256.Sum256([]byte(raw))
	hash := hex.EncodeToString(sum[:])

	t := &APIToken{
		ID:             uuid.NewString(),
		TokenHash:      hash,
		OrganizationID: orgID,
		Role:           role,
		Description:    description,
		CreatedAt:      time.Now().UTC(),
		ExpiresAt:      expiresAt,
	}

	_, err = s.db.ExecContext(ctx,
		`INSERT INTO api_tokens (id, token_hash, organization_id, role, description, created_at, expires_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		t.ID, t.TokenHash, t.OrganizationID, string(t.Role), t.Description, t.CreatedAt, expiresAt,
	)
	if err != nil {
		return "", nil, fmt.Errorf("storing token: %w", err)
	}

	return "brdo_" + raw, t, nil
}

// LookupToken finds an APIToken by its raw bearer value.
func (s *Store) LookupToken(ctx context.Context, rawToken string) (*APIToken, error) {
	if len(rawToken) > 5 && rawToken[:5] == "brdo_" {
		rawToken = rawToken[5:]
	}
	sum := sha256.Sum256([]byte(rawToken))
	hash := hex.EncodeToString(sum[:])

	row := s.db.QueryRowContext(ctx,
		`SELECT id, token_hash, organization_id, role, description, created_at, expires_at
		 FROM api_tokens WHERE token_hash = ?`, hash)

	var t APIToken
	var expiresAt sql.NullTime
	err := row.Scan(&t.ID, &t.TokenHash, &t.OrganizationID, &t.Role, &t.Description, &t.CreatedAt, &expiresAt)
	if err == sql.ErrNoRows {
		return nil, ErrTokenNotFound
	}
	if err != nil {
		return nil, err
	}
	if expiresAt.Valid {
		t.ExpiresAt = &expiresAt.Time
	}
	if t.ExpiresAt != nil && t.ExpiresAt.Before(time.Now()) {
		return nil, ErrTokenExpired
	}
	return &t, nil
}

// AppendAudit records a mutating API action.
func (s *Store) AppendAudit(ctx context.Context, e *AuditEntry) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO audit_log (organization_id, user_id, token_id, action, resource_type, resource_id, ip_address, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		e.OrganizationID, e.UserID, e.TokenID, e.Action, e.ResourceType, e.ResourceID, e.IPAddress, time.Now().UTC(),
	)
	return err
}

func generateToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
