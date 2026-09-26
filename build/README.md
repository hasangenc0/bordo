# build/

The Bordo Build layer — template engine and container build orchestrator.

## What it will do (EP-04)

- Template engine: expand golden-path templates into project skeletons (BRD-005)
- Container builder: run BuildKit or Kaniko to produce OCI images (BRD-007)
- Build job orchestrator: manage build jobs in the k3s cluster or local Docker (BRD-007)

## Planned issues

- **BRD-005** — Template engine + golden-path template contract
- **BRD-006** — java-web-service golden-path template
- **BRD-007** — Container build (BuildKit) → OCI image + push to registry
- **BRD-008** — Build MVP demo end-to-end

## Implementation direction

```
build/
  internal/
    template/
      engine.go      Template expander (text/template)
      loader.go      Load template from filesystem or embedded FS
      manifest.go    template.yaml Go struct
    container/
      builder.go     Builder interface
      local.go       Docker socket implementation (dev mode)
      k8s.go         BuildKit Kubernetes Job (prod mode)
```

The build package is imported by the control plane, not deployed separately.
