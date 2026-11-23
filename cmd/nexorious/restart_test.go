package main

import (
	"context"
	"errors"
	"os"
	"slices"
	"syscall"
	"testing"
	"time"
)

func TestRestartSelf(t *testing.T) {
	t.Run("execs the same binary with the same arguments", func(t *testing.T) {
		var gotArgv0 string
		var gotArgv []string
		code := restartSelf(make(chan os.Signal, 1), func(argv0 string, argv, _ []string) error {
			gotArgv0, gotArgv = argv0, argv
			return errors.New("exec returned") // a real exec does not return
		})
		exe, err := os.Executable()
		if err != nil {
			t.Fatal(err)
		}
		if gotArgv0 != exe || !slices.Equal(gotArgv, os.Args) {
			t.Errorf("exec(%q, %v), want (%q, %v)", gotArgv0, gotArgv, exe, os.Args)
		}
		if code != 1 {
			t.Errorf("exit code after a failed exec = %d, want 1", code)
		}
	})

	t.Run("a termination signal wins over the restart", func(t *testing.T) {
		signals := make(chan os.Signal, 1)
		signals <- syscall.SIGTERM
		execed := false
		code := restartSelf(signals, func(string, []string, []string) error {
			execed = true
			return nil
		})
		if execed {
			t.Error("exec ran after a termination signal")
		}
		if code != 0 {
			t.Errorf("exit code = %d, want 0", code)
		}
	})
}

func TestStopBackground(t *testing.T) {
	t.Run("stops the gate, then the probe, then River", func(t *testing.T) {
		var order []string
		gateDone := make(chan struct{})
		stopGate := func() { order = append(order, "gate"); close(gateDone) }
		err := stopBackground(context.Background(), stopGate, gateDone,
			func(context.Context) error { order = append(order, "probe"); return nil },
			func(context.Context) error { order = append(order, "river"); return nil })
		if err != nil {
			t.Fatal(err)
		}
		if !slices.Equal(order, []string{"gate", "probe", "river"}) {
			t.Errorf("order = %v", order)
		}
	})

	t.Run("a gate that does not stop in time fails before River is touched", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
		defer cancel()
		riverStopped := false
		err := stopBackground(ctx, func() {}, make(chan struct{}),
			func(context.Context) error { return nil },
			func(context.Context) error { riverStopped = true; return nil })
		if err == nil || riverStopped {
			t.Errorf("err = %v, riverStopped = %v; want an error and River untouched", err, riverStopped)
		}
	})

	t.Run("River that does not stop is an error", func(t *testing.T) {
		gateDone := make(chan struct{})
		close(gateDone)
		err := stopBackground(context.Background(), func() {}, gateDone,
			func(context.Context) error { return nil },
			func(context.Context) error { return context.DeadlineExceeded })
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Errorf("err = %v, want DeadlineExceeded", err)
		}
	})
}
