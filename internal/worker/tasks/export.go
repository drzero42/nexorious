package tasks

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/riverqueue/river"
	"github.com/uptrace/bun"

	"github.com/drzero42/nexorious/internal/db/models"
	"github.com/drzero42/nexorious/internal/logging"
	"github.com/drzero42/nexorious/internal/notify"
)

// ── JSON export ───────────────────────────────────────────────────────────────

type ExportJSONArgs struct {
	JobID string `json:"job_id"`
}

func (ExportJSONArgs) Kind() string { return "export_json" }

func (ExportJSONArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{MaxAttempts: 1, Priority: 3}
}

type ExportJSONWorker struct {
	river.WorkerDefaults[ExportJSONArgs]
	DB          *bun.DB
	StoragePath string
}

func (w *ExportJSONWorker) Work(ctx context.Context, job *river.Job[ExportJSONArgs]) error {
	j, err := loadAndStartJob(ctx, w.DB, job.Args.JobID)
	if err != nil {
		slog.ErrorContext(ctx, "export_json: load job", logging.KeyJobID, job.Args.JobID, logging.KeyErr, err, logging.Cat(logging.CategoryDB))
		return nil
	}
	userGames, err := loadUserGamesWithRelations(ctx, w.DB, j.UserID)
	if err != nil {
		markJobFailed(ctx, w.DB, j, fmt.Sprintf("load user games: %v", err))
		return nil
	}
	pools, err := loadPoolsForExport(ctx, w.DB, j.UserID)
	if err != nil {
		markJobFailed(ctx, w.DB, j, fmt.Sprintf("load pools: %v", err))
		return nil
	}
	outPath, err := writeJSONExport(w.StoragePath, j.UserID, userGames, pools)
	if err != nil {
		markJobFailed(ctx, w.DB, j, fmt.Sprintf("write JSON: %v", err))
		return nil
	}
	markJobCompleted(ctx, w.DB, j, outPath)
	return nil
}

// ── CSV export ────────────────────────────────────────────────────────────────

type ExportCSVArgs struct {
	JobID string `json:"job_id"`
}

func (ExportCSVArgs) Kind() string { return "export_csv" }

func (ExportCSVArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{MaxAttempts: 1, Priority: 3}
}

type ExportCSVWorker struct {
	river.WorkerDefaults[ExportCSVArgs]
	DB          *bun.DB
	StoragePath string
}

func (w *ExportCSVWorker) Work(ctx context.Context, job *river.Job[ExportCSVArgs]) error {
	j, err := loadAndStartJob(ctx, w.DB, job.Args.JobID)
	if err != nil {
		slog.ErrorContext(ctx, "export_csv: load job", logging.KeyJobID, job.Args.JobID, logging.KeyErr, err, logging.Cat(logging.CategoryDB))
		return nil
	}
	userGames, err := loadUserGamesWithRelations(ctx, w.DB, j.UserID)
	if err != nil {
		markJobFailed(ctx, w.DB, j, fmt.Sprintf("load user games: %v", err))
		return nil
	}
	outPath, err := writeCSVExport(w.StoragePath, j.UserID, userGames)
	if err != nil {
		markJobFailed(ctx, w.DB, j, fmt.Sprintf("write CSV: %v", err))
		return nil
	}
	markJobCompleted(ctx, w.DB, j, outPath)
	return nil
}

// ── Shared helpers ───────────────────────────────────────────────────────────

// loadAndStartJob loads the Job row and marks it processing with started_at set.
func loadAndStartJob(ctx context.Context, db *bun.DB, jobID string) (*models.Job, error) {
	var job models.Job
	if err := db.NewSelect().Model(&job).Where("id = ?", jobID).Scan(ctx); err != nil {
		return nil, fmt.Errorf("select job: %w", err)
	}

	now := time.Now().UTC()
	job.Status = models.JobStatusProcessing
	job.StartedAt = &now
	if _, err := db.NewUpdate().Model(&job).
		Column("status", "started_at").
		Where("id = ?", job.ID).
		Exec(ctx); err != nil {
		return nil, fmt.Errorf("update job to processing: %w", err)
	}
	return &job, nil
}

