package logging

import (
	"log/slog"
	"net/http"
	"time"
)

// roundTripper wraps a base http.RoundTripper and logs one line per call with
// host, endpoint (path only — query stripped), status, and duration_ms. It
// never mutates the response or error it returns.
type roundTripper struct {
	base http.RoundTripper
}

// NewRoundTripper wraps base with request/duration logging. If base is nil,
// http.DefaultTransport is used.
func NewRoundTripper(base http.RoundTripper) http.RoundTripper {
	if base == nil {
		base = http.DefaultTransport
	}
	return &roundTripper{base: base}
}

func (rt *roundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	start := time.Now()
	resp, err := rt.base.RoundTrip(req)
	dur := time.Since(start).Milliseconds()

	attrs := []any{
		KeyHost, req.URL.Host,
		KeyEndpoint, req.URL.Path, // query intentionally omitted
		KeyDurationMS, dur,
	}
	ctx := req.Context()
	if err != nil {
		attrs = append(attrs, KeyStatus, 0, KeyErr, err.Error(), KeyCategory, string(CategoryExternalAPI))
		slog.WarnContext(ctx, "external api call failed", attrs...)
		return resp, err
	}
	attrs = append(attrs, KeyStatus, resp.StatusCode)
	slog.DebugContext(ctx, "external api call", attrs...)
	return resp, nil
}
