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
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"strings"
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
	CPURL          string // BORDO_CP_URL — control-plane base URL, reachable from this VM
	JoinToken      string // BORDO_JOIN_TOKEN — one-time token from `bordo region add`
	NodeName       string // BORDO_NODE_NAME — defaults to hostname
	K3sVersion     string // BORDO_K3S_VERSION — pinned k3s version
	AdvertiseAddr  string // BORDO_NODE_ADVERTISE_ADDR — address the CP should use to reach the API server
	KubeconfigPath string // BORDO_KUBECONFIG_PATH — defaults to DefaultKubeconfigPath
	SkipK3s        bool   // BORDO_NODE_SKIP_K3S=1 — dry run, skip the actual install (testing)
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// ConfigFromEnv builds a Config from environment variables and validates it.
func ConfigFromEnv() (Config, error) {
	c := Config{
		CPURL:          os.Getenv("BORDO_CP_URL"),
		JoinToken:      os.Getenv("BORDO_JOIN_TOKEN"),
		NodeName:       os.Getenv("BORDO_NODE_NAME"),
		K3sVersion:     envOr("BORDO_K3S_VERSION", DefaultK3sVersion),
		AdvertiseAddr:  os.Getenv("BORDO_NODE_ADVERTISE_ADDR"),
		KubeconfigPath: envOr("BORDO_KUBECONFIG_PATH", DefaultKubeconfigPath),
		SkipK3s:        os.Getenv("BORDO_NODE_SKIP_K3S") == "1",
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
// single-use register token.
func register(ctx context.Context, httpc *http.Client, cpURL, region, registerToken, kubeconfig string) error {
	return postJSON(ctx, httpc, cpURL+"/fleet/register", map[string]string{
		"region":         region,
		"register_token": registerToken,
		"kubeconfig":     kubeconfig,
	}, nil)
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
func installK3s(ctx context.Context, version string) error {
	script := `set -e
curl -sfL https://get.k3s.io -o /tmp/k3s-install.sh
INSTALL_K3S_VERSION="` + version + `" sh /tmp/k3s-install.sh --write-kubeconfig-mode 644
rm -f /tmp/k3s-install.sh`
	cmd := exec.CommandContext(ctx, "sh", "-c", script)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

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
	if err := installK3s(ctx, cfg.K3sVersion); err != nil {
		return fmt.Errorf("install k3s: %w", err)
	}

	kc, err := readKubeconfig(cfg.KubeconfigPath, cfg.AdvertiseAddr)
	if err != nil {
		return fmt.Errorf("read kubeconfig at %s: %w", cfg.KubeconfigPath, err)
	}

	fmt.Println("[bordo-node] registering cluster with control plane...")
	if err := register(ctx, httpc, cfg.CPURL, rr.Region, rr.RegisterToken, kc); err != nil {
		return fmt.Errorf("register: %w", err)
	}
	fmt.Printf("[bordo-node] node registered — region %q is ready for deploys\n", rr.Region)
	return nil
}
