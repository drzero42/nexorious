package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/drzero42/nexorious/internal/cliclient"
	"github.com/drzero42/nexorious/internal/cliui"
)

func newBackupCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "backup",
		Short: "Manage server backups (admin)",
	}
	cmd.AddCommand(
		newBackupListCmd(),
		newBackupCreateCmd(),
		newBackupRmCmd(),
		newBackupDownloadCmd(),
		newBackupRestoreCmd(),
		newBackupScheduleCmd(),
	)
	return cmd
}

// humanBackupBytes formats a byte count as a short human-readable string.
func humanBackupBytes(n int64) string {
	switch {
	case n >= 1<<30:
		return strconv.FormatFloat(float64(n)/float64(1<<30), 'f', 1, 64) + " GiB"
	case n >= 1<<20:
		return strconv.FormatFloat(float64(n)/float64(1<<20), 'f', 1, 64) + " MiB"
	case n >= 1<<10:
		return strconv.FormatFloat(float64(n)/float64(1<<10), 'f', 1, 64) + " KiB"
	default:
		return strconv.FormatInt(n, 10) + " B"
	}
}

func newBackupListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List stored backups",
		RunE: func(cmd *cobra.Command, _ []string) error {
			out := cmd.OutOrStdout()
			p, _, err := resolveProfile(cmd)
			if err != nil {
				return err
			}
			backups, err := cliclient.New(p.URL).ListBackups(cmd.Context(), p.Key)
			if err != nil {
				return fmt.Errorf("list backups: %w", err)
			}

			if flagBool(cmd, "json") {
				return cliui.EncodeJSON(out, backups)
			}
			if flagBool(cmd, "quiet") {
				for i := range backups {
					fmt.Fprintln(out, backups[i].ID)
				}
				return nil
			}
			if len(backups) == 0 {
				fmt.Fprintln(out, "No backups.")
				return nil
			}
			tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
			fmt.Fprintln(tw, "ID\tTYPE\tSIZE\tCREATED\tGAMES\tTAGS")
			for i := range backups {
				b := &backups[i]
				fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%d\t%d\n",
					b.ID, b.BackupType, humanBackupBytes(b.SizeBytes),
					b.CreatedAt, b.Stats.Games, b.Stats.Tags)
			}
			return tw.Flush()
		},
	}
}

func newBackupCreateCmd() *cobra.Command {
	var outPath string
	var download bool
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Trigger a manual backup",
		Long: `Trigger a manual backup and wait for it to finish.

With --download the new archive is also downloaded, to --out (a path, or "-"
for stdout) or, without --out, to the server filename (else backup-<id>.tar.gz).`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			out := cmd.OutOrStdout()
			errW := cmd.ErrOrStderr()
			asJSON, quiet := flagBool(cmd, "json"), flagBool(cmd, "quiet")
			// Validate flags before the backup exists, so a usage error never
			// leaves a backup behind.
			if cmd.Flags().Changed("out") && !download {
				return fmt.Errorf("--out needs --download")
			}
			toStdout := download && outPath == "-"
			if toStdout && (asJSON || quiet) {
				return fmt.Errorf("--out - writes the archive to stdout; it can't be combined with --json or --quiet")
			}

			p, _, err := resolveProfile(cmd)
			if err != nil {
				return err
			}
			c := cliclient.New(p.URL)
			res, err := c.CreateBackup(cmd.Context(), p.Key)
			if err != nil {
				return fmt.Errorf("create backup: %w", err)
			}

			// With the archive on stdout, the status line goes to stderr.
			statusW := out
			if toStdout {
				statusW = errW
			}
			switch {
			case asJSON:
				if err := cliui.EncodeJSON(out, res); err != nil {
					return err
				}
			case quiet:
				fmt.Fprintln(out, res.BackupID)
			default:
				fmt.Fprintf(statusW, "created backup %s\n", res.BackupID)
			}
			if !download {
				return nil
			}

			destPath, err := saveBackup(cmd.Context(), c, p.Key, res.BackupID, outPath, out)
			if err != nil {
				return fmt.Errorf("%w (backup %s was created; retry with: nexctl backup download %s)", err, res.BackupID, res.BackupID)
			}
			if destPath != "" {
				// Keep stdout to the id / JSON under --quiet and --json.
				doneW := out
				if asJSON || quiet {
					doneW = errW
				}
				fmt.Fprintf(doneW, "downloaded to %s\n", destPath)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&download, "download", false, "Download the new backup once it is created")
	cmd.Flags().StringVar(&outPath, "out", "", `With --download: output path; "-" for stdout; empty uses the server filename (else backup-<id>.tar.gz)`)
	return cmd
}

func newBackupRmCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "rm <id>",
		Short: "Delete a backup",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()
			in := bufio.NewReader(cmd.InOrStdin())
			p, _, err := resolveProfile(cmd)
			if err != nil {
				return err
			}
			ok, err := cliui.Confirm(in, out, fmt.Sprintf("Delete backup %s?", args[0]), flagBool(cmd, "yes"))
			if err != nil {
				return err
			}
			if !ok {
				fmt.Fprintln(out, "Aborted.")
				return nil
			}
			if err := cliclient.New(p.URL).DeleteBackup(cmd.Context(), p.Key, args[0]); err != nil {
				return fmt.Errorf("delete backup: %w", err)
			}
			fmt.Fprintf(out, "removed backup %s\n", args[0])
			return nil
		},
	}
}

func newBackupDownloadCmd() *cobra.Command {
	var outPath string
	var latest bool
	cmd := &cobra.Command{
		Use:   "download [<id>]",
		Short: "Download a backup archive",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			outW := cmd.OutOrStdout()
			hasID := len(args) == 1
			if hasID && latest {
				return fmt.Errorf("specify either an id or --latest, not both")
			}
			if !hasID && !latest {
				return fmt.Errorf("specify an id or --latest")
			}

			p, _, err := resolveProfile(cmd)
			if err != nil {
				return err
			}
			c := cliclient.New(p.URL)

			id := ""
			if hasID {
				id = args[0]
			} else {
				id, err = latestBackupID(cmd.Context(), c, p.Key)
				if err != nil {
					return err
				}
			}

			destPath, err := saveBackup(cmd.Context(), c, p.Key, id, outPath, outW)
			if err != nil {
				return err
			}
			if destPath != "" {
				fmt.Fprintf(outW, "downloaded to %s\n", destPath)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&outPath, "out", "", `Output path; "-" for stdout; empty uses the server filename (else backup-<id>.tar.gz)`)
	cmd.Flags().BoolVar(&latest, "latest", false, "Download the most recent backup instead of an explicit id")
	return cmd
}

// saveBackup downloads backup id to outPath: "-" streams it to stdout,
// empty uses the server-suggested filename (else backup-<id>.tar.gz). It
// returns the path written, or "" when the archive went to stdout. It prints
// nothing itself, so stdout carries only the archive under "-".
func saveBackup(ctx context.Context, c *cliclient.Client, key, id, outPath string, stdout io.Writer) (string, error) {
	serverName, body, err := c.OpenBackupDownload(ctx, key, id)
	if err != nil {
		return "", fmt.Errorf("download backup: %w", err)
	}
	defer func() { _ = body.Close() }()

	if outPath == "-" {
		if _, err := io.Copy(stdout, body); err != nil {
			return "", fmt.Errorf("stream backup: %w", err)
		}
		return "", nil
	}

	// Prefer an explicit --out, then the server-suggested filename,
	// then a predictable fallback.
	destPath := outPath
	if destPath == "" {
		destPath = serverName
	}
	if destPath == "" {
		destPath = fmt.Sprintf("backup-%s.tar.gz", id)
	}
	if err := writeFileAtomic(destPath, body); err != nil {
		return "", fmt.Errorf("download backup: %w", err)
	}
	return destPath, nil
}

// latestBackupID returns the id of the newest backup, resolving "latest" by
// sorting the list by created_at descending client-side rather than trusting
// the server's array order.
func latestBackupID(ctx context.Context, c *cliclient.Client, key string) (string, error) {
	backups, err := c.ListBackups(ctx, key)
	if err != nil {
		return "", fmt.Errorf("list backups: %w", err)
	}
	if len(backups) == 0 {
		return "", fmt.Errorf("no backups available")
	}
	sort.Slice(backups, func(i, j int) bool {
		ti, errI := time.Parse(time.RFC3339, backups[i].CreatedAt)
		tj, errJ := time.Parse(time.RFC3339, backups[j].CreatedAt)
		if errI != nil || errJ != nil {
			// Fall back to lexical order on unparseable timestamps; RFC3339
			// sorts correctly lexically anyway for same-offset values.
			return backups[i].CreatedAt > backups[j].CreatedAt
		}
		return ti.After(tj)
	})
	return backups[0].ID, nil
}

func newBackupRestoreCmd() *cobra.Command {
	var filePath string
	cmd := &cobra.Command{
		Use:   "restore [<id>]",
		Short: "Restore database from a backup",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()
			in := bufio.NewReader(cmd.InOrStdin())
			hasID := len(args) == 1
			hasFile := cmd.Flags().Changed("file")

			if hasID == hasFile {
				return fmt.Errorf("specify exactly one of <id> or --file")
			}

			p, _, err := resolveProfile(cmd)
			if err != nil {
				return err
			}

			confirmMsg := "WARNING: Restoring replaces the whole database, cancels all running and queued " +
				"background work, and restarts the server; every user has to log in again. Proceed?"
			ok, err := cliui.Confirm(in, out, confirmMsg, flagBool(cmd, "yes"))
			if err != nil {
				return err
			}
			if !ok {
				fmt.Fprintln(out, "Aborted.")
				return nil
			}

			c := cliclient.New(p.URL)
			if hasFile {
				f, err := os.Open(filePath) //nolint:gosec // operator-supplied restore archive path
				if err != nil {
					return fmt.Errorf("open file: %w", err)
				}
				defer func() { _ = f.Close() }()
				filename := filePath
				if idx := strings.LastIndexByte(filePath, '/'); idx >= 0 {
					filename = filePath[idx+1:]
				}
				err = runRestore(cmd.Context(), out, c, func(ctx context.Context) (*cliclient.RestoreResult, error) {
					return c.RestoreBackupUpload(ctx, p.Key, filename, f)
				})
				if err != nil {
					return fmt.Errorf("restore upload: %w", err)
				}
				return nil
			}

			err = runRestore(cmd.Context(), out, c, func(ctx context.Context) (*cliclient.RestoreResult, error) {
				return c.RestoreBackup(ctx, p.Key, args[0])
			})
			if err != nil {
				return fmt.Errorf("restore: %w", err)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&filePath, "file", "", "Path to a backup archive to restore from")
	return cmd
}

func newBackupScheduleCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "schedule",
		Short: "Show or update the backup schedule",
		RunE: func(cmd *cobra.Command, _ []string) error {
			out := cmd.OutOrStdout()
			p, _, err := resolveProfile(cmd)
			if err != nil {
				return err
			}
			cfg, err := cliclient.New(p.URL).GetBackupConfig(cmd.Context(), p.Key)
			if err != nil {
				return fmt.Errorf("get backup config: %w", err)
			}
			if flagBool(cmd, "json") {
				return cliui.EncodeJSON(out, cfg)
			}
			fmt.Fprintf(out, "schedule:        %s\n", cfg.Schedule)
			fmt.Fprintf(out, "schedule_time:   %s\n", cfg.ScheduleTime)
			fmt.Fprintf(out, "schedule_day:    %d\n", cfg.ScheduleDay)
			fmt.Fprintf(out, "retention_mode:  %s\n", cfg.RetentionMode)
			fmt.Fprintf(out, "retention_value: %d\n", cfg.RetentionValue)
			fmt.Fprintf(out, "updated_at:      %s\n", cfg.UpdatedAt)
			return nil
		},
	}
	cmd.AddCommand(newBackupScheduleSetCmd())
	return cmd
}

