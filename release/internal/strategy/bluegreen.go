package strategy

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"text/template"
)

// BlueGreenRollout manages two Deployments: <project>-blue and <project>-green.
// It deploys to the inactive slot, then switches the Service selector.
type BlueGreenRollout struct {
	Config      RolloutConfig
	Project     string
	Region      string
	ManifestDir string
	Kubeconfig  string
	NewTag      string
}

// Execute detects the active color, deploys to the inactive slot, and switches the Service.
func (b *BlueGreenRollout) Execute(ctx context.Context) error {
	active, err := b.ActiveColor(ctx)
	if err != nil {
		// no existing service; default to blue being active (so we deploy to green)
		active = "blue"
	}
	inactive := "green"
	if active == "green" {
		inactive = "blue"
	}

	// write inactive-slot Deployment
	if err := os.MkdirAll(b.ManifestDir, 0o755); err != nil {
		return fmt.Errorf("create manifest dir: %w", err)
	}
	if err := renderTmpl(filepath.Join(b.ManifestDir, "deployment-"+inactive+".yaml"), bgDeployTmpl, map[string]any{
		"Project":  b.Project,
		"ImageTag": b.NewTag,
		"Color":    inactive,
	}); err != nil {
		return fmt.Errorf("write %s deployment: %w", inactive, err)
	}

	// apply inactive deployment
	if err := kubectlApply(ctx, b.ManifestDir, b.Kubeconfig); err != nil {
		return fmt.Errorf("apply %s deployment: %w", inactive, err)
	}

	// wait for rollout
	waitArgs := []string{"rollout", "status", "deployment/" + b.Project + "-" + inactive}
	if b.Kubeconfig != "" {
		waitArgs = append(waitArgs, "--kubeconfig", b.Kubeconfig)
	}
	waitCmd := exec.CommandContext(ctx, "kubectl", waitArgs...)
	waitCmd.Stdout = os.Stdout
	waitCmd.Stderr = os.Stderr
	if err := waitCmd.Run(); err != nil {
		return fmt.Errorf("rollout wait for %s: %w", inactive, err)
	}

	// switch Service selector to inactive (which is now the new active)
	if err := b.switchSelector(ctx, inactive); err != nil {
		return fmt.Errorf("switch selector to %s: %w", inactive, err)
	}

	return nil
}

// ActiveColor reads the Service selector label "color" to determine the currently active slot.
func (b *BlueGreenRollout) ActiveColor(ctx context.Context) (string, error) {
	args := []string{"get", "service", b.Project,
		"-o", `jsonpath={.spec.selector.color}`}
	if b.Kubeconfig != "" {
		args = append(args, "--kubeconfig", b.Kubeconfig)
	}
	cmd := exec.CommandContext(ctx, "kubectl", args...)
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return "", err
	}
	color := strings.TrimSpace(out.String())
	if color != "blue" && color != "green" {
		return "", fmt.Errorf("unexpected color %q", color)
	}
	return color, nil
}

// switchSelector patches the Service selector to point to newColor.
func (b *BlueGreenRollout) switchSelector(ctx context.Context, newColor string) error {
	patch := fmt.Sprintf(`{"spec":{"selector":{"color":%q}}}`, newColor)
	args := []string{"patch", "service", b.Project, "--type=merge", "--patch", patch}
	if b.Kubeconfig != "" {
		args = append(args, "--kubeconfig", b.Kubeconfig)
	}
	cmd := exec.CommandContext(ctx, "kubectl", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		// If the service doesn't exist yet, write a new one pointing to newColor
		return b.writeNewService(ctx, newColor)
	}
	return nil
}

