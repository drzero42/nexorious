package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/drzero42/nexorious/internal/cliauth"
	"github.com/drzero42/nexorious/internal/clicfg"
	"github.com/drzero42/nexorious/internal/cliui"
)

// Injected at build time via -ldflags.
var (
	version = "dev"
	commit  = "unknown"
)

var errNoSubcommand = errors.New("no subcommand provided")

func newRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "nexctl",
		Short: "Nexorious CLI client — manage a remote collection from the terminal",
		Long: "nexctl is a REST client for a Nexorious server. Authenticate with\n" +
			"`nexctl account login`, then manage your collection, pools, tags, sync,\n" +
			"and more. Use --profile to target one of several configured servers.",
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := cmd.Help(); err != nil {
				return err
			}
			return errNoSubcommand
		},
	}

	pf := root.PersistentFlags()
	pf.String("profile", "", "Config profile to use (default: the current profile)")
	pf.Bool("json", false, "Emit machine-readable JSON")
	pf.BoolP("quiet", "q", false, "Emit only bare ids/values for piping")
	pf.BoolP("yes", "y", false, "Skip confirmation prompts")

	root.AddCommand(newVersionCmd())
	root.AddCommand(newAccountCmd())
	root.AddCommand(newProfileCmd())
	root.AddCommand(newGameCmd())
	root.AddCommand(newTagCmd())
	root.AddCommand(newPoolCmd())
	root.AddCommand(newSyncCmd())
	root.AddCommand(newJobCmd())
	root.AddCommand(newImportCmd())
	root.AddCommand(newExportCmd())
	root.AddCommand(newBackupCmd())
	root.AddCommand(newAdminCmd())
	root.AddCommand(newConfigCmd())
	root.AddCommand(newMCPCmd())
	root.AddCommand(newSetupCmd())
	root.AddCommand(newMigrateCmd())
	root.AddCommand(newChangelogCmd())
	root.AddCommand(newDoctorCmd())
	root.AddCommand(newLoginCmd(), newLogoutCmd()) // top-level aliases for `account login`/`logout`

	return root
}

// resolveServerURL resolves the target server URL for unauthenticated
// (setup/migrate) commands: the --url flag if set, else the current profile's
// stored URL, else cliauth.DefaultServerURL. Unlike resolveProfile, no API key
// is required — the setup and migration zones are pre-bootstrap.
func resolveServerURL(cmd *cobra.Command) string {
	if u, _ := cmd.Flags().GetString("url"); u != "" { //nolint:errcheck // absent flag yields ""
		return u
	}
	if cfg, err := clicfg.Load(); err == nil {
		if p, ok := cfg.ProfileNamed(profileName(cmd, cfg)); ok && p.URL != "" {
			return p.URL
		}
	}
	return cliauth.DefaultServerURL
}

// profileName returns the --profile value, defaulting to the current profile.
func profileName(cmd *cobra.Command, cfg *clicfg.Config) string {
	name, _ := cmd.Flags().GetString("profile") //nolint:errcheck // absent flag yields ""
	if name == "" {
		name = cfg.CurrentName()
	}
	return name
}

// resolveProfile loads config and returns the active profile, erroring with a
// login hint when no API key is stored for it.
func resolveProfile(cmd *cobra.Command) (clicfg.Profile, *clicfg.Config, error) {
	cfg, err := clicfg.Load()
	if err != nil {
		return clicfg.Profile{}, nil, err
	}
	name := profileName(cmd, cfg)
	p, ok := cfg.ProfileNamed(name)
	if !ok || p.Key == "" {
		return clicfg.Profile{}, nil, fmt.Errorf("not logged in to profile %q (run `nexctl account login` first)", name)
	}
	return p, cfg, nil
}

// flagBool reads an inherited persistent bool flag.
func flagBool(cmd *cobra.Command, name string) bool {
	v, _ := cmd.Flags().GetBool(name) //nolint:errcheck // absent flag yields false
	return v
}

// exitInterrupted is the exit status for an interrupt whose signal is unknown:
// the SIGINT convention (128 + 2). A known signal exits 128 + its number, so
// SIGTERM gives 143.
const exitInterrupted = 130

// interruptGrace is how long main waits, after a signal, for the command to
// return on its own before force-exiting. Requests and poll waits honour ctx
// and return well within it; reads blocked on stdin (prompts) never do.
const interruptGrace = time.Second

// signalCause is the context.Cause of a run cancelled by a signal. It records
// which signal arrived so the exit status can follow the 128+N convention
// (signal.NotifyContext's cause is unexported and only carries a message).
type signalCause struct{ sig os.Signal }

func (c signalCause) Error() string { return c.sig.String() + " signal received" }

func main() {
	ctx, cancel := context.WithCancelCause(context.Background())
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, os.Interrupt, syscall.SIGTERM)
	go func() { cancel(signalCause{<-sigs}) }()
	stop := func() { signal.Stop(sigs) }

	code := runRoot(ctx, stop, newRootCmd(), os.Stderr, interruptGrace)
	stop() // os.Exit skips deferred calls
	os.Exit(code)
}

// interruptedExitCode maps the signal that cancelled ctx to 128 + its number.
func interruptedExitCode(ctx context.Context) int {
	var sc signalCause
	if errors.As(context.Cause(ctx), &sc) {
		if n, ok := sc.sig.(syscall.Signal); ok {
			return 128 + int(n)
		}
	}
	return exitInterrupted
}

// runRoot executes root under ctx and returns the process exit code. Once ctx
// is cancelled (a signal arrived) it calls stop, so a second signal gets the
// default behaviour and kills the process, then waits up to grace for the
// command to return before giving up on it. An interrupted run reports a bare
// "interrupted" rather than a wrapped "… context canceled" error.
func runRoot(ctx context.Context, stop func(), root *cobra.Command, stderr io.Writer, grace time.Duration) int {
	done := make(chan error, 1)
	go func() { done <- root.ExecuteContext(ctx) }()

	var err error
	select {
	case err = <-done:
	case <-ctx.Done():
		stop()
		select {
		case err = <-done:
		case <-time.After(grace):
			// Still blocked, e.g. in a prompt: don't leave echo off.
			cliui.RestoreTerminal(stderr)
			fmt.Fprintln(stderr, "interrupted")
			return interruptedExitCode(ctx)
		}
	}

	switch {
	case err == nil:
		return 0
	case ctx.Err() != nil || errors.Is(err, context.Canceled):
		// Any failure after a signal is a consequence of it.
		fmt.Fprintln(stderr, "interrupted")
		return interruptedExitCode(ctx)
	case errors.Is(err, errNoSubcommand):
		return 1
	default:
		fmt.Fprintln(stderr, "error:", err)
		return 1
	}
}
