package record_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/charandeep-motive/memsh/internal/db"
	"github.com/charandeep-motive/memsh/internal/record"
)

func TestStoreUpsertsFrequencyPerDirectory(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	database := openStore(t)
	defer database.Close()

	firstUse := time.Unix(1_700_000_000, 0)
	secondUse := firstUse.Add(time.Hour)

	// Same command in the same directory bumps that directory's frequency.
	for _, usedAt := range []time.Time{firstUse, secondUse} {
		if err := record.Store(ctx, database, record.Entry{
			Command:   "git status",
			Directory: "/tmp/project-a",
			ExitCode:  0,
			UsedAt:    usedAt,
		}); err != nil {
			t.Fatalf("store command in project-a: %v", err)
		}
	}

	// Same command in a different directory is tracked as its own row.
	if err := record.Store(ctx, database, record.Entry{
		Command:   "git status",
		Directory: "/tmp/project-b",
		ExitCode:  0,
		UsedAt:    secondUse,
	}); err != nil {
		t.Fatalf("store command in project-b: %v", err)
	}

	var rowCount int
	if err := database.QueryRowContext(ctx, `SELECT COUNT(*) FROM commands WHERE command = ?`, "git status").Scan(&rowCount); err != nil {
		t.Fatalf("count rows: %v", err)
	}
	if rowCount != 2 {
		t.Fatalf("row count = %d, want 2 (one per directory)", rowCount)
	}

	var frequencyA int
	var lastUsedA int64
	if err := database.QueryRowContext(ctx, `
		SELECT frequency, last_used FROM commands WHERE command = ? AND directory = ?
	`, "git status", "/tmp/project-a").Scan(&frequencyA, &lastUsedA); err != nil {
		t.Fatalf("query project-a row: %v", err)
	}
	if frequencyA != 2 {
		t.Fatalf("project-a frequency = %d, want 2", frequencyA)
	}
	if lastUsedA != secondUse.Unix() {
		t.Fatalf("project-a last_used = %d, want %d", lastUsedA, secondUse.Unix())
	}

	var frequencyB int
	if err := database.QueryRowContext(ctx, `
		SELECT frequency FROM commands WHERE command = ? AND directory = ?
	`, "git status", "/tmp/project-b").Scan(&frequencyB); err != nil {
		t.Fatalf("query project-b row: %v", err)
	}
	if frequencyB != 1 {
		t.Fatalf("project-b frequency = %d, want 1", frequencyB)
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

func TestStoreSkipsMemshCommands(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	database := openStore(t)
	defer database.Close()

	commands := []string{
		"memsh stats",
		"memsh clear",
		"memsh destroy",
		"memsh delete \"git status\"",
	}

	for _, command := range commands {
		if err := record.Store(ctx, database, record.Entry{
			Command:   command,
			Directory: "/tmp/project-a",
			ExitCode:  0,
			UsedAt:    time.Unix(1_700_000_000, 0),
		}); err != nil {
			t.Fatalf("store memsh command %q: %v", command, err)
		}
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
