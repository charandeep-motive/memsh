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
	Text           string
	Directory      string
	Limit          int
	Now            time.Time
	DirectoryAware bool
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

	// Rows are keyed by (command, directory), so aggregate to one candidate
	// per command. cwd-specific frequency and recency are computed alongside
	// the global totals so directory-aware ranking can prefer the current
	// directory without a second query.
	rows, err := database.QueryContext(ctx, `
		SELECT command,
			SUM(frequency) AS total_freq,
			MAX(last_used) AS recent,
			MAX(CASE WHEN directory = ? THEN 1 ELSE 0 END) AS in_cwd,
			COALESCE(MAX(CASE WHEN directory = ? THEN frequency END), 0) AS cwd_freq,
			COALESCE(MAX(CASE WHEN directory = ? THEN last_used END), 0) AS cwd_recent
		FROM commands
		WHERE lower(command) LIKE '%' || lower(?) || '%'
		GROUP BY command
		LIMIT 200
	`, query.Directory, query.Directory, query.Directory, trimmed)
	if err != nil {
		return nil, fmt.Errorf("search commands: %w", err)
	}
	defer rows.Close()

	results := make([]Result, 0, limit)
	for rows.Next() {
		var command string
		var totalFreq, recent, inCwd, cwdFreq, cwdRecent int64
		if err := rows.Scan(&command, &totalFreq, &recent, &inCwd, &cwdFreq, &cwdRecent); err != nil {
			return nil, fmt.Errorf("scan candidate: %w", err)
		}

		candidate := Candidate{Command: command}
		matchesCwd := query.DirectoryAware && query.Directory != "" && inCwd == 1
		if matchesCwd {
			candidate.Frequency = int(cwdFreq)
			candidate.LastUsed = time.Unix(cwdRecent, 0)
			candidate.Directory = query.Directory
		} else {
			candidate.Frequency = int(totalFreq)
			candidate.LastUsed = time.Unix(recent, 0)
		}

		results = append(results, Result{
			Command:   candidate.Command,
			Score:     Score(candidate, trimmed, query.Directory, now, query.DirectoryAware),
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
