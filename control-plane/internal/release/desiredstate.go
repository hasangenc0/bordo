package release

import (
	"context"
	"database/sql"
	"strings"
)

// DesiredState returns the Kubernetes manifests a region should be running: the
// latest active release per project, each rendered and joined with "---"
// separators. This is what the on-cluster reconciler pulls and applies
// (ADR-0006 Phase 2) instead of the control plane pushing via kubectl.
//
// "Active" means a release in reconciling or deployed status. When a project has
// several, the most recently updated one wins.
func DesiredState(ctx context.Context, db *sql.DB, region string) (string, error) {
	rows, err := db.QueryContext(ctx,
		`SELECT project_id, image_tag, strategy
		 FROM bordo_releases
		 WHERE region = ? AND status IN ('reconciling','deployed')
		 ORDER BY project_id, updated_at DESC`, region)
	if err != nil {
		return "", err
	}
	defer rows.Close()

	var (
		manifests []string
		seen      = map[string]bool{}
	)
	for rows.Next() {
		var projectID, imageTag, strategy string
		if err := rows.Scan(&projectID, &imageTag, &strategy); err != nil {
			return "", err
		}
		if seen[projectID] { // keep only the latest active release per project
			continue
		}
		seen[projectID] = true
		m, err := renderManifest(projectID, imageTag, strategy)
		if err != nil {
			return "", err
		}
		manifests = append(manifests, strings.TrimSpace(m))
	}
	if err := rows.Err(); err != nil {
		return "", err
	}
	return strings.Join(manifests, "\n---\n"), nil
}
