// Package node implements the Bordo node agent (ADR-0006 Phase 1).
//
// It runs ON a target VM, redeems a single-use join token *outbound* to the
// control plane, installs k3s, and registers the resulting cluster back with
// the control plane using a single-use register token. Because the node reaches
// out, the control plane never needs an SSH key or inbound access to the VM.
package node

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"text/template"
	"time"
)

// DefaultK3sVersion is the pinned k3s version the agent installs. Pinning (vs a
// floating "latest") is part of the ADR-0006 supply-chain hardening.
const DefaultK3sVersion = "v1.30.5+k3s1"

// DefaultKubeconfigPath is where k3s writes the cluster admin kubeconfig.
const DefaultKubeconfigPath = "/etc/rancher/k3s/k3s.yaml"

// Config is the node agent's runtime configuration, normally from env vars set
// by install-node.sh.
type Config struct {
	CPURL            string // BORDO_CP_URL — control-plane base URL, reachable from this VM
	JoinToken        string // BORDO_JOIN_TOKEN — one-time token from `bordo region add`
	NodeName         string // BORDO_NODE_NAME — defaults to hostname
	K3sVersion       string // BORDO_K3S_VERSION — pinned k3s version
	K3sInstallSHA256 string // BORDO_K3S_INSTALL_SHA256 — expected sha256 of the k3s install script
	K3sExec          string // BORDO_K3S_EXEC — k3s server flags (INSTALL_K3S_EXEC); default trims memory
	AdvertiseAddr    string // BORDO_NODE_ADVERTISE_ADDR — address the CP should use to reach the API server
	KubeconfigPath   string // BORDO_KUBECONFIG_PATH — defaults to DefaultKubeconfigPath
	ReconcilerImage  string // BORDO_RECONCILER_IMAGE — on-cluster reconciler image
	AppsNamespace    string // BORDO_APPS_NAMESPACE — namespace apps deploy into
	SkipK3s          bool   // BORDO_NODE_SKIP_K3S=1 — dry run, skip the actual install (testing)
}

// DefaultReconcilerImage is the on-cluster reconciler container image.
const DefaultReconcilerImage = "ghcr.io/hasangenc0/bordo-reconciler:latest"

// DefaultAppsNamespace is where user workloads are deployed.
const DefaultAppsNamespace = "bordo-apps"

// DefaultK3sExec trims k3s to the essentials: no Traefik ingress, no ServiceLB,
// no metrics-server. These components add hundreds of MB and aren't needed for
// Bordo's deploy flow, which matters on small/shared VMs (ADR-0006 Phase 3).
const DefaultK3sExec = "--disable traefik --disable servicelb --disable metrics-server"

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// ConfigFromEnv builds a Config from environment variables and validates it.
func ConfigFromEnv() (Config, error) {
	c := Config{
		CPURL:            os.Getenv("BORDO_CP_URL"),
		JoinToken:        os.Getenv("BORDO_JOIN_TOKEN"),
		NodeName:         os.Getenv("BORDO_NODE_NAME"),
		K3sVersion:       envOr("BORDO_K3S_VERSION", DefaultK3sVersion),
		K3sInstallSHA256: os.Getenv("BORDO_K3S_INSTALL_SHA256"),
		K3sExec:          envOr("BORDO_K3S_EXEC", DefaultK3sExec),
		AdvertiseAddr:    os.Getenv("BORDO_NODE_ADVERTISE_ADDR"),
		KubeconfigPath:   envOr("BORDO_KUBECONFIG_PATH", DefaultKubeconfigPath),
		ReconcilerImage:  envOr("BORDO_RECONCILER_IMAGE", DefaultReconcilerImage),
		AppsNamespace:    envOr("BORDO_APPS_NAMESPACE", DefaultAppsNamespace),
		SkipK3s:          os.Getenv("BORDO_NODE_SKIP_K3S") == "1",
	}
	if c.NodeName == "" {
		c.NodeName, _ = os.Hostname()
	}
	if c.CPURL == "" {
		return c, errors.New("BORDO_CP_URL is required")
	}
	if c.JoinToken == "" {
		return c, errors.New("BORDO_JOIN_TOKEN is required")
	}
	c.CPURL = strings.TrimRight(c.CPURL, "/")
	return c, nil
}

type redeemResponse struct {
	Region        string `json:"region"`
	RegisterToken string `json:"register_token"`
}

