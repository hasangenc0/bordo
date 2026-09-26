# examples/

End-to-end example applications and demo scripts.

## Planned examples (EP-12)

| Directory | Description | Issue |
|---|---|---|
| `build-mvp-demo.sh` | Shell script: new java service → scaffold → build → artifact | BRD-008 |
| `java-web-service-demo/` | Expanded java-web-service template + expected output | BRD-090 |

## build-mvp-demo.sh (planned)

This is the canonical Build MVP demonstration script. When BRD-008 is complete,
running this script on a machine with the control plane and Docker available should
produce a working OCI image.

```bash
#!/usr/bin/env bash
# Full Build MVP demo
# Prerequisites: bordod running, Docker daemon, local registry on :5000

set -euo pipefail

echo "=== Bordo Build MVP Demo ==="
./bin/bordo project create demo-api --template java-web-service
./bin/bordo build trigger demo-api --context /tmp/demo-api
./bin/bordo build list demo-api
docker pull localhost:5000/demo-api:latest
echo "✓ Demo complete"
```

## Status

🚧 Examples not yet created — see BRD-008 and BRD-090.