// loadUserGamesWithRelations queries all UserGames for a user with their
// Game, Platforms, Tags, and Tags.Tag relations loaded.
func loadUserGamesWithRelations(ctx context.Context, db *bun.DB, userID string) ([]models.UserGame, error) {
	var ugs []models.UserGame
	if err := db.NewSelect().
		Model(&ugs).
		Relation("Game").
		Relation("Platforms").
		Relation("Tags").
		Relation("Tags.Tag").
		Where("user_game.user_id = ?", userID).
		Scan(ctx); err != nil {
		return nil, err
	}
	return ugs, nil
}

// markJobFailed sets the job status to failed with an error message. errMsg is
// scrubbed of URL query strings before persisting (#937).
func markJobFailed(ctx context.Context, db *bun.DB, job *models.Job, errMsg string) {
	errMsg = logging.ScrubURLQueries(errMsg)
	now := time.Now().UTC()
	job.Status = models.JobStatusFailed
	job.ErrorMessage = &errMsg
	job.CompletedAt = &now
	if _, err := db.NewUpdate().Model(job).
		Column("status", "error_message", "completed_at").
		Where("id = ?", job.ID).
		Exec(ctx); err != nil {
		slog.ErrorContext(ctx, "export: markJobFailed", logging.KeyJobID, job.ID, logging.KeyErr, err, logging.Cat(logging.CategoryDB))
	}
	notify.Emit(ctx, db, notify.EmitParams{
		Type: notify.TypeExportFailed, Scope: notify.ScopeUser, ActorUserID: job.UserID,
		Payload:  notify.ExportFailedPayload{JobID: job.ID, Error: errMsg},
		DedupKey: job.ID + ":" + notify.TypeExportFailed,
	})
}

// markJobCompleted sets the job status to completed, recording the output file path.
func markJobCompleted(ctx context.Context, db *bun.DB, job *models.Job, filePath string) {
	now := time.Now().UTC()
	job.Status = models.JobStatusCompleted
	job.FilePath = &filePath
	job.CompletedAt = &now
	if _, err := db.NewUpdate().Model(job).
		Column("status", "file_path", "completed_at").
		Where("id = ?", job.ID).
		Exec(ctx); err != nil {
		slog.ErrorContext(ctx, "export: markJobCompleted", logging.KeyJobID, job.ID, logging.KeyErr, err, logging.Cat(logging.CategoryDB))
	}
	notify.Emit(ctx, db, notify.EmitParams{
		Type: notify.TypeExportCompleted, Scope: notify.ScopeUser, ActorUserID: job.UserID,
		Payload:  notify.ExportCompletedPayload{JobID: job.ID, FilePath: filePath},
		DedupKey: job.ID + ":" + notify.TypeExportCompleted,
	})
}

// exportsDir returns (and creates) the exports subdirectory under storagePath.
func exportsDir(storagePath string) (string, error) {
	dir := filepath.Join(storagePath, "exports")
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return "", fmt.Errorf("create exports dir: %w", err)
	}
	return dir, nil
}

// ── JSON format helpers ───────────────────────────────────────────────────────

type exportGameJSON struct {
	IGDBID         int32                `json:"igdb_id"`
	Title          string               `json:"title"`
	PlayStatus     *string              `json:"play_status"`
	PersonalRating *int32               `json:"personal_rating"`
	IsLoved        bool                 `json:"is_loved"`
	IsWishlisted   bool                 `json:"is_wishlisted"`
	PersonalNotes  *string              `json:"personal_notes"`
	CreatedAt      string               `json:"created_at"`
	UpdatedAt      string               `json:"updated_at"`
	Platforms      []exportPlatformJSON `json:"platforms"`
	Tags           []exportTagJSON      `json:"tags"`
}

type exportPlatformJSON struct {
	Platform        *string  `json:"platform"`
	Storefront      *string  `json:"storefront"`
	OwnershipStatus *string  `json:"ownership_status"`
	AcquiredDate    *string  `json:"acquired_date"`
	HoursPlayed     *float64 `json:"hours_played"`
	IsAvailable     bool     `json:"is_available"`
}

type exportTagJSON struct {
	Name  string  `json:"name"`
	Color *string `json:"color"`
}

