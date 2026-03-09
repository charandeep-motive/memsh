#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

INSTALL_BIN_DIR="${MEMSH_INSTALL_BIN_DIR:-$HOME/.local/bin}"
INSTALL_CONFIG_DIR="${MEMSH_INSTALL_CONFIG_DIR:-$HOME/.config/memsh}"
ZSHRC_PATH="${MEMSH_ZSHRC:-$HOME/.zshrc}"

MEMSH_BINARY_SOURCE=""
MEMSH_PLUGIN_SOURCE=""

say() {
  printf '%s\n' "$*"
}

fail() {
  printf 'memsh install error: %s\n' "$*" >&2
  exit 1
}

append_if_missing() {
  local target_file="$1"
  local line="$2"

  touch "$target_file"
  if ! grep -Fqx "$line" "$target_file"; then
    printf '%s\n' "$line" >> "$target_file"
  fi
}

resolve_binary_source() {
  if [[ -f "$SCRIPT_DIR/go.mod" && -d "$SCRIPT_DIR/cmd/memsh" ]]; then
    command -v go >/dev/null 2>&1 || fail "Go is required to build memsh from source"
    say "Building memsh binary..."
    mkdir -p "$SCRIPT_DIR/bin"
    go build -o "$SCRIPT_DIR/bin/memsh" "$SCRIPT_DIR/cmd/memsh"
    MEMSH_BINARY_SOURCE="$SCRIPT_DIR/bin/memsh"
    return
  fi

  if [[ -x "$SCRIPT_DIR/bin/memsh" ]]; then
    MEMSH_BINARY_SOURCE="$SCRIPT_DIR/bin/memsh"
    return
  fi

  if [[ -x "$SCRIPT_DIR/memsh" ]]; then
    MEMSH_BINARY_SOURCE="$SCRIPT_DIR/memsh"
    return
  fi

  fail "could not find a memsh binary or source tree"
}

resolve_plugin_source() {
  if [[ -f "$SCRIPT_DIR/memsh.zsh" ]]; then
    MEMSH_PLUGIN_SOURCE="$SCRIPT_DIR/memsh.zsh"
    return
  fi

  if [[ -f "$SCRIPT_DIR/shell/memsh.zsh" ]]; then
    MEMSH_PLUGIN_SOURCE="$SCRIPT_DIR/shell/memsh.zsh"
    return
  fi

  fail "could not find memsh.zsh"
}

if [[ "$(uname -s)" != "Darwin" ]]; then
  fail "this installer currently supports macOS only"
fi

if ! command -v zsh >/dev/null 2>&1; then
  fail "zsh is required"
fi

resolve_binary_source
resolve_plugin_source

mkdir -p "$INSTALL_BIN_DIR" "$INSTALL_CONFIG_DIR"
install "$MEMSH_BINARY_SOURCE" "$INSTALL_BIN_DIR/memsh"
install "$MEMSH_PLUGIN_SOURCE" "$INSTALL_CONFIG_DIR/memsh.zsh"

append_if_missing "$ZSHRC_PATH" 'export PATH="$HOME/.local/bin:$PATH"'
append_if_missing "$ZSHRC_PATH" 'source ~/.config/memsh/memsh.zsh'

say "memsh installed"
say "  binary: $INSTALL_BIN_DIR/memsh"
say "  plugin: $INSTALL_CONFIG_DIR/memsh.zsh"

if ! command -v fzf >/dev/null 2>&1; then
  say "optional: install fzf for the nicer picker UI"
  say "  brew install fzf"
fi

say ""
say "Next steps:"
say "  1. Reload your shell: source $ZSHRC_PATH"
say "  2. Verify: memsh help && memsh doctor"