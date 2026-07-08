# Command Output Logging — Design Spec

**Date:** 2026-07-08  
**Status:** Approved

---

## Overview

Add transparent command output logging to memsh. When enabled, the output of every successfully recorded shell command is captured to a file. Users can browse logs via `memsh logs`, which opens a picker UI with inline previews. The regular memsh picker also gains timestamps on every row.

---

## Goals

- Auto-capture stdout+stderr for every command memsh already records (exit code 0, non-memsh commands).
- Store logs as plain files next to the SQLite database; only the file path lives in the DB.
- Configurable retention (default 10 days); old files are pruned automatically.
- Zero data loss: all existing `commands` table data is preserved; the new `command_logs` table is purely additive.
- No overflow in any UI — everything fits within the memsh box or the terminal viewport.

---

## Settings

Two new entries added to `internal/config/settings.go`:

| Key | Default | Hint | Description |
|---|---|---|---|
| `MEMSH_SAVE_LOGS` | `0` | `0\|1` | Enable command output capture |
| `MEMSH_LOG_RETENTION_DAYS` | `10` | `integer >= 1` | Days to retain log files |

Both follow the existing `SettingSpec` / validator pattern. `MEMSH_LOG_RETENTION_DAYS` uses `validateMinIntSetting(value, 1)`.

---

## File Storage

- **Log directory:** `<DataDir>/logs/` — sibling of `memsh.db`, resolved via `config.ResolvePaths()`.
- **File naming:** `<unix_timestamp>_<8-hex-random>.log` — no command text in the filename (avoids unsafe characters and length limits).
- **`Paths` struct** gains a `LogsDir string` field populated by `ResolvePaths()`.

---

## Database — Schema Version 2

`schemaVersion` bumps from `1` → `2`. The migration adds one new table only; the existing `commands` table and all its rows are untouched.

```sql
CREATE TABLE IF NOT EXISTS command_logs (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    command     TEXT    NOT NULL,
    directory   TEXT    NOT NULL DEFAULT '',
    executed_at INTEGER NOT NULL,
    exit_code   INTEGER NOT NULL DEFAULT 0,
    log_file    TEXT    NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_command_logs_executed_at
    ON command_logs(executed_at DESC);
```

`command_logs` is a per-execution table — one row per run. It is intentionally separate from `commands` (which deduplicates by `(command, directory)` and tracks frequency).

---

## Shell Capture — `memsh.zsh`

> **Revised during implementation (2026-07-08).** The per-command `accept-line`
> wrapper described below was replaced with a **whole-session** approach, because
> rewriting `BUFFER` made the executed command line (`script … -c …`) visible in
> the terminal and in history. The shipped design instead re-execs the interactive
> shell once under a single `script` session (`script -qaF <session-log> zsh` on
> macOS; `-F`/`--flush` forces real-time flushing — BSD `script` otherwise buffers
> for 30s) and slices each command's output out of that recording by byte offset
> (`preexec` records the start offset, `precmd` reads the end offset and copies the
> in-between bytes into a per-command log file). The typed command line is never
> modified. The session recording is a transient scratch file removed on shell exit
> (`zshexit` hook). Per-command slicing still only stores logs for successful,
> non-`memsh` commands. The original per-command-wrapper design is retained below
> for historical context.

### (Superseded) New ZLE widget: `memsh_accept_line`

Registered as `accept-line` when `MEMSH_SAVE_LOGS=1`. For every non-empty buffer that isn't a memsh-internal command:

1. Generate a log file path in zsh (no subprocess needed):
   ```zsh
   local ts=$(date +%s)
   local rand=$(od -An -N4 -tx1 /dev/urandom | tr -d ' \n')
   MEMSH_LOG_FILE="${MEMSH_LOG_DIR}/${ts}_${rand}.log"
   ```
