# ADR-0004: k3s on Any VM as the Substrate

**Status:** Accepted  
**Date:** 2026-09-26  
**Deciders:** Project founders

## Context

The platform must be "multi-region ready and infinitely scalable, deployable to any VM
in any region." We need to decide on the runtime substrate for user workloads.

Options range from bare-metal process management, through Docker Compose, through full
Kubernetes. The "any VM" constraint means we cannot assume a managed Kubernetes service
(EKS, GKE, AKS).

## Decision

Bordo will use **k3s** (a lightweight, certified Kubernetes distribution by Rancher/SUSE)
as the workload substrate in every region. The fleet bootstrap agent (a Go process named
`bordo-fleet`) SSHes into a target VM, installs k3s and containerd, and registers the
resulting cluster with the Bordo control plane.

Each region is one k3s cluster (single-node or multi-node). The Bordo control plane
maintains a fleet of these clusters and orchestrates cross-region operations.

Why k3s specifically:
- Single binary (~60 MB), installs in ~30 seconds on a stock VM
- Certified Kubernetes — all k8s tooling works (Helm, kubectl, Flux, etc.)
- Built-in SQLite state (no etcd for small clusters, can switch to etcd for large ones)
- Active upstream support (CNCF sandbox project, widely used)

## Alternatives considered

- **Full Kubernetes (kubeadm)** — heavier, requires more VMs, not "any VM" friendly
- **Docker Compose** — simpler but no multi-node scheduling, no Kubernetes ecosystem
- **Nomad** — good alternative, but smaller ecosystem and less tooling
- **Bare-metal processes** — maximum simplicity but no container isolation, no
  scheduling, no horizontal scaling story
- **Bring-your-own-k8s (kubeconfig)** — easier initial implementation but loses the
  "any VM" guarantee. Kept as a future option (BRD-011 may add BYOK support later)

## Consequences

- Bordo workloads run as standard Kubernetes workloads — any k8s knowledge transfers.
- The fleet bootstrap requires SSH access to target VMs; this is a security boundary
  to manage carefully (key management is addressed in EP-11).
- k3s clusters can grow from 1 to N nodes by bootstrapping additional VMs in the
  same region.
- The control plane uses the Kubernetes Go client (`client-go`) to manage workloads
  in each regional cluster.
- Future: a `bordo region add --kubeconfig <path>` path for users who already have k8s.
