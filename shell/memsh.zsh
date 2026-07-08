autoload -Uz add-zsh-hook

if ! whence compdef >/dev/null 2>&1; then
  autoload -Uz compinit
  compinit -u
fi

if [[ -r "${XDG_CONFIG_HOME:-$HOME/.config}/memsh/settings.zsh" ]]; then
  source "${XDG_CONFIG_HOME:-$HOME/.config}/memsh/settings.zsh"
fi

: ${MEMSH_BIN:=memsh}
: ${MEMSH_AUTOSUGGEST:=1}
: ${MEMSH_AUTOSUGGEST_MIN_CHARS:=2}
: ${MEMSH_MAX_SUGGESTIONS:=5}
: ${MEMSH_SAVE_LOGS:=0}
: ${MEMSH_LOG_RETENTION_DAYS:=10}

typeset -g MEMSH_LAST_COMMAND=""
typeset -g MEMSH_PENDING_COMMAND=""
typeset -g MEMSH_LOG_FILE=""
typeset -g MEMSH_LOG_DIR=""
typeset -ga MEMSH_SUGGESTIONS_CACHE=()

# Pre-compute the log directory once so we don't shell out on every command.
if [[ "$MEMSH_SAVE_LOGS" == "1" ]]; then
  MEMSH_LOG_DIR="$("$MEMSH_BIN" log-dir 2>/dev/null)"
  mkdir -p "$MEMSH_LOG_DIR" 2>/dev/null
fi

memsh_preexec() {
  if [[ -n "$MEMSH_PENDING_COMMAND" ]]; then
    MEMSH_LAST_COMMAND="$MEMSH_PENDING_COMMAND"
  else
    MEMSH_LAST_COMMAND="$1"
  fi
}

memsh_precmd() {
  local exit_code=$?
  local command_text="$MEMSH_LAST_COMMAND"
  local log_file="$MEMSH_LOG_FILE"

  MEMSH_LAST_COMMAND=""
  MEMSH_PENDING_COMMAND=""
  MEMSH_LOG_FILE=""

  if [[ -z "$command_text" ]]; then
    return
  fi

  local record_args=(record --command "$command_text" --directory "$PWD" --exit-code "$exit_code")
  if [[ -n "$log_file" ]]; then
    record_args+=(--log-file "$log_file")
  fi

  "$MEMSH_BIN" "${record_args[@]}" >/dev/null 2>&1 &!
}

# memsh_accept_line intercepts command execution to capture output via `script`
# when MEMSH_SAVE_LOGS=1. The original BUFFER is saved so precmd records the
# real command, not the script wrapper.
memsh_accept_line() {
  local original_buffer="$BUFFER"
  local trimmed="${original_buffer//[[:space:]]/}"

  if [[ "$MEMSH_SAVE_LOGS" == "1" && -n "$trimmed" && "$original_buffer" != memsh\ * && "$original_buffer" != "memsh" ]]; then
    local ts rand
    ts=$(date +%s)
    rand=$(od -An -N4 -tx1 /dev/urandom 2>/dev/null | tr -d ' \n')
    if [[ -n "$MEMSH_LOG_DIR" && -n "$rand" ]]; then
      MEMSH_PENDING_COMMAND="$original_buffer"
      MEMSH_LOG_FILE="${MEMSH_LOG_DIR}/${ts}_${rand}.log"
      BUFFER="script -q ${(q)MEMSH_LOG_FILE} -- zsh -i -c ${(q)original_buffer}"
    fi
  fi

  zle .accept-line
}

_memsh_complete() {
  local query="$BUFFER"
  local -a suggestions

  if [[ -z "$query" ]]; then
    return 1
  fi

  suggestions=("${(@f)$("$MEMSH_BIN" search --query "$query" --limit "$MEMSH_MAX_SUGGESTIONS" --directory "$PWD" 2>/dev/null)}")

  if (( ${#suggestions[@]} == 0 )); then
    return 1
  fi

  compadd -Q -U -- "${suggestions[@]}"
}

memsh_load_suggestions() {
  local query="$BUFFER"
  local trimmed_query="${BUFFER//[[:space:]]/}"
  local -a raw_suggestions
  local suggestion

  MEMSH_SUGGESTIONS_CACHE=()

  if [[ -z "$query" || -z "$trimmed_query" ]]; then
    return 1
  fi

  raw_suggestions=("${(@f)$("$MEMSH_BIN" search --query "$query" --limit "$MEMSH_MAX_SUGGESTIONS" --directory "$PWD" 2>/dev/null)}")

  for suggestion in "${raw_suggestions[@]}"; do
    if [[ -n "${suggestion//[[:space:]]/}" ]]; then
      MEMSH_SUGGESTIONS_CACHE+=("$suggestion")
    fi
  done

  (( ${#MEMSH_SUGGESTIONS_CACHE[@]} > 0 ))
}

memsh_pick_suggestion() {
  local selection
  local temp_file

  if ! memsh_load_suggestions; then
    zle -M "memsh: no suggestions"
    return
  fi

  temp_file=$(mktemp -t memsh-pick.XXXXXX) || return 1
  "$MEMSH_BIN" pick --query "$BUFFER" --output-file "$temp_file"

  if [[ -f "$temp_file" ]]; then
    selection=$(<"$temp_file")
    rm -f "$temp_file"
  fi

  if [[ -n "$selection" ]]; then
    BUFFER="$selection"
    CURSOR=${#BUFFER}
  fi

  zle redisplay
}

memsh_maybe_suggest() {
  local trimmed_buffer="${BUFFER//[[:space:]]/}"

  if [[ "$MEMSH_AUTOSUGGEST" != "1" ]]; then
    zle -M ""
    return
  fi

  if [[ -z "$trimmed_buffer" ]] || (( ${#trimmed_buffer} < MEMSH_AUTOSUGGEST_MIN_CHARS )); then
    zle -M ""
    return
  fi

  if memsh_load_suggestions; then
    zle -M "memsh: ${#MEMSH_SUGGESTIONS_CACHE[@]} suggestions, press ↓ or Ctrl-Space"
  else
    zle -M ""
  fi
}

memsh_self_insert() {
  zle .self-insert -- "$@"
  memsh_maybe_suggest
}

memsh_backward_delete_char() {
  zle .backward-delete-char -- "$@"
  memsh_maybe_suggest
}

memsh_down_or_pick() {
  local trimmed_buffer="${BUFFER//[[:space:]]/}"

  if [[ -n "$trimmed_buffer" ]] && (( ${#trimmed_buffer} >= MEMSH_AUTOSUGGEST_MIN_CHARS )); then
    memsh_pick_suggestion
    return
  fi

  zle .down-line-or-history -- "$@"
}

zle -C memsh-suggest complete-word _memsh_complete
zle -N memsh-pick-suggestion memsh_pick_suggestion
zle -N self-insert memsh_self_insert
zle -N backward-delete-char memsh_backward_delete_char
zle -N down-line-or-history memsh_down_or_pick

# Register accept-line override only when log capture is enabled.
if [[ "$MEMSH_SAVE_LOGS" == "1" ]]; then
  zle -N accept-line memsh_accept_line
fi

bindkey '^ ' memsh-pick-suggestion
bindkey '^[[B' down-line-or-history
bindkey '^[OB' down-line-or-history
add-zsh-hook preexec memsh_preexec
add-zsh-hook precmd memsh_precmd
