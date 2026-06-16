package tasks_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/riverqueue/river"

	"github.com/drzero42/nexorious/internal/config"
	"github.com/drzero42/nexorious/internal/db/models"
	"github.com/drzero42/nexorious/internal/ratelimit"
	"github.com/drzero42/nexorious/internal/services/igdb"
	"github.com/drzero42/nexorious/internal/worker/tasks"
)

func TestImport_RoundTripPreservesUserData(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()

	// Seed platform + storefront referenced by the source game.
	if _, err := testDB.ExecContext(ctx,
		`INSERT INTO platforms (name, display_name) VALUES ('pc-windows', 'PC (Windows)') ON CONFLICT (name) DO NOTHING`); err != nil {
		t.Fatalf("seed platform: %v", err)
	}
	if _, err := testDB.ExecContext(ctx,
		`INSERT INTO storefronts (name, display_name) VALUES ('steam', 'Steam') ON CONFLICT (name) DO NOTHING`); err != nil {
		t.Fatalf("seed storefront: %v", err)
	}

	// Source user with one fully-populated game.
	srcUser := uuid.NewString()
	insertTestUser(t, testDB, srcUser)

	game := &models.Game{ID: 7777, Title: "Round Trip", LastUpdated: time.Now(), CreatedAt: time.Now()}
	if _, err := testDB.NewInsert().Model(game).Exec(ctx); err != nil {
		t.Fatalf("insert game: %v", err)
	}

	status := "completed"
	rating := int32(4)
	notes := "loved it"
	ug := &models.UserGame{
		ID: uuid.NewString(), UserID: srcUser, GameID: 7777,
		PlayStatus: &status, PersonalRating: &rating, IsLoved: true, PersonalNotes: &notes,
		CreatedAt: time.Now().UTC().Truncate(time.Second), UpdatedAt: time.Now().UTC().Truncate(time.Second),
	}
	if _, err := testDB.NewInsert().Model(ug).Exec(ctx); err != nil {
		t.Fatalf("insert user_game: %v", err)
	}

	plat := "pc-windows"
	store := "steam"
	own := "owned"
	hours := 12.5
	acq := time.Date(2024, 12, 25, 0, 0, 0, 0, time.UTC)
	ugp := &models.UserGamePlatform{
		ID: uuid.NewString(), UserGameID: ug.ID, Platform: &plat, Storefront: &store,
		OwnershipStatus: &own, HoursPlayed: &hours, AcquiredDate: &acq, IsAvailable: false,
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	if _, err := testDB.NewInsert().Model(ugp).Exec(ctx); err != nil {
		t.Fatalf("insert ugp: %v", err)
	}

	color := "#7C3AED"
	tag := &models.Tag{ID: uuid.NewString(), UserID: srcUser, Name: "metroidvania", Color: &color, CreatedAt: time.Now(), UpdatedAt: time.Now()}
	if _, err := testDB.NewInsert().Model(tag).Exec(ctx); err != nil {
		t.Fatalf("insert tag: %v", err)
	}
	if _, err := testDB.NewInsert().Model(&models.UserGameTag{ID: uuid.NewString(), UserGameID: ug.ID, TagID: tag.ID, CreatedAt: time.Now()}).Exec(ctx); err != nil {
		t.Fatalf("insert user_game_tag: %v", err)
	}

	// Second game, plus a platform-less wishlisted game.
	game2 := &models.Game{ID: 7778, Title: "Queued", LastUpdated: time.Now(), CreatedAt: time.Now()}
	if _, err := testDB.NewInsert().Model(game2).Exec(ctx); err != nil {
		t.Fatalf("insert game2: %v", err)
	}
	ug2 := &models.UserGame{ID: uuid.NewString(), UserID: srcUser, GameID: 7778, CreatedAt: time.Now(), UpdatedAt: time.Now()}
	if _, err := testDB.NewInsert().Model(ug2).Exec(ctx); err != nil {
		t.Fatalf("insert ug2: %v", err)
	}
	gameW := &models.Game{ID: 7779, Title: "Wished", LastUpdated: time.Now(), CreatedAt: time.Now()}
	if _, err := testDB.NewInsert().Model(gameW).Exec(ctx); err != nil {
		t.Fatalf("insert gameW: %v", err)
	}
	ugW := &models.UserGame{ID: uuid.NewString(), UserID: srcUser, GameID: 7779, IsWishlisted: true, CreatedAt: time.Now(), UpdatedAt: time.Now()}
	if _, err := testDB.NewInsert().Model(ugW).Exec(ctx); err != nil {
		t.Fatalf("insert ugW: %v", err)
	}
	// Pool: ug (7777) Candidate, ug2 (7778) queued at 0.
	poolID := uuid.NewString()
	if _, err := testDB.ExecContext(ctx,
		`INSERT INTO pools (id, user_id, name, color, position, filter) VALUES (?, ?, 'Backlog', '#abc', 0, '{"loved":true}')`,
		poolID, srcUser); err != nil {
		t.Fatalf("insert pool: %v", err)
	}
	if _, err := testDB.ExecContext(ctx,
		`INSERT INTO pool_games (id, pool_id, user_game_id, position) VALUES (?, ?, ?, NULL)`, uuid.NewString(), poolID, ug.ID); err != nil {
		t.Fatalf("insert pg candidate: %v", err)
	}
	if _, err := testDB.ExecContext(ctx,
		`INSERT INTO pool_games (id, pool_id, user_game_id, position) VALUES (?, ?, ?, 0)`, uuid.NewString(), poolID, ug2.ID); err != nil {
		t.Fatalf("insert pg queued: %v", err)
	}

	// Export.
	ugs, err := tasks.LoadUserGamesWithRelationsForTest(ctx, testDB, srcUser)
	if err != nil {
		t.Fatalf("load source games: %v", err)
	}
	pools, err := tasks.LoadPoolsForExportForTest(ctx, testDB, srcUser)
	if err != nil {
		t.Fatalf("load pools: %v", err)
	}
	doc := tasks.BuildJSONDocForTest(ugs, pools)
	if len(doc.Games) != 3 {
		t.Fatalf("expected 3 exported games, got %d", len(doc.Games))
	}

	// Import into a fresh user.
	dstUser := uuid.NewString()
	insertTestUser(t, testDB, dstUser)
	jobID := uuid.NewString()
	insertTestJob(t, testDB, jobID, dstUser, len(doc.Games))

	// Stash pools BEFORE running items so the completion transition applies them.
	poolsPayload := make([]map[string]any, 0, len(doc.Pools))
	for _, p := range doc.Pools {
		members := make([]map[string]any, 0, len(p.Games))
		for _, m := range p.Games {
			members = append(members, map[string]any{"igdb_id": m.IGDBID, "position": m.Position})
		}
		poolsPayload = append(poolsPayload, map[string]any{
			"name": p.Name, "color": p.Color, "position": p.Position, "games": members,
		})
	}
	insertTestPoolsItem(t, testDB, jobID, dstUser, poolsPayload)

	// Insert all game items first (mirroring the handler's dispatch), then process
	// them — otherwise the job would finalize after the first item (when no other
	// game item is pending yet) and pools would apply before all games exist.
	itemIDs := make([]string, 0, len(doc.Games))
	for _, g := range doc.Games {
		raw, err := json.Marshal(g)
		if err != nil {
			t.Fatalf("marshal exported game: %v", err)
		}
		var asMap map[string]any
		if err := json.Unmarshal(raw, &asMap); err != nil {
			t.Fatalf("game to map: %v", err)
		}
		itemIDs = append(itemIDs, insertTestJobItem(t, testDB, jobID, dstUser, asMap))
	}

	w := &tasks.ImportItemWorker{DB: testDB, IGDBClient: igdb.NewClient(&config.Config{}, ratelimit.NewLocal(100, 100)), StoragePath: ""}
	for _, itemID := range itemIDs {
		if err := w.Work(ctx, &river.Job[tasks.ImportItemArgs]{Args: tasks.ImportItemArgs{JobItemID: itemID}}); err != nil {
			t.Fatalf("import work: %v", err)
		}
	}

	// Assert destination user-owned data.
	var got models.UserGame
	if err := testDB.NewSelect().Model(&got).Where("user_id = ? AND game_id = ?", dstUser, int32(7777)).Scan(ctx); err != nil {
		t.Fatalf("dst user_game not found: %v", err)
	}
	if got.PlayStatus == nil || *got.PlayStatus != "completed" {
		t.Errorf("play_status = %v, want completed", got.PlayStatus)
	}
	if got.PersonalRating == nil || *got.PersonalRating != 4 {
		t.Errorf("personal_rating = %v, want 4", got.PersonalRating)
	}
	if !got.IsLoved {
		t.Errorf("is_loved = false, want true")
	}
	if got.PersonalNotes == nil || *got.PersonalNotes != "loved it" {
		t.Errorf("personal_notes = %v, want 'loved it'", got.PersonalNotes)
	}

	var gotP models.UserGamePlatform
	if err := testDB.NewSelect().Model(&gotP).Where("user_game_id = ?", got.ID).Scan(ctx); err != nil {
		t.Fatalf("dst platform not found: %v", err)
	}
	if gotP.Platform == nil || *gotP.Platform != "pc-windows" {
		t.Errorf("platform = %v, want pc-windows", gotP.Platform)
	}
	if gotP.Storefront == nil || *gotP.Storefront != "steam" {
		t.Errorf("storefront = %v, want steam", gotP.Storefront)
	}
	if gotP.OwnershipStatus == nil || *gotP.OwnershipStatus != "owned" {
		t.Errorf("ownership = %v, want owned", gotP.OwnershipStatus)
	}
	if gotP.HoursPlayed == nil || *gotP.HoursPlayed != 12.5 {
		t.Errorf("hours = %v, want 12.5", gotP.HoursPlayed)
	}
	if gotP.AcquiredDate == nil || gotP.AcquiredDate.Format("2006-01-02") != "2024-12-25" {
		t.Errorf("acquired_date = %v, want 2024-12-25", gotP.AcquiredDate)
	}
	if gotP.IsAvailable {
		t.Errorf("is_available = true, want false (round-trip)")
	}

	// Platform-less wishlisted game keeps its flag.
	var gotW models.UserGame
	if err := testDB.NewSelect().Model(&gotW).Where("user_id = ? AND game_id = ?", dstUser, int32(7779)).Scan(ctx); err != nil {
		t.Fatalf("dst wishlist game not found: %v", err)
	}
	if !gotW.IsWishlisted {
		t.Errorf("is_wishlisted = false, want true")
	}

	// Pool restored with both members.
	var members int
	if err := testDB.NewRaw(
		`SELECT COUNT(*) FROM pool_games pg JOIN pools p ON p.id = pg.pool_id
		 WHERE p.user_id = ? AND p.name = 'Backlog'`, dstUser,
	).Scan(ctx, &members); err != nil {
		t.Fatalf("count pool members: %v", err)
	}
	if members != 2 {
		t.Errorf("pool members = %d, want 2", members)
	}

	var tagCount int
	if err := testDB.NewRaw(
		`SELECT COUNT(*) FROM user_game_tags ugt JOIN tags tg ON tg.id = ugt.tag_id
		 WHERE ugt.user_game_id = ? AND LOWER(tg.name) = 'metroidvania'`, got.ID).Scan(ctx, &tagCount); err != nil {
		t.Fatalf("count tags: %v", err)
	}
	if tagCount != 1 {
		t.Errorf("tag count = %d, want 1", tagCount)
	}
}
