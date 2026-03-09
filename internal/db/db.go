package db

import (
	"context"
	"database/sql"
	"fmt"
	"math"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

type Store struct {
	*sql.DB
}

type Stats struct {
	TotalRecords   int
	UniqueCommands int
	TopCommands    []string
}

func PruneLeastUsedCommands(ctx context.Context, store *Store) (int64, error) {
	var total int64
	if err := store.QueryRowContext(ctx, `SELECT COUNT(*) FROM commands`).Scan(&total); err != nil {
		return 0, fmt.Errorf("count commands: %w", err)
	}

	if total == 0 {
		return 0, nil
	}

	pruneCount := int64(math.Ceil(float64(total) * 0.10))
	if pruneCount < 1 {
		pruneCount = 1
	}

	result, err := store.ExecContext(ctx, `
		DELETE FROM commands
		WHERE id IN (
			SELECT id
			FROM commands
			ORDER BY frequency ASC, last_used ASC, id ASC
			LIMIT ?
		)
	`, pruneCount)
	if err != nil {
		return 0, fmt.Errorf("prune commands: %w", err)
	}

	deleted, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("count pruned rows: %w", err)
	}

	return deleted, nil
}

func DestroyCommands(ctx context.Context, store *Store) (int64, error) {
	result, err := store.ExecContext(ctx, `DELETE FROM commands`)
	if err != nil {
		return 0, fmt.Errorf("destroy commands: %w", err)
	}

	deleted, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("count destroyed rows: %w", err)
	}

	return deleted, nil
}

func DeleteCommand(ctx context.Context, store *Store, command string) (bool, error) {
	result, err := store.ExecContext(ctx, `DELETE FROM commands WHERE command = ?`, command)
	if err != nil {
		return false, fmt.Errorf("delete command: %w", err)
	}

	deleted, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("count deleted rows: %w", err)
	}

	return deleted > 0, nil
}

func Open(ctx context.Context, databasePath string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(databasePath), 0o755); err != nil {
		return nil, fmt.Errorf("create data dir: %w", err)
	}

	database, err := sql.Open("sqlite", databasePath)
	if err != nil {
		return nil, fmt.Errorf("open sqlite database: %w", err)
	}

	if _, err := database.ExecContext(ctx, "PRAGMA journal_mode=WAL;"); err != nil {
		database.Close()
		return nil, fmt.Errorf("enable wal mode: %w", err)
	}

	if _, err := database.ExecContext(ctx, "PRAGMA busy_timeout=1000;"); err != nil {
		database.Close()
		return nil, fmt.Errorf("set busy timeout: %w", err)
	}

	if _, err := database.ExecContext(ctx, schema); err != nil {
		database.Close()
		return nil, fmt.Errorf("initialize schema: %w", err)
	}

	return &Store{DB: database}, nil
}

func ReadStats(ctx context.Context, store *Store) (Stats, error) {
	var stats Stats

	if err := store.QueryRowContext(ctx, `SELECT COUNT(*) FROM commands`).Scan(&stats.TotalRecords); err != nil {
		return Stats{}, fmt.Errorf("read total records: %w", err)
	}

	stats.UniqueCommands = stats.TotalRecords

	rows, err := store.QueryContext(ctx, `
		SELECT command
		FROM commands
		ORDER BY frequency DESC, last_used DESC
		LIMIT 5
	`)
	if err != nil {
		return Stats{}, fmt.Errorf("read top commands: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var command string
		if err := rows.Scan(&command); err != nil {
			return Stats{}, fmt.Errorf("scan top command: %w", err)
		}
		stats.TopCommands = append(stats.TopCommands, command)
	}

	if err := rows.Err(); err != nil {
		return Stats{}, fmt.Errorf("iterate top commands: %w", err)
	}

	return stats, nil
}
