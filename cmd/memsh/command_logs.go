package main

import (
	"bytes"
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

	data, err := os.ReadFile(selected)
	if err != nil {
		if os.IsNotExist(err) {
			fmt.Fprintln(output, "log file not found (expired)")
			return nil
		}
		return err
	}

	// Flatten carriage-return overwrites (e.g. git/download progress bars) and
	// drop script headers before paging, keeping ANSI colours. less reads the
	// cleaned content from stdin and the keyboard from the controlling TTY.
	cmd := exec.Command("less", "-R")
	cmd.Stdin = bytes.NewReader(ui.RenderForPager(data))
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
