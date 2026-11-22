package api_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/drzero42/nexorious/internal/db/models"
)

// An insert that leaves a timestamp unset must get the column's DEFAULT now(),
// not Go's zero time (#1314: without nullzero Bun wrote 0001-01-01 explicitly,
// which is how import job items ended up at year 1 in #1311). The struct must
// also get the stored value back, since handlers return it to the client.
func TestModelInsert_UnsetTimestampsUseDBDefault(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()
	insertAuthTestUser(t, testDB, "u-ts", "tsuser", "pass123", true, false)
	insertJob(t, testDB, "job-ts", "u-ts", "import", "csv", "processing")

	item := &models.JobItem{
		ID: "item-ts", JobID: "job-ts", UserID: "u-ts", ItemKey: "game_0", SourceTitle: "Celeste",
		SourceMetadata: json.RawMessage(`{}`), Status: "pending",
		Result: json.RawMessage(`{}`), IGDBCandidates: json.RawMessage(`[]`),
	}
	tag := &models.Tag{ID: "tag-ts", UserID: "u-ts", Name: "cozy"}
	game := &models.Game{ID: 920001, Title: "Celeste"}

	for _, m := range []any{item, tag, game} {
		if _, err := testDB.NewInsert().Model(m).Exec(ctx); err != nil {
			t.Fatalf("insert %T: %v", m, err)
		}
	}

	for _, c := range []struct {
		name, query string
		returned    time.Time
	}{
		{"job_items.created_at", `SELECT created_at FROM job_items WHERE id = 'item-ts'`, item.CreatedAt},
		{"tags.created_at", `SELECT created_at FROM tags WHERE id = 'tag-ts'`, tag.CreatedAt},
		{"tags.updated_at", `SELECT updated_at FROM tags WHERE id = 'tag-ts'`, tag.UpdatedAt},
		{"games.created_at", `SELECT created_at FROM games WHERE id = 920001`, game.CreatedAt},
		{"games.last_updated", `SELECT last_updated FROM games WHERE id = 920001`, game.LastUpdated},
	} {
		var stored time.Time
		if err := testDB.NewRaw(c.query).Scan(ctx, &stored); err != nil {
			t.Fatalf("%s: read: %v", c.name, err)
		}
		if age := time.Since(stored); age < -time.Minute || age > time.Minute {
			t.Errorf("%s = %v, want the database's now()", c.name, stored)
		}
		if !c.returned.Equal(stored) {
			t.Errorf("%s: struct holds %v after insert, want the stored %v", c.name, c.returned, stored)
		}
	}
}
