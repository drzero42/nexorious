package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/labstack/echo/v5"

	"github.com/drzero42/nexorious/internal/auth"
	"github.com/drzero42/nexorious/internal/db/models"
	"github.com/drzero42/nexorious/internal/services/csvmap"
	"github.com/drzero42/nexorious/internal/services/importmodel"
)

// csvMapping is the flat, frontend-shaped request body the mapping dialog POSTs
// as the "mapping" form field. It is translated to a csvmap.Config (simple
// subset only) by buildCSVConfig.
type csvMapping struct {
	Columns struct {
		Title        string `json:"title"`
		IGDBID       string `json:"igdb_id"`
		Platform     string `json:"platform"`
		Storefront   string `json:"storefront"`
		Rating       string `json:"rating"`
		Notes        string `json:"notes"`
		AcquiredDate string `json:"acquired_date"`
		HoursPlayed  string `json:"hours_played"`
		Tags         string `json:"tags"`
		Loved        string `json:"loved"`
	} `json:"columns"`
	Status struct {
		Column   string            `json:"column"`
		ValueMap map[string]string `json:"value_map"`
	} `json:"status"`
	RatingScale  int  `json:"rating_scale"`
	MergeByTitle bool `json:"merge_by_title"`
}

// buildCSVConfig translates the dialog mapping into a csvmap.Config expressing
// only the simple subset. It errors on an empty title or an invalid rating
// scale; advanced engine slots are never populated.
func buildCSVConfig(m csvMapping) (csvmap.Config, error) {
	if strings.TrimSpace(m.Columns.Title) == "" {
		return csvmap.Config{}, fmt.Errorf("a title column is required")
	}

	cfg := csvmap.Config{
		Columns: csvmap.ColumnMap{
			Title:  m.Columns.Title,
			IGDBID: m.Columns.IGDBID,
			Tags:   m.Columns.Tags,
			Loved:  m.Columns.Loved,
		},
		Grouping: csvmap.GroupingConfig{MergeByTitle: m.MergeByTitle},
	}

	if m.Columns.Notes != "" {
		cfg.Notes.Column = m.Columns.Notes
	}

	if m.Status.Column != "" {
		vm := make(map[string]string, len(m.Status.ValueMap))
		for k, v := range m.Status.ValueMap {
			vm[strings.ToLower(strings.TrimSpace(k))] = v
		}
		cfg.Status.Column = &csvmap.StatusColumn{
			Column:   m.Status.Column,
			ValueMap: vm,
			Default:  "not_started",
		}
	}

	if m.Columns.Platform != "" {
		cfg.Platform.Simple = &csvmap.PlatformSimple{
			PlatformColumn:     m.Columns.Platform,
			StorefrontColumn:   m.Columns.Storefront,
			AcquiredDateColumn: m.Columns.AcquiredDate,
		}
	}

	if m.Columns.Rating != "" {
		if m.RatingScale != 5 && m.RatingScale != 10 && m.RatingScale != 100 {
			return csvmap.Config{}, fmt.Errorf("rating scale must be 5, 10, or 100")
		}
		cfg.Columns.Rating = m.Columns.Rating
		cfg.Rating = &csvmap.RatingConfig{Scale: m.RatingScale, Truncate: false}
	}

	if m.Columns.HoursPlayed != "" {
		cfg.Columns.HoursPlayed = m.Columns.HoursPlayed
		cfg.Duration = &csvmap.DurationConfig{Format: "decimal"}
	}

	return cfg, nil
}

const csvDistinctCap = 50

// csvColumnInfo is one column's name plus a capped set of its distinct non-empty
// values, used to drive the mapping dialog's status-value rows.
type csvColumnInfo struct {
	Name              string   `json:"name"`
	DistinctValues    []string `json:"distinct_values"`
	DistinctTruncated bool     `json:"distinct_truncated"`
}

// csvPresetInfo is one selectable known CSV format for the import dialog dropdown.
type csvPresetInfo struct {
	Slug string `json:"slug"`
	Name string `json:"name"`
}

type csvInspectResponse struct {
	Headers          []string                `json:"headers"`
	RowCount         int                     `json:"row_count"`
	Columns          []csvColumnInfo         `json:"columns"`
	SuggestedMapping csvmap.SuggestedMapping `json:"suggested_mapping"`
	Presets          []csvPresetInfo         `json:"presets"`
}

// readUploadFile parses the multipart form and reads the "file" field, enforcing
// the 50 MB limit. Returns the bytes or an *echo.HTTPError.
func (h *ImportHandler) readUploadFile(c *echo.Context) ([]byte, error) {
	if err := c.Request().ParseMultipartForm(maxImportBodyBytes); err != nil {
		return nil, echo.NewHTTPError(http.StatusBadRequest, "failed to parse multipart form")
	}
	file, _, err := c.Request().FormFile("file")
	if err != nil {
		return nil, echo.NewHTTPError(http.StatusBadRequest, "missing file field")
	}
	defer func() { _ = file.Close() }()
	lr := io.LimitReader(file, maxImportBodyBytes+1)
	body, err := io.ReadAll(lr)
	if err != nil {
		return nil, echo.NewHTTPError(http.StatusInternalServerError, "failed to read file")
	}
	if len(body) > maxImportBodyBytes {
		return nil, echo.NewHTTPError(http.StatusRequestEntityTooLarge, "file exceeds 50 MB limit")
	}
	return body, nil
}

