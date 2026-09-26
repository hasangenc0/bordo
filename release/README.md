# release/

The Bordo Release layer — GitOps reconciler and progressive delivery.

## What it will do (EP-05)

- Write desired state to a GitOps repository when `bordo deploy` is called (BRD-020)
- Reconcile each regional cluster to match desired state (BRD-020)
- Progressive delivery: canary and blue-green strategies (BRD-021)
- Multi-region rollout with health-check gates (BRD-021)
- Rollback to a previous version (BRD-021)

## Planned issues

- **BRD-020** — GitOps reconciler & desired-state repo layout
- **BRD-021** — Progressive delivery (canary / blue-green)

## Dependencies

Depends on EP-03 (fleet / k3s clusters) and EP-04 (container images to deploy).

## Design sketch

```
release/
  internal/
    reconciler.go    Watches git repo, applies manifests to k3s clusters
    strategy/
      rolling.go     Default rolling update
      canary.go      Canary traffic splitting
      bluegreen.go   Blue-green deployment
    gitops/
      repo.go        Git operations (clone, commit, push, pull)
      layout.go      Desired-state directory layout
```

Open question: external git repo vs embedded — see `.memory/open-questions.md` Q2.
