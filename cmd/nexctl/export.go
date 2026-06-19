package main

import (
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/drzero42/nexorious/internal/cliclient"
	"github.com/drzero42/nexorious/internal/cliui"
)

// exportPollInterval is the sleep between job-status polls. A package-level var
// so tests can set it to zero without any sleep overhead.
var exportPollInterval = 2 * time.Second

func newExportCmd() *cobra.Command {
	var format, out string
	var noWait bool

	cmd := &cobra.Command{
		Use:   "export",
		Short: "Export your library to a file",
		RunE: func(cmd *cobra.Command, _ []string) error {
			outW := cmd.OutOrStdout()
			// --out names a download destination, which --no-wait never reaches;
			// rejecting the combination avoids silently ignoring --out.
			if noWait && out != "" {
				return fmt.Errorf("--out and --no-wait are mutually exclusive")
			}
			p, _, err := resolveProfile(cmd)
			if err != nil {
				return err
			}

			// Format is validated by cliclient.TriggerExport (before any network
			// call), so the command layer does not re-check it.
			c := cliclient.New(p.URL)
			res, err := c.TriggerExport(p.Key, format)
			if err != nil {
				return fmt.Errorf("trigger export: %w", err)
			}

			if noWait {
				if flagBool(cmd, "json") {
					return cliui.EncodeJSON(outW, res)
				}
				if flagBool(cmd, "quiet") {
					fmt.Fprintln(outW, res.JobID)
					return nil
				}
				fmt.Fprintf(outW, "export job %s queued (%d games)\n", res.JobID, res.EstimatedItems)
				return nil
			}

			// Poll until the job reaches a terminal state.
			var job *cliclient.Job
			for {
				job, err = c.GetJob(p.Key, res.JobID)
				if err != nil {
					return fmt.Errorf("poll job: %w", err)
				}
				if job.IsTerminal {
					break
				}
				time.Sleep(exportPollInterval)
			}

			if job.Status != "completed" {
				if job.ErrorMessage != nil {
					return fmt.Errorf("export job %s: %s", job.Status, *job.ErrorMessage)
				}
				return fmt.Errorf("export job ended with status %s", job.Status)
			}

			// Resolve the destination writer / path.
			if out == "-" {
				// Stream directly to stdout; no extra output so the stream is clean.
				return c.DownloadExport(p.Key, res.JobID, outW)
			}

			destPath := out
			if destPath == "" {
				destPath = fmt.Sprintf("nexorious_export_%s.%s", res.JobID, format)
			}

			f, err := os.Create(destPath) //nolint:gosec // operator-supplied output path, not network-derived
			if err != nil {
				return fmt.Errorf("create output file: %w", err)
			}
			defer func() { _ = f.Close() }()

			if err := c.DownloadExport(p.Key, res.JobID, f); err != nil {
				return fmt.Errorf("download export: %w", err)
			}
			fmt.Fprintf(outW, "exported to %s\n", destPath)
			return nil
		},
	}

	f := cmd.Flags()
	f.StringVar(&format, "format", "json", "Export format: json or csv")
	f.StringVar(&out, "out", "", `Output path; "-" for stdout; empty uses nexorious_export_<job-id>.<ext>`)
	f.BoolVar(&noWait, "no-wait", false, "Trigger the export and return without polling or downloading")
	return cmd
}
