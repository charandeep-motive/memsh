# Command Output Logging Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add transparent command output capture via `script`, store log file paths in a new `command_logs` DB table, and expose them through a new `memsh logs` BubbleTea picker with inline preview; also add last-used timestamps to the regular picker.

**Architecture:** The zsh shell hook intercepts `accept-line` to wrap commands with `script -q logfile -- zsh -i -c <cmd>` when `MEMSH_SAVE_LOGS=1`; the Go binary gains a `--log-file` flag on `record`, a `command_logs` DB table (schema v2 migration, additive only), and a new `memsh logs` subcommand backed by a BubbleTea model with an inline preview pane. The regular picker gains a `PickerItem{Display, Value}` type so timestamps are shown without contaminating the returned command text.

**Tech Stack:** Go 1.21+, SQLite via `modernc.org/sqlite`, BubbleTea + Bubbles + Lipgloss for TUI, zsh for shell integration.

## Global Constraints

- Module path: `github.com/charandeep-motive/memsh`
- No new external Go dependencies — use only packages already in `go.mod`.
- Schema changes must be additive: existing `commands` table and all its rows must survive every migration untouched.
- All UI must fit within the memsh box — no horizontal or vertical overflow.
- Only record logs for commands with exit code 0 and that are not memsh-internal commands (consistent with existing `record.Store` logic).
- zsh-only shell integration (no bash/fish changes).
- Conventional commit format: `<type>(<JIRA-KEY>): <subject>` — use `MEMSH-19` as the Jira key for all commits in this feature.

---

## File Map

| File | Change |
|---|---|
| `internal/config/paths.go` | Add `LogsDir string` to `Paths`; populate in `ResolvePaths()` |
| `internal/config/settings.go` | Add `MEMSH_SAVE_LOGS` and `MEMSH_LOG_RETENTION_DAYS` specs |
| `internal/config/access.go` | Add `SaveLogsEnabled()` and `LogRetentionDays()` accessors |
| `internal/config/access_test.go` | Add tests for both new accessors |
| `internal/db/schema.go` | Bump `schemaVersion` to 2; add `command_logs` DDL; add v1→v2 migration |
| `internal/db/db.go` | Add `CommandEntry`, `CommandLog` types; change `ListCommands` → `[]CommandEntry`; add `InsertCommandLog`, `ListCommandLogs`, `PruneCommandLogs` |
| `internal/db/migrate_test.go` | Update `ListCommands` call sites for new return type; add v2 migration test |
| `internal/ui/picker.go` | Add `PickerItem{Display, Value}` type; update `RunCommandPicker` signature and internals |
| `internal/ui/picker_test.go` | No change needed (only tests `truncate`) |
| `internal/ui/logs_picker.go` | New: BubbleTea model for logs picker with inline preview |
| `cmd/memsh/command_picker.go` | Update to build `[]ui.PickerItem` from `[]db.CommandEntry` with timestamp prefix |
| `cmd/memsh/command_record.go` | Add `--log-file` flag; call `db.InsertCommandLog` when flag is set |
| `cmd/memsh/command_logs.go` | New: `runLogs`, `runLogDir` |
| `cmd/memsh/command_maintenance.go` | Add `db.PruneCommandLogs` call inside `runClear` |
| `cmd/memsh/command_stats.go` | Print `logs_dir` in `runDoctor` |
| `cmd/memsh/router.go` | Add `logs` and `log-dir` cases |
| `shell/memsh.zsh` | Add `memsh_accept_line` ZLE widget; update `memsh_preexec` and `memsh_precmd`; initialise `MEMSH_LOG_DIR` |

---

### Task 1: Config — paths, settings, accessors

**Files:**
- Modify: `internal/config/paths.go`
- Modify: `internal/config/settings.go`
- Modify: `internal/config/access.go`
- Modify: `internal/config/access_test.go`

**Interfaces:**
- Produces:
  - `config.Paths.LogsDir string`
  - `config.SaveLogsEnabled() bool`
  - `config.LogRetentionDays() int`

---

- [ ] **Step 1: Add `LogsDir` to `Paths` and populate it in `ResolvePaths`**

In `internal/config/paths.go`, add the field and its initialisation:

```go
type Paths struct {
	ConfigDir    string
	SettingsPath string
	DataDir      string
	DatabasePath string
	LogsDir      string  // ← new
}

func ResolvePaths() (Paths, error) {
	configRoot, err := resolveConfigRoot()
	if err != nil {
		return Paths{}, err
	}

	dataRoot, err := resolveDataRoot()
	if err != nil {
		return Paths{}, err
	}

	configDir := filepath.Join(configRoot, "memsh")
	dataDir := filepath.Join(dataRoot, "memsh")
	return Paths{
		ConfigDir:    configDir,
		SettingsPath: filepath.Join(configDir, "settings.zsh"),
		DataDir:      dataDir,
		DatabasePath: filepath.Join(dataDir, "memsh.db"),
		LogsDir:      filepath.Join(dataDir, "logs"),  // ← new
	}, nil
}
```

- [ ] **Step 2: Add two new `SettingSpec` entries**

In `internal/config/settings.go`, append to `settingSpecs` (keep the slice sorted by key name for readability):

```go
{
    Key:         "MEMSH_LOG_RETENTION_DAYS",
    Default:     "10",
    Description: "Days to retain command output log files (requires MEMSH_SAVE_LOGS=1)",
    ValueHint:   "integer >= 1",
    Validator: func(value string) error {
        return validateMinIntSetting(value, 1)
    },
},
{
    Key:         "MEMSH_SAVE_LOGS",
    Default:     "0",
    Description: "Capture command output to log files for later review with `memsh logs`",
    ValueHint:   "0|1",
    Validator:   validateBoolSetting,
},
```

- [ ] **Step 3: Add typed accessors to `internal/config/access.go`**

Append to the file:

```go
// SaveLogsEnabled reports whether command output capture is turned on.
func SaveLogsEnabled() bool {
	return strings.TrimSpace(os.Getenv("MEMSH_SAVE_LOGS")) == "1"
}

// LogRetentionDays returns the number of days to keep command log files.
func LogRetentionDays() int {
	return positiveIntSetting("MEMSH_LOG_RETENTION_DAYS", 1)
}
```

- [ ] **Step 4: Write tests for the new accessors**

In `internal/config/access_test.go`, add (follow the pattern of any existing tests in that file — check if it exists first and append, or create it):

