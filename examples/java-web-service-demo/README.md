# Example: Java Web Service — Build MVP

This example walks through the complete Build layer flow using the
`java-web-service` golden-path template:

```
create project → scaffold template → docker build → push artifact → run locally
```

---

## Prerequisites

- Bordo built: `make build` (produces `./bin/bordod` and `./bin/bordo`)
- Docker running
- A local OCI registry (or any accessible registry)
- `curl`, `jq` (optional but used in examples)

---

## Setup

**1. Start a local registry** (skip if you have one):

```bash
docker run -d -p 5000:5000 --name bordo-registry registry:2
```

**2. Start the control plane** (in a separate terminal):

```bash
./bin/bordod serve --log-format text
```

**3. Configure the CLI**:

```bash
./bin/bordo config set server http://localhost:7401
```

**4. Verify health**:

```bash
curl -s http://localhost:7401/healthz
# {"status":"ok"}
```

---

## Step-by-step walkthrough

### Create a project

```bash
./bin/bordo project create hello-service --template java-web-service
```

Output:
```
created project hello-service (id: a1b2c3d4-e5f6-...)
```

### Scaffold the template

```bash
./bin/bordo template expand java-web-service \
  --var "ProjectName=hello-service" \
  --var "GroupId=com.example" \
  --output /tmp/hello-service
```

Output:
```
✓ expanded java-web-service → /tmp/hello-service (8 files)
```

Inspect:
```bash
find /tmp/hello-service -type f
# /tmp/hello-service/pom.xml
# /tmp/hello-service/Dockerfile
# /tmp/hello-service/mvnw
# /tmp/hello-service/src/main/java/com/example/Application.java
# /tmp/hello-service/src/main/java/com/example/controller/HelloController.java
# /tmp/hello-service/src/main/resources/application.yaml
# /tmp/hello-service/src/test/java/com/example/ApplicationTests.java
# /tmp/hello-service/template.yaml
```

### Build the container image

```bash
./bin/bordo build trigger hello-service \
  --registry localhost:5000 \
  --tag v0.1.0
```

Output:
```
build triggered: a1b2c3d4 (status: pending)
```

Check status:
```bash
./bin/bordo build list hello-service
# ID         STATUS     IMAGE                          TAG
# --------------------------------------------------------
# a1b2c3d4   queued     localhost:5000/hello-service   v0.1.0
```

### Run the built image

Once the build completes and the image is pushed to your local registry:

```bash
docker pull localhost:5000/hello-service:v0.1.0
docker run -d --name hello-service -p 8080:8080 localhost:5000/hello-service:v0.1.0
```

### Test the service

```bash
curl http://localhost:8080/api/v1/hello
# {"message":"hello from hello-service"}

curl http://localhost:8080/actuator/health
# {"status":"UP"}
```

### Cleanup

```bash
docker stop hello-service && docker rm hello-service
./bin/bordo project delete hello-service --yes
```

---

## Run the automated demo

The `demo.sh` script automates all of the above steps:

```bash
cd /path/to/bordo
bash examples/java-web-service-demo/demo.sh
```

See [`expected-output.txt`](expected-output.txt) for what a successful run looks like.

---

## Template anatomy

The `java-web-service` template lives in `templates/java-web-service/`. Key files:

| File | Purpose |
|---|---|
| `template.yaml` | Bordo manifest — declares variables, health config, OTel flags |
| `pom.xml` | Spring Boot 3.3, Java 25, OTel, actuator, Prometheus metrics |
| `Dockerfile` | Multi-stage: Maven build → Temurin 21 JRE runtime |
| `src/.../HelloController.java` | `GET /api/v1/hello` → JSON response |
| `src/.../application.yaml` | Port 8080, actuator, OTel tracing endpoint |

Variables in template files use Go template syntax: `{{.ProjectName}}`, `{{.GroupId}}`.
The `bordo template expand` command substitutes them before writing files to disk.
