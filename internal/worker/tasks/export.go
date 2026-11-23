package tasks

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
	"github.com/uptrace/bun"

	"github.com/drzero42/nexorious/internal/db/models"
	"github.com/drzero42/nexorious/internal/joblifecycle"
	"github.com/drzero42/nexorious/internal/logging"
)

// ── JSON export ───────────────────────────────────────────────────────────────

// An export has no items: its worker is its dispatch (internal/joblifecycle).
// It starts the dispatch, writes the file, then marks the dispatch done with
// the file's path, which completes the Job.

type ExportJSONArgs struct {
	JobID string `json:"job_id"`
}

func (ExportJSONArgs) Kind() string { return "export_json" }

// InsertOpts: re-running an export is safe, so a hard kill does not fail it.
func (ExportJSONArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{MaxAttempts: 5, Priority: 3}
}

type ExportJSONWorker struct {
	river.WorkerDefaults[ExportJSONArgs]
	DB          *bun.DB
	LC          *joblifecycle.Module
	StoragePath string
}

// Timeout leaves room for a large library; River's default is a minute.
func (w *ExportJSONWorker) Timeout(*river.Job[ExportJSONArgs]) time.Duration { return exportTimeout }

func (w *ExportJSONWorker) Work(ctx context.Context, job *river.Job[ExportJSONArgs]) error {
	return runExport(ctx, w.DB, w.LC, w.StoragePath, job.Args.JobID, job.JobRow, "json",
		func(ctx context.Context, userID string) (func(path string) error, error) {
			userGames, err := loadUserGamesWithRelations(ctx, w.DB, userID)
			if err != nil {
				return nil, fmt.Errorf("load user games: %w", err)
			}
			pools, err := loadPoolsForExport(ctx, w.DB, userID)
			if err != nil {
				return nil, fmt.Errorf("load pools: %w", err)
			}
			return func(path string) error { return writeJSONExport(path, userGames, pools) }, nil
		})
}

// ── CSV export ────────────────────────────────────────────────────────────────

type ExportCSVArgs struct {
	JobID string `json:"job_id"`
}

func (ExportCSVArgs) Kind() string { return "export_csv" }

func (ExportCSVArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{MaxAttempts: 5, Priority: 3}
}

type ExportCSVWorker struct {
	river.WorkerDefaults[ExportCSVArgs]
	DB          *bun.DB
	LC          *joblifecycle.Module
	StoragePath string
}

func (w *ExportCSVWorker) Timeout(*river.Job[ExportCSVArgs]) time.Duration { return exportTimeout }

func (w *ExportCSVWorker) Work(ctx context.Context, job *river.Job[ExportCSVArgs]) error {
	return runExport(ctx, w.DB, w.LC, w.StoragePath, job.Args.JobID, job.JobRow, "csv",
		func(ctx context.Context, userID string) (func(path string) error, error) {
			userGames, err := loadUserGamesWithRelations(ctx, w.DB, userID)
			if err != nil {
				return nil, fmt.Errorf("load user games: %w", err)
			}
			return func(path string) error { return writeCSVExport(path, userGames) }, nil
		})
}

// ── Shared helpers ───────────────────────────────────────────────────────────

const exportTimeout = 30 * time.Minute

// runExport runs one export. load reads the library (a database error is
// returned to River, so it is retried) and returns the function that writes
// the file. The file is named by Job and River attempt, written before the
// transition, and removed only when this run knows the Job will not refer to
// it: it was interrupted before the transition, the module refused the
// transition, or this run failed the Job. After an untyped error from the
// transition the commit may have happened, so the file stays; the daily sweep
// removes files no Job refers to.
func runExport(ctx context.Context, db *bun.DB, lc *joblifecycle.Module, storagePath, jobID string, rj *rivertype.JobRow, ext string,
	load func(ctx context.Context, userID string) (func(path string) error, error),
) error {
	ctx = logging.WithJobID(ctx, jobID)
	j, err := lc.StartDispatch(ctx, jobID, rj.ID)
	if err != nil {
		return exitOrRetry(ctx, err)
	}
	fail := func(msg string) error {
		return exitOrRetry(ctx, lc.Fail(ctx, jobID, joblifecycle.Failure{Message: msg, CallerRiverJobID: rj.ID}))
	}
	write, err := load(ctx, j.UserID)
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if lastAttempt(rj) {
			return fail(err.Error())
		}
		return err
	}
	dir, err := exportsDir(storagePath)
	if err != nil {
		return fail(err.Error())
	}
	path := filepath.Join(dir, exportFileName(jobID, rj.Attempt, ext))
	remove := func() {
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			slog.WarnContext(ctx, "export: remove file", logging.KeyErr, err)
		}
	}
	if err := write(path); err != nil {
		remove()
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return fail(fmt.Sprintf("write %s: %v", strings.ToUpper(ext), err))
	}
	if ctx.Err() != nil {
		remove()
		return ctx.Err()
	}
	err = lc.MarkDispatched(ctx, jobID, rj.ID, joblifecycle.Dispatched{FilePath: &path})
	switch {
	case err == nil:
		return nil
	case ctx.Err() != nil:
		return ctx.Err()
	case joblifecycle.IsRefusal(err):
		remove()
		return nil
	default:
		return err
	}
}

// exportFileName names an export's file by its Job and River attempt, so two
// exports never share a path. A run repeated after a clean shutdown keeps its
// attempt number and overwrites its own earlier file.
func exportFileName(jobID string, attempt int, ext string) string {
	return fmt.Sprintf("%s_%d.%s", jobID, attempt, ext)
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

func writeJSONExport(outPath string, ugs []models.UserGame, pools []exportPoolJSON) (err error) {
	doc := buildJSONDoc(ugs, pools)

	f, err := os.Create(outPath) //nolint:gosec // outPath is an internally-derived export path under storagePath, not user input
	if err != nil {
		return fmt.Errorf("create export file: %w", err)
	}
	defer func() {
		if cerr := f.Close(); cerr != nil && err == nil {
			err = cerr
		}
	}()

	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	if err := enc.Encode(doc); err != nil {
		return fmt.Errorf("encode JSON: %w", err)
	}
	return nil
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

func writeCSVExport(outPath string, ugs []models.UserGame) (err error) {
	f, err := os.Create(outPath) //nolint:gosec // outPath is an internally-derived export path under storagePath, not user input
	if err != nil {
		return fmt.Errorf("create CSV file: %w", err)
	}
	defer func() {
		if cerr := f.Close(); cerr != nil && err == nil {
			err = cerr
		}
	}()

	w := csv.NewWriter(f)
	if err := w.Write(csvHeaders); err != nil {
		return fmt.Errorf("write CSV header: %w", err)
	}

	for _, ug := range ugs {
		row := buildCSVRow(ug)
		if err := w.Write(row); err != nil {
			return fmt.Errorf("write CSV row: %w", err)
		}
	}
	w.Flush()
	if err := w.Error(); err != nil {
		return fmt.Errorf("flush CSV: %w", err)
	}
	return nil
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
