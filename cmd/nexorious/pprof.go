package main

import (
	"errors"
	"log/slog"
	"net/http"
	"net/http/pprof"
	"time"

	"github.com/drzero42/nexorious/internal/logging"
)

// startPprofServer launches a loopback-only net/http/pprof listener in a
// background goroutine. It is gated by PPROF_ENABLED (default off) and must bind
// a loopback address (PPROF_ADDR, default 127.0.0.1:6060) — profiling is never
// exposed publicly; reach it via `kubectl port-forward` + `go tool pprof`.
//
// A dedicated ServeMux (not DefaultServeMux) keeps the pprof handlers off any
// other server. ReadHeaderTimeout is set to satisfy gosec G112; no write timeout
// is set because /debug/pprof/profile streams for its full duration.
func startPprofServer(addr string) {
	mux := http.NewServeMux()
	mux.HandleFunc("/debug/pprof/", pprof.Index)
	mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
	mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	mux.HandleFunc("/debug/pprof/trace", pprof.Trace)

	srv := &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		slog.Info("pprof listener starting", "addr", addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("pprof listener failed", logging.KeyErr, err, logging.Cat(logging.CategoryConfig))
		}
	}()
}
