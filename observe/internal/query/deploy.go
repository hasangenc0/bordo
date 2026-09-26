package query

import (
	"context"
	"fmt"
	"os/exec"
)

// DeployStack applies the observability kustomization to a cluster using kubectl.
// manifestDir is the path to deploy/observability/.
// kubeconfig is the path to the cluster's kubeconfig file.
func DeployStack(ctx context.Context, manifestDir, kubeconfig string) error {
	kubectlPath, err := exec.LookPath("kubectl")
	if err != nil {
		return fmt.Errorf("kubectl not found on PATH: %w", err)
	}

	cmd := exec.CommandContext(ctx, kubectlPath, "apply", "-k", manifestDir, "--kubeconfig", kubeconfig)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("kubectl apply: %w\n%s", err, out)
	}
	return nil
}
