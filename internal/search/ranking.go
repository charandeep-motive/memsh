package search

import (
	"math"
	"strings"
	"time"
)

type Candidate struct {
	Command   string
	Frequency int
	LastUsed  time.Time
	Directory string
	ExitCode  int
}

// directoryMatchBonus is added when directory awareness is enabled and a
// candidate was used in the current directory. It is large enough to lift
// current-directory commands above unrelated global commands regardless of
// their frequency or recency, while candidates within the directory still
// order among themselves by the usual signals.
const directoryMatchBonus = 40

func Score(candidate Candidate, query string, cwd string, now time.Time, directoryAware bool) float64 {
	trimmedQuery := strings.ToLower(strings.TrimSpace(query))
	command := strings.ToLower(candidate.Command)

	score := 3 * math.Log1p(float64(candidate.Frequency))

	ageHours := now.Sub(candidate.LastUsed).Hours()
	if ageHours < 0 {
		ageHours = 0
	}
	score += 12 / (1 + ageHours)

	if strings.HasPrefix(command, trimmedQuery) {
		score += 10
	} else if strings.Contains(command, trimmedQuery) {
		score += 4
	}

	if directoryAware && cwd != "" && candidate.Directory == cwd {
		score += directoryMatchBonus
	}

	return score
}
