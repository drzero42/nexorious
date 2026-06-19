package main

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"

	"github.com/drzero42/nexorious/internal/cliclient"
	"github.com/drzero42/nexorious/internal/cliui"
)

// printCSVAutoResolution reports how the server's auto mode mapped the CSV: the
// detected preset, or the guessed column mapping (so an auto import isn't
// silent about being a guess). A nil envelope prints nothing.
func printCSVAutoResolution(out io.Writer, a *cliclient.CSVAutoResolution) {
	if a == nil {
		return
	}
	switch a.Mode {
	case "preset":
		if a.Preset != nil {
			fmt.Fprintf(out, "Auto-detected preset: %s (%s)\n", a.Preset.Slug, a.Preset.Name)
		}
	case "guessed":
		fmt.Fprintln(out, "No preset matched; guessed column mapping:")
		if a.Mapping != nil {
			m := a.Mapping
			pairs := []struct{ field, val string }{
				{"title", m.Columns.Title}, {"igdb_id", m.Columns.IGDBID},
				{"platform", m.Columns.Platform}, {"storefront", m.Columns.Storefront},
				{"rating", m.Columns.Rating}, {"notes", m.Columns.Notes},
				{"acquired_date", m.Columns.AcquiredDate}, {"hours_played", m.Columns.HoursPlayed},
				{"tags", m.Columns.Tags}, {"loved", m.Columns.Loved},
				{"status", m.Status.Column},
			}
			for _, pr := range pairs {
				if pr.val != "" {
					fmt.Fprintf(out, "  %s=%s\n", pr.field, pr.val)
				}
			}
		}
		fmt.Fprintln(out, "Review the import job before applying.")
	}
}

