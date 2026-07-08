package main

import (
	"context"
	"fmt"
	"os"
)

// run dispatches a parsed argument list to the matching command handler.
func run(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return runInteractiveSearch(ctx, os.Stdout)
	}

	switch args[0] {
	case "help", "--help", "-h":
		printUsage()
		return nil
	case "settings":
		return runSettings(args[1:], os.Stdout)
	case "pick":
		return runPick(ctx, args[1:], os.Stdout)
	case "clear", "--clear":
		return runClear(ctx, os.Stdin, os.Stdout)
	case "destroy", "--destroy":
		return runDestroy(ctx, os.Stdin, os.Stdout)
	case "delete", "--delete":
		return runDelete(ctx, args[1:])
	case "version", "--version", "-v":
		fmt.Println(version)
		return nil
	case "record":
		return runRecord(ctx, args[1:])
	case "search":
		return runSearch(ctx, args[1:])
	case "stats":
		return runStats(ctx)
	case "doctor":
		return runDoctor(ctx)
	case "logs":
		return runLogs(ctx, args[1:], os.Stdout)
	case "log-dir":
		return runLogDir()
	default:
		return fmt.Errorf("unknown command %q", args[0])
	}
}