// redeem exchanges the join token for the region and a single-use register
// token. It is the outbound call that proves the node was invited.
func redeem(ctx context.Context, httpc *http.Client, cpURL, joinToken string) (redeemResponse, error) {
	var out redeemResponse
	if err := postJSON(ctx, httpc, cpURL+"/fleet/redeem",
		map[string]string{"token": joinToken}, &out); err != nil {
		return out, err
	}
	if out.Region == "" || out.RegisterToken == "" {
		return out, errors.New("redeem: control plane returned an incomplete response")
	}
	return out, nil
}

// register sends the cluster kubeconfig back to the control plane, consuming the
// single-use register token, and returns the region's long-lived cluster token
// (ADR-0006 Phase 2) the on-cluster reconciler will use to pull desired state.
func register(ctx context.Context, httpc *http.Client, cpURL, region, registerToken, kubeconfig string) (string, error) {
	var out struct {
		ClusterToken string `json:"cluster_token"`
	}
	if err := postJSON(ctx, httpc, cpURL+"/fleet/register", map[string]string{
		"region":         region,
		"register_token": registerToken,
		"kubeconfig":     kubeconfig,
	}, &out); err != nil {
		return "", err
	}
	return out.ClusterToken, nil
}

func postJSON(ctx context.Context, httpc *http.Client, url string, body, out any) error {
	data, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := httpc.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		var e struct {
			Error string `json:"error"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&e)
		if e.Error == "" {
			e.Error = resp.Status
		}
		return fmt.Errorf("%s: %s", url, e.Error)
	}
	if out != nil {
		return json.NewDecoder(resp.Body).Decode(out)
	}
	return nil
}

// installK3s runs the pinned k3s installer. This is the one step that cannot be
// unit-tested — it mutates the host — so it is isolated here and guarded by
// Config.SkipK3s. The installer verifies the k3s binary against the published
// checksum for the pinned version (INSTALL_K3S_VERSION) as its integrity check.
func installK3s(ctx context.Context, version, expectedSHA, k3sExec string) error {
	scriptPath := filepath.Join(os.TempDir(), "k3s-install.sh")
	if err := downloadFile(ctx, "https://get.k3s.io", scriptPath); err != nil {
		return fmt.Errorf("download k3s installer: %w", err)
	}
	defer os.Remove(scriptPath)

	// Supply-chain hardening: verify the installer script against a pinned
	// sha256 when provided. The pinned INSTALL_K3S_VERSION additionally makes
	// the installer verify the k3s binary against the release's own checksums.
	if expectedSHA != "" {
		if err := verifyFileSHA256(scriptPath, expectedSHA); err != nil {
			return fmt.Errorf("k3s installer checksum: %w", err)
		}
		fmt.Println("[bordo-node] k3s installer checksum verified")
	} else {
		fmt.Println("[bordo-node] warning: BORDO_K3S_INSTALL_SHA256 not set — the installer script is not pinned (the k3s binary is still verified against the pinned version)")
	}

	cmd := exec.CommandContext(ctx, "sh", scriptPath, "--write-kubeconfig-mode", "644")
	cmd.Env = append(os.Environ(),
		"INSTALL_K3S_VERSION="+version,
		"INSTALL_K3S_EXEC="+k3sExec,
	)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// downloadFile fetches url to dest.
func downloadFile(ctx context.Context, url, dest string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("GET %s: %s", url, resp.Status)
	}
	f, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = io.Copy(f, resp.Body)
	return err
}

// verifyFileSHA256 checks that the file at path matches expected (hex sha256).
func verifyFileSHA256(path, expected string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	sum := sha256.Sum256(data)
	got := hex.EncodeToString(sum[:])
	if !strings.EqualFold(got, strings.TrimSpace(expected)) {
		return fmt.Errorf("sha256 mismatch: got %s, want %s", got, expected)
	}
	return nil
}

type reconcilerVars struct {
	CPURL        string
	ClusterToken string
	Image        string
	Namespace    string
}

// renderReconcilerManifests produces the Kubernetes manifests that install the
// on-cluster reconciler (ADR-0006 Phase 2): the reconciler runs in bordo-system
// with a ServiceAccount bound to a Role scoped to the apps namespace only — it
// is NOT cluster-admin. The cluster token is delivered via a Secret.
func renderReconcilerManifests(v reconcilerVars) string {
	var buf bytes.Buffer
	_ = reconcilerTmpl.Execute(&buf, v)
	return buf.String()
}

var reconcilerTmpl = template.Must(template.New("reconciler").Parse(`apiVersion: v1
kind: Namespace
metadata:
  name: bordo-system
---
apiVersion: v1
kind: Namespace
metadata:
  name: {{.Namespace}}
---
apiVersion: v1
kind: ServiceAccount
metadata:
  name: bordo-reconciler
  namespace: bordo-system
---
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: bordo-reconciler
  namespace: {{.Namespace}}
rules:
  - apiGroups: ["apps"]
    resources: ["deployments", "replicasets"]
    verbs: ["get", "list", "watch", "create", "update", "patch", "delete"]
  - apiGroups: [""]
    resources: ["services", "pods", "configmaps", "secrets"]
    verbs: ["get", "list", "watch", "create", "update", "patch", "delete"]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: bordo-reconciler
  namespace: {{.Namespace}}
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: Role
  name: bordo-reconciler
subjects:
  - kind: ServiceAccount
    name: bordo-reconciler
    namespace: bordo-system
---
apiVersion: v1
kind: Secret
metadata:
  name: bordo-reconciler-creds
  namespace: bordo-system
type: Opaque
stringData:
  cluster-token: {{.ClusterToken}}
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: bordo-reconciler
  namespace: bordo-system
spec:
  replicas: 1
  selector:
    matchLabels:
      app: bordo-reconciler
  template:
    metadata:
      labels:
        app: bordo-reconciler
    spec:
      serviceAccountName: bordo-reconciler
      containers:
        - name: reconciler
          image: {{.Image}}
          env:
            - name: BORDO_CP_URL
              value: "{{.CPURL}}"
            - name: BORDO_APPS_NAMESPACE
              value: "{{.Namespace}}"
            - name: BORDO_CLUSTER_TOKEN
              valueFrom:
                secretKeyRef:
                  name: bordo-reconciler-creds
                  key: cluster-token
`))

// readKubeconfig reads the k3s kubeconfig and rewrites the server address from
// the loopback default to an address the control plane can reach.
func readKubeconfig(path, advertiseAddr string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	kc := string(data)
	if advertiseAddr != "" {
		kc = strings.ReplaceAll(kc, "127.0.0.1", advertiseAddr)
		kc = strings.ReplaceAll(kc, "localhost", advertiseAddr)
	}
	return kc, nil
}

// Run executes the full node bootstrap: redeem → install k3s → register.
func Run(ctx context.Context, cfg Config) error {
	httpc := &http.Client{Timeout: 30 * time.Second}

	fmt.Println("[bordo-node] redeeming join token...")
	rr, err := redeem(ctx, httpc, cfg.CPURL, cfg.JoinToken)
	if err != nil {
		return fmt.Errorf("redeem join token: %w", err)
	}
	fmt.Printf("[bordo-node] joined region %q\n", rr.Region)

	if cfg.SkipK3s {
		fmt.Println("[bordo-node] BORDO_NODE_SKIP_K3S=1 — skipping k3s install and registration")
		return nil
	}

	fmt.Printf("[bordo-node] installing k3s %s...\n", cfg.K3sVersion)
	if err := installK3s(ctx, cfg.K3sVersion, cfg.K3sInstallSHA256, cfg.K3sExec); err != nil {
		return fmt.Errorf("install k3s: %w", err)
	}

	kc, err := readKubeconfig(cfg.KubeconfigPath, cfg.AdvertiseAddr)
	if err != nil {
		return fmt.Errorf("read kubeconfig at %s: %w", cfg.KubeconfigPath, err)
	}

	fmt.Println("[bordo-node] registering cluster with control plane...")
	clusterToken, err := register(ctx, httpc, cfg.CPURL, rr.Region, rr.RegisterToken, kc)
	if err != nil {
		return fmt.Errorf("register: %w", err)
	}

	fmt.Println("[bordo-node] deploying the on-cluster reconciler...")
	manifests := renderReconcilerManifests(reconcilerVars{
		CPURL:        cfg.CPURL,
		ClusterToken: clusterToken,
		Image:        cfg.ReconcilerImage,
		Namespace:    cfg.AppsNamespace,
	})
	if err := kubectlApply(ctx, cfg.KubeconfigPath, manifests); err != nil {
		return fmt.Errorf("deploy reconciler: %w", err)
	}

	fmt.Printf("[bordo-node] node registered — region %q is reconciling via the on-cluster agent\n", rr.Region)
	return nil
}

// kubectlApply pipes manifests to `kubectl apply` using the local k3s
// kubeconfig. Used during bootstrap to install the reconciler; after that the
// reconciler applies app manifests itself.
func kubectlApply(ctx context.Context, kubeconfigPath, manifests string) error {
	cmd := exec.CommandContext(ctx, "kubectl", "--kubeconfig", kubeconfigPath, "apply", "-f", "-")
	cmd.Stdin = strings.NewReader(manifests)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
