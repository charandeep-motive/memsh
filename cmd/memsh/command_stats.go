package main

import (
	"context"
	"fmt"

	"github.com/charandeep-motive/memsh/internal/config"
	"github.com/charandeep-motive/memsh/internal/db"
)

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
	fmt.Printf("logs_dir=%s\n", paths.LogsDir)
	fmt.Printf("unique_commands=%d\n", stats.UniqueCommands)
	return nil
}
