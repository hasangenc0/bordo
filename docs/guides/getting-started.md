# Getting Started with Bordo

Bordo is a self-hostable software factory. This guide walks you through building the
binaries, running the control plane locally, and completing the Build MVP flow:
**create a project → scaffold a Java service → trigger a container build**.

---

## Prerequisites

- **Go 1.22+** — `go version` should print `go1.22` or newer
- **Git**
- **Docker 24+** — needed for container builds (optional for the first steps)
- macOS or Linux (Windows: use WSL2)

Check:

```bash
go version    # go version go1.22.x ...
docker info   # should not error
```

---

## 1. Clone and build

```bash
git clone https://github.com/bordo-io/bordo.git
cd bordo

# Build all three binaries into ./bin/
make build
```

Expected output:

```
→ building bordod (control-plane)
→ building bordo (CLI)
→ building bordo-fleet (fleet agent)
```

Add `./bin` to your PATH for this session:

```bash
export PATH="$PWD/bin:$PATH"
```

Verify:

```bash
bordod version
# Bordo control-plane dev (commit: none, built: ...)

bordo version
# bordo CLI dev
```

---

## 2. Start the control plane

```bash
bordod serve
```

Expected output:

```
time=... level=INFO msg="bordo control-plane starting" version=dev port=7401
time=... level=INFO msg="database migrated" migrations=1
time=... level=INFO msg="listening" addr=:7401
```

The control plane listens on **port 7401**. It stores state in `~/.bordo/bordod.db`
(SQLite, created automatically on first run).

Health check (in a second terminal):

```bash
curl -s http://localhost:7401/healthz | jq .
# { "status": "ok" }

curl -s http://localhost:7401/version | jq .
# { "version": "dev", "commit": "none", "build_time": "..." }
```

To run with human-readable logs:

```bash
bordod serve --log-format text
```

To use a custom config file:

```bash
bordod serve --config /path/to/bordod.yaml
```

---

## 3. Configure the CLI

Point the CLI at your running control plane:

```bash
bordo config set server http://localhost:7401
```

Verify:

```bash
bordo config get server
# http://localhost:7401
```

The CLI stores its config in `~/.bordo/config.yaml`.

---

## 4. Create your first project

```bash
bordo project create hello-service --template java-web-service
```

Expected output:

```
created project hello-service (id: a1b2c3d4-...)
```

List all projects:

```bash
bordo project list
# ID         NAME            TEMPLATE            STATUS
# --------------------------------------------------------
# a1b2c3d4   hello-service   java-web-service    created
```

Get project details:

```bash
bordo project get hello-service
# id:         a1b2c3d4-...
# name:       hello-service
# template:   java-web-service
# status:     created
# git_repo:
# created_at: 2026-09-26T...
```

---

## 5. Scaffold from a template

Expand the `java-web-service` golden-path template into a directory:

```bash
bordo template expand java-web-service \
  --var "ProjectName=hello-service" \
  --var "GroupId=com.example" \
  --output /tmp/hello-service
```

Expected output:

```
✓ expanded java-web-service → /tmp/hello-service (8 files)
```

Inspect what was generated:

```bash
ls /tmp/hello-service
# Dockerfile  mvnw  pom.xml  src/  template.yaml
```

The scaffold includes:
- `pom.xml` — Spring Boot 3.3, Java 25, OTel, actuator, Prometheus
- `Dockerfile` — multi-stage Maven build → Temurin JRE runtime
- `src/main/java/com/example/Application.java` — Spring Boot entry point
- `src/main/java/com/example/controller/HelloController.java` — `GET /api/v1/hello`
- `src/main/resources/application.yaml` — port 8080, actuator, OTel tracing

---

## 6. Build and push a container image

> Requires Docker running and a local OCI registry. Start one with:
> ```bash
> docker run -d -p 5000:5000 --name bordo-registry registry:2
> ```

Trigger a build:

```bash
bordo build trigger hello-service --registry localhost:5000 --tag v0.1.0
```

Expected output:

```
build triggered: a1b2c3d4 (status: pending)
```

Check build status:

```bash
bordo build list hello-service
# ID         STATUS     IMAGE                          TAG
# --------------------------------------------------------
# a1b2c3d4   queued     localhost:5000/hello-service   v0.1.0
```

View build logs:

```bash
bordo build logs hello-service
```

> **Note**: The build orchestrator (`build/`) calls `docker buildx build` and pushes to
> the specified registry. Docker must be running on the same machine as `bordod`.

---

## 7. (Optional) Bootstrap a k3s region

> Requires a Linux VM reachable by SSH with `sudo` access.

```bash
bordo-fleet region add \
  --name us-east-1 \
  --host 203.0.113.42 \
  --user ubuntu \
  --key ~/.ssh/id_ed25519
```

This SSHes into the VM, installs k3s, and registers the region with the control plane.

List regions:

```bash
# via the control-plane API
curl -s http://localhost:7401/v1/regions | jq .
```

---

## Next steps

- Explore all templates: `bordo template list`
- Read the layer concepts: [`docs/concepts/`](../concepts/)
- Run the end-to-end demo: `bash examples/build-mvp-demo.sh`
- Check what's in progress: [`tracker/BOARD.md`](../../tracker/BOARD.md)
- Add a region and deploy: [`tracker/backlog/BRD-012-multi-region-fleet-controller.md`](../../tracker/backlog/BRD-012-multi-region-fleet-controller.md)

---

## Troubleshooting

**`bordod: command not found`**  
Run `export PATH="$PWD/bin:$PATH"` from the repo root, or use `./bin/bordod`.

**`dial tcp 127.0.0.1:7401: connect refused`**  
`bordod serve` is not running. Start it in a separate terminal.

**`config file not found`**  
Run `bordo config set server http://localhost:7401` to create `~/.bordo/config.yaml`.

**`project not found`**  
Use the exact project name or the full UUID. `bordo project list` shows both.

**`docker: command not found` during build**  
Install Docker Desktop (macOS) or Docker Engine (Linux). The build orchestrator
requires Docker to be running locally.

**SQLite `database is locked`**  
Only one `bordod` instance can run per database file. Check for a stale process:
`lsof ~/.bordo/bordod.db`.

**Port 7401 already in use**  
Override: `bordod serve --port 7402` (or set `BORDO_SERVER_PORT=7402`).