```go
package config_test

import (
	"testing"

	"github.com/charandeep-motive/memsh/internal/config"
)

func TestSaveLogsEnabledDefaultsOff(t *testing.T) {
	t.Setenv("MEMSH_SAVE_LOGS", "")
	if config.SaveLogsEnabled() {
		t.Error("SaveLogsEnabled() = true with empty env, want false")
	}
}

func TestSaveLogsEnabledWhenSet(t *testing.T) {
	t.Setenv("MEMSH_SAVE_LOGS", "1")
	if !config.SaveLogsEnabled() {
		t.Error("SaveLogsEnabled() = false with MEMSH_SAVE_LOGS=1, want true")
	}
}

func TestLogRetentionDaysDefault(t *testing.T) {
	t.Setenv("MEMSH_LOG_RETENTION_DAYS", "")
	if got := config.LogRetentionDays(); got != 10 {
		t.Errorf("LogRetentionDays() = %d, want 10", got)
	}
}

func TestLogRetentionDaysCustom(t *testing.T) {
	t.Setenv("MEMSH_LOG_RETENTION_DAYS", "30")
	if got := config.LogRetentionDays(); got != 30 {
		t.Errorf("LogRetentionDays() = %d, want 30", got)
	}
}
```

- [ ] **Step 5: Run tests**

```bash
cd /Users/charandeep.kumar/Downloads/projecy/memsh
go test ./internal/config/...
```

Expected: all PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/config/paths.go internal/config/settings.go internal/config/access.go internal/config/access_test.go
git commit -m "feat(MEMSH-19): add LogsDir path and log settings/accessors"
```

---

### Task 2: DB — schema v2, `CommandEntry`, `CommandLog`, new DB functions

**Files:**
- Modify: `internal/db/schema.go`
- Modify: `internal/db/db.go`
- Modify: `internal/db/migrate_test.go`

**Interfaces:**
- Consumes: nothing new from prior tasks at this layer
- Produces:
  - `db.CommandEntry{Command string, LastUsed time.Time}`
  - `db.CommandLog{ID int64, Command, Directory, LogFile string, ExecutedAt time.Time, ExitCode int}`
  - `db.ListCommands(ctx, store, limit int, directory string, directoryAware bool) ([]CommandEntry, error)` — **signature unchanged, return type changed**
  - `db.InsertCommandLog(ctx context.Context, store *Store, command, directory string, executedAt time.Time, exitCode int, logFile string) error`
  - `db.ListCommandLogs(ctx context.Context, store *Store, limit int) ([]CommandLog, error)`
  - `db.PruneCommandLogs(ctx context.Context, store *Store, logsDir string, retentionDays int) (int64, error)`

---

- [ ] **Step 1: Write a failing test for the v2 migration**

In `internal/db/migrate_test.go`, add at the end of the file:

```go
func TestSchemaMigratesV1ToV2AddsCommandLogs(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "memsh.db")

	// Build a v1 database (composite-key schema, no command_logs table).
	v1db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open v1 database: %v", err)
	}
	v1Schema := `
	CREATE TABLE commands (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		command TEXT NOT NULL,
		frequency INTEGER NOT NULL DEFAULT 1,
		last_used INTEGER NOT NULL,
		directory TEXT NOT NULL DEFAULT '',
		exit_code INTEGER NOT NULL DEFAULT 0,
		UNIQUE(command, directory)
	);
	PRAGMA user_version = 1;
	`
	if _, err := v1db.ExecContext(ctx, v1Schema); err != nil {
		t.Fatalf("create v1 schema: %v", err)
	}
	if _, err := v1db.ExecContext(ctx,
		`INSERT INTO commands (command, frequency, last_used, directory, exit_code) VALUES ('git status', 3, 1700000000, '/repo', 0)`,
	); err != nil {
		t.Fatalf("seed v1 row: %v", err)
	}
	if err := v1db.Close(); err != nil {
		t.Fatalf("close v1 database: %v", err)
	}

	// Open through memsh — should migrate to v2.
	store, err := db.Open(ctx, dbPath)
	if err != nil {
		t.Fatalf("open and migrate: %v", err)
	}
	defer store.Close()

	// Existing row is preserved.
	var freq int
	if err := store.QueryRowContext(ctx,
		`SELECT frequency FROM commands WHERE command = 'git status'`,
	).Scan(&freq); err != nil {
		t.Fatalf("query migrated row: %v", err)
	}
	if freq != 3 {
		t.Fatalf("frequency = %d, want 3", freq)
	}

	// command_logs table now exists and is writable.
	if _, err := store.ExecContext(ctx,
		`INSERT INTO command_logs (command, directory, executed_at, exit_code, log_file) VALUES ('git status', '/repo', 1700000001, 0, '/tmp/test.log')`,
	); err != nil {
		t.Fatalf("insert into command_logs after migration: %v", err)
	}
}
```

- [ ] **Step 2: Run the test to confirm it fails**

```bash
go test ./internal/db/... -run TestSchemaMigratesV1ToV2AddsCommandLogs -v
```

Expected: FAIL — `command_logs` table does not exist.

- [ ] **Step 3: Update `internal/db/schema.go`**

Replace the entire file:

```go
package db

const schemaVersion = 2

// schema is applied to a brand-new database (no prior tables).
const schema = `
CREATE TABLE IF NOT EXISTS commands (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    command TEXT NOT NULL,
    frequency INTEGER NOT NULL DEFAULT 1,
    last_used INTEGER NOT NULL,
    directory TEXT NOT NULL DEFAULT '',
    exit_code INTEGER NOT NULL DEFAULT 0,
    UNIQUE(command, directory)
);

CREATE INDEX IF NOT EXISTS idx_commands_last_used ON commands(last_used DESC);
CREATE INDEX IF NOT EXISTS idx_commands_frequency ON commands(frequency DESC);

CREATE TABLE IF NOT EXISTS command_logs (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    command     TEXT    NOT NULL,
    directory   TEXT    NOT NULL DEFAULT '',
    executed_at INTEGER NOT NULL,
    exit_code   INTEGER NOT NULL DEFAULT 0,
    log_file    TEXT    NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_command_logs_executed_at ON command_logs(executed_at DESC);
`

