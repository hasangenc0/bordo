package strategy

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"text/template"
	"time"
)

// CanaryRollout performs a canary deployment using weighted replica counts.
// It steps through Config.CanarySteps, optionally querying VictoriaMetrics
// for error rate before each promotion.
type CanaryRollout struct {
	Config      RolloutConfig
	Project     string
	Region      string
	ManifestDir string // path to desired-state/<region>/<project>/
	Kubeconfig  string
	StableTag   string
	CanaryTag   string
}

// Execute runs the full canary rollout.
func (c *CanaryRollout) Execute(ctx context.Context) error {
	for _, weight := range c.Config.CanarySteps {
		if err := c.generateCanaryManifests(ctx, weight); err != nil {
			return fmt.Errorf("canary step %d%%: generate manifests: %w", weight, err)
		}
		if err := kubectlApply(ctx, c.ManifestDir, c.Kubeconfig); err != nil {
			return fmt.Errorf("canary step %d%%: apply: %w", weight, err)
		}
		if c.Config.AutoAnalysis && weight < 100 {
			// wait a moment for traffic to settle
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(30 * time.Second):
			}
			if err := c.checkErrorRate(ctx); err != nil {
				return fmt.Errorf("canary step %d%%: analysis failed: %w", weight, err)
			}
		}
	}
	return nil
}

// generateCanaryManifests writes stable and canary Deployments with replica
// counts proportional to weight (0–100). At weight=100 only the canary remains.
func (c *CanaryRollout) generateCanaryManifests(_ context.Context, weight int) error {
	if err := os.MkdirAll(c.ManifestDir, 0o755); err != nil {
		return err
	}

	totalReplicas := 4 // total pod budget
	canaryReplicas := (totalReplicas * weight) / 100
	if canaryReplicas < 1 {
		canaryReplicas = 1
	}
	stableReplicas := totalReplicas - canaryReplicas
	if stableReplicas < 0 {
		stableReplicas = 0
	}

	// canary deployment
	if err := renderTmpl(filepath.Join(c.ManifestDir, "deployment-canary.yaml"), canaryDeployTmpl, map[string]any{
		"Project":  c.Project,
		"ImageTag": c.CanaryTag,
		"Replicas": canaryReplicas,
		"Slot":     "canary",
	}); err != nil {
		return fmt.Errorf("write canary deployment: %w", err)
	}

	// stable deployment (zero replicas at weight=100 means stable is gone)
	if stableReplicas > 0 {
		if err := renderTmpl(filepath.Join(c.ManifestDir, "deployment-stable.yaml"), canaryDeployTmpl, map[string]any{
			"Project":  c.Project,
			"ImageTag": c.StableTag,
			"Replicas": stableReplicas,
			"Slot":     "stable",
		}); err != nil {
			return fmt.Errorf("write stable deployment: %w", err)
		}
	} else {
		// remove stable manifest once fully promoted
		_ = os.Remove(filepath.Join(c.ManifestDir, "deployment-stable.yaml"))
	}

	// shared service selects all pods for the project (both slots)
	if err := renderTmpl(filepath.Join(c.ManifestDir, "service.yaml"), canaryServiceTmpl, map[string]any{
		"Project": c.Project,
	}); err != nil {
		return fmt.Errorf("write canary service: %w", err)
	}

	return nil
}

// checkErrorRate queries VictoriaMetrics for the error rate over the last 5 min.
func (c *CanaryRollout) checkErrorRate(ctx context.Context) error {
	if c.Config.MetricsURL == "" {
		return nil // no metrics backend configured; skip
	}
	promql := fmt.Sprintf(
		`sum(rate(http_requests_total{app="%s",status=~"5.."}[5m])) / sum(rate(http_requests_total{app="%s"}[5m]))`,
		c.Project, c.Project,
	)
	u, err := url.Parse(c.Config.MetricsURL + "/api/v1/query")
	if err != nil {
		return err
	}
	q := u.Query()
	q.Set("query", promql)
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("metrics query failed: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	var result struct {
		Data struct {
			Result []struct {
				Value [2]json.RawMessage `json:"value"`
			} `json:"result"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &result); err != nil || len(result.Data.Result) == 0 {
		return nil // no data; proceed
	}

	var rate float64
	_ = json.Unmarshal(result.Data.Result[0].Value[1], &rate)
	if rate > c.Config.ErrorRateThreshold {
		return fmt.Errorf("error rate %.2f%% exceeds threshold %.2f%%", rate*100, c.Config.ErrorRateThreshold*100)
	}
	return nil
}

func kubectlApply(ctx context.Context, dir, kubeconfig string) error {
	args := []string{"apply", "-f", dir}
	if kubeconfig != "" {
		args = append(args, "--kubeconfig", kubeconfig)
	}
	cmd := exec.CommandContext(ctx, "kubectl", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func renderTmpl(path, tmplStr string, data any) error {
	t, err := template.New("").Parse(tmplStr)
	if err != nil {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return t.Execute(f, data)
}

const canaryDeployTmpl = `apiVersion: apps/v1
kind: Deployment
metadata:
  name: {{ .Project }}-{{ .Slot }}
  labels:
    app: {{ .Project }}
    slot: {{ .Slot }}
spec:
  replicas: {{ .Replicas }}
  selector:
    matchLabels:
      app: {{ .Project }}
      slot: {{ .Slot }}
  template:
    metadata:
      labels:
        app: {{ .Project }}
        slot: {{ .Slot }}
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

const canaryServiceTmpl = `apiVersion: v1
kind: Service
metadata:
  name: {{ .Project }}
spec:
  selector:
    app: {{ .Project }}
  ports:
    - port: 8080
      targetPort: 8080
  type: ClusterIP
`
