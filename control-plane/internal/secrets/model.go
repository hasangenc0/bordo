package secrets

import "time"

// Secret is a project-scoped secret. The plaintext value is never exposed via API.
type Secret struct {
	ID        string
	ProjectID string
	Key       string
	CreatedAt time.Time
	UpdatedAt time.Time
}
