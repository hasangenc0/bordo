# Getting Started with Bordo

> ⚠️ Bordo is in early development. This guide describes the target experience.
> Track progress in [`tracker/BOARD.md`](../../tracker/BOARD.md).

## Prerequisites

- A Linux VM (or your local machine) to run the control plane
- At least one additional Linux VM reachable via SSH (for your first region)
- Go 1.22+ (to build from source)
- Docker on the region VMs

## 1. Build Bordo

```bash
git clone https://github.com/your-org/bordo.git
cd bordo
make build
export PATH="$PATH:$PWD/bin"
```

## 2. Start the control plane

```bash
bordod serve
# Listening on :7400 (gRPC) and :7401 (HTTP / REST gateway)
# State: ~/.bordo/bordod.db (SQLite)
```

## 3. Log in with the CLI

```bash
bordo login --server localhost:7401
# Opens browser for OAuth, or use --token for API token auth
```

## 4. Add your first region

```bash
bordo region add \
  --name my-region \
  --host <VM_IP> \
  --key ~/.ssh/id_ed25519 \
  --user ubuntu
```

Bordo will SSH into the VM, install k3s, and register the region. Check status:

```bash
bordo region list
# NAME        STATUS    NODES  CLUSTERS  UPDATED
# my-region   healthy   1      1         just now
```

## 5. Create your first project

```bash
bordo project create my-api \
  --template java-web-service \
  --region my-region
```

This scaffolds a Java Spring Boot service, registers it in the catalog, and triggers
an initial build.

## 6. Watch the build

```bash
bordo build logs my-api
# [builder] Pulling base image...
# [builder] Compiling...
# [builder] Pushing image to registry...
# ✓ Build complete: registry.bordo.local/my-api:abc123
```

## 7. Deploy it

```bash
bordo deploy my-api latest --region my-region
```

## 8. Open the console

```bash
open http://localhost:7401
```

Chat with the agent: `show me the status of my-api` or `deploy my-api to my-region`.

---

## Next steps

- Add more regions: `bordo region add --name eu-west-1 ...`
- Explore templates: `bordo template list`
- Read the concepts: [`docs/concepts/`](../concepts/)
- Contribute: [`CONTRIBUTING.md`](../../CONTRIBUTING.md)
