#!/bin/sh
# Install the bordo CLI. Usage:
#   curl -fsSL https://raw.githubusercontent.com/hasangenc0/bordo/master/install.sh | sh
#
# Env overrides:
#   BORDO_INSTALL_DIR   install location (default: /usr/local/bin, falls back to ~/.local/bin)
#   BORDO_VERSION       specific tag to install (default: latest release)
set -eu

REPO="hasangenc0/bordo"
BIN="bordo"

# ── detect OS/arch ──────────────────────────────────────────────────────────
OS="$(uname -s | tr '[:upper:]' '[:lower:]')"
ARCH="$(uname -m)"
case "$OS" in
  linux|darwin) ;;
  *) echo "unsupported OS: $OS" >&2; exit 1 ;;
esac
case "$ARCH" in
  x86_64|amd64) ARCH="amd64" ;;
  aarch64|arm64) ARCH="arm64" ;;
  *) echo "unsupported architecture: $ARCH" >&2; exit 1 ;;
esac

# ── resolve version ─────────────────────────────────────────────────────────
TAG="${BORDO_VERSION:-}"
if [ -z "$TAG" ]; then
  TAG="$(curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" \
    | grep '"tag_name"' | head -1 | cut -d'"' -f4)"
fi
if [ -z "$TAG" ]; then
  echo "could not determine latest release tag" >&2
  exit 1
fi

URL="https://github.com/${REPO}/releases/download/${TAG}/${BIN}_${OS}_${ARCH}.tar.gz"
echo "Downloading ${BIN} ${TAG} (${OS}/${ARCH})..."

TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT
curl -fsSL "$URL" | tar -xz -C "$TMP"

# ── choose install dir ──────────────────────────────────────────────────────
DEST="${BORDO_INSTALL_DIR:-/usr/local/bin}"
if [ ! -d "$DEST" ] || [ ! -w "$DEST" ]; then
  if [ "$(id -u)" -ne 0 ] && command -v sudo >/dev/null 2>&1; then
    sudo install -m 0755 "$TMP/$BIN" "$DEST/$BIN"
  else
    DEST="$HOME/.local/bin"
    mkdir -p "$DEST"
    install -m 0755 "$TMP/$BIN" "$DEST/$BIN"
  fi
else
  install -m 0755 "$TMP/$BIN" "$DEST/$BIN"
fi

echo "Installed $BIN to $DEST/$BIN"
case ":$PATH:" in
  *":$DEST:"*) ;;
  *) echo "NOTE: add $DEST to your PATH:  export PATH=\"$DEST:\$PATH\"" ;;
esac
"$DEST/$BIN" version || true