// migrateToCompositeKey rebuilds a legacy table (UNIQUE on command alone).
const migrateToCompositeKey = `
CREATE TABLE commands_migrated (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    command TEXT NOT NULL,
    frequency INTEGER NOT NULL DEFAULT 1,
    last_used INTEGER NOT NULL,
    directory TEXT NOT NULL DEFAULT '',
    exit_code INTEGER NOT NULL DEFAULT 0,
    UNIQUE(command, directory)
);

INSERT INTO commands_migrated (id, command, frequency, last_used, directory, exit_code)
    SELECT id, command, frequency, last_used, directory, exit_code FROM commands;

DROP TABLE commands;
ALTER TABLE commands_migrated RENAME TO commands;

CREATE INDEX IF NOT EXISTS idx_commands_last_used ON commands(last_used DESC);
CREATE INDEX IF NOT EXISTS idx_commands_frequency ON commands(frequency DESC);
`

// migrateAddCommandLogs adds the command_logs table to a v1 database.
const migrateAddCommandLogs = `
CREATE TABLE IF NOT EXISTS command_logs (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    command     TEXT    NOT NULL,
    directory   TEXT    NOT NULL DEFAULT '',
    executed_at INTEGER NOT NULL,
    exit_code   INTEGER NOT NULL DEFAULT 0,
    log_file    TEXT    NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_command_logs_executed_at ON command_logs(executed_at DESC);
`
```

- [ ] **Step 4: Update `migrate()` in `internal/db/db.go` to handle v1→v2**

The `migrate` function currently handles v0→v1. Extend it to also apply v1→v2. Replace the `migrate` function body (the function signature stays the same):

```go
func migrate(ctx context.Context, database *sql.DB) error {
	var userVersion int
	if err := database.QueryRowContext(ctx, "PRAGMA user_version;").Scan(&userVersion); err != nil {
		return fmt.Errorf("read schema version: %w", err)
	}

	if userVersion >= schemaVersion {
		// Already current; ensure tables exist for brand-new files.
		if _, err := database.ExecContext(ctx, schema); err != nil {
			return fmt.Errorf("ensure schema: %w", err)
		}
		return nil
	}

	var tableExists int
	if err := database.QueryRowContext(ctx,
		"SELECT count(*) FROM sqlite_master WHERE type='table' AND name='commands';",
	).Scan(&tableExists); err != nil {
		return fmt.Errorf("inspect schema: %w", err)
	}

	tx, err := database.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin migration: %w", err)
	}
	defer tx.Rollback()

	if tableExists == 0 {
		// Fresh install — apply full schema.
		if _, err := tx.ExecContext(ctx, schema); err != nil {
			return fmt.Errorf("create schema: %w", err)
		}
	} else if userVersion == 0 {
		// v0 → v1: rebuild commands with composite key.
		if _, err := tx.ExecContext(ctx, migrateToCompositeKey); err != nil {
			return fmt.Errorf("migrate to composite key: %w", err)
		}
		// v1 → v2: add command_logs.
		if _, err := tx.ExecContext(ctx, migrateAddCommandLogs); err != nil {
			return fmt.Errorf("migrate add command_logs: %w", err)
		}
	} else if userVersion == 1 {
		// v1 → v2: add command_logs.
		if _, err := tx.ExecContext(ctx, migrateAddCommandLogs); err != nil {
			return fmt.Errorf("migrate add command_logs: %w", err)
		}
	}

	if _, err := tx.ExecContext(ctx, fmt.Sprintf("PRAGMA user_version = %d;", schemaVersion)); err != nil {
		return fmt.Errorf("set schema version: %w", err)
	}

	return tx.Commit()
}
```

- [ ] **Step 5: Add `CommandEntry`, `CommandLog` types and new functions to `internal/db/db.go`**

Add immediately after the `Stats` type definition:

```go
// CommandEntry is a command with its last-used timestamp, returned by ListCommands.
type CommandEntry struct {
	Command  string
	LastUsed time.Time
}

// CommandLog is a single recorded execution of a command with its captured output path.
type CommandLog struct {
	ID         int64
	Command    string
	Directory  string
	ExecutedAt time.Time
	ExitCode   int
	LogFile    string
}
```

Add `"time"` to the import block if not already present.

- [ ] **Step 6: Change `ListCommands` return type to `[]CommandEntry`**

Replace the `ListCommands` function in `internal/db/db.go`:

```go
func ListCommands(ctx context.Context, store *Store, limit int, directory string, directoryAware bool) ([]CommandEntry, error) {
	var query string
	var args []any

	if directoryAware && directory != "" {
		query = `
			SELECT command, MAX(last_used) as last_used
			FROM commands
			GROUP BY command
			ORDER BY MAX(CASE WHEN directory = ? THEN 1 ELSE 0 END) DESC,
				CASE WHEN MAX(CASE WHEN directory = ? THEN 1 ELSE 0 END) = 1
					THEN MAX(CASE WHEN directory = ? THEN frequency END)
					ELSE SUM(frequency) END DESC,
				CASE WHEN MAX(CASE WHEN directory = ? THEN 1 ELSE 0 END) = 1
					THEN MAX(CASE WHEN directory = ? THEN last_used END)
					ELSE MAX(last_used) END DESC
		`
		args = []any{directory, directory, directory, directory, directory}
	} else {
		query = `
			SELECT command, MAX(last_used) as last_used
			FROM commands
			GROUP BY command
			ORDER BY SUM(frequency) DESC, MAX(last_used) DESC
		`
	}

	if limit > 0 {
		query += ` LIMIT ?`
		args = append(args, limit)
	}

	rows, err := store.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list commands: %w", err)
	}
	defer rows.Close()

	entries := []CommandEntry{}
	for rows.Next() {
		var command string
		var lastUsedUnix int64
		if err := rows.Scan(&command, &lastUsedUnix); err != nil {
			return nil, fmt.Errorf("scan command: %w", err)
		}
		entries = append(entries, CommandEntry{
			Command:  command,
			LastUsed: time.Unix(lastUsedUnix, 0),
		})
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate commands: %w", err)
	}

	return entries, nil
}
```

- [ ] **Step 7: Add `InsertCommandLog`, `ListCommandLogs`, `PruneCommandLogs` to `internal/db/db.go`**

Append after `ListCommands`:

```go
// InsertCommandLog records a single command execution and its log file path.
func InsertCommandLog(ctx context.Context, store *Store, command, directory string, executedAt time.Time, exitCode int, logFile string) error {
	_, err := store.ExecContext(ctx, `
		INSERT INTO command_logs (command, directory, executed_at, exit_code, log_file)
		VALUES (?, ?, ?, ?, ?)
	`, command, directory, executedAt.Unix(), exitCode, logFile)
	if err != nil {
		return fmt.Errorf("insert command log: %w", err)
	}
	return nil
}

