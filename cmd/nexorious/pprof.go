package main

import (
	"errors"
	"log/slog"
	"net"
	"net/http"
	"net/http/pprof"
	"time"

	"github.com/drzero42/nexorious/internal/logging"
)

// startPprofServer launches a net/http/pprof listener in a background goroutine.
// It is gated by PPROF_ENABLED (default off) and is meant to bind a loopback
// address (PPROF_ADDR, default 127.0.0.1:6060); reach it via `kubectl
// port-forward` + `go tool pprof`. The endpoint is unauthenticated and serves
// full heap dumps (process memory holds decrypted storefront credentials), so a
// non-loopback PPROF_ADDR logs a loud warning — but still binds, leaving the
// operator in control.
//
// A dedicated ServeMux (not DefaultServeMux) keeps the pprof handlers off any
// other server. ReadHeaderTimeout is set to satisfy gosec G112; no write timeout
// is set because /debug/pprof/profile streams for its full duration.
func startPprofServer(addr string) {
	if loopback, err := pprofAddrIsLoopback(addr); err != nil {
		slog.Warn("pprof addr could not be parsed; binding as-is", "addr", addr,
			logging.KeyErr, err, logging.Cat(logging.CategoryConfig))
	} else if !loopback {
		slog.Warn("pprof listener is binding a non-loopback address; the endpoint is "+
			"unauthenticated and exposes heap dumps (decrypted credentials) and a "+
			"CPU-profile DoS vector — keep PPROF_ADDR on loopback", "addr", addr,
			logging.Cat(logging.CategoryConfig))
	}

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

// pprofAddrIsLoopback reports whether addr (a host:port bind address) binds only
// loopback interfaces. An empty host ("" or ":6060") binds every interface and
// is reported as non-loopback. A literal IP is checked directly; a hostname is
// resolved and is loopback only if every resolved address is loopback.
func pprofAddrIsLoopback(addr string) (bool, error) {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return false, err
	}
	if host == "" {
		return false, nil
	}
	if ip := net.ParseIP(host); ip != nil {
		return ip.IsLoopback(), nil
	}
	ips, err := net.LookupIP(host)
	if err != nil {
		return false, err
	}
	if len(ips) == 0 {
		return false, nil
	}
	for _, ip := range ips {
		if !ip.IsLoopback() {
			return false, nil
		}
	}
	return true, nil
}
