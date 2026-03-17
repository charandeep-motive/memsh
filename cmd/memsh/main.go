package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/charandeep-motive/memsh/internal/config"
	"github.com/charandeep-motive/memsh/internal/db"
	"github.com/charandeep-motive/memsh/internal/record"
	"github.com/charandeep-motive/memsh/internal/search"
	"github.com/charandeep-motive/memsh/internal/ui"
)

const version = "0.1.0"

func main() {
	ctx := context.Background()
	if err := run(ctx, os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

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
	default:
		return fmt.Errorf("unknown command %q", args[0])
	}
}

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

func runSearch(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("search", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	query := fs.String("query", "", "query prefix or substring")
	limit := fs.Int("limit", defaultSuggestionLimit(), "max results")
	cwd := fs.String("directory", "", "current working directory")

	if err := fs.Parse(args); err != nil {
		return err
	}

	if *query == "" && fs.NArg() > 0 {
		*query = strings.Join(fs.Args(), " ")
	}

	if strings.TrimSpace(*query) == "" {
		return errors.New("search query is required")
	}

	database, err := openDatabase(ctx)
	if err != nil {
		return err
	}
	defer database.Close()

	results, err := search.Find(ctx, database, search.Query{
		Text:      *query,
		Directory: *cwd,
		Limit:     *limit,
		Now:       time.Now(),
	})
	if err != nil {
		return err
	}

	for _, result := range results {
		fmt.Println(result.Command)
	}

	return nil
}

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

func runStats(ctx context.Context) error {
	database, err := openDatabase(ctx)
	if err != nil {
		return err
	}
	defer database.Close()

	stats, err := db.ReadStats(ctx, database)
	if err != nil {
		return err
	}

	fmt.Printf("total records: %d\n", stats.TotalRecords)
	fmt.Printf("unique commands: %d\n", stats.UniqueCommands)
	for _, command := range stats.TopCommands {
		fmt.Printf("top: %s\n", command)
	}

	return nil
}

func runDoctor(ctx context.Context) error {
	paths, err := config.ResolvePaths()
	if err != nil {
		return err
	}

	database, err := db.Open(ctx, paths.DatabasePath)
	if err != nil {
		return err
	}
	defer database.Close()

	stats, err := db.ReadStats(ctx, database)
	if err != nil {
		return err
	}

	fmt.Printf("data_dir=%s\n", paths.DataDir)
	fmt.Printf("config_dir=%s\n", paths.ConfigDir)
	fmt.Printf("settings=%s\n", paths.SettingsPath)
	fmt.Printf("database=%s\n", paths.DatabasePath)
	fmt.Printf("unique_commands=%d\n", stats.UniqueCommands)
	return nil
}

func runSettings(args []string, output io.Writer) error {
	paths, err := config.ResolvePaths()
	if err != nil {
		return err
	}

	stored, err := config.ReadSettingsFile(paths.SettingsPath)
	if err != nil {
		return err
	}

	if len(args) == 0 || args[0] == "list" {
		return printSettings(output, paths.SettingsPath, stored)
	}

	switch args[0] {
	case "path":
		_, err := fmt.Fprintln(output, paths.SettingsPath)
		return err
	case "set":
		if len(args) < 3 {
			return errors.New("usage: memsh settings set KEY VALUE")
		}
		key := strings.TrimSpace(args[1])
		value := strings.TrimSpace(strings.Join(args[2:], " "))
		if err := config.ValidateSetting(key, value); err != nil {
			return err
		}
		stored[key] = value
		if err := config.WriteSettingsFile(paths.SettingsPath, stored); err != nil {
			return err
		}
		_, err := fmt.Fprintf(output, "saved %s=%s\nreload shell: source ~/.zshrc\n", key, value)
		return err
	case "unset":
		if len(args) != 2 {
			return errors.New("usage: memsh settings unset KEY")
		}
		key := strings.TrimSpace(args[1])
		if _, ok := config.LookupSetting(key); !ok {
			return fmt.Errorf("unsupported setting %q", key)
		}
		delete(stored, key)
		if err := config.WriteSettingsFile(paths.SettingsPath, stored); err != nil {
			return err
		}
		_, err := fmt.Fprintf(output, "removed %s\nreload shell: source ~/.zshrc\n", key)
		return err
	default:
		return fmt.Errorf("unknown settings command %q", args[0])
	}
}

func printSettings(output io.Writer, settingsPath string, stored map[string]string) error {
	defaults := config.DefaultSettings()
	if _, err := fmt.Fprintf(output, "settings_file=%s\n", settingsPath); err != nil {
		return err
	}

	for _, spec := range config.SupportedSettings() {
		value, ok := stored[spec.Key]
		source := "default"
		if ok {
			source = "saved"
		} else {
			value = defaults[spec.Key]
		}

		if _, err := fmt.Fprintf(output, "%s=%s [%s]\n", spec.Key, value, source); err != nil {
			return err
		}
		if _, err := fmt.Fprintf(output, "  %s (%s)\n", spec.Description, spec.ValueHint); err != nil {
			return err
		}
	}

	_, err := fmt.Fprintln(output, "\ncommands: memsh settings set KEY VALUE | memsh settings unset KEY | memsh settings path")
	return err
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

	commands, err := db.ListCommands(ctx, database, 0)
	if err != nil {
		return err
	}

	if len(commands) == 0 {
		return errors.New("no stored commands found")
	}

	selection, err := ui.RunCommandPicker(title, commands, initialQuery, output)
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

func openDatabase(ctx context.Context) (*db.Store, error) {
	paths, err := config.ResolvePaths()
	if err != nil {
		return nil, err
	}

	return db.Open(ctx, paths.DatabasePath)
}

func printUsage() {
	fmt.Println(`memsh records shell commands and returns ranked suggestions.

Usage:
  memsh
	Open the interactive command search box

  memsh help
	Show this help text

  memsh settings
	List configurable memsh settings and current values

  memsh settings set MEMSH_MAX_SUGGESTIONS 10
	Persist a memsh setting under ~/.config/memsh/settings.zsh

  memsh pick --query "git"
	Open the interactive picker with a pre-filled query

  memsh delete "git status"
	Delete an exact command from the suggestion database

  memsh clear
	Prune the least-used 10% of stored commands

  memsh destroy
	Destroy all stored commands

  memsh record --command "git status" --directory "$PWD" --exit-code 0
	Record a successful command

  memsh search --query "git" --limit 5
	Search for up to 5 ranked suggestions

  memsh stats
	Show stored command stats

  memsh doctor
	Show resolved paths and DB health

  memsh version
	Show memsh version`)
}

func defaultSuggestionLimit() int {
	value := strings.TrimSpace(os.Getenv("MEMSH_MAX_SUGGESTIONS"))
	if value == "" {
		return 5
	}

	limit, err := strconv.Atoi(value)
	if err != nil || limit <= 0 {
		return 5
	}

	return limit
}

func confirmAction(input io.Reader, output io.Writer, prompt string) (bool, error) {
	reader := bufio.NewReader(input)

	for {
		fmt.Fprint(output, prompt)
		answer, err := reader.ReadString('\n')
		if err != nil && !errors.Is(err, io.EOF) {
			return false, fmt.Errorf("read confirmation: %w", err)
		}

		answer = strings.TrimSpace(strings.ToLower(answer))
		switch answer {
		case "", "y", "yes":
			return true, nil
		case "n", "no":
			return false, nil
		}

		if errors.Is(err, io.EOF) {
			return false, nil
		}

		fmt.Fprintln(output, "Please answer Y or n.")
	}
}