// ListCommandLogs returns recorded command executions ordered by most recent first.
// Pass limit=0 for all rows.
func ListCommandLogs(ctx context.Context, store *Store, limit int) ([]CommandLog, error) {
	query := `
		SELECT id, command, directory, executed_at, exit_code, log_file
		FROM command_logs
		ORDER BY executed_at DESC
	`
	var args []any
	if limit > 0 {
		query += ` LIMIT ?`
		args = append(args, limit)
	}

	rows, err := store.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list command logs: %w", err)
	}
	defer rows.Close()

	var logs []CommandLog
	for rows.Next() {
		var cl CommandLog
		var executedAtUnix int64
		if err := rows.Scan(&cl.ID, &cl.Command, &cl.Directory, &executedAtUnix, &cl.ExitCode, &cl.LogFile); err != nil {
			return nil, fmt.Errorf("scan command log: %w", err)
		}
		cl.ExecutedAt = time.Unix(executedAtUnix, 0)
		logs = append(logs, cl)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate command logs: %w", err)
	}
	return logs, nil
}

// PruneCommandLogs deletes command_logs rows older than retentionDays and removes
// their log files from disk. Returns the number of rows deleted.
func PruneCommandLogs(ctx context.Context, store *Store, logsDir string, retentionDays int) (int64, error) {
	cutoff := time.Now().Unix() - int64(retentionDays)*86400

	rows, err := store.QueryContext(ctx,
		`SELECT log_file FROM command_logs WHERE executed_at < ?`, cutoff)
	if err != nil {
		return 0, fmt.Errorf("query stale logs: %w", err)
	}
	var files []string
	for rows.Next() {
		var f string
		if err := rows.Scan(&f); err != nil {
			rows.Close()
			return 0, fmt.Errorf("scan log file: %w", err)
		}
		files = append(files, f)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, fmt.Errorf("iterate stale logs: %w", err)
	}

	for _, f := range files {
		if err := os.Remove(f); err != nil && !os.IsNotExist(err) {
			return 0, fmt.Errorf("delete log file %s: %w", f, err)
		}
	}

	result, err := store.ExecContext(ctx,
		`DELETE FROM command_logs WHERE executed_at < ?`, cutoff)
	if err != nil {
		return 0, fmt.Errorf("delete stale log rows: %w", err)
	}
	deleted, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("count deleted rows: %w", err)
	}
	return deleted, nil
}
```

Make sure `"os"` is in the import block of `db.go`.

- [ ] **Step 8: Fix the existing test that calls `ListCommands`**

In `internal/db/migrate_test.go`, the `TestListCommandsDirectoryAwareRanksCurrentDirectoryFirst` test uses `ListCommands` and checks `[]string`. Update both call sites:

```go
// Replace:
global, err := db.ListCommands(ctx, store, 0, "/cwd", false)
// ...
if len(global) == 0 || global[0] != "global thing" {
    t.Fatalf("global order = %v, want 'global thing' first", global)
}

// With:
global, err := db.ListCommands(ctx, store, 0, "/cwd", false)
// ...
if len(global) == 0 || global[0].Command != "global thing" {
    t.Fatalf("global order = %v, want 'global thing' first", global)
}
```

And for the directory-aware check:

```go
// Replace:
if len(aware) < 2 {
    t.Fatalf("dir-aware order = %v, want at least 2 commands", aware)
}
if aware[0] != "local thing" {
    t.Fatalf("dir-aware first = %q, want 'local thing'", aware[0])
}

// With:
if len(aware) < 2 {
    t.Fatalf("dir-aware order = %v, want at least 2 commands", aware)
}
if aware[0].Command != "local thing" {
    t.Fatalf("dir-aware first = %q, want 'local thing'", aware[0].Command)
}
```

- [ ] **Step 9: Run all DB tests**

```bash
go test ./internal/db/... -v
```

Expected: all PASS including `TestSchemaMigratesV1ToV2AddsCommandLogs`.

- [ ] **Step 10: Commit**

```bash
git add internal/db/schema.go internal/db/db.go internal/db/migrate_test.go
git commit -m "feat(MEMSH-19): add command_logs schema v2 and DB functions"
```

---

### Task 3: Regular picker — `PickerItem` type + timestamp prefix display

**Files:**
- Modify: `internal/ui/picker.go`
- Modify: `cmd/memsh/command_picker.go`

**Interfaces:**
- Consumes: `db.CommandEntry{Command, LastUsed}` from Task 2
- Produces:
  - `ui.PickerItem{Display string, Value string}` — Display is shown/searched; Value is returned on selection
  - `ui.RunCommandPicker(title string, items []PickerItem, initialQuery string, output io.Writer) (string, error)` — returns `item.Value`

---

- [ ] **Step 1: Add `PickerItem` and update `RunCommandPicker` in `internal/ui/picker.go`**

Add the type before `pickerModel`:

```go
// PickerItem pairs a display string (shown and searched in the UI) with the
// underlying value returned on selection. When Value is empty, Display is used.
type PickerItem struct {
	Display string
	Value   string
}

func (p PickerItem) value() string {
	if p.Value != "" {
		return p.Value
	}
	return p.Display
}
```

Update `pickerModel` — replace `allCommands []string` and `filtered []string` with `PickerItem` slices, and `selected string` stays:

```go
type pickerModel struct {
	title    string
	help     string
	input    textinput.Model
	allItems []PickerItem
	filtered []PickerItem
	cursor   int
	selected string
	width    int
	height   int
	quitting bool
	cancelled bool
}
```

Update `RunCommandPicker` signature:

```go
func RunCommandPicker(title string, items []PickerItem, initialQuery string, output io.Writer) (string, error) {
	input := textinput.New()
	input.Placeholder = "Search commands"
	input.SetValue(initialQuery)
	input.Focus()
	input.CharLimit = 0
	input.Width = 50

	model := pickerModel{
		title:    title,
		help:     "Type to filter, Up/Down to move, Enter to select, Esc to cancel",
		input:    input,
		allItems: items,
	}
	model.filtered = model.filterItems()

	program := tea.NewProgram(model, tea.WithOutput(output))
	finalModel, err := program.Run()
	if err != nil {
		return "", err
	}

	result := finalModel.(pickerModel)
	if result.cancelled {
		return "", nil
	}

	return result.selected, nil
}
```

Update `Update` — replace `m.selected = m.filtered[m.cursor]` with:

```go
case "enter":
    if len(m.filtered) == 0 {
        return m, nil
    }
    m.selected = m.filtered[m.cursor].value()
    m.quitting = true
    return m, tea.Quit
