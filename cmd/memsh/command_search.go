package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/charandeep-motive/memsh/internal/config"
	"github.com/charandeep-motive/memsh/internal/search"
)

func runSearch(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("search", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	query := fs.String("query", "", "query prefix or substring")
	limit := fs.Int("limit", config.SuggestionLimit(), "max results")
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
		Text:           *query,
		Directory:      *cwd,
		Limit:          *limit,
		Now:            time.Now(),
		DirectoryAware: config.DirectoryAwarenessEnabled(),
	})
	if err != nil {
		return err
	}

	for _, result := range results {
		fmt.Println(result.Command)
	}

	return nil
}
