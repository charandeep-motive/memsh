package main

import (
	"context"

	"github.com/charandeep-motive/memsh/internal/config"
	"github.com/charandeep-motive/memsh/internal/db"
)

// openDatabase resolves the configured database path and opens the store.
func openDatabase(ctx context.Context) (*db.Store, error) {
	paths, err := config.ResolvePaths()
	if err != nil {
		return nil, err
	}

	return db.Open(ctx, paths.DatabasePath)
}
