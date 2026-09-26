// Package rbac provides the Bordo RBAC data model and token management.
package rbac

import "time"

// Role is a user's permission level within an organization.
type Role string

const (
	RoleOwner     Role = "owner"
	RoleAdmin     Role = "admin"
	RoleDeveloper Role = "developer"
	RoleViewer    Role = "viewer"
)

// Organization is the top-level tenancy unit.
type Organization struct {
	ID        string
	Name      string
	CreatedAt time.Time
}

// User is a member of an organization with a role.
type User struct {
	ID             string
	OrganizationID string
	Email          string
	Role           Role
	CreatedAt      time.Time
}

// APIToken is a long-lived credential scoped to an org + role.
type APIToken struct {
	ID             string
	TokenHash      string // SHA-256 hex of the raw bearer token
	OrganizationID string
	Role           Role
	Description    string
	CreatedAt      time.Time
	ExpiresAt      *time.Time
}

// AuditEntry records a single mutating API call.
type AuditEntry struct {
	ID             int64
	OrganizationID string
	UserID         string
	TokenID        string
	Action         string
	ResourceType   string
	ResourceID     string
	IPAddress      string
	CreatedAt      time.Time
}
