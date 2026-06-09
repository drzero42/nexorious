package scheduler

import (
	"context"
	"log/slog"
	"strings"

	"github.com/riverqueue/river"
	"github.com/uptrace/bun"

	"github.com/drzero42/nexorious/internal/notify"
	"github.com/drzero42/nexorious/internal/services/updatecheck"
)

// ── CheckForUpdates ───────────────────────────────────────────────────────────

type CheckForUpdatesArgs struct{}

func (CheckForUpdatesArgs) Kind() string { return "check_for_updates" }

// CheckForUpdatesWorker fetches the latest stable GitHub release, stores it
// in the shared State (read by /api/version), and emits an
// admin.version.available event once per release when the running build is
// behind. Failures are logged and never fail the run.
type CheckForUpdatesWorker struct {
	river.WorkerDefaults[CheckForUpdatesArgs]
	DB             *bun.DB
	State          *updatecheck.State
	Client         *updatecheck.Client
	RunningVersion string
	Enabled        bool
}

func (w *CheckForUpdatesWorker) Work(ctx context.Context, _ *river.Job[CheckForUpdatesArgs]) error {
	if !w.Enabled {
		return nil
	}

	rel, err := w.Client.FetchLatest(ctx)
	if err != nil {
		slog.Warn("update check: fetch latest release failed", "err", err)
		return nil
	}

	latest := strings.TrimPrefix(rel.TagName, "v")
	w.State.Set(latest, rel.HTMLURL)

	if !updatecheck.UpdateAvailable(w.RunningVersion, latest) {
		return nil
	}

	notify.Emit(ctx, w.DB, notify.EmitParams{
		Type:  notify.TypeAdminVersionAvailable,
		Scope: notify.ScopeAdmin,
		Payload: notify.VersionAvailablePayload{
			CurrentVersion:   w.RunningVersion,
			AvailableVersion: latest,
			ReleaseURL:       rel.HTMLURL,
		},
		// The dedup row lives in the events table and is pruned after
		// NOTIFY_EVENTS_RETENTION_DAYS, so "once per release" is bounded by the
		// retention window: an instance still outdated after pruning re-notifies.
		DedupKey: notify.TypeAdminVersionAvailable + ":" + latest,
	})
	return nil
}
