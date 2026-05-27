package tasks_test

import (
	"context"
	"testing"
	"time"

	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"

	"github.com/drzero42/nexorious/internal/config"
	"github.com/drzero42/nexorious/internal/ratelimit"
	igdbsvc "github.com/drzero42/nexorious/internal/services/igdb"
	"github.com/drzero42/nexorious/internal/worker/tasks"
)

func TestMetadataFetchWorker_Success(t *testing.T) {
	truncateAllTables(t)
	// Bare game row: title set, description NULL (as created by sync Stage 2/3).
	insertTestGame(t, 3001, "Old Title", time.Now().Add(-24*time.Hour))

	// No cover field — avoids a real IGDB CDN download in tests.
	gamesJSON := `[{"id":3001,"name":"Fetched Title","slug":"fetched-title","summary":"A great game"}]`
	srv := igdbTestServer(t, gamesJSON)
	defer srv.Close()
	igdbClient := newTestIGDBClient(t, srv)

	w := &tasks.MetadataFetchWorker{DB: testDB, IGDBClient: igdbClient, StoragePath: t.TempDir()}
	job := &river.Job[tasks.MetadataFetchArgs]{
		JobRow: &rivertype.JobRow{Attempt: 1, MaxAttempts: 3},
		Args:   tasks.MetadataFetchArgs{GameID: 3001},
	}
	if err := w.Work(context.Background(), job); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}

	ctx := context.Background()
	var title string
	var description *string
	_ = testDB.NewRaw(`SELECT title, description FROM games WHERE id = 3001`).Scan(ctx, &title, &description)
	if title != "Fetched Title" {
		t.Errorf("title: want 'Fetched Title', got %q", title)
	}
	if description == nil || *description != "A great game" {
		t.Errorf("description: want 'A great game', got %v", description)
	}
}

func TestMetadataFetchWorker_IGDBNotConfigured(t *testing.T) {
	truncateAllTables(t)
	insertTestGame(t, 3005, "Untouched", time.Now().Add(-24*time.Hour))

	unconfigured := igdbsvc.NewClient(&config.Config{}, ratelimit.NewLocal(100, 100))
	w := &tasks.MetadataFetchWorker{DB: testDB, IGDBClient: unconfigured, StoragePath: t.TempDir()}
	job := &river.Job[tasks.MetadataFetchArgs]{
		JobRow: &rivertype.JobRow{Attempt: 1, MaxAttempts: 3},
		Args:   tasks.MetadataFetchArgs{GameID: 3005},
	}
	// Returns nil (no retry) and leaves the game untouched.
	if err := w.Work(context.Background(), job); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}

	var description *string
	_ = testDB.NewRaw(`SELECT description FROM games WHERE id = 3005`).Scan(context.Background(), &description)
	if description != nil {
		t.Errorf("description: want nil (untouched), got %v", *description)
	}
}

func TestMetadataFetchWorker_RetriesThenGivesUp(t *testing.T) {
	truncateAllTables(t)
	insertTestGame(t, 3010, "Not In IGDB", time.Now().Add(-24*time.Hour))

	// Empty IGDB response → FetchFullMetadata returns ErrGameNotFound.
	srv := igdbTestServer(t, `[]`)
	defer srv.Close()
	igdbClient := newTestIGDBClient(t, srv)
	w := &tasks.MetadataFetchWorker{DB: testDB, IGDBClient: igdbClient, StoragePath: t.TempDir()}

	// Non-final attempt → returns an error so River retries.
	nonFinal := &river.Job[tasks.MetadataFetchArgs]{
		JobRow: &rivertype.JobRow{Attempt: 1, MaxAttempts: 3},
		Args:   tasks.MetadataFetchArgs{GameID: 3010},
	}
	if err := w.Work(context.Background(), nonFinal); err == nil {
		t.Error("non-final attempt: want error (so River retries), got nil")
	}

	// Final attempt → logs at error and returns nil (no further retry, no noise).
	final := &river.Job[tasks.MetadataFetchArgs]{
		JobRow: &rivertype.JobRow{Attempt: 3, MaxAttempts: 3},
		Args:   tasks.MetadataFetchArgs{GameID: 3010},
	}
	if err := w.Work(context.Background(), final); err != nil {
		t.Errorf("final attempt: want nil, got %v", err)
	}
}
