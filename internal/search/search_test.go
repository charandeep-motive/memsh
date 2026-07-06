package search_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/charandeep-motive/memsh/internal/db"
	"github.com/charandeep-motive/memsh/internal/record"
	"github.com/charandeep-motive/memsh/internal/search"
)

func TestFindRanksPrefixAndLimitsResults(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	database := openStore(t)
	defer database.Close()

	now := time.Unix(1_700_000_500, 0)
	entries := []struct {
		command   string
		directory string
		usedAt    time.Time
		repeats   int
	}{
		{command: "kubectl get pods", directory: "/repo", usedAt: now.Add(-30 * time.Minute), repeats: 4},
		{command: "kubectl config use-context dev", directory: "/repo", usedAt: now.Add(-2 * time.Hour), repeats: 3},
		{command: "kubectl config use-context prod", directory: "/repo", usedAt: now.Add(-90 * time.Minute), repeats: 2},
		{command: "kubectl top pods", directory: "/repo", usedAt: now.Add(-3 * time.Hour), repeats: 1},
		{command: "kubectl describe pod api", directory: "/repo", usedAt: now.Add(-4 * time.Hour), repeats: 1},
		{command: "kubectl logs deployment/api", directory: "/repo", usedAt: now.Add(-5 * time.Hour), repeats: 1},
	}

	for _, entry := range entries {
		for range entry.repeats {
			if err := record.Store(ctx, database, record.Entry{
				Command:   entry.command,
				Directory: entry.directory,
				ExitCode:  0,
				UsedAt:    entry.usedAt,
			}); err != nil {
				t.Fatalf("store %q: %v", entry.command, err)
			}
		}
	}

	results, err := search.Find(ctx, database, search.Query{
		Text:      "kubectl",
		Directory: "/repo",
		Limit:     5,
		Now:       now,
	})
	if err != nil {
		t.Fatalf("find results: %v", err)
	}

	if len(results) != 5 {
		t.Fatalf("len(results) = %d, want 5", len(results))
	}

	if results[0].Command != "kubectl get pods" {
		t.Fatalf("top result = %q, want kubectl get pods", results[0].Command)
	}

	for i := 1; i < len(results); i++ {
		if results[i-1].Score < results[i].Score {
			t.Fatalf("results not sorted by descending score: %+v then %+v", results[i-1], results[i])
		}
	}
}

func TestFindDirectoryAwarePrefersCurrentDirectory(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	database := openStore(t)
	defer database.Close()

	now := time.Unix(1_700_000_500, 0)

	// A frequent, recent match used in another directory.
	for range 8 {
		if err := record.Store(ctx, database, record.Entry{
			Command:   "kubectl get pods",
			Directory: "/other",
			ExitCode:  0,
			UsedAt:    now.Add(-10 * time.Minute),
		}); err != nil {
			t.Fatalf("store other-dir command: %v", err)
		}
	}

	// A single, older match used in the current directory.
	if err := record.Store(ctx, database, record.Entry{
		Command:   "kubectl get svc",
		Directory: "/cwd",
		ExitCode:  0,
		UsedAt:    now.Add(-6 * time.Hour),
	}); err != nil {
		t.Fatalf("store cwd command: %v", err)
	}

	// Global mode: the frequent, recent command wins.
	global, err := search.Find(ctx, database, search.Query{Text: "kubectl", Directory: "/cwd", Limit: 5, Now: now})
	if err != nil {
		t.Fatalf("find global: %v", err)
	}
	if len(global) == 0 || global[0].Command != "kubectl get pods" {
		t.Fatalf("global top = %+v, want 'kubectl get pods'", global)
	}

	// Directory-aware mode: the current-directory command is lifted to the top.
	aware, err := search.Find(ctx, database, search.Query{Text: "kubectl", Directory: "/cwd", Limit: 5, Now: now, DirectoryAware: true})
	if err != nil {
		t.Fatalf("find dir-aware: %v", err)
	}
	if len(aware) == 0 || aware[0].Command != "kubectl get svc" {
		t.Fatalf("dir-aware top = %+v, want 'kubectl get svc'", aware)
	}
}

func openStore(t *testing.T) *db.Store {
	t.Helper()

	store, err := db.Open(context.Background(), filepath.Join(t.TempDir(), "memsh.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}

	return store
}
