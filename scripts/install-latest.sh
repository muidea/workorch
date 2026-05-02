#!/usr/bin/env bash
set -euo pipefail

REPO="${WORKORCH_REPO:-muidea/workorch}"
BIN_NAME="${WORKORCH_BIN_NAME:-workorchd}"
INSTALL_DIR="${WORKORCH_INSTALL_DIR:-/usr/local/bin}"
VERSION="${WORKORCH_VERSION:-latest}"
TMP_DIR="$(mktemp -d)"
cleanup() { rm -rf "$TMP_DIR"; }
trap cleanup EXIT

detect_platform() {
  local os arch
  case "$(uname -s)" in
    Linux)  os=linux ;;
    Darwin) os=darwin ;;
    *)      echo "unsupported OS: $(uname -s)" >&2; exit 1 ;;
  esac
  case "$(uname -m)" in
    x86_64|amd64) arch=amd64 ;;
    aarch64|arm64) arch=arm64 ;;
    *)            echo "unsupported arch: $(uname -m)" >&2; exit 1 ;;
  esac
  echo "$os/$arch"
}

platform="$(detect_platform)"
os="$(echo "$platform" | cut -d/ -f1)"
arch="$(echo "$platform" | cut -d/ -f2)"

echo "=== workorch install ==="
echo "repo     = $REPO"
echo "version  = $VERSION"
echo "platform = $platform"
echo "dest     = $INSTALL_DIR/$BIN_NAME"
echo ""

if command -v gh &>/dev/null; then
  echo "downloading via gh CLI..."
  if [[ "$VERSION" == "latest" ]]; then
    gh release download --repo "$REPO" --pattern "*$os-$arch.tar.gz" --dir "$TMP_DIR"
  else
    gh release download "$VERSION" --repo "$REPO" --pattern "*$os-$arch.tar.gz" --dir "$TMP_DIR"
  fi
else
  if [[ "$VERSION" == "latest" ]]; then
    api_url="https://api.github.com/repos/$REPO/releases/latest"
  else
    api_url="https://api.github.com/repos/$REPO/releases/tags/$VERSION"
  fi
  echo "fetching release info from $api_url..."
  download_url="$(curl -fsS "$api_url" \
    | grep -o "https://[^\"]*workorchd-[^\"]*-$os-$arch.tar.gz" \
    | head -n1)"
  if [[ -z "$download_url" ]]; then
    echo "no release asset found for $os/$arch" >&2
    exit 1
  fi
  echo "downloading $download_url..."
  curl -fsSL -o "$TMP_DIR/release.tar.gz" "$download_url"
fi

tarball="$(ls "$TMP_DIR"/*.tar.gz 2>/dev/null | head -n1)"
if [[ -z "$tarball" ]]; then
  echo "no tarball found in $TMP_DIR" >&2
  exit 1
fi

echo "extracting $tarball..."
tar -xzf "$tarball" -C "$TMP_DIR"
binary="$(find "$TMP_DIR" -name "$BIN_NAME" -type f | head -n1)"
if [[ -z "$binary" ]]; then
  # The tarball contains workorchd-<tag>-<os>-<arch>
  binary="$(find "$TMP_DIR" -name "workorchd-*" -type f | head -n1)"
fi
if [[ -z "$binary" ]]; then
  echo "binary not found in tarball" >&2
  exit 1
fi

mkdir -p "$INSTALL_DIR"
cp "$binary" "$INSTALL_DIR/$BIN_NAME"
chmod +x "$INSTALL_DIR/$BIN_NAME"

echo "installed $BIN_NAME $VERSION to $INSTALL_DIR/$BIN_NAME"

if [[ ":$PATH:" != *":$INSTALL_DIR:"* ]]; then
  echo "note: add $INSTALL_DIR to your PATH if not already present"
fi
