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
typeset -g MEMSH_LOG_DIR=""
typeset -g MEMSH_CMD_START_OFFSET=""
typeset -ga MEMSH_SUGGESTIONS_CACHE=()

# Output capture (MEMSH_SAVE_LOGS=1) runs the interactive shell inside a single
# `script` session that records the terminal to MEMSH_SESSION_LOG. Each command's
# output is sliced out of that recording by byte offset (preexec → precmd) into a
# per-command file, so the typed command line is never rewritten and stays exactly
# as entered in the terminal and in history. The session recording is a transient
# scratch file removed when the shell exits.
if [[ "$MEMSH_SAVE_LOGS" == "1" ]]; then
  MEMSH_LOG_DIR="$("$MEMSH_BIN" log-dir 2>/dev/null)"
  [[ -n "$MEMSH_LOG_DIR" ]] && mkdir -p "$MEMSH_LOG_DIR" 2>/dev/null

  # Re-exec the interactive shell under `script` exactly once. The exported guard
  # variable stops the inner shell from re-execing again. If anything required is
  # missing, the shell continues normally without capture.
  if [[ -o interactive && -z "$MEMSH_SCRIPT_SESSION" && -n "$MEMSH_LOG_DIR" ]] \
     && command -v script >/dev/null 2>&1; then
    export MEMSH_SESSION_LOG="$(mktemp "${MEMSH_LOG_DIR}/session.XXXXXX" 2>/dev/null)"
    if [[ -n "$MEMSH_SESSION_LOG" ]]; then
      export MEMSH_SCRIPT_SESSION=1
      # BSD script (macOS): `script [opts] file command...`; -F flushes after
      # every write (default flush interval is 30s, which would delay capture).
      # util-linux (Linux): `script [opts] -c command file`; --flush likewise.
      if [[ "$OSTYPE" == darwin* ]]; then
        exec script -qaF "$MEMSH_SESSION_LOG" zsh
      else
        exec script -qa --flush -c zsh "$MEMSH_SESSION_LOG"
      fi
    else
      unset MEMSH_SESSION_LOG
    fi
  fi
fi

# memsh_session_size prints the current byte size of the session recording, or
# nothing when no session is active.
memsh_session_size() {
  [[ -n "$MEMSH_SESSION_LOG" && -r "$MEMSH_SESSION_LOG" ]] || return 1
  wc -c < "$MEMSH_SESSION_LOG" 2>/dev/null | tr -d '[:space:]'
}

memsh_preexec() {
  MEMSH_LAST_COMMAND="$1"
  MEMSH_CMD_START_OFFSET="$(memsh_session_size)"
}

memsh_precmd() {
  local exit_code=$?
  local command_text="$MEMSH_LAST_COMMAND"
  local start="$MEMSH_CMD_START_OFFSET"

  MEMSH_LAST_COMMAND=""
  MEMSH_CMD_START_OFFSET=""

  if [[ -z "$command_text" ]]; then
    return
  fi

  # Slice this command's output out of the session recording, but only for a
  # successful, non-memsh command — matching what `memsh record` will store.
  local log_file=""
  if [[ -n "$MEMSH_SESSION_LOG" && -n "$start" && "$exit_code" == "0" \
        && "$command_text" != "memsh" && "$command_text" != memsh\ * ]]; then
    local end
    end="$(memsh_session_size)"
    if [[ -n "$end" ]] && (( end > start )); then
      local ts rand
      ts="$(date +%s)"
      rand="$(od -An -N4 -tx1 /dev/urandom 2>/dev/null | tr -d ' \n')"
      if [[ -n "$rand" ]]; then
        log_file="${MEMSH_LOG_DIR}/${ts}_${rand}.log"
        tail -c "+$((start + 1))" "$MEMSH_SESSION_LOG" 2>/dev/null \
          | head -c "$((end - start))" > "$log_file" 2>/dev/null
        # Drop an empty slice so we never store a reference to a blank file.
        [[ -s "$log_file" ]] || { rm -f "$log_file" 2>/dev/null; log_file=""; }
      fi
    fi
  fi

  local record_args=(record --command "$command_text" --directory "$PWD" --exit-code "$exit_code")
  if [[ -n "$log_file" ]]; then
    record_args+=(--log-file "$log_file")
  fi

  "$MEMSH_BIN" "${record_args[@]}" >/dev/null 2>&1 &!
}

# memsh_cleanup_session removes the transient session recording on shell exit.
memsh_cleanup_session() {
  [[ -n "$MEMSH_SESSION_LOG" && -e "$MEMSH_SESSION_LOG" ]] && rm -f "$MEMSH_SESSION_LOG" 2>/dev/null
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

bindkey '^ ' memsh-pick-suggestion
bindkey '^[[B' down-line-or-history
bindkey '^[OB' down-line-or-history
add-zsh-hook preexec memsh_preexec
add-zsh-hook precmd memsh_precmd
add-zsh-hook zshexit memsh_cleanup_session