2. Save the original command: `MEMSH_PENDING_COMMAND="$BUFFER"`.
3. Rewrite `BUFFER`:
   ```zsh
   BUFFER="script -q ${(q)MEMSH_LOG_FILE} -- zsh -i -c ${(q)MEMSH_PENDING_COMMAND}"
   ```
4. Call `zle .accept-line`.

`MEMSH_LOG_DIR` is initialised once on shell load from `memsh log-dir` (a new lightweight Go subcommand that just prints the logs directory path).

### Updated `memsh_preexec`

When `MEMSH_PENDING_COMMAND` is set, use it as the recorded command text instead of `$1` (which would be the `script …` wrapper):

```zsh
memsh_preexec() {
  if [[ -n "$MEMSH_PENDING_COMMAND" ]]; then
    MEMSH_LAST_COMMAND="$MEMSH_PENDING_COMMAND"
  else
    MEMSH_LAST_COMMAND="$1"
  fi
}
```

### Updated `memsh_precmd`

Pass `--log-file` when a log path was prepared:

```zsh
memsh_precmd() {
  local exit_code=$?
  local command_text="$MEMSH_LAST_COMMAND"
  local log_file="$MEMSH_LOG_FILE"

  MEMSH_LAST_COMMAND=""
  MEMSH_PENDING_COMMAND=""
  MEMSH_LOG_FILE=""

  [[ -z "$command_text" ]] && return

  local record_args=(record --command "$command_text" --directory "$PWD" --exit-code "$exit_code")
  [[ -n "$log_file" ]] && record_args+=(--log-file "$log_file")

  "$MEMSH_BIN" "${record_args[@]}" >/dev/null 2>&1 &!
}
```

### Known caveat

`script … zsh -i -c <cmd>` spawns a new `zsh` process that loads `.zshrc`. Aliases and shell functions work. Local variables set in the current session are **not** inherited. This is invisible for the vast majority of commands (`git`, `kubectl`, `npm`, etc.) and is documented in the `MEMSH_SAVE_LOGS` setting description.

---

## Go Binary Changes

### `internal/config/paths.go`

Add `LogsDir string` to `Paths`; populate as `filepath.Join(dataDir, "logs")`.

### `internal/config/settings.go`

Add two new `SettingSpec` entries for `MEMSH_SAVE_LOGS` and `MEMSH_LOG_RETENTION_DAYS`.

### `internal/db/schema.go`

- Bump `schemaVersion` to `2`.
- Add `command_logs` table + index to the fresh-install `schema` constant.
- Add a v1→v2 migration constant (`migrateAddCommandLogs`) that runs `CREATE TABLE IF NOT EXISTS command_logs …` inside a transaction.
- Update `migrate()` to apply the new migration step.

### `internal/db/db.go`

New functions:

```go
// InsertCommandLog records a single command execution + log file path.
func InsertCommandLog(ctx, store, command, directory string, executedAt time.Time, exitCode int, logFile string) error

// ListCommandLogs returns rows ordered by executed_at DESC with optional limit.
func ListCommandLogs(ctx, store, limit int) ([]CommandLog, error)

// PruneCommandLogs deletes rows (and their log files) older than retentionDays.
func PruneCommandLogs(ctx, store, logsDir string, retentionDays int) (int64, error)

type CommandLog struct {
    ID         int64
    Command    string
    Directory  string
    ExecutedAt time.Time
    ExitCode   int
    LogFile    string
}
```

### `cmd/memsh/command_record.go`

Add `--log-file` flag (empty string = no log). When non-empty and exit code is 0 and command is not internal, call `db.InsertCommandLog(…)` after the existing `record.Store(…)` call.

### `cmd/memsh/router.go`

Add two new cases:
```go
case "logs":
    return runLogs(ctx, args[1:], os.Stdout)
case "log-dir":
    return runLogDir()
```

### `cmd/memsh/command_logs.go` (new file)

