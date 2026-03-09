package main

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"memsh/internal/config"
	"memsh/internal/db"
	"memsh/internal/record"
)

func TestRunDeleteRemovesCommand(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())

	ctx := context.Background()
	store, err := openDatabase(ctx)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}

	if err := record.Store(ctx, store, record.Entry{Command: "git status", ExitCode: 0}); err != nil {
		store.Close()
		t.Fatalf("seed command: %v", err)
	}
	store.Close()

	output, err := captureStdout(func() error {
		return run(ctx, []string{"--delete", "git status"})
	})
	if err != nil {
		t.Fatalf("run delete: %v", err)
	}

	if !strings.Contains(output, "deleted: git status") {
		t.Fatalf("delete output = %q, want deleted confirmation", output)
	}

	paths, err := config.ResolvePaths()
	if err != nil {
		t.Fatalf("resolve paths: %v", err)
	}

	store, err = db.Open(ctx, filepath.Join(paths.DataDir, "memsh.db"))
	if err != nil {
		t.Fatalf("reopen database: %v", err)
	}
	defer store.Close()

	stats, err := db.ReadStats(ctx, store)
	if err != nil {
		t.Fatalf("read stats: %v", err)
	}

	if stats.TotalRecords != 0 {
		t.Fatalf("total records = %d, want 0", stats.TotalRecords)
	}
}

func TestRunHelpPrintsUsage(t *testing.T) {
	output, err := captureStdout(func() error {
		return run(context.Background(), []string{"--help"})
	})
	if err != nil {
		t.Fatalf("run help: %v", err)
	}

	if !strings.Contains(output, "memsh --delete \"git status\"") {
		t.Fatalf("help output missing delete usage: %q", output)
	}

	if !strings.Contains(output, "memsh --help") {
		t.Fatalf("help output missing help usage: %q", output)
	}
}

func captureStdout(runFunc func() error) (string, error) {
	originalStdout := os.Stdout
	reader, writer, err := os.Pipe()
	if err != nil {
		return "", err
	}

	os.Stdout = writer
	runErr := runFunc()
	writer.Close()
	os.Stdout = originalStdout

	var buffer bytes.Buffer
	if _, err := io.Copy(&buffer, reader); err != nil {
		return "", err
	}

	reader.Close()
	return buffer.String(), runErr
}
