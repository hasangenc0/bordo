package fleet

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// EnsureImagePullSecret creates or updates a kubernetes.io/dockerconfigjson Secret
// named "bordo-registry-creds" in the given namespace on the cluster pointed to by
// kubeconfigPath.
func EnsureImagePullSecret(ctx context.Context, kubeconfigPath, registryServer, username, password, namespace string) error {
	authStr := base64.StdEncoding.EncodeToString([]byte(username + ":" + password))
	dockerConfig := map[string]any{
		"auths": map[string]any{
			registryServer: map[string]string{
				"username": username,
				"password": password,
				"auth":     authStr,
			},
		},
	}
	dockerConfigJSON, err := json.Marshal(dockerConfig)
	if err != nil {
		return fmt.Errorf("marshal docker config: %w", err)
	}

	manifest := fmt.Sprintf(`apiVersion: v1
kind: Secret
metadata:
  name: bordo-registry-creds
  namespace: %s
type: kubernetes.io/dockerconfigjson
data:
  .dockerconfigjson: %s
`, namespace, base64.StdEncoding.EncodeToString(dockerConfigJSON))

	kubectlPath, err := exec.LookPath("kubectl")
	if err != nil {
		return fmt.Errorf("kubectl not found in PATH")
	}

	// Ensure namespace exists first.
	nsManifest := fmt.Sprintf("apiVersion: v1\nkind: Namespace\nmetadata:\n  name: %s\n", namespace)
	nsCmd := exec.CommandContext(ctx, kubectlPath, "--kubeconfig", kubeconfigPath, "apply", "-f", "-")
	nsCmd.Stdin = strings.NewReader(nsManifest)
	var nsOut bytes.Buffer
	nsCmd.Stdout = &nsOut
	nsCmd.Stderr = &nsOut
	_ = nsCmd.Run() // ignore error — namespace may already exist

	cmd := exec.CommandContext(ctx, kubectlPath, "--kubeconfig", kubeconfigPath, "apply", "-f", "-")
	cmd.Stdin = strings.NewReader(manifest)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("kubectl apply imagePullSecret: %w\noutput: %s", err, out.String())
	}
	return nil
}

// WriteTempKubeconfig writes kubeconfig YAML to a temp file and returns its path.
// Caller must remove the file when done.
func WriteTempKubeconfig(kubeconfig string) (string, error) {
	kf, err := os.CreateTemp("", "bordo-kubeconfig-*.yaml")
	if err != nil {
		return "", fmt.Errorf("create temp kubeconfig: %w", err)
	}
	if _, err := kf.WriteString(kubeconfig); err != nil {
		kf.Close()
		os.Remove(kf.Name())
		return "", err
	}
	kf.Close()
	return kf.Name(), nil
}
