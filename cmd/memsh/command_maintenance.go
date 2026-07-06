package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/charandeep-motive/memsh/internal/db"
)

func runDelete(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return errors.New("delete command text is required")
	}

	command := strings.TrimSpace(strings.Join(args, " "))
	if command == "" {
		return errors.New("delete command text is required")
	}

	database, err := openDatabase(ctx)
	if err != nil {
		return err
	}
	defer database.Close()

	deleted, err := db.DeleteCommand(ctx, database, command)
	if err != nil {
		return err
	}

	if !deleted {
		return fmt.Errorf("command not found: %q", command)
	}

	fmt.Printf("deleted: %s\n", command)
	return nil
}

func runClear(ctx context.Context, input io.Reader, output io.Writer) error {
	confirmed, err := confirmAction(input, output, "Prune the least-used 10% of stored commands? [Y/n]: ")
	if err != nil {
		return err
	}
	if !confirmed {
		fmt.Fprintln(output, "clear cancelled")
		return nil
	}

	database, err := openDatabase(ctx)
	if err != nil {
		return err
	}
	defer database.Close()

	cleared, err := db.PruneLeastUsedCommands(ctx, database)
	if err != nil {
		return err
	}

	fmt.Fprintf(output, "cleared: %d commands\n", cleared)
	return nil
}

func runDestroy(ctx context.Context, input io.Reader, output io.Writer) error {
	confirmed, err := confirmAction(input, output, "Destroy all stored commands? [Y/n]: ")
	if err != nil {
		return err
	}
	if !confirmed {
		fmt.Fprintln(output, "destroy cancelled")
		return nil
	}

	database, err := openDatabase(ctx)
	if err != nil {
		return err
	}
	defer database.Close()

	destroyed, err := db.DestroyCommands(ctx, database)
	if err != nil {
		return err
	}

	fmt.Fprintf(output, "destroyed: %d commands\n", destroyed)
	return nil
}
