package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"github.com/drzero42/nexorious/internal/logging"
)

// stopBackground is a restore's StopBackground hook (#1341). It switches off
// the River start gate and the database probe for good, then stops River
// without a graceful drain, all within the deadline of ctx.
//
// The gate goes first and is waited for: in a restore from the setup zone it is
// still polling, and a Start it is in the middle of must have returned before
// River is stopped, or River could start after the stop. The probe would
// otherwise react to the restore terminating its connections.
func stopBackground(
	ctx context.Context,
	stopGate context.CancelFunc, gateDone <-chan struct{},
	stopProbe func(context.Context) error,
	stopRiver func(context.Context) error,
) error {
	stopGate()
	select {
	case <-gateDone:
	case <-ctx.Done():
		return fmt.Errorf("the River start gate did not stop: %w", ctx.Err())
	}
	if err := stopProbe(ctx); err != nil {
		return err
	}
	if err := stopRiver(ctx); err != nil {
		return fmt.Errorf("stop River: %w", err)
	}
	return nil
}

// restartSelf replaces the process with a fresh copy of itself, after a
// restore: same executable, arguments and environment, same process id, so a
// supervisor sees no exit. It returns the exit code to use when it does not
// exec: 0 when a termination signal has arrived (the new image would not know
// it was asked to stop), 1 when the exec failed, so the supervisor restarts
// the process. exec is syscall.Exec; tests replace it.
func restartSelf(signals <-chan os.Signal, exec func(argv0 string, argv, envv []string) error) int {
	select {
	case sig := <-signals:
		slog.Info("termination signal received — exiting instead of restarting", "signal", sig.String())
		return 0
	default:
	}
	exe, err := os.Executable()
	if err == nil {
		err = exec(exe, os.Args, os.Environ())
	}
	// exec returns only when it failed.
	slog.Error("restarting the server failed — exiting so the supervisor restarts it",
		logging.KeyErr, err, logging.Cat(logging.CategoryConfig))
	return 1
}
