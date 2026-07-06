package db_test

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/charandeep-motive/memsh/internal/db"
	"github.com/charandeep-motive/memsh/internal/record"

	_ "modernc.org/sqlite"
)

// legacySchema mirrors the pre-directory-awareness schema, where command was
// unique on its own.
const legacySchema = `
CREATE TABLE commands (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    command TEXT NOT NULL UNIQUE,
    frequency INTEGER NOT NULL DEFAULT 1,
    last_used INTEGER NOT NULL,
    directory TEXT NOT NULL DEFAULT '',
    exit_code INTEGER NOT NULL DEFAULT 0
);
`

func TestOpenMigratesLegacyDatabaseWithoutDataLoss(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "memsh.db")

	// Build a legacy database directly, as an older memsh version would have.
	legacy, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open legacy database: %v", err)
	}
	if _, err := legacy.ExecContext(ctx, legacySchema); err != nil {
		t.Fatalf("create legacy schema: %v", err)
	}
	seed := []struct {
		command   string
		frequency int
		lastUsed  int64
		directory string
	}{
		{"git status", 5, 1_700_000_000, "/repo"},
		{"kubectl get pods", 3, 1_700_000_100, "/repo"},
		{"docker ps", 1, 1_700_000_200, ""},
	}
	for _, s := range seed {
		if _, err := legacy.ExecContext(ctx,
			`INSERT INTO commands (command, frequency, last_used, directory, exit_code) VALUES (?, ?, ?, ?, 0)`,
			s.command, s.frequency, s.lastUsed, s.directory); err != nil {
			t.Fatalf("seed legacy row %q: %v", s.command, err)
		}
	}
	if err := legacy.Close(); err != nil {
		t.Fatalf("close legacy database: %v", err)
	}

	// Opening through memsh migrates the schema in place.
	store, err := db.Open(ctx, dbPath)
	if err != nil {
		t.Fatalf("open and migrate: %v", err)
	}
	defer store.Close()

	// Every legacy row is preserved with its frequency, recency, directory.
	for _, s := range seed {
		var frequency int
		var lastUsed int64
		var directory string
		if err := store.QueryRowContext(ctx,
			`SELECT frequency, last_used, directory FROM commands WHERE command = ?`, s.command,
		).Scan(&frequency, &lastUsed, &directory); err != nil {
			t.Fatalf("query migrated row %q: %v", s.command, err)
		}
		if frequency != s.frequency || lastUsed != s.lastUsed || directory != s.directory {
			t.Fatalf("row %q = (freq %d, last %d, dir %q), want (freq %d, last %d, dir %q)",
				s.command, frequency, lastUsed, directory, s.frequency, s.lastUsed, s.directory)
		}
	}

	// After migration the composite key allows the same command in a new
	// directory to be tracked separately rather than overwriting.
	if err := record.Store(ctx, store, record.Entry{Command: "git status", Directory: "/other", ExitCode: 0}); err != nil {
		t.Fatalf("store command in new directory: %v", err)
	}
	var rowCount int
	if err := store.QueryRowContext(ctx, `SELECT COUNT(*) FROM commands WHERE command = ?`, "git status").Scan(&rowCount); err != nil {
		t.Fatalf("count git status rows: %v", err)
	}
	if rowCount != 2 {
		t.Fatalf("git status row count = %d, want 2 after recording in a new directory", rowCount)
	}
}

func TestListCommandsDirectoryAwareRanksCurrentDirectoryFirst(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store, err := db.Open(ctx, filepath.Join(t.TempDir(), "memsh.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()

	// A very frequent command used elsewhere, and a rarely used command in cwd.
	for range 10 {
		if err := record.Store(ctx, store, record.Entry{Command: "global thing", Directory: "/elsewhere", ExitCode: 0}); err != nil {
			t.Fatalf("seed global command: %v", err)
		}
	}
	if err := record.Store(ctx, store, record.Entry{Command: "local thing", Directory: "/cwd", ExitCode: 0}); err != nil {
		t.Fatalf("seed local command: %v", err)
	}

	// Global mode ranks the frequent command first.
	global, err := db.ListCommands(ctx, store, 0, "/cwd", false)
	if err != nil {
		t.Fatalf("list global: %v", err)
	}
	if len(global) == 0 || global[0] != "global thing" {
		t.Fatalf("global order = %v, want 'global thing' first", global)
	}

	// Directory-aware mode lifts the current-directory command above the
	// far more frequent global command.
	aware, err := db.ListCommands(ctx, store, 0, "/cwd", true)
	if err != nil {
		t.Fatalf("list dir-aware: %v", err)
	}
	if len(aware) < 2 {
		t.Fatalf("dir-aware order = %v, want at least 2 commands", aware)
	}
	if aware[0] != "local thing" {
		t.Fatalf("dir-aware first = %q, want 'local thing'", aware[0])
	}
}