```

Update `View` — replace references to `item.command` (the old `visibleCommand` type) with the new type. First update `visibleCommand`:

```go
type visibleCommand struct {
	index int
	item  PickerItem
}
```

Update `visibleCommands()` to return `[]visibleCommand` using `allItems`/`filtered`:

```go
func (m pickerModel) visibleCommands() []visibleCommand {
	if len(m.filtered) == 0 {
		return nil
	}

	maxItems := config.PickerMaxItems()
	if m.height > 12 {
		maxItems = min(12, m.height-8)
	}

	start := max(0, m.cursor-(maxItems/2))
	end := min(len(m.filtered), start+maxItems)
	if end-start < maxItems {
		start = max(0, end-maxItems)
	}

	items := make([]visibleCommand, 0, end-start)
	for index := start; index < end; index++ {
		items = append(items, visibleCommand{index: index, item: m.filtered[index]})
	}
	return items
}
```

Update the `View` render loop:

```go
for _, item := range visibleItems {
    if item.index == m.cursor {
        lines = append(lines, selectedStyle.Render("  "+item.item.Display))
    } else {
        lines = append(lines, normalStyle.Render("  "+truncate(item.item.Display, rowWidth)))
    }
}
```

Update `filterItems()` — replace `filterCommands()`:

```go
func (m pickerModel) filterItems() []PickerItem {
	query := strings.ToLower(strings.TrimSpace(m.input.Value()))
	if query == "" {
		return append([]PickerItem(nil), m.allItems...)
	}

	filtered := []PickerItem{}
	for _, item := range m.allItems {
		if strings.Contains(strings.ToLower(item.Display), query) {
			filtered = append(filtered, item)
		}
	}
	return filtered
}
```

Update calls to `filterItems` in `Update`:

```go
m.input, cmd = m.input.Update(msg)
m.filtered = m.filterItems()
if m.cursor >= len(m.filtered) {
    m.cursor = max(0, len(m.filtered)-1)
}
```

- [ ] **Step 2: Update `command_picker.go` to build `[]ui.PickerItem` with timestamp prefix**

In `cmd/memsh/command_picker.go`, update the import block to include `"time"` and `"fmt"` if not present. Replace the block that calls `db.ListCommands` and `ui.RunCommandPicker`:

```go
func runInteractivePicker(ctx context.Context, output io.Writer, initialQuery string, title string, outputFile string) error {
	database, err := openDatabase(ctx)
	if err != nil {
		return err
	}
	defer database.Close()

	directoryAware := config.DirectoryAwarenessEnabled()
	cwd := ""
	if directoryAware {
		if wd, wdErr := os.Getwd(); wdErr == nil {
			cwd = wd
		}
	}

	entries, err := db.ListCommands(ctx, database, 0, cwd, directoryAware)
	if err != nil {
		return err
	}

	if len(entries) == 0 {
		return errors.New("no stored commands found")
	}

	items := make([]ui.PickerItem, len(entries))
	for i, e := range entries {
		ts := formatTimestamp(e.LastUsed)
		items[i] = ui.PickerItem{
			Display: ts + "  " + e.Command,
			Value:   e.Command,
		}
	}

	selection, err := ui.RunCommandPicker(title, items, initialQuery, output)
	if err != nil {
		return err
	}
	if strings.TrimSpace(selection) == "" {
		return nil
	}

	if outputFile != "" {
		return os.WriteFile(outputFile, []byte(selection), 0o600)
	}

	_, err = fmt.Fprintln(output, selection)
	return err
}

// formatTimestamp formats a time as "Jan _2 15:04" (12 chars, space-padded day).
func formatTimestamp(t time.Time) string {
	return t.Format("Jan _2 15:04")
}
```

Also add `"time"` to imports and `"github.com/charandeep-motive/memsh/internal/db"` if not already imported.

- [ ] **Step 3: Build to catch compile errors**

```bash
go build ./...
```

Expected: no errors.

- [ ] **Step 4: Run all tests**

```bash
go test ./...
```

Expected: all PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/ui/picker.go cmd/memsh/command_picker.go
git commit -m "feat(MEMSH-19): add PickerItem type and timestamp prefix to regular picker"
```

---

### Task 4: `memsh record` — `--log-file` flag

**Files:**
- Modify: `cmd/memsh/command_record.go`

**Interfaces:**
- Consumes: `db.InsertCommandLog` from Task 2
- Produces: `memsh record --log-file <path>` CLI flag

---

- [ ] **Step 1: Add `--log-file` flag and conditional `InsertCommandLog` call**

Replace `cmd/memsh/command_record.go` entirely:

```go
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/charandeep-motive/memsh/internal/db"
	"github.com/charandeep-motive/memsh/internal/record"
)

func runRecord(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("record", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	command := fs.String("command", "", "command text to store")
	directory := fs.String("directory", "", "working directory for the command")
	exitCode := fs.Int("exit-code", 0, "command exit code")
	usedAt := fs.String("used-at", "", "unix timestamp override")
	logFile := fs.String("log-file", "", "path to captured output log file")

	if err := fs.Parse(args); err != nil {
		return err
	}

	entry := record.Entry{
		Command:   *command,
		Directory: *directory,
		ExitCode:  *exitCode,
		UsedAt:    time.Now(),
	}

	if *usedAt != "" {
		ts, err := strconv.ParseInt(*usedAt, 10, 64)
		if err != nil {
			return fmt.Errorf("parse used-at: %w", err)
		}
		entry.UsedAt = time.Unix(ts, 0)
	}

	database, err := openDatabase(ctx)
	if err != nil {
		return err
	}
	defer database.Close()

	if err := record.Store(ctx, database, entry); err != nil {
		return err
	}

	// Store log file reference when capture was enabled and command succeeded.
	if *logFile != "" && *exitCode == 0 && !isInternalRecordCommand(*command) {
		if err := db.InsertCommandLog(ctx, database,
			strings.TrimSpace(*command),
			*directory,
			entry.UsedAt,
			*exitCode,
			*logFile,
		); err != nil {
			// Non-fatal: log insertion failure should not affect command recording.
			fmt.Fprintf(os.Stderr, "memsh: store log reference: %v\n", err)
		}
	}

	return nil
}

func isInternalRecordCommand(command string) bool {
	cmd := strings.TrimSpace(command)
	return cmd == "memsh" || strings.HasPrefix(cmd, "memsh ")
}
```

