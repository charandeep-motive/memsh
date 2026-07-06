package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/charandeep-motive/memsh/internal/record"
)

func runRecord(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("record", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	command := fs.String("command", "", "command text to store")
	directory := fs.String("directory", "", "working directory for the command")
	exitCode := fs.Int("exit-code", 0, "command exit code")
	usedAt := fs.String("used-at", "", "unix timestamp override")

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

	return record.Store(ctx, database, entry)
}
