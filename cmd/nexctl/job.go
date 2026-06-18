package main

import (
	"fmt"
	"net/url"
	"strconv"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/drzero42/nexorious/internal/cliclient"
	"github.com/drzero42/nexorious/internal/cliui"
)

// yesNo renders a bool as a human-friendly yes/no.
func yesNo(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}

func newJobCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "job", Short: "Inspect and manage background jobs"}
	cmd.AddCommand(newJobListCmd(), newJobShowCmd())
	return cmd
}

func newJobListCmd() *cobra.Command {
	var (
		status, jobType, source string
		sortBy, order           string
		limit, page             int
	)
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List background jobs",
		RunE: func(cmd *cobra.Command, _ []string) error {
			out := cmd.OutOrStdout()
			p, _, err := resolveProfile(cmd)
			if err != nil {
				return err
			}

			params := url.Values{}
			setIf(params, "status", status)
			setIf(params, "job_type", jobType)
			setIf(params, "source", source)
			setIf(params, "sort_by", sortBy)
			setIf(params, "sort_order", order)
			if limit > 0 {
				params.Set("per_page", strconv.Itoa(limit))
			}
			if page > 0 {
				params.Set("page", strconv.Itoa(page))
			}

			res, err := cliclient.New(p.URL).ListJobs(p.Key, params)
			if err != nil {
				return fmt.Errorf("list jobs failed: %w", err)
			}

			if flagBool(cmd, "json") {
				return cliui.EncodeJSON(out, res)
			}
			if flagBool(cmd, "quiet") {
				for i := range res.Jobs {
					fmt.Fprintln(out, res.Jobs[i].ID)
				}
				return nil
			}
			if len(res.Jobs) == 0 {
				fmt.Fprintln(out, "No jobs.")
				return nil
			}
			tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
			fmt.Fprintln(tw, "ID\tTYPE\tSOURCE\tSTATUS\tITEMS\tCREATED")
			for i := range res.Jobs {
				j := &res.Jobs[i]
				fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%d\t%s\n",
					j.ID, j.JobType, j.Source, j.Status, j.TotalItems, j.CreatedAt)
			}
			if err := tw.Flush(); err != nil {
				return err
			}
			fmt.Fprintf(out, "\n%d of %d (page %d/%d)\n", len(res.Jobs), res.Total, max1(res.Page), max1(res.TotalPages))
			return nil
		},
	}
	f := cmd.Flags()
	f.StringVar(&status, "status", "", "Filter by job status")
	f.StringVar(&jobType, "type", "", "Filter by job type")
	f.StringVar(&source, "source", "", "Filter by source")
	f.StringVar(&sortBy, "sort", "", "Sort field (created_at, started_at, completed_at, job_type, status)")
	f.StringVar(&order, "order", "", "Sort order (asc|desc)")
	f.IntVar(&limit, "limit", 0, "Max results per page")
	f.IntVar(&page, "page", 0, "Page number")
	return cmd
}

func newJobShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show <id>",
		Short: "Show details and progress for a job",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()
			p, _, err := resolveProfile(cmd)
			if err != nil {
				return err
			}
			job, err := cliclient.New(p.URL).GetJob(p.Key, args[0])
			if err != nil {
				return fmt.Errorf("get job failed: %w", err)
			}
			if flagBool(cmd, "json") {
				return cliui.EncodeJSON(out, job)
			}

			derefStr := func(s *string) string {
				if s == nil {
					return "-"
				}
				return *s
			}

			duration := "-"
			if job.DurationSeconds != nil {
				duration = strconv.FormatFloat(*job.DurationSeconds, 'f', 2, 64) + "s"
			}

			errMsg := "none"
			if job.ErrorMessage != nil {
				errMsg = *job.ErrorMessage
			}

			fmt.Fprintf(out, "id:         %s\n", job.ID)
			fmt.Fprintf(out, "type:       %s\n", job.JobType)
			fmt.Fprintf(out, "source:     %s\n", job.Source)
			fmt.Fprintf(out, "status:     %s\n", job.Status)
			fmt.Fprintf(out, "priority:   %s\n", job.Priority)
			fmt.Fprintf(out, "items:      %d\n", job.TotalItems)
			fmt.Fprintf(out, "file:       %s\n", derefStr(job.FilePath))
			fmt.Fprintf(out, "terminal:   %s\n", yesNo(job.IsTerminal))
			fmt.Fprintf(out, "auto-retry: %s\n", yesNo(job.AutoRetryDone))
			fmt.Fprintf(out, "created:    %s\n", job.CreatedAt)
			fmt.Fprintf(out, "started:    %s\n", derefStr(job.StartedAt))
			fmt.Fprintf(out, "completed:  %s\n", derefStr(job.CompletedAt))
			fmt.Fprintf(out, "duration:   %s\n", duration)
			fmt.Fprintf(out, "error:      %s\n", errMsg)
			fmt.Fprintln(out, "\nPROGRESS:")
			pr := job.Progress
			fmt.Fprintf(out, "  pending:        %d\n", pr.Pending)
			fmt.Fprintf(out, "  processing:     %d\n", pr.Processing)
			fmt.Fprintf(out, "  completed:      %d\n", pr.Completed)
			fmt.Fprintf(out, "  pending_review: %d\n", pr.PendingReview)
			fmt.Fprintf(out, "  skipped:        %d\n", pr.Skipped)
			fmt.Fprintf(out, "  failed:         %d\n", pr.Failed)
			fmt.Fprintf(out, "  total:          %d\n", pr.Total)
			fmt.Fprintf(out, "  percent:        %d\n", pr.Percent)
			return nil
		},
	}
}