`runLogs` opens the database, calls `db.ListCommandLogs`, and launches `ui.RunLogsPicker`. On selection, if the log file exists, exec `less -R <logfile>`. If the file is missing, print `"log file not found (expired)"` and exit 0.

`runLogDir` prints `paths.LogsDir` to stdout. Used by the shell to pre-compute `MEMSH_LOG_DIR`.

### `cmd/memsh/command_maintenance.go`

Wire `db.PruneCommandLogs` into the existing maintenance flow, reading `MEMSH_LOG_RETENTION_DAYS` via `config.LogRetentionDays()`.

---

## Timestamp Display — Both Pickers

### Format

All command rows in both the regular picker and the logs picker display:

```
  Jul  8 14:23  git status
▶ Jul  8 14:25  git push origin main
```

- Timestamp prefix is right-padded to a fixed width (`"Jan _2 15:04"` — Go time format, 12 chars).
- Non-focused rows truncate the command portion to fit `rowWidth - 14` (12 timestamp + 2 separator spaces).
- The focused/selected row still wraps as today.

### Regular picker changes (`internal/db/db.go`)

`ListCommands` is updated to return `[]CommandEntry` (instead of `[]string`):

```go
type CommandEntry struct {
    Command  string
    LastUsed time.Time
}
```

Callers in `command_picker.go`, `command_search.go` updated accordingly. The picker formats entries as `"Jan _2 15:04  <command>"` before passing to `ui.RunCommandPicker`.

---

## `memsh logs` Picker UI

Built as a new BubbleTea model in `internal/ui/logs_picker.go`, reusing the same styling tokens as `picker.go`.

### Layout (fixed sections, no overflow)

```
╭──────────────────────────────────────────────────────╮
│ memsh logs                                           │
│                                                      │
│ memsh> git___                                        │
│ ──────────────────────────────────────────────────── │
│                                                      │
│   Jul  8 14:23  git status                           │
│ ▶ Jul  8 14:25  git push origin main                 │
│   Jul  8 14:30  kubectl get pods                     │
│                                                      │
│ ── preview ─────────────────────────────────────── │
│   Enumerating objects: 5, done.                      │
│   Counting objects: 100% (5/5)                       │
│   Writing objects: 100% (3/3)                        │
│   To github.com/user/repo.git                        │
│      abc1234..def5678  main -> main                  │
│                                                      │
│ Enter: view full log · Esc: cancel                   │
╰──────────────────────────────────────────────────────╯
```

### Preview section rules

- Shows 5–8 lines from the log file of the focused row (last N lines if file > 8 lines).
- Lines are truncated to `rowWidth` to prevent box overflow.
- BSD `script` header/footer lines (`Script started on …` / `Script done on …`) are stripped before display.
- ANSI escape sequences are stripped for the preview (raw bytes shown cleanly); full log opened in `less -R` preserves colors.
- If log file is missing: preview section shows `[log expired]` in dim style.
- If log file is empty: preview section shows `[no output captured]`.

### Filtering

Search filters on command text only (not timestamp). Same fuzzy contains-match as the regular picker.

### Height allocation

The picker box height = title (1) + blank (1) + input (1) + separator (1) + blank (1) + command rows (up to `maxItems`, default 5 for logs picker) + blank (1) + preview header (1) + preview lines (up to 8) + blank (1) + help (1). If terminal is too short, `maxItems` and preview lines are reduced proportionally.

---

## Log Cleanup

`PruneCommandLogs`:
1. Queries `command_logs` where `executed_at < now - retentionDays * 86400`.
2. For each row, deletes the log file (ignores `os.IsNotExist`).
3. Deletes the DB rows in a single `DELETE WHERE executed_at < ?`.
4. Returns count of deleted rows.

Called from `runMaintenance` alongside the existing `PruneLeastUsedCommands`.

---

## Out of Scope

- Windows / fish / bash support (zsh only, consistent with existing shell integration).
- Log compression.
- Per-command log enable/disable (global toggle only).
- Replaying commands from logs.
