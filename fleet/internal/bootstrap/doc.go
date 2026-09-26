// Package bootstrap will implement the k3s bootstrap agent.
//
// Responsibilities (BRD-010):
//   - SSH to a target VM using a provided key
//   - Detect OS and use the appropriate package manager
//   - Install Docker and k3s
//   - Retrieve /etc/rancher/k3s/k3s.yaml (the cluster kubeconfig)
//   - Return the kubeconfig to the control plane for encrypted storage
//   - Stream step-by-step progress to the caller
//
// See tracker/backlog/BRD-010-k3s-bootstrap-agent.md.
package bootstrap