- [ ] **Step 2: Build**

```bash
go build ./...
```

Expected: no errors.

- [ ] **Step 3: Smoke-test the flag manually**

```bash
echo "test output" > /tmp/test-capture.log
./bin/memsh record --command "echo hello" --directory "$PWD" --exit-code 0 --log-file /tmp/test-capture.log
```

Expected: exits 0, no errors printed.

- [ ] **Step 4: Commit**

```bash
git add cmd/memsh/command_record.go
git commit -m "feat(MEMSH-19): add --log-file flag to memsh record"
```

---

### Task 5: `memsh logs` and `memsh log-dir` commands

**Files:**
- Create: `cmd/memsh/command_logs.go`
- Modify: `cmd/memsh/router.go`
- Modify: `cmd/memsh/command_stats.go`

**Interfaces:**
- Consumes:
  - `config.ResolvePaths().LogsDir` from Task 1
  - `db.ListCommandLogs` from Task 2
  - `ui.RunLogsPicker` from Task 6 (Task 5 and 6 can be done together — `runLogs` stubs `RunLogsPicker` first)
- Produces:
  - `memsh log-dir` — prints `LogsDir` to stdout
  - `memsh logs` — opens logs picker

**Note:** Implement `runLogDir` and a stub `runLogs` now (Task 5). Full `RunLogsPicker` UI is in Task 6. Wire them together then.

---

- [ ] **Step 1: Add `logs` and `log-dir` to the router**

In `cmd/memsh/router.go`, add to the `switch` block:

```go
case "logs":
    return runLogs(ctx, args[1:], os.Stdout)
case "log-dir":
    return runLogDir()
```

- [ ] **Step 2: Create `cmd/memsh/command_logs.go`**

```go
package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"

	"github.com/charandeep-motive/memsh/internal/config"
	"github.com/charandeep-motive/memsh/internal/db"
	"github.com/charandeep-motive/memsh/internal/ui"
)

// runLogDir prints the logs directory path to stdout.
// Used by the zsh hook to pre-compute MEMSH_LOG_DIR on shell start.
func runLogDir() error {
	paths, err := config.ResolvePaths()
	if err != nil {
		return err
	}
	fmt.Println(paths.LogsDir)
	return nil
}

// runLogs opens the interactive logs picker.
func runLogs(ctx context.Context, _ []string, output io.Writer) error {
	database, err := openDatabase(ctx)
	if err != nil {
		return err
	}
	defer database.Close()

	logs, err := db.ListCommandLogs(ctx, database, 0)
	if err != nil {
		return err
	}

	if len(logs) == 0 {
		fmt.Fprintln(output, "no command logs found — enable with: memsh settings set MEMSH_SAVE_LOGS 1")
		return nil
	}

	selected, err := ui.RunLogsPicker("memsh logs", logs, output)
	if err != nil {
		return err
	}

	if selected == "" {
		return nil
	}

	if _, err := os.Stat(selected); os.IsNotExist(err) {
		fmt.Fprintln(output, "log file not found (expired)")
		return nil
	}

	// Open full log in less, preserving ANSI colours.
	cmd := exec.Command("less", "-R", selected)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
```

- [ ] **Step 3: Update `runDoctor` in `command_stats.go` to print `logs_dir`**

Add after the `database=` line:

```go
fmt.Printf("logs_dir=%s\n", paths.LogsDir)
```

- [ ] **Step 4: Build (will fail until `ui.RunLogsPicker` is defined in Task 6 — add a temporary stub)**

In `internal/ui/logs_picker.go`, create a temporary stub so the build succeeds:

```go
package ui

import (
	"io"

	"github.com/charandeep-motive/memsh/internal/db"
)

// RunLogsPicker opens the interactive logs picker and returns the selected log file path.
// Returns "" if the user cancels.
func RunLogsPicker(title string, logs []db.CommandLog, output io.Writer) (string, error) {
	// Implemented in Task 6.
	return "", nil
}
```

```bash
go build ./...
```

Expected: no errors.

- [ ] **Step 5: Commit**

```bash
git add cmd/memsh/router.go cmd/memsh/command_logs.go cmd/memsh/command_stats.go internal/ui/logs_picker.go
git commit -m "feat(MEMSH-19): add memsh logs and memsh log-dir commands"
```

---

### Task 6: Logs picker BubbleTea UI

**Files:**
- Modify: `internal/ui/logs_picker.go` (replace stub from Task 5)

**Interfaces:**
- Consumes: `db.CommandLog{ID, Command, Directory, ExecutedAt, ExitCode, LogFile}` from Task 2
- Produces: `ui.RunLogsPicker(title string, logs []db.CommandLog, output io.Writer) (string, error)` — returns selected `LogFile` path or `""`

---

- [ ] **Step 1: Replace the stub with the full BubbleTea model**

Replace `internal/ui/logs_picker.go` entirely:

```go
package ui

import (
	"io"
	"os"
	"regexp"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/charandeep-motive/memsh/internal/config"
	"github.com/charandeep-motive/memsh/internal/db"
)

// ansiEscape matches ANSI terminal escape sequences for stripping.
var ansiEscape = regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]`)

type logsPickerModel struct {
	title     string
	help      string
	input     textinput.Model
	allLogs   []db.CommandLog
	filtered  []db.CommandLog
	cursor    int
	selected  string
	preview   []string
	width     int
	height    int
	quitting  bool
	cancelled bool
}

// RunLogsPicker opens the interactive logs picker and returns the selected log file path.
// Returns "" if the user cancels or nothing is selected.
func RunLogsPicker(title string, logs []db.CommandLog, output io.Writer) (string, error) {
	input := textinput.New()
	input.Placeholder = "Search commands"
	input.Focus()
	input.CharLimit = 0
	input.Width = 50

	model := logsPickerModel{
		title:   title,
		help:    "Type to filter, Up/Down to move, Enter to view log, Esc to cancel",
		input:   input,
		allLogs: logs,
	}
	model.filtered = model.filterLogs()
	model.preview = model.loadPreview()

	program := tea.NewProgram(model, tea.WithOutput(output))
	finalModel, err := program.Run()
	if err != nil {
		return "", err
	}

	result := finalModel.(logsPickerModel)
	if result.cancelled {
		return "", nil
	}
	return result.selected, nil
}

