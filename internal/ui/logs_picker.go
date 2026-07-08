package ui

import (
	"io"

	"github.com/charandeep-motive/memsh/internal/db"
)

// RunLogsPicker opens the interactive logs picker and returns the selected log file path.
// Returns "" if the user cancels.
func RunLogsPicker(title string, logs []db.CommandLog, output io.Writer) (string, error) {
	// Implemented in Task 6.
	return "", nil
}
