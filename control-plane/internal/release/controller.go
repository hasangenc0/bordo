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

	"github.com/bordo-io/bordo/control-plane/internal/fleet"
	githubpkg "github.com/bordo-io/bordo/control-plane/internal/github"
)

// Controller watches for reconciling releases and applies them to k3s clusters.
// kubectl must be in PATH inside the container.
type Controller struct {
	db      *sql.DB
	logger  *slog.Logger
	ghStore *githubpkg.Store // may be nil
}

// NewController creates a release controller.
func NewController(db *sql.DB, logger *slog.Logger, ghStore *githubpkg.Store) *Controller {
	return &Controller{db: db, logger: logger, ghStore: ghStore}
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

	kubeconfigPath, err := fleet.WriteTempKubeconfig(kubeconfig)
	if err != nil {
		return err
	}
	defer os.Remove(kubeconfigPath)

	var logBuf strings.Builder

	// Ensure image pull secret if using GHCR.
	if strings.HasPrefix(imageTag, "ghcr.io/") {
		if token, err := c.githubToken(ctx); err == nil && token != "" {
			// Extract owner from ghcr.io/owner/repo:sha
			parts := strings.SplitN(strings.TrimPrefix(imageTag, "ghcr.io/"), "/", 2)
			owner := parts[0]
			if pullErr := fleet.EnsureImagePullSecret(ctx, kubeconfigPath, "ghcr.io", owner, token, "bordo-apps"); pullErr != nil {
				logBuf.WriteString(fmt.Sprintf("[warn] image pull secret: %v\n", pullErr))
			} else {
				logBuf.WriteString("[bordo] image pull secret applied\n")
			}
		}
	}

	manifest, err := renderManifest(projectID, imageTag, strategy)
	if err != nil {
		return fmt.Errorf("render manifest: %w", err)
	}

	kubectlPath, err := exec.LookPath("kubectl")
	if err != nil {
		return fmt.Errorf("kubectl not found in PATH — add 'RUN apk add --no-cache kubectl' to Dockerfile.bordod")
	}

	applyCmd := exec.CommandContext(ctx, kubectlPath,
		"--kubeconfig", kubeconfigPath,
		"apply", "--namespace", "bordo-apps", "-f", "-")
	applyCmd.Stdin = strings.NewReader(manifest)
	var applyOut bytes.Buffer
	applyCmd.Stdout = &applyOut
	applyCmd.Stderr = &applyOut

	if err := applyCmd.Run(); err != nil {
		logBuf.WriteString(applyOut.String())
		c.setLog(ctx, id, logBuf.String())
		return fmt.Errorf("kubectl apply: %w\noutput: %s", err, applyOut.String())
	}
	logBuf.WriteString(applyOut.String())

	// Wait for rollout to complete.
	deployName := appLabel(projectID)
	rolloutCmd := exec.CommandContext(ctx, kubectlPath,
		"--kubeconfig", kubeconfigPath,
		"rollout", "status",
		fmt.Sprintf("deployment/%s", deployName),
		"--namespace", "bordo-apps",
		"--timeout=300s")
	var rolloutOut bytes.Buffer
	rolloutCmd.Stdout = &rolloutOut
	rolloutCmd.Stderr = &rolloutOut

	if err := rolloutCmd.Run(); err != nil {
		logBuf.WriteString(rolloutOut.String())
		c.setLog(ctx, id, logBuf.String())
		return fmt.Errorf("rollout failed: %w\noutput: %s", err, rolloutOut.String())
	}
	logBuf.WriteString(rolloutOut.String())
	c.setLog(ctx, id, logBuf.String())
	return nil
}

func (c *Controller) setStatus(ctx context.Context, id, status string) {
	_, _ = c.db.ExecContext(ctx,
		`UPDATE bordo_releases SET status = ?, updated_at = ? WHERE id = ?`,
		status, time.Now().UTC().Format(time.RFC3339), id)
}

func (c *Controller) setLog(ctx context.Context, id, log string) {
	_, _ = c.db.ExecContext(ctx,
		`UPDATE bordo_releases SET log = ?, updated_at = ? WHERE id = ?`,
		log, time.Now().UTC().Format(time.RFC3339), id)
}

// githubToken returns a fresh GitHub installation token (or user token as fallback).
func (c *Controller) githubToken(ctx context.Context) (string, error) {
	if c.ghStore == nil {
		return "", nil
	}
	creds, err := c.ghStore.LoadApp(ctx)
	if err != nil || creds == nil {
		userToken, _, err2 := c.ghStore.LoadUserToken(ctx)
		return userToken, err2
	}
	installID, _, _, err := c.ghStore.LoadInstallationID(ctx)
	if err != nil || installID == 0 {
		userToken, _, err2 := c.ghStore.LoadUserToken(ctx)
		return userToken, err2
	}
	jwtToken, err := githubpkg.GenerateJWT(creds.AppID, creds.PrivateKeyPEM)
	if err != nil {
		return "", err
	}
	return githubpkg.GetInstallationToken(jwtToken, installID)
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
      {{- if .UseImagePullSecret}}
      imagePullSecrets:
        - name: bordo-registry-creds
      {{- end}}
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
	AppLabel           string
	ProjectID          string
	ImageTag           string
	Replicas           int
	Port               int
	UseImagePullSecret bool
}

func renderManifest(projectID, imageTag, strategy string) (string, error) {
	replicas := 1
	if strategy == "canary" || strategy == "blue-green" {
		replicas = 2
	}

	port := 8080
	lower := strings.ToLower(imageTag)
	if strings.Contains(lower, "node") || strings.Contains(lower, "react") || strings.Contains(lower, "ts-") {
		port = 3000
	}

	vars := manifestVars{
		AppLabel:           appLabel(projectID),
		ProjectID:          projectID,
		ImageTag:           imageTag,
		Replicas:           replicas,
		Port:               port,
		UseImagePullSecret: strings.HasPrefix(imageTag, "ghcr.io/"),
	}

	var buf bytes.Buffer
	if err := manifestTmpl.Execute(&buf, vars); err != nil {
		return "", err
	}
	return buf.String(), nil
}
