#!/bin/sh
# Bordo node agent installer (ADR-0006 Phase 1). Run this ON a target VM to join
# it to a Bordo region:
#
#   curl -fsSL https://raw.githubusercontent.com/hasangenc0/bordo/master/install-node.sh | \
#     BORDO_CP_URL=https://your-control-plane:7401 BORDO_JOIN_TOKEN=<token> sh
#
# It downloads the bordo-node agent, which redeems the single-use join token
# outbound, installs k3s, and registers the cluster. Nothing SSHes into this VM.
#
# Env:
#   BORDO_CP_URL      control-plane URL reachable from this VM (required)
#   BORDO_JOIN_TOKEN  one-time token from `bordo region add` (required)
#   BORDO_VERSION     release tag to install (default: latest)
set -eu

REPO="hasangenc0/bordo"
BIN="bordo-node"

: "${BORDO_CP_URL:?BORDO_CP_URL is required (control-plane URL reachable from this VM)}"
: "${BORDO_JOIN_TOKEN:?BORDO_JOIN_TOKEN is required (run 'bordo region add' to mint one)}"

OS="$(uname -s | tr '[:upper:]' '[:lower:]')"
if [ "$OS" != "linux" ]; then
  echo "bordo-node runs on Linux VMs (k3s target); detected: $OS" >&2
  exit 1
fi
ARCH="$(uname -m)"
case "$ARCH" in
  x86_64|amd64) ARCH="amd64" ;;
  aarch64|arm64) ARCH="arm64" ;;
  *) echo "unsupported architecture: $ARCH" >&2; exit 1 ;;
esac

TAG="${BORDO_VERSION:-}"
if [ -z "$TAG" ]; then
  TAG="$(curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" \
    | grep '"tag_name"' | head -1 | cut -d'"' -f4)"
fi
[ -n "$TAG" ] || { echo "could not determine latest release tag" >&2; exit 1; }

URL="https://github.com/${REPO}/releases/download/${TAG}/${BIN}_${OS}_${ARCH}.tar.gz"
echo "Downloading ${BIN} ${TAG} (${OS}/${ARCH})..."

TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT
curl -fsSL "$URL" | tar -xz -C "$TMP"

DEST="/usr/local/bin/${BIN}"
if [ "$(id -u)" -eq 0 ]; then
  install -m 0755 "$TMP/$BIN" "$DEST"
elif command -v sudo >/dev/null 2>&1; then
  sudo install -m 0755 "$TMP/$BIN" "$DEST"
else
  DEST="$HOME/.local/bin/${BIN}"
  mkdir -p "$(dirname "$DEST")"
  install -m 0755 "$TMP/$BIN" "$DEST"
fi

echo "Running node bootstrap (redeem → install k3s → register)..."
exec "$DEST"
