package api

import (
	"fmt"
	"strings"

	"github.com/drzero42/nexorious/internal/services/csvmap"
)

// csvMapping is the flat, frontend-shaped request body the mapping dialog POSTs
// as the "mapping" form field. It is translated to a csvmap.Config (simple
// subset only) by buildCSVConfig.
type csvMapping struct {
	Columns struct {
		Title        string `json:"title"`
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
			Title: m.Columns.Title,
			Tags:  m.Columns.Tags,
			Loved: m.Columns.Loved,
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
