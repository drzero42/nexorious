package main

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/spf13/cobra"
)

func TestRootCmd_Structure(t *testing.T) {
	root := newRootCmd()
	if root.Use != "nexctl" {
		t.Errorf("root.Use = %q, want nexctl", root.Use)
	}
	for _, f := range []string{"profile", "json", "quiet", "yes"} {
		if root.PersistentFlags().Lookup(f) == nil {
			t.Errorf("expected persistent flag --%s", f)
		}
	}
	want := map[string]bool{"version": false, "account": false, "profile": false, "game": false, "tag": false, "pool": false, "sync": false, "job": false, "import": false, "export": false, "backup": false, "admin": false, "config": false, "login": false, "logout": false}
	for _, sub := range root.Commands() {
		if _, ok := want[sub.Name()]; ok {
			want[sub.Name()] = true
		}
	}
	for name, found := range want {
		if !found {
			t.Errorf("expected subcommand %q", name)
		}
	}
}

func TestVersionCmd(t *testing.T) {
	withTestVersion(t)
	// Point config resolution at an empty dir so the command can't read a real
	// developer profile; with no profile the server line is just "unavailable".
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	root := newRootCmd()
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetErr(&buf)
	root.SetArgs([]string{"version"})
	if err := root.Execute(); err != nil {
		t.Fatalf("version: %v", err)
	}
	got := buf.String()
	if !strings.Contains(got, "nexctl ") || !strings.Contains(got, "9.9.9-test") || !strings.Contains(got, "cafef00d") {
		t.Errorf("version output = %q", got)
	}
}

// runRootFor wraps a RunE in a bare root command and runs it through runRoot
// with a short grace, cancelling ctx as sig would once the command has started.
func runRootFor(t *testing.T, sig os.Signal, run func(cmd *cobra.Command) error) (int, string, time.Duration) {
	t.Helper()
	ctx, cancel := context.WithCancelCause(t.Context())
	defer cancel(nil)
	started := make(chan struct{})
	root := &cobra.Command{Use: "nexctl", SilenceUsage: true, SilenceErrors: true,
		RunE: func(cmd *cobra.Command, _ []string) error { close(started); return run(cmd) }}
	root.SetArgs(nil)
	go func() { <-started; cancel(signalCause{sig}) }()
	var stderr bytes.Buffer
	t0 := time.Now()
	code := runRoot(ctx, func() {}, root, &stderr, 200*time.Millisecond)
	return code, stderr.String(), time.Since(t0)
}

// A command that unwinds with a wrapped context.Canceled after Ctrl-C exits
// 130 with a bare "interrupted", not "error: poll job: … context canceled".
func TestRunRootInterruptedCommandReturns(t *testing.T) {
	code, stderr, _ := runRootFor(t, os.Interrupt, func(cmd *cobra.Command) error {
		<-cmd.Context().Done()
		return fmt.Errorf("poll job: %w", cmd.Context().Err())
	})
	if code != 130 {
		t.Errorf("exit code = %d, want 130", code)
	}
	if strings.TrimSpace(stderr) != "interrupted" {
		t.Errorf("stderr = %q, want just \"interrupted\"", stderr)
	}
}

// A command blocked somewhere that never sees ctx (a stdin prompt) must not
// keep the process alive: runRoot gives up after the grace period.
func TestRunRootForceExitsBlockedCommand(t *testing.T) {
	block := make(chan struct{})
	t.Cleanup(func() { close(block) })
	code, stderr, elapsed := runRootFor(t, os.Interrupt, func(*cobra.Command) error { <-block; return nil })
	if code != 130 {
		t.Errorf("exit code = %d, want 130", code)
	}
	if strings.TrimSpace(stderr) != "interrupted" {
		t.Errorf("stderr = %q, want just \"interrupted\"", stderr)
	}
	if elapsed > 5*time.Second {
		t.Errorf("took %s to give up on a blocked command", elapsed)
	}
}

// The exit status follows 128+N for the signal that arrived: SIGTERM (from a
// supervisor or an MCP client stopping the bridge) is 143, not Ctrl-C's 130.
func TestRunRootExitCodeFollowsSignal(t *testing.T) {
	for _, tc := range []struct {
		sig  os.Signal
		want int
	}{{os.Interrupt, 130}, {syscall.SIGTERM, 143}} {
		code, _, _ := runRootFor(t, tc.sig, func(cmd *cobra.Command) error {
			<-cmd.Context().Done()
			return cmd.Context().Err()
		})
		if code != tc.want {
			t.Errorf("%v: exit code = %d, want %d", tc.sig, code, tc.want)
		}
	}
}
