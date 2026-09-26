package strategy

import (
	"context"
	"fmt"
	"net/http"
	"time"
)

// MultiRegionDeploy executes a rollout sequentially across multiple regions,
// gating each transition on a health check against the control plane.
type MultiRegionDeploy struct {
	Regions   []string // ordered list of region names
	Config    RolloutConfig
	CPBaseURL string // control-plane base URL for health checks
	DeployFn  func(ctx context.Context, region string) error
}

// Execute runs DeployFn for each region in order, health-checking between them.
func (m *MultiRegionDeploy) Execute(ctx context.Context) error {
	for i, region := range m.Regions {
		if err := m.DeployFn(ctx, region); err != nil {
			return fmt.Errorf("deploy to region %q: %w", region, err)
		}
		// gate: health-check the region before proceeding to the next
		if i < len(m.Regions)-1 {
			if err := m.waitHealthy(ctx, region); err != nil {
				return fmt.Errorf("region %q unhealthy after deploy; stopping multi-region rollout: %w", region, err)
			}
		}
	}
	return nil
}

// waitHealthy polls the control-plane's region health endpoint until healthy or timeout.
func (m *MultiRegionDeploy) waitHealthy(ctx context.Context, region string) error {
	deadline := time.Now().Add(5 * time.Minute)
	for time.Now().Before(deadline) {
		if err := ctx.Err(); err != nil {
			return err
		}
		url := fmt.Sprintf("%s/v1/fleet/regions/%s/health", m.CPBaseURL, region)
		resp, err := http.Get(url) //nolint:noctx // best-effort check
		if err == nil && resp.StatusCode == http.StatusOK {
			resp.Body.Close()
			return nil
		}
		if resp != nil {
			resp.Body.Close()
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(15 * time.Second):
		}
	}
	return fmt.Errorf("timed out waiting for region %q to become healthy", region)
}
