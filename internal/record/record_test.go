package record_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"memsh/internal/db"
	"memsh/internal/record"
)

func TestStoreUpsertsFrequency(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	database := openStore(t)
	defer database.Close()

	firstUse := time.Unix(1_700_000_000, 0)
	secondUse := firstUse.Add(time.Hour)

	if err := record.Store(ctx, database, record.Entry{
		Command:   "git status",
		Directory: "/tmp/project-a",
		ExitCode:  0,
		UsedAt:    firstUse,
	}); err != nil {
		t.Fatalf("store first command: %v", err)
	}

	if err := record.Store(ctx, database, record.Entry{
		Command:   "git status",
		Directory: "/tmp/project-b",
		ExitCode:  0,
		UsedAt:    secondUse,
	}); err != nil {
		t.Fatalf("store duplicate command: %v", err)
	}

	var frequency int
	var lastUsed int64
	var directory string
	var exitCode int
	if err := database.QueryRowContext(ctx, `
		SELECT frequency, last_used, directory, exit_code
		FROM commands
		WHERE command = ?
	`, "git status").Scan(&frequency, &lastUsed, &directory, &exitCode); err != nil {
		t.Fatalf("query stored row: %v", err)
	}

	if frequency != 2 {
		t.Fatalf("frequency = %d, want 2", frequency)
	}

	if lastUsed != secondUse.Unix() {
		t.Fatalf("last_used = %d, want %d", lastUsed, secondUse.Unix())
	}

	if directory != "/tmp/project-b" {
		t.Fatalf("directory = %q, want /tmp/project-b", directory)
	}

	if exitCode != 0 {
		t.Fatalf("exit_code = %d, want 0", exitCode)
	}
}

func TestStoreSkipsFailedCommands(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	database := openStore(t)
	defer database.Close()

	if err := record.Store(ctx, database, record.Entry{
		Command:   "git statsu",
		Directory: "/tmp/project-a",
		ExitCode:  127,
		UsedAt:    time.Unix(1_700_000_000, 0),
	}); err != nil {
		t.Fatalf("store failed command: %v", err)
	}

	var count int
	if err := database.QueryRowContext(ctx, `SELECT COUNT(*) FROM commands`).Scan(&count); err != nil {
		t.Fatalf("count commands: %v", err)
	}

	if count != 0 {
		t.Fatalf("count = %d, want 0", count)
	}
}

func openStore(t *testing.T) *db.Store {
	t.Helper()

	store, err := db.Open(context.Background(), filepath.Join(t.TempDir(), "memsh.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}

	return store
}