func newImportCSVCmd() *cobra.Command {
	var (
		inspect       bool
		preset        string
		titleCol      string
		igdbIDCol     string
		platformCol   string
		storefrontCol string
		acqDateCol    string
		ratingCol     string
		ratingScale   int
		hoursCol      string
		notesCol      string
		tagsCol       string
		lovedCol      string
		statusCol     string
		statusMap     []string
		mergeByTitle  bool
	)

	cmd := &cobra.Command{
		Use:   "csv <file>",
		Short: "Import a CSV file (preset, manual column mapping, or --inspect)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()
			p, _, err := resolveProfile(cmd)
			if err != nil {
				return err
			}
			filename, data, err := readImportFile(args[0])
			if err != nil {
				return err
			}
			c := cliclient.New(p.URL)

			// --inspect: print server inspection and return.
			if inspect {
				info, err := c.InspectCSV(p.Key, filename, data)
				if err != nil {
					return fmt.Errorf("inspect CSV failed: %w", err)
				}
				if flagBool(cmd, "json") {
					return cliui.EncodeJSON(out, info)
				}
				fmt.Fprintf(out, "Headers: %s\n", strings.Join(info.Headers, ", "))
				fmt.Fprintf(out, "Rows: %d\n", info.RowCount)
				if info.Detected != nil {
					fmt.Fprintf(out, "Detected preset: %s (%s)\n", info.Detected.Slug, info.Detected.Name)
				} else {
					fmt.Fprintln(out, "Detected preset: none")
				}
				if len(info.Presets) > 0 {
					slugs := make([]string, len(info.Presets))
					for i, pr := range info.Presets {
						slugs[i] = pr.Slug
					}
					fmt.Fprintf(out, "Available presets: %s\n", strings.Join(slugs, ", "))
				} else {
					fmt.Fprintln(out, "Available presets: none")
				}
				fmt.Fprintln(out, "Suggested mapping:")
				sm := info.SuggestedMapping
				if sm.Columns.Title != "" {
					fmt.Fprintf(out, "  title: %s\n", sm.Columns.Title)
				}
				if sm.Columns.IGDBID != "" {
					fmt.Fprintf(out, "  igdb_id: %s\n", sm.Columns.IGDBID)
				}
				if sm.Columns.Platform != "" {
					fmt.Fprintf(out, "  platform: %s\n", sm.Columns.Platform)
				}
				if sm.Columns.Storefront != "" {
					fmt.Fprintf(out, "  storefront: %s\n", sm.Columns.Storefront)
				}
				if sm.Columns.Rating != "" {
					fmt.Fprintf(out, "  rating: %s\n", sm.Columns.Rating)
				}
				if sm.Columns.Notes != "" {
					fmt.Fprintf(out, "  notes: %s\n", sm.Columns.Notes)
				}
				if sm.Columns.AcquiredDate != "" {
					fmt.Fprintf(out, "  acquired_date: %s\n", sm.Columns.AcquiredDate)
				}
				if sm.Columns.HoursPlayed != "" {
					fmt.Fprintf(out, "  hours_played: %s\n", sm.Columns.HoursPlayed)
				}
				if sm.Columns.Tags != "" {
					fmt.Fprintf(out, "  tags: %s\n", sm.Columns.Tags)
				}
				if sm.Columns.Loved != "" {
					fmt.Fprintf(out, "  loved: %s\n", sm.Columns.Loved)
				}
				if sm.Status.Column != "" {
					fmt.Fprintf(out, "  status column: %s\n", sm.Status.Column)
				}
				fmt.Fprintf(out, "  rating_scale: %d\n", sm.RatingScale)
				fmt.Fprintf(out, "  merge_by_title: %v\n", sm.MergeByTitle)
				return nil
			}

			// Check mutual exclusion: --preset vs manual column flags.
			manualFlags := []string{
				"title-col", "igdb-id-col", "platform-col", "storefront-col",
				"acquired-date-col", "rating-col", "hours-col", "notes-col",
				"tags-col", "loved-col", "status-col", "status-map", "merge-by-title",
			}
			anyManual := false
			for _, name := range manualFlags {
				if cmd.Flags().Changed(name) {
					anyManual = true
					break
				}
			}
			if preset != "" && anyManual {
				return fmt.Errorf("use --preset OR column-mapping flags, not both")
			}

			// Preset path.
			if preset != "" {
				res, err := c.ImportCSV(p.Key, filename, data, preset, nil)
				if err != nil {
					return fmt.Errorf("import CSV failed: %w", err)
				}
				return printImportResult(cmd, res)
			}

			// Auto path: no preset and no manual flags -> let the server detect
			// a preset or guess the mapping in one call.
			if !anyManual {
				res, err := c.ImportCSV(p.Key, filename, data, "", nil)
				if err != nil {
					return fmt.Errorf("import CSV failed: %w", err)
				}
				if !flagBool(cmd, "json") {
					printCSVAutoResolution(out, res.Auto)
				}
				return printImportResult(cmd, res)
			}

			// Manual mapping path.
			if strings.TrimSpace(titleCol) == "" {
				return fmt.Errorf("a --title-col is required (or use --preset, or run with no flags to auto-detect)")
			}
			if cmd.Flags().Changed("status-map") && statusCol == "" {
				return fmt.Errorf("--status-map requires --status-col")
			}

			// Build columns map with only non-empty values.
			columns := map[string]string{}
			if titleCol != "" {
				columns["title"] = titleCol
			}
			if igdbIDCol != "" {
				columns["igdb_id"] = igdbIDCol
			}
			if platformCol != "" {
				columns["platform"] = platformCol
			}
			if storefrontCol != "" {
				columns["storefront"] = storefrontCol
			}
			if ratingCol != "" {
				columns["rating"] = ratingCol
			}
			if notesCol != "" {
				columns["notes"] = notesCol
			}
			if acqDateCol != "" {
				columns["acquired_date"] = acqDateCol
			}
			if hoursCol != "" {
				columns["hours_played"] = hoursCol
			}
			if tagsCol != "" {
				columns["tags"] = tagsCol
			}
			if lovedCol != "" {
				columns["loved"] = lovedCol
			}

			mapping := map[string]any{
				"columns":        columns,
				"merge_by_title": mergeByTitle,
			}

			// Status column + value map.
			if statusCol != "" {
				vm := map[string]string{}
				for _, entry := range statusMap {
					idx := strings.Index(entry, "=")
					if idx < 0 {
						return fmt.Errorf("--status-map entry %q must be in raw=canonical form", entry)
					}
					vm[entry[:idx]] = entry[idx+1:]
				}
				mapping["status"] = map[string]any{
					"column":    statusCol,
					"value_map": vm,
				}
			}

			// Rating scale (only set when a rating column is provided).
			if ratingCol != "" {
				if ratingScale != 5 && ratingScale != 10 && ratingScale != 100 {
					return fmt.Errorf("--rating-scale must be 5, 10, or 100")
				}
				mapping["rating_scale"] = ratingScale
			}

			mappingJSON, err := json.Marshal(mapping)
			if err != nil {
				return fmt.Errorf("marshal mapping: %w", err)
			}
			res, err := c.ImportCSV(p.Key, filename, data, "", json.RawMessage(mappingJSON))
			if err != nil {
				return fmt.Errorf("import CSV failed: %w", err)
			}
			return printImportResult(cmd, res)
		},
	}

	f := cmd.Flags()
	f.BoolVar(&inspect, "inspect", false, "Print server inspection (headers, row count, detected preset, suggested mapping) and return")
	f.StringVar(&preset, "preset", "", "Use a server-side CSV preset (e.g. grouvee)")
	f.StringVar(&titleCol, "title-col", "", "Column containing the game title")
	f.StringVar(&igdbIDCol, "igdb-id-col", "", "Column containing IGDB IDs")
	f.StringVar(&platformCol, "platform-col", "", "Column containing platform names")
	f.StringVar(&storefrontCol, "storefront-col", "", "Column containing storefront names")
	f.StringVar(&acqDateCol, "acquired-date-col", "", "Column containing acquisition dates")
	f.StringVar(&ratingCol, "rating-col", "", "Column containing ratings")
	f.IntVar(&ratingScale, "rating-scale", 10, "Rating scale (5, 10, or 100); required when --rating-col is set")
	f.StringVar(&hoursCol, "hours-col", "", "Column containing hours played")
	f.StringVar(&notesCol, "notes-col", "", "Column containing notes")
	f.StringVar(&tagsCol, "tags-col", "", "Column containing tags")
	f.StringVar(&lovedCol, "loved-col", "", "Column containing loved/favourite flags")
	f.StringVar(&statusCol, "status-col", "", "Column containing play status values")
	f.StringArrayVar(&statusMap, "status-map", nil, "Map a raw status value to a canonical one (raw=canonical); repeatable")
	f.BoolVar(&mergeByTitle, "merge-by-title", false, "Merge entries that share the same title")

	return cmd
}
