package api

import (
	"log/slog"
	"testing"
)

func TestRequestLogLevel(t *testing.T) {
	cases := []struct {
		name      string
		status    int
		routePath string
		want      slog.Level
	}{
		{"server error", 500, "/api/user-games", slog.LevelError},
		{"server error on asset", 503, "/*", slog.LevelError},
		{"client error", 404, "/api/games/:id", slog.LevelWarn},
		{"client error wins over quiet", 401, "/api/jobs/pending-review-count", slog.LevelWarn},
		{"spa shell asset", 200, "/*", slog.LevelDebug},
		{"static css", 200, "/static/app.css", slog.LevelDebug},
		{"static cover art", 200, "/static/cover_art/*", slog.LevelDebug},
		{"logos prefix", 200, "/logos/storefronts/steam/icon.svg", slog.LevelDebug},
		{"poll pending-review", 200, "/api/jobs/pending-review-count", slog.LevelDebug},
		{"poll job status", 200, "/api/jobs/status/:job_type", slog.LevelDebug},
		{"redirect on shell", 302, "/*", slog.LevelDebug},
		{"meaningful api call", 200, "/api/user-games", slog.LevelInfo},
		{"meaningful api 201", 201, "/api/user-games", slog.LevelInfo},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := requestLogLevel(tc.status, tc.routePath); got != tc.want {
				t.Errorf("requestLogLevel(%d, %q) = %v, want %v", tc.status, tc.routePath, got, tc.want)
			}
		})
	}
}
