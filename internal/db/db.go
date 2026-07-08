package db

import (
	"context"
	"database/sql"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"time"

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

// CommandEntry is a command with its last-used timestamp, returned by ListCommands.
type CommandEntry struct {
	Command  string
	LastUsed time.Time
}

// CommandLog is a single recorded execution of a command with its captured output path.
type CommandLog struct {
	ID         int64
	Command    string
	Directory  string
	ExecutedAt time.Time
	ExitCode   int
	LogFile    string
}

// ListCommands returns distinct commands ordered for the picker. In global
// mode commands are ranked by total frequency and recency across all
// directories. When directoryAware is set and a directory is provided,
// commands used in that directory rank first (by their in-directory
// frequency and recency), followed by everything else as a global fallback.
func ListCommands(ctx context.Context, store *Store, limit int, directory string, directoryAware bool) ([]CommandEntry, error) {
	var query string
	var args []any

	if directoryAware && directory != "" {
		query = `
			SELECT command, MAX(last_used) as last_used
			FROM commands
			GROUP BY command
			ORDER BY MAX(CASE WHEN directory = ? THEN 1 ELSE 0 END) DESC,
				CASE WHEN MAX(CASE WHEN directory = ? THEN 1 ELSE 0 END) = 1
					THEN MAX(CASE WHEN directory = ? THEN frequency END)
					ELSE SUM(frequency) END DESC,
				CASE WHEN MAX(CASE WHEN directory = ? THEN 1 ELSE 0 END) = 1
					THEN MAX(CASE WHEN directory = ? THEN last_used END)
					ELSE MAX(last_used) END DESC
		`
		args = []any{directory, directory, directory, directory, directory}
	} else {
		query = `
			SELECT command, MAX(last_used) as last_used
			FROM commands
			GROUP BY command
			ORDER BY SUM(frequency) DESC, MAX(last_used) DESC
		`
	}

	if limit > 0 {
		query += ` LIMIT ?`
		args = append(args, limit)
	}

	rows, err := store.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list commands: %w", err)
	}
	defer rows.Close()

	entries := []CommandEntry{}
	for rows.Next() {
		var command string
		var lastUsedUnix int64
		if err := rows.Scan(&command, &lastUsedUnix); err != nil {
			return nil, fmt.Errorf("scan command: %w", err)
		}
		entries = append(entries, CommandEntry{
			Command:  command,
			LastUsed: time.Unix(lastUsedUnix, 0),
		})
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate commands: %w", err)
	}

	return entries, nil
}

// InsertCommandLog records a single command execution and its log file path.
func InsertCommandLog(ctx context.Context, store *Store, command, directory string, executedAt time.Time, exitCode int, logFile string) error {
	_, err := store.ExecContext(ctx, `
		INSERT INTO command_logs (command, directory, executed_at, exit_code, log_file)
		VALUES (?, ?, ?, ?, ?)
	`, command, directory, executedAt.Unix(), exitCode, logFile)
	if err != nil {
		return fmt.Errorf("insert command log: %w", err)
	}
	return nil
}

// ListCommandLogs returns recorded command executions ordered by most recent first.
// Pass limit=0 for all rows.
func ListCommandLogs(ctx context.Context, store *Store, limit int) ([]CommandLog, error) {
	query := `
		SELECT id, command, directory, executed_at, exit_code, log_file
		FROM command_logs
		ORDER BY executed_at DESC
	`
	var args []any
	if limit > 0 {
		query += ` LIMIT ?`
		args = append(args, limit)
	}

	rows, err := store.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list command logs: %w", err)
	}
	defer rows.Close()

	var logs []CommandLog
	for rows.Next() {
		var cl CommandLog
		var executedAtUnix int64
		if err := rows.Scan(&cl.ID, &cl.Command, &cl.Directory, &executedAtUnix, &cl.ExitCode, &cl.LogFile); err != nil {
			return nil, fmt.Errorf("scan command log: %w", err)
		}
		cl.ExecutedAt = time.Unix(executedAtUnix, 0)
		logs = append(logs, cl)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate command logs: %w", err)
	}
	return logs, nil
}

