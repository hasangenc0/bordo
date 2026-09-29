package fleet

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
)

// BootstrapConfig holds SSH and region parameters for bootstrapping a k3s node.
type BootstrapConfig struct {
	Host       string // hostname or IP
	Port       int    // SSH port (default 22)
	User       string // SSH user (default "root")
	SSHKey     string // PEM-encoded private key content
	RegionName string // logical region name
}

// bootstrapK3s SSHes into the host and installs k3s, returning the kubeconfig YAML.
func bootstrapK3s(ctx context.Context, cfg BootstrapConfig, out io.Writer) (string, error) {
	if cfg.Port == 0 {
		cfg.Port = 22
	}
	if cfg.User == "" {
		cfg.User = "root"
	}

	client, err := dialSSHFromPEM(cfg)
	if err != nil {
		return "", fmt.Errorf("SSH connect to %s@%s:%d: %w", cfg.User, cfg.Host, cfg.Port, err)
	}
	defer client.Close()

	fmt.Fprintf(out, "[bootstrap] connected to %s@%s:%d\n", cfg.User, cfg.Host, cfg.Port)

	osID, err := sshRun(client, "cat /etc/os-release 2>/dev/null | grep '^ID=' | cut -d= -f2 | tr -d '\"'")
	if err != nil {
		return "", fmt.Errorf("detecting OS: %w", err)
	}
	osID = strings.TrimSpace(osID)
	fmt.Fprintf(out, "[bootstrap] detected OS: %s\n", osID)

	if err := installCurl(client, osID, out); err != nil {
		return "", fmt.Errorf("installing curl: %w", err)
	}

	fmt.Fprintf(out, "[bootstrap] installing k3s...\n")
	if _, err := sshRun(client, "curl -sfL https://get.k3s.io | sh -s - --write-kubeconfig-mode 644"); err != nil {
		return "", fmt.Errorf("installing k3s: %w", err)
	}
	fmt.Fprintf(out, "[bootstrap] k3s installed\n")

	if err := waitNodeReady(ctx, client, out); err != nil {
		return "", fmt.Errorf("waiting for node ready: %w", err)
	}

	kubeconfig, err := sshRun(client, "cat /etc/rancher/k3s/k3s.yaml")
	if err != nil {
		return "", fmt.Errorf("reading kubeconfig: %w", err)
	}
	kubeconfig = strings.ReplaceAll(kubeconfig, "127.0.0.1", cfg.Host)

	nodeName, _ := sshRun(client, "kubectl --kubeconfig /etc/rancher/k3s/k3s.yaml get nodes -o jsonpath='{.items[0].metadata.name}'")
	fmt.Fprintf(out, "[bootstrap] node %s is Ready\n", strings.TrimSpace(nodeName))

	return kubeconfig, nil
}

func dialSSHFromPEM(cfg BootstrapConfig) (*ssh.Client, error) {
	signer, err := ssh.ParsePrivateKey([]byte(cfg.SSHKey))
	if err != nil {
		return nil, fmt.Errorf("parsing private key: %w", err)
	}

	sshCfg := &ssh.ClientConfig{
		User:            cfg.User,
		Auth:            []ssh.AuthMethod{ssh.PublicKeys(signer)},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(), //nolint:gosec // user-supplied key, warn in docs
		Timeout:         30 * time.Second,
	}

	addr := fmt.Sprintf("%s:%d", cfg.Host, cfg.Port)
	return ssh.Dial("tcp", addr, sshCfg)
}

func installCurl(client *ssh.Client, osID string, out io.Writer) error {
	var cmd string
	switch osID {
	case "ubuntu", "debian":
		cmd = "export DEBIAN_FRONTEND=noninteractive && apt-get update -q && apt-get install -yq curl"
	case "rhel", "centos", "fedora", "rocky", "almalinux":
		cmd = "yum install -y curl"
	case "alpine":
		cmd = "apk add --no-cache curl"
	default:
		fmt.Fprintf(out, "[bootstrap] unknown OS %q, skipping dep install\n", osID)
		return nil
	}
	fmt.Fprintf(out, "[bootstrap] installing curl for %s\n", osID)
	_, err := sshRun(client, cmd)
	return err
}

func waitNodeReady(ctx context.Context, client *ssh.Client, out io.Writer) error {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	deadline := time.NewTimer(5 * time.Minute)
	defer deadline.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-deadline.C:
			return fmt.Errorf("timed out waiting for node to become Ready")
		case <-ticker.C:
			result, err := sshRun(client, "kubectl --kubeconfig /etc/rancher/k3s/k3s.yaml get nodes --no-headers 2>/dev/null | grep -c Ready || echo 0")
			if err == nil && strings.TrimSpace(result) != "0" {
				return nil
			}
			fmt.Fprintf(out, "[bootstrap] waiting for node to become Ready...\n")
		}
	}
}

func sshRun(client *ssh.Client, cmd string) (string, error) {
	sess, err := client.NewSession()
	if err != nil {
		return "", fmt.Errorf("new session: %w", err)
	}
	defer sess.Close()
	out, err := sess.CombinedOutput(cmd)
	return string(out), err
}
