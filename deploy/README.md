# deploy/

Kubernetes manifests and Helm charts for deploying Bordo itself.

## Structure (planned)

```
deploy/
  helm/
    bordo/             Helm chart for the Bordo control plane
      Chart.yaml
      values.yaml
      templates/
        deployment.yaml
        service.yaml
        configmap.yaml
        ...
  manifests/
    bordo-namespace.yaml
    bordo-rbac.yaml
    ...
```

## Usage (planned)

```bash
# Install Bordo on an existing k8s cluster
helm install bordo deploy/helm/bordo \
  --namespace bordo-system \
  --create-namespace \
  --set image.tag=v0.1.0
```

## Status

🚧 Helm chart and manifests not yet created. Bordo currently runs as a standalone
binary (`./bin/bordod serve`). Kubernetes-native deployment is a later milestone.