// PruneCommandLogs deletes command_logs rows older than retentionDays and removes
// their log files from disk. Returns the number of rows deleted.
func PruneCommandLogs(ctx context.Context, store *Store, logsDir string, retentionDays int) (int64, error) {
	cutoff := time.Now().Unix() - int64(retentionDays)*86400

	// Collect file paths before deleting rows.
	rows, err := store.QueryContext(ctx,
		`SELECT log_file FROM command_logs WHERE executed_at < ?`, cutoff)
	if err != nil {
		return 0, fmt.Errorf("query stale logs: %w", err)
	}
	var files []string
	for rows.Next() {
		var f string
		if err := rows.Scan(&f); err != nil {
			rows.Close()
			return 0, fmt.Errorf("scan log file: %w", err)
		}
		files = append(files, f)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, fmt.Errorf("iterate stale logs: %w", err)
	}

	// Delete DB rows first — if this fails, files remain on disk (harmless).
	result, err := store.ExecContext(ctx,
		`DELETE FROM command_logs WHERE executed_at < ?`, cutoff)
	if err != nil {
		return 0, fmt.Errorf("delete stale log rows: %w", err)
	}
	deleted, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("count deleted rows: %w", err)
	}

	// Delete files after rows are gone — orphaned files are benign.
	for _, f := range files {
		if !filepath.IsAbs(f) {
			f = filepath.Join(logsDir, f)
		}
		if err := os.Remove(f); err != nil && !os.IsNotExist(err) {
			return 0, fmt.Errorf("delete log file %s: %w", f, err)
		}
	}

	return deleted, nil
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

	if err := migrate(ctx, database); err != nil {
		database.Close()
		return nil, fmt.Errorf("initialize schema: %w", err)
	}

	return &Store{DB: database}, nil
}

// migrate brings the database up to the current schemaVersion, preserving
// existing rows. Fresh databases get the latest schema directly; legacy
// databases (UNIQUE on command alone) are rebuilt into the composite
// (command, directory) key. Progress is tracked via PRAGMA user_version.
func migrate(ctx context.Context, database *sql.DB) error {
	var userVersion int
	if err := database.QueryRowContext(ctx, "PRAGMA user_version;").Scan(&userVersion); err != nil {
		return fmt.Errorf("read schema version: %w", err)
	}

	if userVersion >= schemaVersion {
		// Already current; ensure the table exists for brand-new files.
		if _, err := database.ExecContext(ctx, schema); err != nil {
			return fmt.Errorf("ensure schema: %w", err)
		}
		return nil
	}

	var tableExists int
	if err := database.QueryRowContext(ctx,
		"SELECT count(*) FROM sqlite_master WHERE type='table' AND name='commands';",
	).Scan(&tableExists); err != nil {
		return fmt.Errorf("inspect schema: %w", err)
	}

	tx, err := database.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin migration: %w", err)
	}
	defer tx.Rollback()

	if tableExists == 0 {
		// Fresh install — apply full schema.
		if _, err := tx.ExecContext(ctx, schema); err != nil {
			return fmt.Errorf("create schema: %w", err)
		}
	} else if userVersion == 0 {
		// v0 → v1: rebuild commands with composite key.
		if _, err := tx.ExecContext(ctx, migrateToCompositeKey); err != nil {
			return fmt.Errorf("migrate to composite key: %w", err)
		}
		// v1 → v2: add command_logs.
		if _, err := tx.ExecContext(ctx, migrateAddCommandLogs); err != nil {
			return fmt.Errorf("migrate add command_logs: %w", err)
		}
	} else if userVersion == 1 {
		// v1 → v2: add command_logs.
		if _, err := tx.ExecContext(ctx, migrateAddCommandLogs); err != nil {
			return fmt.Errorf("migrate add command_logs: %w", err)
		}
	}

	if _, err := tx.ExecContext(ctx, fmt.Sprintf("PRAGMA user_version = %d;", schemaVersion)); err != nil {
		return fmt.Errorf("set schema version: %w", err)
	}

	return tx.Commit()
}

func ReadStats(ctx context.Context, store *Store) (Stats, error) {
	var stats Stats

	if err := store.QueryRowContext(ctx, `SELECT COUNT(*) FROM commands`).Scan(&stats.TotalRecords); err != nil {
		return Stats{}, fmt.Errorf("read total records: %w", err)
	}

	if err := store.QueryRowContext(ctx, `SELECT COUNT(DISTINCT command) FROM commands`).Scan(&stats.UniqueCommands); err != nil {
		return Stats{}, fmt.Errorf("read unique commands: %w", err)
	}

	rows, err := store.QueryContext(ctx, `
		SELECT command
		FROM commands
		GROUP BY command
		ORDER BY SUM(frequency) DESC, MAX(last_used) DESC
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
