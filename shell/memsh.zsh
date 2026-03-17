autoload -Uz add-zsh-hook

if ! whence compdef >/dev/null 2>&1; then
  autoload -Uz compinit
  compinit -u
fi

: ${MEMSH_BIN:=memsh}
: ${MEMSH_AUTOSUGGEST:=1}
: ${MEMSH_AUTOSUGGEST_MIN_CHARS:=2}
: ${MEMSH_MAX_SUGGESTIONS:=5}

typeset -g MEMSH_LAST_COMMAND=""
typeset -ga MEMSH_SUGGESTIONS_CACHE=()

memsh_preexec() {
  MEMSH_LAST_COMMAND="$1"
}

memsh_precmd() {
  local exit_code=$?
  local command_text="$MEMSH_LAST_COMMAND"

  if [[ -z "$command_text" ]]; then
    return
  fi

  "$MEMSH_BIN" record \
    --command "$command_text" \
    --directory "$PWD" \
    --exit-code "$exit_code" >/dev/null 2>&1 &!

  MEMSH_LAST_COMMAND=""
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