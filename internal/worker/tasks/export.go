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
		slog.Error("export_json: load job", "job_id", job.Args.JobID, "err", err)
		return nil
	}
	userGames, err := loadUserGamesWithRelations(ctx, w.DB, j.UserID)
	if err != nil {
		markJobFailed(ctx, w.DB, j, fmt.Sprintf("load user games: %v", err))
		return nil
	}
	outPath, err := writeJSONExport(w.StoragePath, j.UserID, userGames)
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
		slog.Error("export_csv: load job", "job_id", job.Args.JobID, "err", err)
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

// markJobFailed sets the job status to failed with an error message.
func markJobFailed(ctx context.Context, db *bun.DB, job *models.Job, errMsg string) {
	now := time.Now().UTC()
	job.Status = models.JobStatusFailed
	job.ErrorMessage = &errMsg
	job.CompletedAt = &now
	if _, err := db.NewUpdate().Model(job).
		Column("status", "error_message", "completed_at").
		Where("id = ?", job.ID).
		Exec(ctx); err != nil {
		slog.Error("export: markJobFailed", "job_id", job.ID, "err", err)
	}
	notify.Emit(ctx, db, notify.EmitParams{
		Type: notify.TypeExportFailed, Scope: notify.ScopeUser, ActorUserID: job.UserID,
		Payload:  map[string]any{"job_id": job.ID, "error": errMsg},
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
		slog.Error("export: markJobCompleted", "job_id", job.ID, "err", err)
	}
	notify.Emit(ctx, db, notify.EmitParams{
		Type: notify.TypeExportCompleted, Scope: notify.ScopeUser, ActorUserID: job.UserID,
		Payload:  map[string]any{"job_id": job.ID, "file_path": filePath},
		DedupKey: job.ID + ":" + notify.TypeExportCompleted,
	})
}

// exportsDir returns (and creates) the exports subdirectory under storagePath.
func exportsDir(storagePath string) (string, error) {
	dir := filepath.Join(storagePath, "exports")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("create exports dir: %w", err)
	}
	return dir, nil
}

// ── JSON format helpers ───────────────────────────────────────────────────────

type exportGameJSON struct {
	IGDBID         int32                `json:"igdb_id"`
	Title          string               `json:"title"`
	ReleaseYear    *int                 `json:"release_year"`
	PlayStatus     *string              `json:"play_status"`
	PersonalRating *int32               `json:"personal_rating"`
	IsLoved        bool                 `json:"is_loved"`
	HoursPlayed    *float64             `json:"hours_played"`
	PersonalNotes  *string              `json:"personal_notes"`
	Platforms      []exportPlatformJSON `json:"platforms"`
	Tags           []exportTagJSON      `json:"tags"`
	CreatedAt      string               `json:"created_at"`
	UpdatedAt      string               `json:"updated_at"`
}

type exportPlatformJSON struct {
	PlatformID      *string  `json:"platform_id"`
	PlatformName    *string  `json:"platform_name"`
	StorefrontID    *string  `json:"storefront_id"`
	StorefrontName  *string  `json:"storefront_name"`
	StoreGameID     *string  `json:"store_game_id"`
	StoreURL        *string  `json:"store_url"`
	IsAvailable     bool     `json:"is_available"`
	HoursPlayed     *float64 `json:"hours_played"`
	OwnershipStatus *string  `json:"ownership_status"`
	AcquiredDate    *string  `json:"acquired_date"`
}

type exportTagJSON struct {
	Name  string  `json:"name"`
	Color *string `json:"color"`
}

type exportStatsJSON struct {
	ByStatus   map[string]int `json:"by_status"`
	ByPlatform map[string]int `json:"by_platform"`
	TotalHours float64        `json:"total_hours"`
	RatedCount int            `json:"rated_count"`
	LovedCount int            `json:"loved_count"`
}

type exportDocJSON struct {
	ExportVersion string           `json:"export_version"`
	ExportDate    string           `json:"export_date"`
	UserID        string           `json:"user_id"`
	TotalGames    int              `json:"total_games"`
	TotalWishlist int              `json:"total_wishlist"`
	ExportStats   exportStatsJSON  `json:"export_stats"`
	Games         []exportGameJSON `json:"games"`
	Wishlist      []any            `json:"wishlist"`
}

