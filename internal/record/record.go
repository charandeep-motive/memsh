package record

import (
	"context"
	"fmt"
	"strings"
	"time"

	"memsh/internal/db"
)

type Entry struct {
	Command   string
	Directory string
	ExitCode  int
	UsedAt    time.Time
}

func Store(ctx context.Context, database *db.Store, entry Entry) error {
	command := strings.TrimSpace(entry.Command)
	if command == "" {
		return nil
	}

	if entry.ExitCode != 0 {
		return nil
	}

	if isInternalCommand(command) {
		return nil
	}

	usedAt := entry.UsedAt
	if usedAt.IsZero() {
		usedAt = time.Now()
	}

	_, err := database.ExecContext(ctx, `
		INSERT INTO commands (command, frequency, last_used, directory, exit_code)
		VALUES (?, 1, ?, ?, ?)
		ON CONFLICT(command) DO UPDATE SET
			frequency = commands.frequency + 1,
			last_used = excluded.last_used,
			directory = excluded.directory,
			exit_code = excluded.exit_code
	`, command, usedAt.Unix(), entry.Directory, entry.ExitCode)
	if err != nil {
		return fmt.Errorf("store command: %w", err)
	}

	return nil
}

func isInternalCommand(command string) bool {
	return command == "memsh" || strings.HasPrefix(command, "memsh ")
}
