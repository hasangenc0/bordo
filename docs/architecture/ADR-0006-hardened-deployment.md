# ADR-0006: Hardened Deployment — Pull-Based GitOps, Scoped Credentials, No Central Admin Secrets

**Status:** Proposed
**Date:** 2026-10-21
**Deciders:** Project founders
**Supersedes (in part):** the credential/bootstrap mechanics of [ADR-0004](ADR-0004-k3s-any-vm-substrate.md)

## Context

ADR-0004 established k3s-on-any-VM as the substrate. The first implementation of the
bootstrap + deploy path is a **push model**:

1. The operator hands bordod an **SSH private key** for the target VM.
2. bordod SSHes in and installs k3s via `curl -sfL https://get.k3s.io | sh` (unpinned).
3. bordod pulls back the cluster's **admin kubeconfig** and stores it.
4. Deploys happen by bordod running `kubectl apply` (push) with that admin kubeconfig.
5. Private image pulls use the operator's broad GitHub user token as a cluster secret.

This works and is a fine MVP, but it concentrates the platform's entire security risk
in one place. The control plane ends up holding, long-lived and in one SQLite file
(encrypted with a single `.secret_key`):

- SSH **private keys** with root-capable access to every VM in the fleet, and
- cluster-**admin** kubeconfigs for every region.

**Blast radius:** compromise of bordod — a box that also fronts the agent and console —
is compromise of every VM and every workload in the fleet. The credentials are broad
(root / cluster-admin), long-lived (no rotation), and centralized (one store). The
bootstrap is also an unpinned `curl | sh`.

For a solo-builder lab this is acceptable; for anything real it is not. This ADR defines
the hardened model and a phased migration.

## Decision

Move from a **push** model (bordod reaches into VMs and clusters with stored broad
secrets) to a **pull** model (VMs and clusters reach out to the control plane with
short-lived, scoped, revocable credentials). Four pillars:

### 1. Outbound node-join bootstrap — bordod stores no SSH keys

Replace "bordod SSHes in with a stored private key" with an **operator-run, one-time
join**:

- `bordo region add <name>` mints a **single-use, short-TTL join token** (e.g. 15 min,
  scoped to "register one node in region X") and prints a one-line install command.
- The operator runs that command **on the VM** (they already have access to it). It
  installs k3s (pinned + verified, see pillar 4) and a small **node agent**.
- The node agent calls the control plane **outbound** over TLS, redeems the join token,
  and receives a scoped, renewable node credential. It never accepts inbound SSH.

Result: bordod never holds an SSH private key, needs no inbound reach into VMs, and works
behind NAT. Revoking a node is revoking its token — no fleet-wide key rotation.

### 2. Pull-based GitOps — bordod stores no admin kubeconfig

Replace "bordod `kubectl apply` pushes" with **on-cluster reconciliation**:

- Desired state (rendered manifests) is published by the control plane to a per-region
  **desired-state feed** (a git repo, or a signed API feed).
