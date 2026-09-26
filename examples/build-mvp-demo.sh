#!/usr/bin/env bash
# build-mvp-demo.sh — Bordo Build MVP demo
#
# Demonstrates the full Build layer: project create → scaffold → docker build → artifact
#
# Prerequisites:
#   - bordod built:  make build-cp   (produces bin/bordod)
#   - bordo built:   make build-cli  (produces bin/bordo)
#   - Docker running with a local registry:
#       docker run -d -p 5000:5000 --name bordo-registry registry:2
#
# Usage:
#   bash examples/build-mvp-demo.sh

set -euo pipefail

BORDO=${BORDO:-./bin/bordo}
BORDOD=${BORDOD:-./bin/bordod}
REGISTRY=${REGISTRY:-localhost:5000}
PROJECT_NAME=my-demo
TEMPLATE=java-web-service
OUTPUT_DIR=/tmp/bordo-demo-${PROJECT_NAME}

log() { echo "▸ $*"; }
ok()  { echo "✓ $*"; }
err() { echo "✗ $*" >&2; exit 1; }

# ── 1. Start control plane ─────────────────────────────────────────────────────
log "starting bordod..."
"${BORDOD}" serve --log-format text &
BORDOD_PID=$!
trap 'kill ${BORDOD_PID} 2>/dev/null; rm -rf ${OUTPUT_DIR}' EXIT
sleep 1

# ── 2. Configure CLI ───────────────────────────────────────────────────────────
log "configuring bordo CLI..."
"${BORDO}" config set server http://localhost:7401

# ── 3. Health check ────────────────────────────────────────────────────────────
log "checking control-plane health..."
curl -sf http://localhost:7401/healthz | grep -q '"status":"ok"' || err "health check failed"
ok "control plane is healthy"

# ── 4. Create project ──────────────────────────────────────────────────────────
log "creating project ${PROJECT_NAME}..."
"${BORDO}" project create "${PROJECT_NAME}" --template "${TEMPLATE}"
ok "project created"

# ── 5. Verify project in list ──────────────────────────────────────────────────
log "listing projects..."
"${BORDO}" project list | grep -q "${PROJECT_NAME}" || err "project not found in list"
ok "project appears in list"

# ── 6. Scaffold template ───────────────────────────────────────────────────────
log "expanding template ${TEMPLATE} → ${OUTPUT_DIR}..."
rm -rf "${OUTPUT_DIR}"
"${BORDO}" template expand "${TEMPLATE}" \
  --var "ProjectName=${PROJECT_NAME}" \
  --var "GroupId=com.example" \
  --output "${OUTPUT_DIR}"
[[ -f "${OUTPUT_DIR}/pom.xml" ]] || err "pom.xml not found after scaffold"
ok "template expanded to ${OUTPUT_DIR}"

# ── 7. Trigger build ───────────────────────────────────────────────────────────
log "triggering container build..."
"${BORDO}" build trigger "${PROJECT_NAME}" \
  --registry "${REGISTRY}" \
  --tag "demo-$(date +%s)"
ok "build triggered"

# ── 8. Show build list ─────────────────────────────────────────────────────────
log "build history:"
"${BORDO}" build list "${PROJECT_NAME}"

# ── 9. Clean up ────────────────────────────────────────────────────────────────
log "deleting project..."
"${BORDO}" project delete "${PROJECT_NAME}" --yes
ok "project deleted"

echo ""
echo "=================================================="
echo "Build MVP demo complete."
echo "  Image target: ${REGISTRY}/${PROJECT_NAME}:<tag>"
echo "  (Docker required for actual build + push)"
echo "=================================================="
