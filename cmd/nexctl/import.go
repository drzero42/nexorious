package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/drzero42/nexorious/internal/cliclient"
	"github.com/drzero42/nexorious/internal/cliui"
)

func newImportCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "import", Short: "Import a library from a file or migration source"}
	cmd.AddCommand(newImportSourcesCmd(), newImportNexoriousCmd(), newImportRunCmd())
	return cmd
}

// readImportFile reads the file at path and returns its base name and contents.
func readImportFile(path string) (filename string, data []byte, err error) {
	data, err = os.ReadFile(path) //nolint:gosec // path is a CLI argument supplied by the operator, not from network input
	if err != nil {
		return "", nil, fmt.Errorf("read %s: %w", path, err)
	}
	return filepath.Base(path), data, nil
}

// resolveImportSource validates arg against the registered import sources by
// fetching the sources list. Matching is case-insensitive. On success the
// canonical slug is returned; on failure a descriptive error listing valid
// options is returned.
func resolveImportSource(c *cliclient.Client, key, arg string) (string, error) {
	sources, err := c.ListImportSources(key)
	if err != nil {
		return "", fmt.Errorf("list import sources failed: %w", err)
	}
	if len(sources) == 0 {
		return "", fmt.Errorf("no import sources available on this server")
	}
	lower := strings.ToLower(arg)
	for _, s := range sources {
		if strings.ToLower(s.Slug) == lower {
			return s.Slug, nil
		}
	}
	slugs := make([]string, len(sources))
	for i, s := range sources {
		slugs[i] = s.Slug
	}
	return "", fmt.Errorf("unknown import source %q; valid: %s\n(nexorious and csv are separate subcommands)", arg, strings.Join(slugs, ", "))
}

// printImportResult writes a text confirmation or JSON for an import result.
func printImportResult(cmd *cobra.Command, res *cliclient.ImportResult) error {
	out := cmd.OutOrStdout()
	if flagBool(cmd, "json") {
		return cliui.EncodeJSON(out, res)
	}
	msg := fmt.Sprintf("created import job %s (%d games", res.JobID, res.TotalItems)
	if res.SkippedCount > 0 {
		msg += fmt.Sprintf(", %d skipped", res.SkippedCount)
	}
	msg += ")"
	fmt.Fprintln(out, msg)
	return nil
}

func newImportSourcesCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "sources",
		Short: "List available import sources",
		RunE: func(cmd *cobra.Command, _ []string) error {
			out := cmd.OutOrStdout()
			p, _, err := resolveProfile(cmd)
			if err != nil {
				return err
			}
			sources, err := cliclient.New(p.URL).ListImportSources(p.Key)
			if err != nil {
				return fmt.Errorf("list import sources failed: %w", err)
			}
			if flagBool(cmd, "json") {
				return cliui.EncodeJSON(out, sources)
			}
			if flagBool(cmd, "quiet") {
				for _, s := range sources {
					fmt.Fprintln(out, s.Slug)
				}
				return nil
			}
			if len(sources) == 0 {
				fmt.Fprintln(out, "No import sources.")
				return nil
			}
			tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
			fmt.Fprintln(tw, "SLUG\tNAME\tDESCRIPTION")
			for _, s := range sources {
				fmt.Fprintf(tw, "%s\t%s\t%s\n", s.Slug, s.DisplayName, s.Description)
			}
			if err := tw.Flush(); err != nil {
				return err
			}
			fmt.Fprintln(out, "\nDedicated importers: nexorious (Nexorious JSON), csv (run 'import csv <file> --inspect' to discover presets)")
			return nil
		},
	}
}

func newImportNexoriousCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "nexorious <file>",
		Short: "Import a Nexorious JSON export file",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			p, _, err := resolveProfile(cmd)
			if err != nil {
				return err
			}
			filename, data, err := readImportFile(args[0])
			if err != nil {
				return err
			}
			res, err := cliclient.New(p.URL).ImportNexorious(p.Key, filename, data)
			if err != nil {
				return fmt.Errorf("import failed: %w", err)
			}
			return printImportResult(cmd, res)
		},
	}
}

func newImportRunCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "run <source> <file>",
		Short: "Import a file using a registered import source",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			p, _, err := resolveProfile(cmd)
			if err != nil {
				return err
			}
			c := cliclient.New(p.URL)
			slug, err := resolveImportSource(c, p.Key, args[0])
			if err != nil {
				return err
			}
			filename, data, err := readImportFile(args[1])
			if err != nil {
				return err
			}
			res, err := c.ImportSource(p.Key, slug, filename, data)
			if err != nil {
				return fmt.Errorf("import failed: %w", err)
			}
			return printImportResult(cmd, res)
		},
	}
}
