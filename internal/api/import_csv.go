package api

import (
	"encoding/json"
	"errors"
	"fmt"
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

// detectPreset returns the first registered preset whose signature matches the
// uploaded header, or nil if none match. Presets with an empty signature are
// skipped: csvmap.MatchesSignature treats an empty signature as "matches
// anything", which is correct for the manual/generic path but must never
// auto-match here. First match in registry order wins.
func detectPreset(header []string) *csvPresetInfo {
	for _, p := range csvmap.Presets() {
		if len(p.Config.Signature) == 0 {
			continue
		}
		if csvmap.MatchesSignature(header, p.Config) {
			return &csvPresetInfo{Slug: p.Slug, Name: p.DisplayName}
		}
	}
	return nil
}

type csvInspectResponse struct {
	Headers          []string                `json:"headers"`
	RowCount         int                     `json:"row_count"`
	Columns          []csvColumnInfo         `json:"columns"`
	SuggestedMapping csvmap.SuggestedMapping `json:"suggested_mapping"`
	Presets          []csvPresetInfo         `json:"presets"`
	Detected         *csvPresetInfo          `json:"detected,omitempty"`
}

// scanCSV computes, in one pass over the data rows, each column's capped set of
// distinct non-empty values and a data-refined suggested mapping (rating scale
// from the observed max; status value-map from the status column's distinct
// values). records[0] is the header. Shared by the inspect handler and the
// auto import path so the inspect-time and import-time guesses cannot drift.
func scanCSV(records [][]string) (cols []csvColumnInfo, suggested csvmap.SuggestedMapping) {
	header := records[0]
	suggested = csvmap.GuessColumns(header)
	ratingIdx := -1
	if suggested.Columns.Rating != "" {
		for i, name := range header {
			if name == suggested.Columns.Rating {
				ratingIdx = i
				break
			}
		}
	}

	cols = make([]csvColumnInfo, len(header))
	seen := make([]map[string]bool, len(header))
	for i, name := range header {
		cols[i] = csvColumnInfo{Name: name, DistinctValues: []string{}}
		seen[i] = map[string]bool{}
	}

	var ratingMax float64
	for _, rec := range records[1:] {
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
				cols[i].DistinctTruncated = true
			}
		}
	}

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
	return cols, suggested
}

// errCSVAutoNoTitle is returned by resolveCSVAuto when neither a preset nor the
// header guess can identify a title column, so there is nothing to import.
var errCSVAutoNoTitle = errors.New("could not auto-detect a column mapping: no title column found. Inspect the file and choose a preset or map the columns explicitly")

// csvAutoResolution is the outcome of the import-time detect-or-guess policy:
// either a preset matched by header signature, or a data-refined guessed
// mapping. Config is what csvmap.Parse runs with.
type csvAutoResolution struct {
	Config   csvmap.Config
	Detected *csvPresetInfo           // non-nil when a preset signature matched
	Mapping  *csvmap.SuggestedMapping // non-nil when guessed (no preset matched)
}

// resolveCSVAuto applies the detect-or-guess policy to a parsed CSV:
// a matching preset wins; otherwise the data-refined GuessColumns mapping is
// used, or errCSVAutoNoTitle if that yields no title column.
func resolveCSVAuto(records [][]string) (csvAutoResolution, error) {
	header := records[0]
	if d := detectPreset(header); d != nil {
		cfg, ok := csvmap.PresetBySlug(d.Slug)
		if !ok {
			return csvAutoResolution{}, fmt.Errorf("detected preset %q is not registered", d.Slug)
		}
		return csvAutoResolution{Config: cfg, Detected: d}, nil
	}
	_, suggested := scanCSV(records)
	if strings.TrimSpace(suggested.Columns.Title) == "" {
		return csvAutoResolution{}, errCSVAutoNoTitle
	}
	cfg, err := buildCSVConfig(suggestedToCSVMapping(suggested))
	if err != nil {
		return csvAutoResolution{}, err
	}
	return csvAutoResolution{Config: cfg, Mapping: &suggested}, nil
}

