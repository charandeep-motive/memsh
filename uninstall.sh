#!/usr/bin/env bash

set -euo pipefail

INSTALL_BIN_DIR="${MEMSH_INSTALL_BIN_DIR:-$HOME/.local/bin}"
INSTALL_CONFIG_DIR="${MEMSH_INSTALL_CONFIG_DIR:-$HOME/.config/memsh}"
ZSHRC_PATH="${MEMSH_ZSHRC:-$HOME/.zshrc}"
DATA_DIR="${XDG_DATA_HOME:-$HOME/.local/share}/memsh"
REMOVE_DATA="${MEMSH_UNINSTALL_REMOVE_DATA:-0}"

say() {
  printf '%s\n' "$*"
}

strip_line_if_present() {
  local target_file="$1"
  local line="$2"
  local temp_file

  if [[ ! -f "$target_file" ]]; then
    return
  fi

  temp_file="$(mktemp)"
  grep -Fvx "$line" "$target_file" > "$temp_file" || true
  mv "$temp_file" "$target_file"
}

rm -f "$INSTALL_BIN_DIR/memsh"
rm -f "$INSTALL_CONFIG_DIR/memsh.zsh"

strip_line_if_present "$ZSHRC_PATH" 'export PATH="$HOME/.local/bin:$PATH"'
strip_line_if_present "$ZSHRC_PATH" 'source ~/.config/memsh/memsh.zsh'

if [[ "$REMOVE_DATA" == "1" ]]; then
  rm -rf "$DATA_DIR"
fi

say "memsh removed"
say "  binary: $INSTALL_BIN_DIR/memsh"
say "  plugin: $INSTALL_CONFIG_DIR/memsh.zsh"

if [[ "$REMOVE_DATA" == "1" ]]; then
  say "  data: $DATA_DIR"
else
  say "  data preserved: $DATA_DIR"
  say "  set MEMSH_UNINSTALL_REMOVE_DATA=1 to remove stored history too"
fi

say ""
say "Next step:"
say "  Reload your shell: source $ZSHRC_PATH"