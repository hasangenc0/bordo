# Desired-State Repository Layout

The Bordo GitOps reconciler watches a git repository that holds Kubernetes manifests
organized by region and project. Any push to this repo triggers a reconcile cycle.

## Structure

```
desired-state/
  us-east/
    my-service/
      deployment.yaml
      service.yaml
  eu-west/
    my-service/
      deployment.yaml
      service.yaml
  ap-south/
    my-service/
      deployment.yaml
```

- **Top-level directory** = region name (must match a region registered with `bordo region add`)
- **Second-level directory** = project name
- **Files** = any valid Kubernetes manifests (`kubectl apply -f` is called recursively)

## How it works

1. `bordo deploy create <project> <image-tag> --region us-east` writes manifests here
   (via `release.WriteManifests`) and triggers a reconcile.
2. The `Reconciler` polls this repo every 30 seconds (configurable), detects new commits,
   and runs `kubectl apply -f <region-dir> --recursive` using the region's kubeconfig.
3. Status (`reconciling` → `healthy` / `failed`) is propagated back to the control plane.

## Generating manifests with Bordo

```bash
# Deploy version 1.2.0 of my-service to us-east
bordo deploy create <project-id> registry.example.com/my-service:1.2.0 --region us-east

# Check rollout status
bordo deploy status <release-id>

# Roll back if needed
bordo deploy rollback <release-id>
```

## Manual manifest authoring

For advanced cases (custom resource types, sidecars, config maps, etc.) you can write
manifests by hand and push them to the desired-state repo. The reconciler picks them up
on the next poll cycle.