func (b *BlueGreenRollout) writeNewService(ctx context.Context, color string) error {
	svcPath := filepath.Join(b.ManifestDir, "service.yaml")
	if err := renderTmpl(svcPath, bgServiceTmpl, map[string]any{
		"Project": b.Project,
		"Color":   color,
	}); err != nil {
		return fmt.Errorf("write blue-green service: %w", err)
	}
	args := []string{"apply", "-f", svcPath}
	if b.Kubeconfig != "" {
		args = append(args, "--kubeconfig", b.Kubeconfig)
	}
	cmd := exec.CommandContext(ctx, "kubectl", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// WriteBlueGreenManifests writes both -blue and -green Deployments + a Service
// into baseDir/<region>/<project>/.
func WriteBlueGreenManifests(baseDir, region, project, activeTag, inactiveTag, activeColor string) error {
	dir := filepath.Join(baseDir, region, project)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	inactiveColor := "green"
	if activeColor == "green" {
		inactiveColor = "blue"
	}

	if err := renderTmpl(filepath.Join(dir, "deployment-"+activeColor+".yaml"), bgDeployTmpl, map[string]any{
		"Project":  project,
		"ImageTag": activeTag,
		"Color":    activeColor,
	}); err != nil {
		return err
	}
	if err := renderTmpl(filepath.Join(dir, "deployment-"+inactiveColor+".yaml"), bgDeployTmpl, map[string]any{
		"Project":  project,
		"ImageTag": inactiveTag,
		"Color":    inactiveColor,
	}); err != nil {
		return err
	}
	return renderTmpl(filepath.Join(dir, "service.yaml"), bgServiceTmpl, map[string]any{
		"Project": project,
		"Color":   activeColor,
	})
}

// WriteCanaryManifests writes stable and canary Deployments + Service.
func WriteCanaryManifests(baseDir, region, project, stableTag, canaryTag string, canaryWeight int) error {
	dir := filepath.Join(baseDir, region, project)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	total := 4
	canaryReplicas := (total * canaryWeight) / 100
	if canaryReplicas < 1 {
		canaryReplicas = 1
	}
	stableReplicas := total - canaryReplicas
	if stableReplicas < 0 {
		stableReplicas = 0
	}

	if canaryReplicas > 0 {
		if err := renderTmpl(filepath.Join(dir, "deployment-canary.yaml"), canaryDeployTmpl, map[string]any{
			"Project":  project,
			"ImageTag": canaryTag,
			"Replicas": canaryReplicas,
			"Slot":     "canary",
		}); err != nil {
			return err
		}
	}
	if stableReplicas > 0 {
		if err := renderTmpl(filepath.Join(dir, "deployment-stable.yaml"), canaryDeployTmpl, map[string]any{
			"Project":  project,
			"ImageTag": stableTag,
			"Replicas": stableReplicas,
			"Slot":     "stable",
		}); err != nil {
			return err
		}
	}
	return renderTmpl(filepath.Join(dir, "service.yaml"), canaryServiceTmpl, map[string]any{
		"Project": project,
	})
}

// bgDeployTmpl is used by the template package; it must not conflict with canaryDeployTmpl.
// Using a local var so renderTmpl (from canary.go) can parse it.
var _ = template.New // ensure import used

const bgDeployTmpl = `apiVersion: apps/v1
kind: Deployment
metadata:
  name: {{ .Project }}-{{ .Color }}
  labels:
    app: {{ .Project }}
    color: {{ .Color }}
spec:
  replicas: 1
  selector:
    matchLabels:
      app: {{ .Project }}
      color: {{ .Color }}
  template:
    metadata:
      labels:
        app: {{ .Project }}
        color: {{ .Color }}
    spec:
      containers:
        - name: {{ .Project }}
          image: {{ .ImageTag }}
          ports:
            - containerPort: 8080
          readinessProbe:
            httpGet:
              path: /actuator/health
              port: 8080
            initialDelaySeconds: 10
            periodSeconds: 10
`

const bgServiceTmpl = `apiVersion: v1
kind: Service
metadata:
  name: {{ .Project }}
spec:
  selector:
    app: {{ .Project }}
    color: {{ .Color }}
  ports:
    - port: 8080
      targetPort: 8080
  type: ClusterIP
`
