#!/usr/bin/env bash

set -euo pipefail

REPO="charandeep-motive/memsh"

INSTALL_BIN_DIR="${MEMSH_INSTALL_BIN_DIR:-$HOME/.local/bin}"
INSTALL_CONFIG_DIR="${MEMSH_INSTALL_CONFIG_DIR:-$HOME/.config/memsh}"
ZSHRC_PATH="${MEMSH_ZSHRC:-$HOME/.zshrc}"

# Optional release to install. Empty means the latest stable release.
# Set via MEMSH_VERSION=vX.Y.Z or the --version flag (e.g. piped:
# curl ... | bash -s -- --version v0.2.0).
MEMSH_VERSION="${MEMSH_VERSION:-}"

while [[ $# -gt 0 ]]; do
  case "$1" in
    --version)
      MEMSH_VERSION="${2:-}"
      shift 2
      ;;
    --version=*)
      MEMSH_VERSION="${1#*=}"
      shift
      ;;
    *)
      shift
      ;;
  esac
done

# Best-effort directory of this script. Empty when piped via `curl | bash`,
# which is exactly how we detect a remote install below.
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]:-$0}")" 2>/dev/null && pwd || true)"

MEMSH_BINARY_SOURCE=""
MEMSH_PLUGIN_SOURCE=""
TMP_DIR=""

say() {
  printf '%s\n' "$*"
}

fail() {
  printf 'memsh install error: %s\n' "$*" >&2
  exit 1
}

cleanup() {
  if [[ -n "$TMP_DIR" && -d "$TMP_DIR" ]]; then
    rm -rf "$TMP_DIR"
  fi
}
trap cleanup EXIT

append_if_missing() {
  local target_file="$1"
  local line="$2"

  touch "$target_file"
  if ! grep -Fqx "$line" "$target_file"; then
    printf '%s\n' "$line" >> "$target_file"
  fi
}

detect_asset() {
  local arch
  arch="$(uname -m)"
  case "$arch" in
    arm64 | aarch64) echo "memsh-darwin-arm64" ;;
    x86_64 | amd64) echo "memsh-darwin-amd64" ;;
    *) fail "unsupported architecture: $arch" ;;
  esac
}

download() {
  local url="$1"
  local dest="$2"
  curl -fsSL "$url" -o "$dest" || fail "failed to download $url"
}

# resolve_from_source uses a local checkout: build with Go when available, or
# fall back to an already-built binary in the tree. Returns non-zero when this
# is not a checkout (e.g. the script was piped via curl).
resolve_from_source() {
  if [[ -n "$SCRIPT_DIR" && -f "$SCRIPT_DIR/go.mod" && -d "$SCRIPT_DIR/cmd/memsh" ]]; then
    command -v go >/dev/null 2>&1 || fail "Go is required to build memsh from source"
    say "Building memsh binary..."
    mkdir -p "$SCRIPT_DIR/bin"
    go build -o "$SCRIPT_DIR/bin/memsh" "$SCRIPT_DIR/cmd/memsh"
    MEMSH_BINARY_SOURCE="$SCRIPT_DIR/bin/memsh"
    MEMSH_PLUGIN_SOURCE="$SCRIPT_DIR/shell/memsh.zsh"
    return 0
  fi

  if [[ -n "$SCRIPT_DIR" && -x "$SCRIPT_DIR/bin/memsh" && -f "$SCRIPT_DIR/shell/memsh.zsh" ]]; then
    MEMSH_BINARY_SOURCE="$SCRIPT_DIR/bin/memsh"
    MEMSH_PLUGIN_SOURCE="$SCRIPT_DIR/shell/memsh.zsh"
    return 0
  fi

  return 1
}

# resolve_from_release downloads the prebuilt binary and plugin for the current
# architecture from the latest GitHub release.
resolve_from_release() {
  command -v curl >/dev/null 2>&1 || fail "curl is required to download memsh"

  local asset base
  asset="$(detect_asset)"
  if [[ -n "$MEMSH_VERSION" ]]; then
    base="https://github.com/$REPO/releases/download/$MEMSH_VERSION"
  else
    base="https://github.com/$REPO/releases/latest/download"
  fi

  TMP_DIR="$(mktemp -d)"
  say "Downloading memsh release ${MEMSH_VERSION:-latest} ($asset)..."
  download "$base/$asset" "$TMP_DIR/memsh"
  download "$base/memsh.zsh" "$TMP_DIR/memsh.zsh"
  chmod +x "$TMP_DIR/memsh"

  MEMSH_BINARY_SOURCE="$TMP_DIR/memsh"
  MEMSH_PLUGIN_SOURCE="$TMP_DIR/memsh.zsh"
}

if [[ "$(uname -s)" != "Darwin" ]]; then
  fail "this installer currently supports macOS only"
fi

if ! command -v zsh >/dev/null 2>&1; then
  fail "zsh is required"
fi

# An explicit --version always installs that published release, even inside a
# checkout. Otherwise prefer a local source build and fall back to the release.
if [[ -n "$MEMSH_VERSION" ]]; then
  resolve_from_release
elif ! resolve_from_source; then
  resolve_from_release
fi

mkdir -p "$INSTALL_BIN_DIR" "$INSTALL_CONFIG_DIR"
install "$MEMSH_BINARY_SOURCE" "$INSTALL_BIN_DIR/memsh"
install "$MEMSH_PLUGIN_SOURCE" "$INSTALL_CONFIG_DIR/memsh.zsh"

append_if_missing "$ZSHRC_PATH" 'export PATH="$HOME/.local/bin:$PATH"'
append_if_missing "$ZSHRC_PATH" 'source ~/.config/memsh/memsh.zsh'

say "memsh installed"
say "  binary: $INSTALL_BIN_DIR/memsh"
say "  plugin: $INSTALL_CONFIG_DIR/memsh.zsh"

say ""
say "Next steps:"
say "  1. Reload your shell: source $ZSHRC_PATH"
say "  2. Verify: memsh help && memsh doctor"