func (m logsPickerModel) Init() tea.Cmd {
	return textinput.Blink
}

func (m logsPickerModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		if m.width > 0 {
			m.input.Width = min(60, m.width-10)
		}
		return m, nil
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "esc":
			m.cancelled = true
			m.quitting = true
			return m, tea.Quit
		case "enter":
			if len(m.filtered) == 0 {
				return m, nil
			}
			m.selected = m.filtered[m.cursor].LogFile
			m.quitting = true
			return m, tea.Quit
		case "up", "ctrl+p":
			if m.cursor > 0 {
				m.cursor--
				m.preview = m.loadPreview()
			}
			return m, nil
		case "down", "ctrl+n":
			if m.cursor < len(m.filtered)-1 {
				m.cursor++
				m.preview = m.loadPreview()
			}
			return m, nil
		}
	}

	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	m.filtered = m.filterLogs()
	if m.cursor >= len(m.filtered) {
		m.cursor = max(0, len(m.filtered)-1)
	}
	m.preview = m.loadPreview()
	return m, cmd
}

func (m logsPickerModel) View() string {
	if m.quitting {
		return ""
	}

	titleStyle    := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("110"))
	promptStyle   := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("111"))
	helpStyle     := lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	selectedStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("255")).Background(lipgloss.Color("238")).Bold(true)
	normalStyle   := lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
	emptyStyle    := lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Italic(true)
	previewStyle  := lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	borderStyle   := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("8")).Padding(1, 2)

	availableWidth := max(60, m.width-6)
	if configuredWidth := config.PickerWidth(); configuredWidth > 0 {
		availableWidth = min(configuredWidth, availableWidth)
	}
	if availableWidth > 140 {
		availableWidth = 140
	}
	separator := strings.Repeat("─", max(10, availableWidth-4))
	inputLine := promptStyle.Render("memsh> ") + m.input.View()

	lines := []string{titleStyle.Render(m.title), "", inputLine, helpStyle.Render(separator), ""}

	rowWidth := max(10, availableWidth-2-4-2)

	// Command list — up to 5 items in logs picker to leave room for preview.
	maxItems := 5
	if m.height > 20 {
		maxItems = 7
	}

	visible := m.visibleLogs(maxItems)
	if len(visible) == 0 {
		lines = append(lines, emptyStyle.Render("No matching logs"))
	} else {
		for _, v := range visible {
			ts := v.log.ExecutedAt.Format("Jan _2 15:04")
			display := ts + "  " + v.log.Command
			if v.index == m.cursor {
				lines = append(lines, selectedStyle.Render("  "+display))
			} else {
				lines = append(lines, normalStyle.Render("  "+truncate(display, rowWidth)))
			}
		}
	}

	// Preview section.
	lines = append(lines, "", helpStyle.Render("── preview "+strings.Repeat("─", max(5, availableWidth-14))))
	if len(m.preview) == 0 {
		lines = append(lines, emptyStyle.Render("  [no output captured]"))
	} else {
		for _, l := range m.preview {
			lines = append(lines, previewStyle.Render("  "+truncate(l, rowWidth)))
		}
	}

	lines = append(lines, "", helpStyle.Render(m.help))
	content := borderStyle.Width(availableWidth).Render(strings.Join(lines, "\n"))
	return lipgloss.NewStyle().Width(max(availableWidth, lipgloss.Width(content))).Render(content)
}

type visibleLog struct {
	index int
	log   db.CommandLog
}

func (m logsPickerModel) visibleLogs(maxItems int) []visibleLog {
	if len(m.filtered) == 0 {
		return nil
	}

	start := max(0, m.cursor-(maxItems/2))
	end := min(len(m.filtered), start+maxItems)
	if end-start < maxItems {
		start = max(0, end-maxItems)
	}

	items := make([]visibleLog, 0, end-start)
	for i := start; i < end; i++ {
		items = append(items, visibleLog{index: i, log: m.filtered[i]})
	}
	return items
}

func (m logsPickerModel) filterLogs() []db.CommandLog {
	query := strings.ToLower(strings.TrimSpace(m.input.Value()))
	if query == "" {
		return append([]db.CommandLog(nil), m.allLogs...)
	}
	filtered := []db.CommandLog{}
	for _, l := range m.allLogs {
		if strings.Contains(strings.ToLower(l.Command), query) {
			filtered = append(filtered, l)
		}
	}
	return filtered
}

// loadPreview reads up to 8 lines from the focused log file, stripping BSD
// script headers and ANSI escape sequences. Returns nil if nothing to show.
func (m logsPickerModel) loadPreview() []string {
	if len(m.filtered) == 0 {
		return nil
	}
	cl := m.filtered[m.cursor]

	data, err := os.ReadFile(cl.LogFile)
	if err != nil {
		if os.IsNotExist(err) {
			return []string{"[log expired]"}
		}
		return nil
	}
	if len(data) == 0 {
		return []string{"[no output captured]"}
	}

	rawLines := strings.Split(string(data), "\n")

	// Strip BSD script header/footer.
	cleaned := make([]string, 0, len(rawLines))
	for _, l := range rawLines {
		if strings.HasPrefix(l, "Script started on ") || strings.HasPrefix(l, "Script done on ") {
			continue
		}
		// Strip ANSI escape sequences and carriage returns.
		l = ansiEscape.ReplaceAllString(l, "")
		l = strings.ReplaceAll(l, "\r", "")
		if strings.TrimSpace(l) != "" {
			cleaned = append(cleaned, l)
		}
	}

	// Show last up to 8 lines.
	const maxPreviewLines = 8
	if len(cleaned) > maxPreviewLines {
		cleaned = cleaned[len(cleaned)-maxPreviewLines:]
	}
	return cleaned
}
```

- [ ] **Step 2: Build**

```bash
go build ./...
```

Expected: no errors.

- [ ] **Step 3: Smoke test `memsh logs` against a real log file**

```bash
mkdir -p ~/.local/share/memsh/logs
echo "hello world" > ~/.local/share/memsh/logs/test_1234abcd.log

