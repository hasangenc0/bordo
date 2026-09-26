// Package bootstrap implements k3s installation on a remote VM over SSH.
package bootstrap

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
)

// Config holds SSH and region parameters for bootstrapping a k3s node.
type Config struct {
	Host       string // hostname or IP
	Port       int    // SSH port (default 22)
	User       string // SSH user (default "root")
	KeyPath    string // path to private key file on disk
	RegionName string // logical region name
}

// Result is returned after a successful bootstrap.
type Result struct {
	NodeName   string
	Kubeconfig string // raw kubeconfig YAML, server address patched to cfg.Host
	Duration   time.Duration
}

// Bootstrap SSHes into the host and:
//  1. Detects the OS
//  2. Installs k3s via the official installer
//  3. Waits for the node to become Ready
//  4. Retrieves the kubeconfig
//
// Progress lines are written to out as each step completes.
func Bootstrap(ctx context.Context, cfg Config, out io.Writer) (*Result, error) {
	if cfg.Port == 0 {
		cfg.Port = 22
	}
	if cfg.User == "" {
		cfg.User = "root"
	}

	start := time.Now()

	client, err := dialSSH(cfg)
	if err != nil {
		return nil, fmt.Errorf("SSH connect to %s@%s:%d: %w", cfg.User, cfg.Host, cfg.Port, err)
	}
	defer client.Close()

	fmt.Fprintf(out, "[bootstrap] connected to %s@%s:%d\n", cfg.User, cfg.Host, cfg.Port)

	osID, err := runCmd(client, "cat /etc/os-release 2>/dev/null | grep '^ID=' | cut -d= -f2 | tr -d '\"'")
	if err != nil {
		return nil, fmt.Errorf("detecting OS: %w", err)
	}
	osID = strings.TrimSpace(osID)
	fmt.Fprintf(out, "[bootstrap] detected OS: %s\n", osID)

	if err := installDeps(client, osID, out); err != nil {
		return nil, fmt.Errorf("installing dependencies: %w", err)
	}

	fmt.Fprintf(out, "[bootstrap] installing k3s...\n")
	if _, err := runCmd(client, "curl -sfL https://get.k3s.io | sh -s - --write-kubeconfig-mode 644"); err != nil {
		return nil, fmt.Errorf("installing k3s: %w", err)
	}
	fmt.Fprintf(out, "[bootstrap] k3s installed\n")

	if err := waitForNode(ctx, client, out); err != nil {
		return nil, fmt.Errorf("waiting for k3s node: %w", err)
	}

	kubeconfig, err := runCmd(client, "cat /etc/rancher/k3s/k3s.yaml")
	if err != nil {
		return nil, fmt.Errorf("reading kubeconfig: %w", err)
	}
	// k3s defaults to 127.0.0.1; patch to the real host address.
	kubeconfig = strings.ReplaceAll(kubeconfig, "127.0.0.1", cfg.Host)

	nodeName, _ := runCmd(client, "kubectl --kubeconfig /etc/rancher/k3s/k3s.yaml get nodes -o jsonpath='{.items[0].metadata.name}'")
	nodeName = strings.TrimSpace(nodeName)
	fmt.Fprintf(out, "[bootstrap] node %s is Ready\n", nodeName)

	return &Result{
		NodeName:   nodeName,
		Kubeconfig: kubeconfig,
		Duration:   time.Since(start),
	}, nil
}

func installDeps(client *ssh.Client, osID string, out io.Writer) error {
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
	fmt.Fprintf(out, "[bootstrap] installing deps for %s\n", osID)
	_, err := runCmd(client, cmd)
	return err
}

func waitForNode(ctx context.Context, client *ssh.Client, out io.Writer) error {
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
			result, err := runCmd(client, "kubectl --kubeconfig /etc/rancher/k3s/k3s.yaml get nodes --no-headers 2>/dev/null | grep -c Ready || echo 0")
			if err == nil && strings.TrimSpace(result) != "0" {
				return nil
			}
			fmt.Fprintf(out, "[bootstrap] waiting for node to become Ready...\n")
		}
	}
}

// runCmd runs a command on the SSH client and returns combined output.
func runCmd(client *ssh.Client, cmd string) (string, error) {
	sess, err := client.NewSession()
	if err != nil {
		return "", fmt.Errorf("new session: %w", err)
	}
	defer sess.Close()
	out, err := sess.CombinedOutput(cmd)
	return string(out), err
}