type exportPoolJSON struct {
	Name     string               `json:"name"`
	Color    *string              `json:"color"`
	Position int                  `json:"position"`
	Filter   json.RawMessage      `json:"filter,omitempty"`
	Games    []exportPoolGameJSON `json:"games"`
}

type exportPoolGameJSON struct {
	IGDBID   int32 `json:"igdb_id"`
	Position *int  `json:"position"`
}

type exportDocJSON struct {
	Format     string           `json:"format"`
	Version    string           `json:"version"`
	ExportedAt string           `json:"exported_at"`
	Games      []exportGameJSON `json:"games"`
	Pools      []exportPoolJSON `json:"pools"`
}

// loadPoolsForExport returns the user's Play Planning pools with each membership
// translated from the opaque user_game_id to the game's igdb_id (the stable
// cross-instance key). Queued members (position set) sort before Candidates.
func loadPoolsForExport(ctx context.Context, db *bun.DB, userID string) ([]exportPoolJSON, error) {
	var pools []struct {
		ID       string          `bun:"id"`
		Name     string          `bun:"name"`
		Color    *string         `bun:"color"`
		Position int             `bun:"position"`
		Filter   json.RawMessage `bun:"filter"`
	}
	if err := db.NewRaw(
		`SELECT id, name, color, position, filter FROM pools WHERE user_id = ? ORDER BY position`, userID,
	).Scan(ctx, &pools); err != nil {
		return nil, err
	}
	out := make([]exportPoolJSON, 0, len(pools))
	for _, p := range pools {
		var members []struct {
			Position *int  `bun:"position"`
			GameID   int32 `bun:"game_id"`
		}
		if err := db.NewRaw(
			`SELECT pg.position AS position, ug.game_id AS game_id
			 FROM pool_games pg JOIN user_games ug ON ug.id = pg.user_game_id
			 WHERE pg.pool_id = ?
			 ORDER BY pg.position NULLS LAST, ug.game_id`, p.ID,
		).Scan(ctx, &members); err != nil {
			return nil, err
		}
		games := make([]exportPoolGameJSON, 0, len(members))
		for _, m := range members {
			games = append(games, exportPoolGameJSON{IGDBID: m.GameID, Position: m.Position})
		}
		var filter json.RawMessage
		if len(p.Filter) > 0 {
			filter = p.Filter
		}
		out = append(out, exportPoolJSON{
			Name: p.Name, Color: p.Color, Position: p.Position, Filter: filter, Games: games,
		})
	}
	return out, nil
}

func writeJSONExport(storagePath, userID string, ugs []models.UserGame, pools []exportPoolJSON) (string, error) {
	dir, err := exportsDir(storagePath)
	if err != nil {
		return "", err
	}

	ts := time.Now().UTC().Format("20060102_150405")
	filename := fmt.Sprintf("%s_%s.json", userID, ts)
	outPath := filepath.Join(dir, filename)

	doc := buildJSONDoc(ugs, pools)

	f, err := os.Create(outPath) //nolint:gosec // outPath is an internally-derived export path under storagePath, not user input
	if err != nil {
		return "", fmt.Errorf("create export file: %w", err)
	}
	defer func() {
		if cerr := f.Close(); cerr != nil && err == nil {
			err = cerr
		}
	}()

	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	if err := enc.Encode(doc); err != nil {
		return "", fmt.Errorf("encode JSON: %w", err)
	}
	return outPath, nil
}

