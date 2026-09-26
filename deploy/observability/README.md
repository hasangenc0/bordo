# Bordo Observability Stack

Deployed automatically to each regional k3s cluster when a region is added (`bordo region add`).

## Components

| Component | Image | Purpose |
|---|---|---|
| OTel Collector | `otel/opentelemetry-collector-contrib:0.107.0` | DaemonSet; receives OTLP gRPC (:4317) + HTTP (:4318), fans out to all backends |
| VictoriaMetrics | `victoriametrics/victoria-metrics:v1.103.0` | Metrics storage; receives remote_write from OTel Collector |
| Loki | `grafana/loki:3.2.0` | Log aggregation; receives logs from OTel Collector |
| Tempo | `grafana/tempo:2.6.0` | Distributed tracing; receives traces via OTLP |

All components run in the `bordo-observe` namespace.

## Manual deploy

```bash
# Apply to a cluster
kubectl apply -k deploy/observability/ --kubeconfig <path-to-kubeconfig>

# Or with bordo CLI (automatic on region add):
bordo region add --name us-east --host <ip> --key <ssh-key>
```

## Environment variables for control-plane

Set these to enable the observe query API:

```
BORDO_VM_URL=http://<node-ip>:8428
BORDO_LOKI_URL=http://<node-ip>:3100
BORDO_TEMPO_URL=http://<node-ip>:3200
```

Port-forward for local access:
```bash
kubectl port-forward -n bordo-observe svc/victoriametrics 8428:8428
kubectl port-forward -n bordo-observe svc/loki 3100:3100
kubectl port-forward -n bordo-observe svc/tempo 3200:3200
```

## App instrumentation

Apps scaffolded from Bordo templates (java-web-service, java-worker, etc.) include OTel
auto-instrumentation. They send telemetry to the OTel Collector DaemonSet on the node
(`OTEL_EXPORTER_OTLP_ENDPOINT=http://$(NODE_IP):4318`).
