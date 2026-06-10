package api

import (
	"log/slog"
	"strings"
)

// quietRequestRoutes are matched route patterns whose successful responses carry
// no operational signal and arrive in bulk — static assets, the SPA shell, and
// background UI polling. Their access-log lines are emitted at Debug.
var quietRequestRoutes = map[string]bool{
	"/*":                             true, // SPA shell + embedded assets (logos, favicon, JS/CSS)
	"/static/app.css":                true,
	"/health":                        true, // liveness/readiness probes hit this constantly
	"/api/jobs/pending-review-count": true, // polled on a timer by the UI
	"/api/jobs/status/:job_type":     true, // polled on a timer by the UI
}

// isQuietRequestRoute reports whether an access-log line for routePath should be
// demoted to Debug on success.
func isQuietRequestRoute(routePath string) bool {
	if quietRequestRoutes[routePath] {
		return true
	}
	return strings.HasPrefix(routePath, "/static/") || strings.HasPrefix(routePath, "/logos/")
}

// requestLogLevel picks the slog level for an access-log line: server errors are
// Error, client errors Warn, successful quiet (asset/poll) routes Debug, and
// everything else (meaningful API traffic) Info.
func requestLogLevel(status int, routePath string) slog.Level {
	switch {
	case status >= 500:
		return slog.LevelError
	case status >= 400:
		return slog.LevelWarn
	case isQuietRequestRoute(routePath):
		return slog.LevelDebug
	default:
		return slog.LevelInfo
	}
}
