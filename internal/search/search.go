package search

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/charandeep-motive/memsh/internal/db"
)

type Query struct {
	Text      string
	Directory string
	Limit     int
	Now       time.Time
}

type Result struct {
	Command   string
	Score     float64
	Frequency int
	LastUsed  time.Time
	Directory string
}

func Find(ctx context.Context, database *db.Store, query Query) ([]Result, error) {
	trimmed := strings.TrimSpace(query.Text)
	if trimmed == "" {
		return nil, nil
	}

	limit := query.Limit
	if limit <= 0 {
		limit = 5
	}

	now := query.Now
	if now.IsZero() {
		now = time.Now()
	}

	rows, err := database.QueryContext(ctx, `
		SELECT command, frequency, last_used, directory, exit_code
		FROM commands
		WHERE lower(command) LIKE '%' || lower(?) || '%'
		LIMIT 100
	`, trimmed)
	if err != nil {
		return nil, fmt.Errorf("search commands: %w", err)
	}
	defer rows.Close()

	results := make([]Result, 0, limit)
	for rows.Next() {
		var candidate Candidate
		var lastUsed int64
		if err := rows.Scan(&candidate.Command, &candidate.Frequency, &lastUsed, &candidate.Directory, &candidate.ExitCode); err != nil {
			return nil, fmt.Errorf("scan candidate: %w", err)
		}

		candidate.LastUsed = time.Unix(lastUsed, 0)
		results = append(results, Result{
			Command:   candidate.Command,
			Score:     Score(candidate, trimmed, query.Directory, now),
			Frequency: candidate.Frequency,
			LastUsed:  candidate.LastUsed,
			Directory: candidate.Directory,
		})
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate candidates: %w", err)
	}

	sort.Slice(results, func(i, j int) bool {
		if results[i].Score == results[j].Score {
			if results[i].Frequency == results[j].Frequency {
				return results[i].LastUsed.After(results[j].LastUsed)
			}
			return results[i].Frequency > results[j].Frequency
		}
		return results[i].Score > results[j].Score
	})

	if len(results) > limit {
		results = results[:limit]
	}

	return results, nil
}