func writeJSONExport(storagePath, userID string, ugs []models.UserGame) (string, error) {
	dir, err := exportsDir(storagePath)
	if err != nil {
		return "", err
	}

	ts := time.Now().UTC().Format("20060102_150405")
	filename := fmt.Sprintf("%s_%s.json", userID, ts)
	outPath := filepath.Join(dir, filename)

	doc := buildJSONDoc(userID, ugs)

	f, err := os.Create(outPath)
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

func buildJSONDoc(userID string, ugs []models.UserGame) exportDocJSON {
	stats := exportStatsJSON{
		ByStatus:   make(map[string]int),
		ByPlatform: make(map[string]int),
	}

	games := make([]exportGameJSON, 0, len(ugs))
	for _, ug := range ugs {
		// Stats.
		if ug.PlayStatus != nil {
			stats.ByStatus[*ug.PlayStatus]++
		}
		if ug.PersonalRating != nil {
			stats.RatedCount++
		}
		if ug.IsLoved {
			stats.LovedCount++
		}

		// Release year.
		var releaseYear *int
		if ug.Game != nil && ug.Game.ReleaseDate != nil {
			y := ug.Game.ReleaseDate.Year()
			releaseYear = &y
		}

		// Platforms — accumulate per-game total hours from platform rows.
		var ugTotalHours float64
		platforms := make([]exportPlatformJSON, 0, len(ug.Platforms))
		for _, p := range ug.Platforms {
			if p.HoursPlayed != nil {
				ugTotalHours += *p.HoursPlayed
			}
			pj := exportPlatformJSON{
				PlatformID:      p.Platform,
				StorefrontID:    p.Storefront,
				StoreGameID:     p.StoreGameID,
				StoreURL:        p.StoreUrl,
				IsAvailable:     p.IsAvailable,
				HoursPlayed:     p.HoursPlayed,
				OwnershipStatus: p.OwnershipStatus,
			}
			// Display names: prefer original names, fall back to slug.
			if p.OriginalPlatformName != nil {
				pj.PlatformName = p.OriginalPlatformName
			} else {
				pj.PlatformName = p.Platform
			}
			if p.OriginalStorefrontName != nil {
				pj.StorefrontName = p.OriginalStorefrontName
			} else {
				pj.StorefrontName = p.Storefront
			}
			if p.AcquiredDate != nil {
				d := p.AcquiredDate.Format("2006-01-02")
				pj.AcquiredDate = &d
			}
			if p.Platform != nil {
				stats.ByPlatform[*p.Platform]++
			}
			platforms = append(platforms, pj)
		}
		stats.TotalHours += ugTotalHours
		var ugHoursPtr *float64
		if ugTotalHours > 0 {
			ugHoursPtr = &ugTotalHours
		}

		// Tags.
		tags := make([]exportTagJSON, 0, len(ug.Tags))
		for _, ugt := range ug.Tags {
			if ugt.Tag == nil {
				continue
			}
			tags = append(tags, exportTagJSON{
				Name:  ugt.Tag.Name,
				Color: ugt.Tag.Color,
			})
		}

		var title string
		if ug.Game != nil {
			title = ug.Game.Title
		}

		games = append(games, exportGameJSON{
			IGDBID:         ug.GameID,
			Title:          title,
			ReleaseYear:    releaseYear,
			PlayStatus:     ug.PlayStatus,
			PersonalRating: ug.PersonalRating,
			IsLoved:        ug.IsLoved,
			HoursPlayed:    ugHoursPtr,
			PersonalNotes:  ug.PersonalNotes,
			Platforms:      platforms,
			Tags:           tags,
			CreatedAt:      ug.CreatedAt.UTC().Format(time.RFC3339),
			UpdatedAt:      ug.UpdatedAt.UTC().Format(time.RFC3339),
		})
	}

	return exportDocJSON{
		ExportVersion: "1.2",
		ExportDate:    time.Now().UTC().Format(time.RFC3339),
		UserID:        userID,
		TotalGames:    len(games),
		TotalWishlist: 0,
		ExportStats:   stats,
		Games:         games,
		Wishlist:      []any{},
	}
}

// ── CSV format helpers ────────────────────────────────────────────────────────

var csvHeaders = []string{
	"title", "igdb_id", "play_status", "personal_rating", "is_loved",
	"hours_played", "personal_notes", "platforms", "tags", "release_year",
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

	f, err := os.Create(outPath)
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

	releaseYear := ""
	if ug.Game != nil && ug.Game.ReleaseDate != nil {
		releaseYear = strconv.Itoa(ug.Game.ReleaseDate.Year())
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
		releaseYear,
		ug.CreatedAt.UTC().Format(time.RFC3339),
		ug.UpdatedAt.UTC().Format(time.RFC3339),
	}
}
