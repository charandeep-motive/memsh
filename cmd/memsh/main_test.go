package main

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/charandeep-motive/memsh/internal/config"
	"github.com/charandeep-motive/memsh/internal/db"
	"github.com/charandeep-motive/memsh/internal/record"
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
		return run(context.Background(), []string{"help"})
	})
	if err != nil {
		t.Fatalf("run help: %v", err)
	}

	if !strings.Contains(output, "memsh delete \"git status\"") {
		t.Fatalf("help output missing delete usage: %q", output)
	}

	if !strings.Contains(output, "memsh help") {
		t.Fatalf("help output missing help usage: %q", output)
	}

	if !strings.Contains(output, "memsh clear") {
		t.Fatalf("help output missing clear usage: %q", output)
	}

	if !strings.Contains(output, "memsh destroy") {
		t.Fatalf("help output missing destroy usage: %q", output)
	}

	if !strings.Contains(output, "memsh settings") {
		t.Fatalf("help output missing settings usage: %q", output)
	}
}

func TestRunSettingsSetAndUnset(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	var output bytes.Buffer
	if err := runSettings([]string{"set", "MEMSH_MAX_SUGGESTIONS", "9"}, &output); err != nil {
		t.Fatalf("run settings set: %v", err)
	}

	paths, err := config.ResolvePaths()
	if err != nil {
		t.Fatalf("resolve paths: %v", err)
	}

	content, err := os.ReadFile(paths.SettingsPath)
	if err != nil {
		t.Fatalf("read settings file: %v", err)
	}
	if !strings.Contains(string(content), `export MEMSH_MAX_SUGGESTIONS="9"`) {
		t.Fatalf("settings file missing saved value: %q", string(content))
	}

	output.Reset()
	if err := runSettings([]string{"unset", "MEMSH_MAX_SUGGESTIONS"}, &output); err != nil {
		t.Fatalf("run settings unset: %v", err)
	}

	content, err = os.ReadFile(paths.SettingsPath)
	if err != nil {
		t.Fatalf("read settings file after unset: %v", err)
	}
	if strings.Contains(string(content), "MEMSH_MAX_SUGGESTIONS") {
		t.Fatalf("settings file still contains removed key: %q", string(content))
	}
}

func TestRunSettingsRejectsInvalidValue(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	err := runSettings([]string{"set", "MEMSH_AUTOSUGGEST", "yes"}, io.Discard)
	if err == nil {
		t.Fatal("expected invalid settings value error")
	}
}

func TestRunClearPrunesLeastUsedCommands(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())

	ctx := context.Background()
	store, err := openDatabase(ctx)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}

	entries := []struct {
		command string
		repeat  int
	}{
		{command: "git status", repeat: 5},
		{command: "kubectl get pods", repeat: 4},
		{command: "docker ps", repeat: 3},
		{command: "ls", repeat: 2},
		{command: "pwd", repeat: 1},
		{command: "whoami", repeat: 1},
		{command: "date", repeat: 1},
		{command: "env", repeat: 1},
		{command: "make test", repeat: 1},
		{command: "kubectl config current-context", repeat: 1},
	}
	for _, entry := range entries {
		for range entry.repeat {
			if err := record.Store(ctx, store, record.Entry{Command: entry.command, ExitCode: 0}); err != nil {
				store.Close()
				t.Fatalf("seed command %q: %v", entry.command, err)
			}
		}
	}
	store.Close()

	output, err := captureOutputWithInput("y\n", func(input io.Reader, output io.Writer) error {
		return runClear(ctx, input, output)
	})
	if err != nil {
		t.Fatalf("run clear: %v", err)
	}

	if !strings.Contains(output, "cleared: 1 commands") {
		t.Fatalf("clear output = %q, want cleared count", output)
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

	if stats.TotalRecords != 9 {
		t.Fatalf("total records = %d, want 9", stats.TotalRecords)
	}

	var remaining int
	if err := store.QueryRowContext(ctx, `SELECT COUNT(*) FROM commands WHERE command = ?`, "pwd").Scan(&remaining); err != nil {
		t.Fatalf("query pruned command: %v", err)
	}
	if remaining != 0 {
		t.Fatalf("expected least-used command to be pruned")
	}
}

func TestRunDestroyRemovesAllCommands(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())

	ctx := context.Background()
	store, err := openDatabase(ctx)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}

	entries := []string{"git status", "kubectl get pods", "docker ps"}
	for _, command := range entries {
		if err := record.Store(ctx, store, record.Entry{Command: command, ExitCode: 0}); err != nil {
			store.Close()
			t.Fatalf("seed command %q: %v", command, err)
		}
	}
	store.Close()

	output, err := captureOutputWithInput("y\n", func(input io.Reader, output io.Writer) error {
		return runDestroy(ctx, input, output)
	})
	if err != nil {
		t.Fatalf("run destroy: %v", err)
	}

	if !strings.Contains(output, "destroyed: 3 commands") {
		t.Fatalf("destroy output = %q, want destroyed count", output)
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

func TestRunDestroyCancelled(t *testing.T) {
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

	output, err := captureOutputWithInput("n\n", func(input io.Reader, output io.Writer) error {
		return runDestroy(ctx, input, output)
	})
	if err != nil {
		t.Fatalf("run destroy cancelled: %v", err)
	}

	if !strings.Contains(output, "destroy cancelled") {
		t.Fatalf("destroy cancel output = %q, want cancel message", output)
	}
}

func TestDefaultSuggestionLimitFromEnv(t *testing.T) {
	t.Setenv("MEMSH_MAX_SUGGESTIONS", "9")

	if got := defaultSuggestionLimit(); got != 9 {
		t.Fatalf("defaultSuggestionLimit() = %d, want 9", got)
	}
}

func TestDefaultSuggestionLimitFallsBack(t *testing.T) {
	t.Setenv("MEMSH_MAX_SUGGESTIONS", "0")
	if got := defaultSuggestionLimit(); got != 5 {
		t.Fatalf("defaultSuggestionLimit() with zero = %d, want 5", got)
	}

	t.Setenv("MEMSH_MAX_SUGGESTIONS", "bad")
	if got := defaultSuggestionLimit(); got != 5 {
		t.Fatalf("defaultSuggestionLimit() with invalid env = %d, want 5", got)
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

func captureOutputWithInput(input string, runFunc func(io.Reader, io.Writer) error) (string, error) {
	var output bytes.Buffer
	err := runFunc(strings.NewReader(input), &output)
	return output.String(), err
}