- An **on-cluster reconciler** (Flux/Argo, or Bordo's own agent) **pulls** desired state
  and applies it locally using **in-cluster RBAC**.
- bordod only *publishes* desired state; it never holds a cluster-admin kubeconfig and
  cannot `kubectl` into any cluster. Compromising bordod cannot directly touch workloads.

Bordo already has a GitOps reconciler concept (`release/internal/reconciler`), but today
it runs *from* bordod. This pillar moves it *onto* the cluster.

### 3. Least privilege everywhere

- **Deploy identity:** a service account scoped to the `bordo-apps` namespace, not
  cluster-admin.
- **Image pulls:** a dedicated, fine-grained **`read:packages`-only** token (ideally
  minted short-lived per deploy), not the operator's broad GitHub user token.
- **Secrets at rest:** envelope-encrypt bordod's settings; keep `.secret_key` on the
  persistent data volume with `0600`; roadmap to an external KMS / secrets manager so the
  master key is not on the same disk as the ciphertext.

### 4. Verified bootstrap (supply chain)

- Pin the k3s version and **verify the installer checksum/signature** instead of blind
  `curl | sh`. Bordo ships an install script with a pinned version + `sha256` check.

### Trust boundary summary

| Secret | Today (push) | Hardened (pull) |
|---|---|---|
| SSH private key | stored in bordod, long-lived, root | **not stored** — operator runs join on the VM |
| Cluster access | admin kubeconfig in bordod | in-cluster RBAC; bordod holds none |
| Node identity | n/a | short-lived join token → renewable scoped node cred |
| Deploy scope | cluster-admin | `bordo-apps` namespace SA |
| GHCR pull | operator's broad user token | dedicated `read:packages` token, short-lived |
| Bootstrap | unpinned `curl \| sh` | pinned + checksum-verified |

## Alternatives considered

- **Keep the push model, just encrypt harder.** Rejected as the primary path: it reduces
  the chance of key theft but not the blast radius — bordod still holds fleet-wide root.
- **Bring-your-own-kubeconfig only.** Simpler, but still centralizes admin kubeconfigs in
  bordod and drops the "any VM" bootstrap story. Kept as a supported *input* to pillar 2.
- **Full service mesh / SPIFFE identities.** The most rigorous, but far too heavy for the
  solo-builder target. Revisit if Bordo grows into multi-tenant SaaS.

## Phasing

The pillars are independently shippable; order by risk-reduction-per-effort:

- **Phase 0 (today):** SSH push + admin kubeconfig. Keep behind a flag as a fallback.
- **Phase 1 — outbound join (biggest win per effort):** node agent + single-use join
  tokens. **Eliminates stored SSH keys and inbound SSH.**
- **Phase 2 — pull reconciler:** on-cluster reconciler + namespace-scoped SA.
  **Eliminates stored admin kubeconfigs.**
- **Phase 3 — scoped creds + supply chain:** `read:packages` pull tokens, pinned/verified
  k3s installer, external KMS for the settings master key.

## Consequences

**Positive**
- Blast radius drops from "the whole fleet" to "one region's scoped, revocable token."
- bordod stops being a vault of SSH keys and admin kubeconfigs.
- Outbound-only node registration works behind NAT and needs no inbound firewall holes.
- Per-region revocation without fleet-wide rotation.
- Supply-chain-verified bootstrap.

**Negative / costs**
- More moving parts: a node agent and an on-cluster reconciler to build and maintain.
- The node agent is new attack surface (mitigated: outbound-only, scoped creds).
- GitOps introduces a desired-state store to operate.
- More upfront work than the current push model.

**Tradeoff for the solo-builder target:** Phase 1 alone removes the most dangerous
secret (fleet-wide SSH keys) for modest effort and should be done first. Phase 2 is the
larger lift and can follow once the deploy path is otherwise proven.

## Status note

Implementation progress (code-complete + unit-tested unless noted; the live k3s/cluster
paths are isolated and gated, pending validation on a real VM):

- **Phase 0:** implemented (SSH push + admin kubeconfig) — remains the fallback.
- **Phase 1 (outbound join):** join-token core, mint/redeem/register endpoints,
  `bordo region` CLI, and the `bordo-node` agent. Eliminates stored SSH keys.
- **Phase 2 (pull reconciler):** cluster credential, `/fleet/desired-state` feed,
  `release.DesiredState`, the `bordo-reconciler` on-cluster agent, and namespace-scoped
  RBAC installed at join. The control plane has no push path once a region reconciles.
- **Phase 3 (partial):** k3s installer is checksum-verified with a pinned version. Two
  items remain and are intentionally **not** rushed:
  - *Scoped `read:packages` GHCR tokens* require a GitHub App installation (GitHub does
    not let a device-flow user token be programmatically scoped down), so this depends on
    supporting an App-based pull identity, not just more code.
  - *External KMS for the settings master key* needs real KMS infrastructure plus a
    careful re-encryption migration of existing secrets.

**Gating milestone:** a single live VM run must validate Phases 1+2 end to end
(`bordo region add` → run the node command on a VM → k3s installs, node registers,
reconciler pulls and deploys) before any of this is called production-ready.
