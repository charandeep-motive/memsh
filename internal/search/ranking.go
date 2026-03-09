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

func Score(candidate Candidate, query string, cwd string, now time.Time) float64 {
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

	if cwd != "" && candidate.Directory == cwd {
		score += 2
	}

	return score
}