// csvIGDBGuard returns a 400 *echo.HTTPError when IGDB is not configured.
func (h *ImportHandler) csvIGDBGuard() error {
	if h.igdbClient == nil || !h.igdbClient.Configured() {
		return echo.NewHTTPError(http.StatusBadRequest, "IGDB must be configured to import a CSV collection")
	}
	return nil
}

// HandleImportCSVInspect handles POST /api/import/csv/inspect. It parses the
// uploaded CSV and returns headers, data-row count, and per-column distinct
// values (capped) to drive the mapping dialog.
func (h *ImportHandler) HandleImportCSVInspect(c *echo.Context) error {
	userID := auth.UserIDFromContext(c)
	if userID == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "unauthorized")
	}
	if err := h.csvIGDBGuard(); err != nil {
		return err
	}
	body, herr := h.readUploadFile(c)
	if herr != nil {
		return herr
	}

	records, err := csvmap.ReadRecords(body)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "failed to parse CSV: "+err.Error())
	}
	if len(records) == 0 {
		return echo.NewHTTPError(http.StatusBadRequest, "could not read CSV header")
	}
	header := records[0]

	// Guess the column->field mapping from the headers alone so we know which
	// column is the rating column (to track its numeric max below).
	suggested := csvmap.GuessColumns(header)
	ratingIdx := -1
	if suggested.Columns.Rating != "" {
		for i, name := range header {
			if name == suggested.Columns.Rating {
				ratingIdx = i
				break
			}
		}
	}

	cols := make([]csvColumnInfo, len(header))
	seen := make([]map[string]bool, len(header))
	for i, name := range header {
		cols[i] = csvColumnInfo{Name: name, DistinctValues: []string{}}
		seen[i] = map[string]bool{}
	}

	rowCount := 0
	var ratingMax float64
	for _, rec := range records[1:] {
		rowCount++
		for i := range header {
			if i >= len(rec) {
				continue
			}
			v := strings.TrimSpace(rec[i])
			if i == ratingIdx && v != "" {
				if f, perr := strconv.ParseFloat(v, 64); perr == nil && f > ratingMax {
					ratingMax = f
				}
			}
			if v == "" || cols[i].DistinctTruncated || seen[i][v] {
				continue
			}
			if len(cols[i].DistinctValues) < csvDistinctCap {
				seen[i][v] = true
				cols[i].DistinctValues = append(cols[i].DistinctValues, v)
			} else {
				// A distinct value beyond the cap: flag truncation and stop
				// tracking this column so `seen` stays bounded to the cap.
				cols[i].DistinctTruncated = true
			}
		}
	}

	// Refine the suggestion from the data: rating scale from the observed max,
	// and per-value status guesses from the status column's distinct values.
	if suggested.Columns.Rating != "" {
		suggested.RatingScale = csvmap.GuessRatingScale(ratingMax)
	}
	if suggested.Status.Column != "" {
		for _, col := range cols {
			if col.Name == suggested.Status.Column {
				suggested.Status.ValueMap = csvmap.GuessStatusValueMap(col.DistinctValues)
				break
			}
		}
	}

	presets := make([]csvPresetInfo, 0)
	for _, p := range csvmap.Presets() {
		presets = append(presets, csvPresetInfo{Slug: p.Slug, Name: p.DisplayName})
	}

	return c.JSON(http.StatusOK, csvInspectResponse{
		Headers:          header,
		RowCount:         rowCount,
		Columns:          cols,
		SuggestedMapping: suggested,
		Presets:          presets,
	})
}

// HandleImportCSV handles POST /api/import/csv. It parses the uploaded CSV with
// a csvmap.Config built from the "mapping" form field, then hands off to the
// shared import pipeline (enqueueImportJob).
func (h *ImportHandler) HandleImportCSV(c *echo.Context) error {
	userID := auth.UserIDFromContext(c)
	if userID == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "unauthorized")
	}
	if err := h.csvIGDBGuard(); err != nil {
		return err
	}
	body, herr := h.readUploadFile(c)
	if herr != nil {
		return herr
	}

	format := strings.TrimSpace(c.Request().FormValue("format"))
	var cfg csvmap.Config
	if format != "" && format != "generic" {
		preset, ok := csvmap.PresetBySlug(format)
		if !ok {
			return echo.NewHTTPError(http.StatusBadRequest, "unknown CSV format: "+format)
		}
		cfg = preset
	} else {
		mappingJSON := c.Request().FormValue("mapping")
		if mappingJSON == "" {
			return echo.NewHTTPError(http.StatusBadRequest, "missing mapping field")
		}
		var mapping csvMapping
		if err := json.Unmarshal([]byte(mappingJSON), &mapping); err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, "invalid mapping JSON")
		}
		built, err := buildCSVConfig(mapping)
		if err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, err.Error())
		}
		cfg = built
	}

	games, err := csvmap.Parse(body, cfg)
	if err != nil {
		if errors.Is(err, importmodel.ErrInvalidSignature) {
			return echo.NewHTTPError(http.StatusBadRequest, "this file does not match the selected format")
		}
		return echo.NewHTTPError(http.StatusBadRequest, "failed to parse CSV: "+err.Error())
	}
	if len(games) == 0 {
		return echo.NewHTTPError(http.StatusBadRequest, "no games found in file")
	}

	jobID, total, err := h.enqueueImportJob(c.Request().Context(), userID, models.JobSourceCSV, "CSV", games)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, map[string]any{
		"job_id":      jobID,
		"source":      models.JobSourceCSV,
		"status":      models.JobStatusProcessing,
		"message":     fmt.Sprintf("CSV import job created. Matching %d games.", total),
		"total_items": total,
	})
}