func buildJSONDoc(ugs []models.UserGame, pools []exportPoolJSON) exportDocJSON {
	games := make([]exportGameJSON, 0, len(ugs))
	for _, ug := range ugs {
		platforms := make([]exportPlatformJSON, 0, len(ug.Platforms))
		for _, p := range ug.Platforms {
			pj := exportPlatformJSON{
				Platform:        p.Platform,
				Storefront:      p.Storefront,
				OwnershipStatus: p.OwnershipStatus,
				HoursPlayed:     p.HoursPlayed,
				IsAvailable:     p.IsAvailable,
			}
			if p.AcquiredDate != nil {
				d := p.AcquiredDate.Format("2006-01-02")
				pj.AcquiredDate = &d
			}
			platforms = append(platforms, pj)
		}

		tags := make([]exportTagJSON, 0, len(ug.Tags))
		for _, ugt := range ug.Tags {
			if ugt.Tag == nil {
				continue
			}
			tags = append(tags, exportTagJSON{Name: ugt.Tag.Name, Color: ugt.Tag.Color})
		}

		var title string
		if ug.Game != nil {
			title = ug.Game.Title
		}

		games = append(games, exportGameJSON{
			IGDBID:         ug.GameID,
			Title:          title,
			PlayStatus:     ug.PlayStatus,
			PersonalRating: ug.PersonalRating,
			IsLoved:        ug.IsLoved,
			IsWishlisted:   ug.IsWishlisted,
			PersonalNotes:  ug.PersonalNotes,
			CreatedAt:      ug.CreatedAt.UTC().Format(time.RFC3339),
			UpdatedAt:      ug.UpdatedAt.UTC().Format(time.RFC3339),
			Platforms:      platforms,
			Tags:           tags,
		})
	}

	return exportDocJSON{
		Format:     "nexorious-library",
		Version:    "2.1",
		ExportedAt: time.Now().UTC().Format(time.RFC3339),
		Games:      games,
		Pools:      pools,
	}
}

// ── CSV format helpers ────────────────────────────────────────────────────────

var csvHeaders = []string{
	"title", "igdb_id", "play_status", "personal_rating", "is_loved",
	"hours_played", "personal_notes", "platforms", "tags",
	"created_at", "updated_at",
}

func writeCSVExport(storagePath, userID string, ugs []models.UserGame) (string, error) {
	dir, err := exportsDir(storagePath)
	if err != nil {
		return "", err
	}

	ts := time.Now().UTC().Format("20060102_150405")
	filename := fmt.Sprintf("%s_%s.csv", userID, ts)
	outPath := filepath.Join(dir, filename)

	f, err := os.Create(outPath) //nolint:gosec // outPath is an internally-derived export path under storagePath, not user input
	if err != nil {
		return "", fmt.Errorf("create CSV file: %w", err)
	}
	defer func() {
		if cerr := f.Close(); cerr != nil && err == nil {
			err = cerr
		}
	}()

	w := csv.NewWriter(f)
	if err := w.Write(csvHeaders); err != nil {
		return "", fmt.Errorf("write CSV header: %w", err)
	}

	for _, ug := range ugs {
		row := buildCSVRow(ug)
		if err := w.Write(row); err != nil {
			return "", fmt.Errorf("write CSV row: %w", err)
		}
	}
	w.Flush()
	if err := w.Error(); err != nil {
		return "", fmt.Errorf("flush CSV: %w", err)
	}
	return outPath, nil
}

func buildCSVRow(ug models.UserGame) []string {
	title := ""
	if ug.Game != nil {
		title = ug.Game.Title
	}

	playStatus := ""
	if ug.PlayStatus != nil {
		playStatus = *ug.PlayStatus
	}

	rating := ""
	if ug.PersonalRating != nil {
		rating = strconv.Itoa(int(*ug.PersonalRating))
	}

	var totalHoursF float64
	for _, p := range ug.Platforms {
		if p.HoursPlayed != nil {
			totalHoursF += *p.HoursPlayed
		}
	}
	hours := ""
	if totalHoursF > 0 {
		hours = strconv.FormatFloat(totalHoursF, 'f', -1, 64)
	}

	notes := ""
	if ug.PersonalNotes != nil {
		notes = *ug.PersonalNotes
	}

	// Semicolon-joined platform slugs.
	platformSlugs := make([]string, 0, len(ug.Platforms))
	for _, p := range ug.Platforms {
		if p.Platform != nil {
			platformSlugs = append(platformSlugs, *p.Platform)
		}
	}

	// Semicolon-joined tag names.
	tagNames := make([]string, 0, len(ug.Tags))
	for _, ugt := range ug.Tags {
		if ugt.Tag != nil {
			tagNames = append(tagNames, ugt.Tag.Name)
		}
	}

	return []string{
		title,
		strconv.Itoa(int(ug.GameID)),
		playStatus,
		rating,
		strconv.FormatBool(ug.IsLoved),
		hours,
		notes,
		strings.Join(platformSlugs, ";"),
		strings.Join(tagNames, ";"),
		ug.CreatedAt.UTC().Format(time.RFC3339),
		ug.UpdatedAt.UTC().Format(time.RFC3339),
	}
}
