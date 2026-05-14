package scheduler_test

import (
	"context"
	"testing"

	"github.com/drzero42/nexorious-go/internal/config"
	"github.com/drzero42/nexorious-go/internal/scheduler"
	"github.com/drzero42/nexorious-go/internal/worker"
)

// TestScheduler_StartStop verifies that Start creates the gocron scheduler and
// Stop shuts it down without error. No backup service is provided so the
// backup-job registration branch is skipped.
func TestScheduler_StartStop(t *testing.T) {
	truncateAllTables(t)
	pool := worker.NewPool(testDB)
	defer pool.Shutdown()

	cfg := &config.Config{
		MetadataRefreshInterval: "24h",
		StaleJobThreshold:       "4h",
	}

	sched := scheduler.NewScheduler(testDB, pool, nil, cfg)

	ctx := context.Background()
	if err := sched.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// Stop should not panic or error.
	sched.Stop()
}

// TestScheduler_StopWithoutStart verifies that Stop is safe to call before Start.
func TestScheduler_StopWithoutStart(t *testing.T) {
	truncateAllTables(t)
	pool := worker.NewPool(testDB)
	defer pool.Shutdown()

	cfg := &config.Config{
		MetadataRefreshInterval: "24h",
		StaleJobThreshold:       "4h",
	}

	sched := scheduler.NewScheduler(testDB, pool, nil, cfg)
	// Should not panic (scheduler field is nil, guarded by nil check in Stop).
	sched.Stop()
}
