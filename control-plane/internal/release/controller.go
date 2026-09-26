package release

import (
	"bytes"
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"strings"
	"text/template"
	"time"
)

// Controller watches for reconciling releases and applies them to k3s clusters.
// kubectl must be in PATH inside the container (add to Dockerfile: RUN apk add --no-cache kubectl).
type Controller struct {
	db     *sql.DB
	logger *slog.Logger
}

// NewController creates a release controller.
func NewController(db *sql.DB, logger *slog.Logger) *Controller {
	return &Controller{db: db, logger: logger}
}

// Start runs the reconcile loop until ctx is cancelled.
func (c *Controller) Start(ctx context.Context) error {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			c.reconcile(ctx)
		}
	}
}

func (c *Controller) reconcile(ctx context.Context) {
	rows, err := c.db.QueryContext(ctx,
		`SELECT id, project_id, image_tag, region, strategy FROM bordo_releases WHERE status = 'reconciling'`)
	if err != nil {
		c.logger.Warn("release controller: query failed", "err", err)
		return
	}
	defer rows.Close()

	type pending struct {
		id, projectID, imageTag, region, strategy string
	}
	var releases []pending
	for rows.Next() {
		var r pending
		if err := rows.Scan(&r.id, &r.projectID, &r.imageTag, &r.region, &r.strategy); err == nil {
			releases = append(releases, r)
		}
	}

	for _, r := range releases {
		if err := c.apply(ctx, r.id, r.projectID, r.imageTag, r.region, r.strategy); err != nil {
			c.logger.Error("release controller: apply failed", "id", r.id, "err", err)
			c.setStatus(ctx, r.id, "failed")
		} else {
			c.logger.Info("release controller: deployed", "id", r.id, "region", r.region)
			c.setStatus(ctx, r.id, "deployed")
		}
	}
}

func (c *Controller) apply(ctx context.Context, id, projectID, imageTag, regionName, strategy string) error {
	// Look up kubeconfig for the region.
	var kubeconfig string
	err := c.db.QueryRowContext(ctx,
		`SELECT kubeconfig FROM regions WHERE name = ?`, regionName).Scan(&kubeconfig)
	if err == sql.ErrNoRows {
		return fmt.Errorf("region %q not found", regionName)
	}
	if err != nil {
		return fmt.Errorf("lookup region: %w", err)
	}
	if kubeconfig == "" {
		return fmt.Errorf("region %q has no kubeconfig (not yet bootstrapped)", regionName)
	}

	// Write kubeconfig to temp file.
	kf, err := os.CreateTemp("", "bordo-kubeconfig-*.yaml")
	if err != nil {
		return fmt.Errorf("create temp kubeconfig: %w", err)
	}
	defer os.Remove(kf.Name())
	if _, err := kf.WriteString(kubeconfig); err != nil {
		kf.Close()
		return err
	}
	kf.Close()

	// Generate manifest.
	manifest, err := renderManifest(projectID, imageTag, strategy)
	if err != nil {
		return fmt.Errorf("render manifest: %w", err)
	}

	// kubectl apply -f -
	kubectlPath, err := exec.LookPath("kubectl")
	if err != nil {
		return fmt.Errorf("kubectl not found in PATH — add 'RUN apk add --no-cache kubectl' to Dockerfile.bordod")
	}

	cmd := exec.CommandContext(ctx, kubectlPath,
		"--kubeconfig", kf.Name(),
		"apply", "--namespace", "bordo-apps", "-f", "-")
	cmd.Stdin = strings.NewReader(manifest)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("kubectl apply: %w\noutput: %s", err, out.String())
	}
	return nil
}

func (c *Controller) setStatus(ctx context.Context, id, status string) {
	_, _ = c.db.ExecContext(ctx,
		`UPDATE bordo_releases SET status = ?, updated_at = ? WHERE id = ?`,
		status, time.Now().UTC().Format(time.RFC3339), id)
}

// appLabel returns a short, k8s-safe label from a project UUID.
func appLabel(projectID string) string {
	s := strings.ReplaceAll(projectID, "-", "")
	if len(s) > 20 {
		s = s[:20]
	}
	return "bordo-" + s
}

var manifestTmpl = template.Must(template.New("manifest").Parse(`
apiVersion: v1
kind: Namespace
metadata:
  name: bordo-apps
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: {{.AppLabel}}
  namespace: bordo-apps
  labels:
    app: {{.AppLabel}}
    bordo.io/project: {{.ProjectID}}
spec:
  replicas: {{.Replicas}}
  selector:
    matchLabels:
      app: {{.AppLabel}}
  template:
    metadata:
      labels:
        app: {{.AppLabel}}
        bordo.io/project: {{.ProjectID}}
    spec:
      containers:
        - name: app
          image: {{.ImageTag}}
          ports:
            - containerPort: {{.Port}}
          env:
            - name: PORT
              value: "{{.Port}}"
---
apiVersion: v1
kind: Service
metadata:
  name: {{.AppLabel}}
  namespace: bordo-apps
spec:
  selector:
    app: {{.AppLabel}}
  ports:
    - port: 80
      targetPort: {{.Port}}
  type: ClusterIP
`))

type manifestVars struct {
	AppLabel  string
	ProjectID string
	ImageTag  string
	Replicas  int
	Port      int
}

func renderManifest(projectID, imageTag, strategy string) (string, error) {
	replicas := 1
	if strategy == "canary" || strategy == "blue-green" {
		replicas = 2
	}

	// Infer port: node/ts images default to 3000, Java to 8080.
	port := 8080
	lower := strings.ToLower(imageTag)
	if strings.Contains(lower, "node") || strings.Contains(lower, "react") || strings.Contains(lower, "ts-") {
		port = 3000
	}

	vars := manifestVars{
		AppLabel:  appLabel(projectID),
		ProjectID: projectID,
		ImageTag:  imageTag,
		Replicas:  replicas,
		Port:      port,
	}

	var buf bytes.Buffer
	if err := manifestTmpl.Execute(&buf, vars); err != nil {
		return "", err
	}
	return buf.String(), nil
}
