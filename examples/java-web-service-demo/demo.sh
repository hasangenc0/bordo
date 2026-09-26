#!/usr/bin/env bash
# demo.sh — Java Web Service end-to-end demo
#
# Flow: start control plane → create project → scaffold template → trigger build → run → test → cleanup
#
# Prerequisites:
#   make build                    (builds bin/bordod and bin/bordo)
#   docker info                   (Docker must be running)
#   docker run -d -p 5000:5000 --name bordo-registry registry:2   (local registry)
#
# Usage:
#   cd /path/to/bordo
#   bash examples/java-web-service-demo/demo.sh

set -euo pipefail

BORDO="${BORDO:-./bin/bordo}"
BORDOD="${BORDOD:-./bin/bordod}"
REGISTRY="${REGISTRY:-localhost:5000}"
PROJECT_NAME="hello-service"
TEMPLATE="java-web-service"
OUTPUT_DIR="/tmp/bordo-demo-${PROJECT_NAME}"
IMAGE_TAG="demo-$(date +%s)"

log()  { printf '\033[1;34m▸\033[0m %s\n' "$*"; }
ok()   { printf '\033[1;32m✓\033[0m %s\n' "$*"; }
fail() { printf '\033[1;31m✗\033[0m %s\n' "$*" >&2; exit 1; }
sep()  { echo "──────────────────────────────────────────────────"; }

# Ensure binaries exist
[[ -x "${BORDOD}" ]] || fail "bordod not found at ${BORDOD} — run: make build"
[[ -x "${BORDO}"  ]] || fail "bordo not found at ${BORDO} — run: make build"

sep
echo "  Bordo Build MVP Demo"
echo "  Project: ${PROJECT_NAME}  Template: ${TEMPLATE}"
echo "  Registry: ${REGISTRY}     Tag: ${IMAGE_TAG}"
sep

# ── 1. Start control plane ─────────────────────────────────────────────────────
log "starting bordod..."
"${BORDOD}" serve --log-format text &
BORDOD_PID=$!
cleanup() {
  log "stopping bordod (pid ${BORDOD_PID})..."
  kill "${BORDOD_PID}" 2>/dev/null || true
  rm -rf "${OUTPUT_DIR}"
  # Remove docker container if it was started
  docker rm -f "${PROJECT_NAME}" 2>/dev/null || true
}
trap cleanup EXIT

# Give the server a moment to start
sleep 1

# ── 2. Configure CLI ───────────────────────────────────────────────────────────
log "configuring CLI → http://localhost:7401..."
"${BORDO}" config set server http://localhost:7401

# ── 3. Health check ────────────────────────────────────────────────────────────
log "health check..."
for i in 1 2 3; do
  if curl -sf http://localhost:7401/healthz | grep -q '"status":"ok"'; then
    ok "control plane healthy"
    break
  fi
  [[ $i -eq 3 ]] && fail "health check timed out"
  sleep 1
done

# ── 4. Create project ──────────────────────────────────────────────────────────
log "creating project ${PROJECT_NAME}..."
"${BORDO}" project create "${PROJECT_NAME}" --template "${TEMPLATE}"
ok "project created"

# ── 5. List projects ───────────────────────────────────────────────────────────
log "projects:"
"${BORDO}" project list

# ── 6. Scaffold template ───────────────────────────────────────────────────────
log "expanding template → ${OUTPUT_DIR}..."
rm -rf "${OUTPUT_DIR}"
"${BORDO}" template expand "${TEMPLATE}" \
  --var "ProjectName=${PROJECT_NAME}" \
  --var "GroupId=com.example" \
  --output "${OUTPUT_DIR}"
[[ -f "${OUTPUT_DIR}/pom.xml" ]]      || fail "pom.xml missing after scaffold"
[[ -f "${OUTPUT_DIR}/Dockerfile" ]]   || fail "Dockerfile missing after scaffold"
ok "scaffold complete ($(find "${OUTPUT_DIR}" -type f | wc -l | tr -d ' ') files)"

# ── 7. Trigger build ───────────────────────────────────────────────────────────
log "triggering build → ${REGISTRY}/${PROJECT_NAME}:${IMAGE_TAG}..."
"${BORDO}" build trigger "${PROJECT_NAME}" \
  --registry "${REGISTRY}" \
  --tag "${IMAGE_TAG}"
ok "build triggered"

# ── 8. Build list ─────────────────────────────────────────────────────────────
log "build history:"
"${BORDO}" build list "${PROJECT_NAME}"

# ── 9. (Optional) Pull and run image ──────────────────────────────────────────
if docker info &>/dev/null && docker pull "${REGISTRY}/${PROJECT_NAME}:${IMAGE_TAG}" &>/dev/null; then
  log "running container locally..."
  docker run -d --name "${PROJECT_NAME}" -p 8080:8080 \
    "${REGISTRY}/${PROJECT_NAME}:${IMAGE_TAG}"
  sleep 2

  log "testing service..."
  RESPONSE=$(curl -sf http://localhost:8080/api/v1/hello) \
    || fail "service did not respond on :8080"
  echo "  response: ${RESPONSE}"
  echo "${RESPONSE}" | grep -q "hello" || fail "unexpected response"
  ok "service responded correctly"
else
  log "(skipping run/test — image not available locally, Docker build may be queued)"
fi

# ── 10. Cleanup project ────────────────────────────────────────────────────────
log "deleting project..."
"${BORDO}" project delete "${PROJECT_NAME}" --yes
ok "project deleted"

sep
echo "  Demo complete."
echo "  Image target: ${REGISTRY}/${PROJECT_NAME}:${IMAGE_TAG}"
sep