func newBackupScheduleSetCmd() *cobra.Command {
	var frequency, schedTime, retentionMode string
	var day, retentionValue int
	cmd := &cobra.Command{
		Use:   "set",
		Short: "Update backup schedule settings",
		RunE: func(cmd *cobra.Command, _ []string) error {
			out := cmd.OutOrStdout()
			p, _, err := resolveProfile(cmd)
			if err != nil {
				return err
			}
			c := cliclient.New(p.URL)

			// Fetch the current config and overlay only the changed flags. Sending
			// the full struct keeps the unspecified fields (incl. retention) at
			// their current, server-validated values.
			current, err := c.GetBackupConfig(cmd.Context(), p.Key)
			if err != nil {
				return fmt.Errorf("get backup config: %w", err)
			}

			updated := *current
			if cmd.Flags().Changed("frequency") {
				updated.Schedule = frequency
			}
			if cmd.Flags().Changed("time") {
				updated.ScheduleTime = schedTime
			}
			if cmd.Flags().Changed("day") {
				updated.ScheduleDay = day
			}
			if cmd.Flags().Changed("retention-mode") {
				updated.RetentionMode = retentionMode
			}
			if cmd.Flags().Changed("retention-value") {
				updated.RetentionValue = retentionValue
			}

			saved, err := c.UpdateBackupConfig(cmd.Context(), p.Key, updated)
			if err != nil {
				return fmt.Errorf("update backup config: %w", err)
			}
			if flagBool(cmd, "json") {
				return cliui.EncodeJSON(out, saved)
			}
			fmt.Fprintf(out, "schedule:        %s\n", saved.Schedule)
			fmt.Fprintf(out, "schedule_time:   %s\n", saved.ScheduleTime)
			fmt.Fprintf(out, "schedule_day:    %d\n", saved.ScheduleDay)
			fmt.Fprintf(out, "retention_mode:  %s\n", saved.RetentionMode)
			fmt.Fprintf(out, "retention_value: %d\n", saved.RetentionValue)
			fmt.Fprintf(out, "updated_at:      %s\n", saved.UpdatedAt)
			return nil
		},
	}
	f := cmd.Flags()
	f.StringVar(&frequency, "frequency", "", "Backup frequency: manual, daily, or weekly")
	f.StringVar(&schedTime, "time", "", "Time of day for scheduled backups (HH:MM)")
	f.IntVar(&day, "day", 0, "Day of week for weekly backups (0=Sunday … 6=Saturday)")
	f.StringVar(&retentionMode, "retention-mode", "", "Retention mode: days or count")
	f.IntVar(&retentionValue, "retention-value", 0, "Retention amount (days to keep, or number of backups)")
	return cmd
}
