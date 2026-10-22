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
		`SELECT project_id, image_tag, strategy, expose, expose_host
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
		var projectID, imageTag, strategy, expose, exposeHost string
		if err := rows.Scan(&projectID, &imageTag, &strategy, &expose, &exposeHost); err != nil {
			return "", err
		}
		if seen[projectID] { // keep only the latest active release per project
			continue
		}
		seen[projectID] = true
		m, err := renderManifest(projectID, imageTag, strategy, expose, exposeHost)
		if err != nil {
			return "", err
		}
		// Drop any Namespace resource: it's cluster-scoped, and the on-cluster
		// reconciler runs with a namespace-scoped Role (no permission to manage
		// namespaces — by design). The apps namespace is created at node join.
		manifests = append(manifests, strings.TrimSpace(stripNamespaceDocs(m)))
	}
	if err := rows.Err(); err != nil {
		return "", err
	}
	return strings.Join(manifests, "\n---\n"), nil
}

// stripNamespaceDocs removes any `kind: Namespace` document from a multi-doc
// YAML manifest, leaving the rest intact.
func stripNamespaceDocs(manifest string) string {
	docs := strings.Split(manifest, "\n---\n")
	kept := make([]string, 0, len(docs))
	for _, d := range docs {
		isNamespace := false
		for _, line := range strings.Split(d, "\n") {
			if strings.TrimSpace(line) == "kind: Namespace" {
				isNamespace = true
				break
			}
		}
		if !isNamespace {
			kept = append(kept, strings.TrimSpace(d))
		}
	}
	return strings.Join(kept, "\n---\n")
}