// suggestedToCSVMapping copies a csvmap.SuggestedMapping into the flat csvMapping
// shape buildCSVConfig consumes (the two share an identical JSON layout), so the
// guessed mapping flows through the same translation as a user-submitted one.
func suggestedToCSVMapping(s csvmap.SuggestedMapping) csvMapping {
	var m csvMapping
	m.Columns.Title = s.Columns.Title
	m.Columns.IGDBID = s.Columns.IGDBID
	m.Columns.Platform = s.Columns.Platform
	m.Columns.Storefront = s.Columns.Storefront
	m.Columns.Rating = s.Columns.Rating
	m.Columns.Notes = s.Columns.Notes
	m.Columns.AcquiredDate = s.Columns.AcquiredDate
	m.Columns.HoursPlayed = s.Columns.HoursPlayed
	m.Columns.Tags = s.Columns.Tags
	m.Columns.Loved = s.Columns.Loved
	m.Status.Column = s.Status.Column
	m.Status.ValueMap = s.Status.ValueMap
	m.RatingScale = s.RatingScale
	m.MergeByTitle = s.MergeByTitle
	return m
}

// HandleImportCSVInspect handles POST /api/import/csv/inspect. It parses the
// uploaded CSV and returns headers, data-row count, and per-column distinct
// values (capped) to drive the mapping dialog.
func (h *ImportHandler) HandleImportCSVInspect(c *echo.Context) error {
	userID := auth.UserIDFromContext(c)
	if userID == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "unauthorized")
	}
	if err := h.igdbGuard("CSV"); err != nil {
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

	cols, suggested := scanCSV(records)

	presets := make([]csvPresetInfo, 0)
	for _, p := range csvmap.Presets() {
		presets = append(presets, csvPresetInfo{Slug: p.Slug, Name: p.DisplayName})
	}

	return c.JSON(http.StatusOK, csvInspectResponse{
		Headers:          records[0],
		RowCount:         len(records) - 1,
		Columns:          cols,
		SuggestedMapping: suggested,
		Presets:          presets,
		Detected:         detectPreset(records[0]),
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
	if err := h.igdbGuard("CSV"); err != nil {
		return err
	}
	body, herr := h.readUploadFile(c)
	if herr != nil {
		return herr
	}

	format := strings.TrimSpace(c.Request().FormValue("format"))
	mappingJSON := c.Request().FormValue("mapping")
	var cfg csvmap.Config
	var autoRes *csvAutoResolution
	switch {
	case format != "" && format != "generic":
		// Preset path: server-side Config wins; any "mapping" field is ignored.
		preset, ok := csvmap.PresetBySlug(format)
		if !ok {
			return echo.NewHTTPError(http.StatusBadRequest, "unknown CSV format: "+format)
		}
		cfg = preset
	case format == "" && strings.TrimSpace(mappingJSON) == "":
		// Auto path: no format and no mapping -> detect a preset, else guess.
		records, err := csvmap.ReadRecords(body)
		if err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, "failed to parse CSV: "+err.Error())
		}
		if len(records) == 0 {
			return echo.NewHTTPError(http.StatusBadRequest, "could not read CSV header")
		}
		res, err := resolveCSVAuto(records)
		if err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, err.Error())
		}
		cfg = res.Config
		autoRes = &res
	default:
		// Manual path: a mapping is required (covers format=generic + no mapping).
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
	resp := map[string]any{
		"job_id":      jobID,
		"source":      models.JobSourceCSV,
		"status":      models.JobStatusProcessing,
		"message":     fmt.Sprintf("CSV import job created. Matching %d games.", total),
		"total_items": total,
	}
	if autoRes != nil {
		auto := map[string]any{}
		if autoRes.Detected != nil {
			auto["mode"] = "preset"
			auto["preset"] = autoRes.Detected
		} else {
			auto["mode"] = "guessed"
			auto["mapping"] = autoRes.Mapping
		}
		resp["auto"] = auto
	}
	return c.JSON(http.StatusOK, resp)
}
