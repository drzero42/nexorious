package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/drzero42/nexorious/internal/cliclient"
)

// The server restarts itself at the end of a restore (#1341). restartGrace is
// how long nexctl waits for the new process once the old one has stopped
// reporting a restore in progress or stopped answering; tests shorten both.
var (
	restartGrace = 2 * time.Minute
	restartPoll  = time.Second
)

// runRestore sends one restore request through do and, when the server is
// restarting or its answer never arrived, waits for the new server before it
// reports. It prints the outcome to out; a failed restore is the returned
// error.
func runRestore(ctx context.Context, out io.Writer, c *cliclient.Client, do func(context.Context) (*cliclient.RestoreResult, error)) error {
	// The id to wait past when the answer never arrives. Best effort: without
	// it the wait settles for a server that is not restoring.
	var oldID string
	hctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	if h, err := c.HealthInfo(hctx); err == nil {
		oldID = h.InstanceID
	}
	cancel()

	res, err := do(ctx)
	if ctx.Err() != nil {
		return err
	}
	if res != nil && res.InstanceID != "" {
		oldID = res.InstanceID
	}
	if err == nil {
		fmt.Fprintln(out, res.Message)
		if res.MigrationError != "" {
			fmt.Fprintf(out, "Warning: %s.\n", res.MigrationError)
		}
	}
	if !mustWaitForRestart(res, err) {
		return err
	}

	fmt.Fprintln(out, "Waiting for the server to restart…")
	h, werr := c.WaitForNewInstance(ctx, oldID, restartGrace, restartPoll)
	if werr != nil {
		werr = fmt.Errorf("%w; check the server logs", werr)
		if err != nil {
			return fmt.Errorf("%w; %w", err, werr)
		}
		return werr
	}
	state := ""
	if h.Status != "ok" {
		state = fmt.Sprintf(" (state: %s)", h.Status)
	}
	fmt.Fprintf(out, "The server is back%s.\n", state)
	if err != nil && res == nil {
		return fmt.Errorf("no answer to the restore request (%w), so its outcome is unknown; the server has restarted, check its logs", err)
	}
	return err
}

// mustWaitForRestart reports whether a restore's outcome means the server is
// restarting, or may be: a body from the restore handler says so itself; with
// no such body, any 5xx (a panic, a proxy that gave up) and a request that got
// no answer at all may have started the restore.
func mustWaitForRestart(res *cliclient.RestoreResult, err error) bool {
	if res != nil && res.Restarting != nil {
		return *res.Restarting
	}
	if err == nil {
		return false
	}
	var he *cliclient.HTTPError
	if errors.As(err, &he) {
		return he.Status >= 500
	}
	return true
}