# Insert a test row directly (adjust the db path if yours differs).
DB_PATH="$HOME/.local/share/memsh/memsh.db"
sqlite3 "$DB_PATH" "INSERT INTO command_logs (command, directory, executed_at, exit_code, log_file) VALUES ('echo hello', '$PWD', $(date +%s), 0, '$HOME/.local/share/memsh/logs/test_1234abcd.log');"

./bin/memsh logs
```

Expected: logs picker opens, shows `echo hello` with timestamp, preview shows `hello world`. Pressing Enter opens `less -R` with the file. Pressing Esc exits cleanly.

- [ ] **Step 4: Commit**

```bash
git add internal/ui/logs_picker.go
git commit -m "feat(MEMSH-19): add logs picker BubbleTea UI with inline preview"
```

---

### Task 7: Log pruning in `runClear`

**Files:**
- Modify: `cmd/memsh/command_maintenance.go`

**Interfaces:**
- Consumes:
  - `db.PruneCommandLogs(ctx, store, logsDir string, retentionDays int)` from Task 2
  - `config.LogRetentionDays()` from Task 1
  - `config.ResolvePaths().LogsDir` from Task 1

---

- [ ] **Step 1: Wire pruning into `runClear`**

In `cmd/memsh/command_maintenance.go`, update `runClear` to also prune logs after pruning commands. Replace the body:

```go
func runClear(ctx context.Context, input io.Reader, output io.Writer) error {
	confirmed, err := confirmAction(input, output, "Prune the least-used 10%% of stored commands and expired log files? [Y/n]: ")
	if err != nil {
		return err
	}
	if !confirmed {
		fmt.Fprintln(output, "clear cancelled")
		return nil
	}

	paths, err := config.ResolvePaths()
	if err != nil {
		return err
	}

	database, err := db.Open(ctx, paths.DatabasePath)
	if err != nil {
		return err
	}
	defer database.Close()

	cleared, err := db.PruneLeastUsedCommands(ctx, database)
	if err != nil {
		return err
	}
	fmt.Fprintf(output, "cleared: %d commands\n", cleared)

	pruned, err := db.PruneCommandLogs(ctx, database, paths.LogsDir, config.LogRetentionDays())
	if err != nil {
		return err
	}
	if pruned > 0 {
		fmt.Fprintf(output, "pruned: %d expired log files\n", pruned)
	}

	return nil
}
```

Add `"github.com/charandeep-motive/memsh/internal/config"` to the imports in `command_maintenance.go`.

Note: `runClear` previously used `openDatabase(ctx)` (which calls `config.ResolvePaths()` internally). We now call `ResolvePaths()` directly to get `LogsDir` and open the database ourselves. Remove the `openDatabase` call in the updated function.

- [ ] **Step 2: Build**

```bash
go build ./...
```

Expected: no errors.

- [ ] **Step 3: Commit**

```bash
git add cmd/memsh/command_maintenance.go
git commit -m "feat(MEMSH-19): prune expired log files during memsh clear"
```

---

### Task 8: Shell integration — `memsh.zsh`

**Files:**
- Modify: `shell/memsh.zsh`

**Interfaces:**
- Consumes: `memsh log-dir` from Task 5
- No Go changes — pure zsh.

---

- [ ] **Step 1: Add `MEMSH_LOG_DIR` initialisation and new ZLE widget**

Replace `shell/memsh.zsh` entirely with the updated version. The changes from the current file are:

1. Add `MEMSH_LOG_DIR` variable initialised from `memsh log-dir`.
2. Add `MEMSH_PENDING_COMMAND` and `MEMSH_LOG_FILE` globals.
3. Update `memsh_preexec` to prefer `MEMSH_PENDING_COMMAND`.
4. Update `memsh_precmd` to pass `--log-file` and clear state.
5. Add `memsh_accept_line` ZLE widget.
6. Register the widget when `MEMSH_SAVE_LOGS=1`.

```zsh
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
```

- [ ] **Step 2: Build the binary and reinstall**

```bash
make build 2>/dev/null || go build -o bin/memsh ./cmd/memsh
```

- [ ] **Step 3: Enable and test end-to-end**

```bash
# Enable logging in your settings.
./bin/memsh settings set MEMSH_SAVE_LOGS 1

# Reload the shell (source the updated .zsh file).
source shell/memsh.zsh

# Run a command — output should be captured.
git status

# Check the logs dir.
ls ~/.local/share/memsh/logs/

# Open the logs picker.
./bin/memsh logs
```

Expected:
- A `.log` file appears in `~/.local/share/memsh/logs/` after `git status`.
- `memsh logs` shows the `git status` entry with a timestamp prefix.
- Scrolling to it shows the `git status` output in the preview panel.
- Pressing Enter opens the full output in `less -R`.

- [ ] **Step 4: Commit**

```bash
git add shell/memsh.zsh
git commit -m "feat(MEMSH-19): capture command output via script in zsh accept-line"
```

---

## Self-Review: Spec Coverage

| Spec requirement | Task |
|---|---|
| `MEMSH_SAVE_LOGS` setting, default `0` | Task 1 |
| `MEMSH_LOG_RETENTION_DAYS` setting, default `10` | Task 1 |
| `LogsDir` added to `Paths` | Task 1 |
| Schema v2 migration — `command_logs` table, additive only | Task 2 |
| `ListCommands` returns `CommandEntry` with `LastUsed` | Task 2 |
| `InsertCommandLog`, `ListCommandLogs`, `PruneCommandLogs` | Task 2 |
| `PickerItem{Display, Value}` — timestamp shown, command returned | Task 3 |
| Regular picker shows `Jan _2 15:04  <command>` timestamp prefix | Task 3 |
| `memsh record --log-file` flag | Task 4 |
| `memsh log-dir` command | Task 5 |
| `memsh logs` command | Task 5 |
| `runDoctor` prints `logs_dir` | Task 5 |
| Logs picker with inline preview, ANSI strip, BSD header strip | Task 6 |
| Preview: expired → `[log expired]`, empty → `[no output captured]` | Task 6 |
| Log pruning on `memsh clear` | Task 7 |
| zsh `memsh_accept_line` ZLE widget with `script` wrapper | Task 8 |
| `memsh_preexec` uses `MEMSH_PENDING_COMMAND` | Task 8 |
| `memsh_precmd` passes `--log-file` and clears state | Task 8 |
| `MEMSH_LOG_DIR` pre-computed on shell load | Task 8 |
| Existing `commands` data preserved | Task 2 migration design |
