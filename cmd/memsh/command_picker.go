package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/charandeep-motive/memsh/internal/config"
	"github.com/charandeep-motive/memsh/internal/db"
	"github.com/charandeep-motive/memsh/internal/ui"
)

// formatTimestamp formats a time as "Jan _2 15:04" (12 chars, space-padded day).
func formatTimestamp(t time.Time) string {
	return t.Format("Jan _2 15:04")
}

func runInteractiveSearch(ctx context.Context, output io.Writer) error {
	return runInteractivePicker(ctx, output, "", "memsh command search", "")
}

func runPick(ctx context.Context, args []string, output io.Writer) error {
	fs := flag.NewFlagSet("pick", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	query := fs.String("query", "", "initial query for the picker")
	outputFile := fs.String("output-file", "", "write the selected command to a file instead of stdout")
	if err := fs.Parse(args); err != nil {
		return err
	}

	return runInteractivePicker(ctx, output, *query, "memsh suggestions", *outputFile)
}

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
