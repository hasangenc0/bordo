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
//
// It also returns the IDs of the releases included in the manifest bundle, so the
// reconciler can acknowledge which releases it applied (ADR-0006 Phase 2 feedback
// loop) and the control plane can advance them from reconciling to deployed.
func DesiredState(ctx context.Context, db *sql.DB, region string) (string, []string, error) {
	known := knownProjectIDs(ctx, db)
	rows, err := db.QueryContext(ctx,
		`SELECT id, project_id, image_tag, strategy, expose, expose_host
		 FROM bordo_releases
		 WHERE region = ? AND status IN ('reconciling','deployed')
		 ORDER BY project_id, updated_at DESC`, region)
	if err != nil {
		return "", nil, err
	}
	defer rows.Close()

	var (
		manifests  []string
		releaseIDs []string
		seen       = map[string]bool{}
	)
	for rows.Next() {
		var id, projectID, imageTag, strategy, expose, exposeHost string
		if err := rows.Scan(&id, &projectID, &imageTag, &strategy, &expose, &exposeHost); err != nil {
			return "", nil, err
		}
		projectID = canonicalProjectID(known, projectID)
		if seen[projectID] { // keep only the latest active release per project
			continue
		}
		seen[projectID] = true
		m, err := renderManifest(projectID, imageTag, strategy, expose, exposeHost)
		if err != nil {
			return "", nil, err
		}
		// Drop any Namespace resource: it's cluster-scoped, and the on-cluster
		// reconciler runs with a namespace-scoped Role (no permission to manage
		// namespaces — by design). The apps namespace is created at node join.
		manifests = append(manifests, strings.TrimSpace(stripNamespaceDocs(m)))
		releaseIDs = append(releaseIDs, id)
	}
	if err := rows.Err(); err != nil {
		return "", nil, err
	}
	return strings.Join(manifests, "\n---\n"), releaseIDs, nil
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

// knownProjectIDs lists registered project ids and names (name -> id handled by caller map).
func knownProjectIDs(ctx context.Context, db *sql.DB) map[string]string {
	out := map[string]string{} // id or name -> id
	rows, err := db.QueryContext(ctx, `SELECT id, name FROM projects`)
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var id, name string
		if rows.Scan(&id, &name) == nil {
			out[id] = id
			if name != "" {
				out[name] = id
			}
		}
	}
	return out
}

// canonicalProjectID maps a stored project_id (full id, name, or an id prefix an older
// caller truncated) to the project's full id, so one project never gets two sets of
// resources. Unknown or ambiguous values are returned unchanged.
func canonicalProjectID(known map[string]string, v string) string {
	if id, ok := known[v]; ok {
		return id
	}
	if len(v) < 6 {
		return v
	}
	match := ""
	for k, id := range known {
		if strings.HasPrefix(k, v) {
			if match != "" && match != id {
				return v
			}
			match = id
		}
	}
	if match != "" {
		return match
	}
	return v
}
