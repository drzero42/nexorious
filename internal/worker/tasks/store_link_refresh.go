package tasks

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
	"github.com/uptrace/bun"

	"github.com/drzero42/nexorious/internal/db/models"
	"github.com/drzero42/nexorious/internal/notify"
	"github.com/drzero42/nexorious/internal/services/storelink"
)

// resolvableStorefronts are the slugs the enrichment worker can resolve.
var resolvableStorefronts = []string{"steam", "gog", "epic-games-store", "playstation-store"}

// resolverFactory builds a storelink.Resolver for a (storefront, user). The real
// implementation (with creds) is wired in cmd/nexorious/serve.go.
type resolverFactory func(ctx context.Context, storefront, userID string) (storelink.Resolver, error)

// ── Dispatch worker ──────────────────────────────────────────────────────────

// StoreLinkRefreshDispatchArgs drives a store-link enrichment pass. When UserID
// and Storefront are set the pass is scoped to that one group (sync-triggered);
// when empty it covers all resolvable groups (admin). Force=false resolves only
// rows with a null store_link; Force=true re-resolves all rows.
type StoreLinkRefreshDispatchArgs struct {
	UserID     string `json:"user_id,omitempty"`
	Storefront string `json:"storefront,omitempty"`
	Force      bool   `json:"force,omitempty"`
}

func (StoreLinkRefreshDispatchArgs) Kind() string { return "store_link_refresh_dispatch" }

func (StoreLinkRefreshDispatchArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{MaxAttempts: 1, Priority: 3}
}

type StoreLinkRefreshDispatchWorker struct {
	river.WorkerDefaults[StoreLinkRefreshDispatchArgs]
	DB          *bun.DB
	RiverClient *river.Client[pgx.Tx]
}

type storeLinkGroup struct {
	UserID     string `bun:"user_id"`
	Storefront string `bun:"storefront"`
}

// SelectGroups returns the distinct (user, storefront) groups to enrich and the
// total number of target rows across them. Exported for testing.
func (w *StoreLinkRefreshDispatchWorker) SelectGroups(ctx context.Context, args StoreLinkRefreshDispatchArgs) ([]storeLinkGroup, int, error) {
	var groups []storeLinkGroup
	if err := w.DB.NewRaw(`
		SELECT DISTINCT user_id, storefront
		FROM external_games
		WHERE storefront IN (?)
		  AND is_available = true
		  AND (?::bool OR store_link IS NULL)
		  AND (? = '' OR user_id = ?)
		  AND (? = '' OR storefront = ?)
		ORDER BY user_id, storefront`,
		bun.List(resolvableStorefronts),
		args.Force,
		args.UserID, args.UserID,
		args.Storefront, args.Storefront,
	).Scan(ctx, &groups); err != nil {
		return nil, 0, fmt.Errorf("select groups: %w", err)
	}
	var total int
	if err := w.DB.NewRaw(`
		SELECT count(*)
		FROM external_games
		WHERE storefront IN (?)
		  AND is_available = true
		  AND (?::bool OR store_link IS NULL)
		  AND (? = '' OR user_id = ?)
		  AND (? = '' OR storefront = ?)`,
		bun.List(resolvableStorefronts),
		args.Force,
		args.UserID, args.UserID,
		args.Storefront, args.Storefront,
	).Scan(ctx, &total); err != nil {
		return nil, 0, fmt.Errorf("count rows: %w", err)
	}
	return groups, total, nil
}

func (w *StoreLinkRefreshDispatchWorker) Work(ctx context.Context, job *river.Job[StoreLinkRefreshDispatchArgs]) error {
	args := job.Args

	// Self-heal any predecessor wedged by an orphaned item before evaluating the
	// active-job guard, so a stuck job can never block refreshes permanently.
	reapStuckStoreLinkJobs(ctx, w.DB)

	source := models.JobSourceSystem
	if args.Storefront != "" {
		source = args.Storefront
	}
	var existing string
	guard := `SELECT id FROM jobs WHERE job_type = ? AND status IN ('pending','processing') AND source = ?`
	guardArgs := []any{models.JobTypeStoreLinkRefresh, source}
	if args.UserID != "" {
		guard += ` AND user_id = ?`
		guardArgs = append(guardArgs, args.UserID)
	}
	guard += ` LIMIT 1`
	err := w.DB.NewRaw(guard, guardArgs...).Scan(ctx, &existing)
	if err == nil {
		slog.Info("store_link_refresh_dispatch: equivalent job active, skipping", "existing", existing)
		return nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		slog.Error("store_link_refresh_dispatch: guard query", "err", err)
		return nil
	}

	groups, total, err := w.SelectGroups(ctx, args)
	if err != nil {
		slog.Error("store_link_refresh_dispatch: select groups", "err", err)
		return nil
	}
	if len(groups) == 0 {
		return nil
	}

	jobUserID := args.UserID
	if jobUserID == "" {
		if e := w.DB.NewRaw(`SELECT id FROM users WHERE is_admin = true LIMIT 1`).Scan(ctx, &jobUserID); e != nil {
			slog.Error("store_link_refresh_dispatch: no admin user", "err", e)
			return nil
		}
	}

	jobID := uuid.NewString()
	itemIDs := make([]string, 0, len(groups))
	if err := w.DB.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		if _, e := tx.NewRaw(
			`INSERT INTO jobs (id, user_id, job_type, source, status, priority, total_items, created_at)
			 VALUES (?, ?, ?, ?, 'processing', 'low', ?, now())`,
			jobID, jobUserID, models.JobTypeStoreLinkRefresh, source, len(groups),
		).Exec(ctx); e != nil {
			return fmt.Errorf("insert job: %w", e)
		}
		for _, g := range groups {
			itemID := uuid.NewString()
			itemIDs = append(itemIDs, itemID)
			meta, _ := json.Marshal(map[string]any{"storefront": g.Storefront, "force": args.Force}) //nolint:errcheck // fixed map
			if _, e := tx.NewRaw(
				`INSERT INTO job_items (id, job_id, user_id, item_key, source_title, source_metadata, status, result, igdb_candidates, created_at)
				 VALUES (?, ?, ?, ?, ?, ?, 'pending', '{}', '[]', now())`,
				itemID, jobID, g.UserID, g.Storefront, g.Storefront, json.RawMessage(meta),
			).Exec(ctx); e != nil {
				return fmt.Errorf("insert job_item: %w", e)
			}
		}
		return nil
	}); err != nil {
		slog.Error("store_link_refresh_dispatch: tx failed", "err", err)
		notify.Emit(ctx, w.DB, notify.EmitParams{
			Type: notify.TypeAdminMaintFailed, Scope: notify.ScopeAdmin,
			Payload: notify.MaintPayload{Action: "store_link_refresh_dispatch", Error: err.Error()},
		})
		return nil
	}

	for _, itemID := range itemIDs {
		if e := EnqueueOrFail(ctx, w.DB, w.RiverClient, itemID, StoreLinkRefreshItemArgs{JobItemID: itemID}); e != nil {
			slog.Error("store_link_refresh_dispatch: enqueue item", "err", e, "item_id", itemID)
		}
	}
	slog.Info("store_link_refresh_dispatch: job created", "job_id", jobID, "groups", len(groups), "rows", total)
	return nil
}

// ── Item worker ───────────────────────────────────────────────────────────────

type StoreLinkRefreshItemArgs struct {
	JobItemID string `json:"job_item_id"`
}

func (StoreLinkRefreshItemArgs) Kind() string { return "store_link_refresh_item" }

func (StoreLinkRefreshItemArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{MaxAttempts: 3, Priority: 3}
}

type StoreLinkRefreshItemWorker struct {
	river.WorkerDefaults[StoreLinkRefreshItemArgs]
	DB          *bun.DB
	ResolverFor resolverFactory
}

// Timeout overrides River's 1-minute default: an item resolves a whole
// (user, storefront) group, which for a large rate-limited storefront (PSN
// makes one ~200ms-throttled call per title) easily exceeds a minute. The bound
// is generous rather than disabled (-1) so a hung upstream still hits the
// deadline; the work loop then sees the cancelled context, the item is finalized
// failed, and the next incremental refresh re-resolves its still-null rows.
func (w *StoreLinkRefreshItemWorker) Timeout(*river.Job[StoreLinkRefreshItemArgs]) time.Duration {
	return 30 * time.Minute
}

func (w *StoreLinkRefreshItemWorker) Work(ctx context.Context, job *river.Job[StoreLinkRefreshItemArgs]) error {
	if err := w.ProcessItem(ctx, job.Args.JobItemID); err != nil {
		// Only the pre-load failure path returns an error; surface it so River
		// retries (the item was never loaded, so nothing could be marked). Every
		// other path finalizes the item itself.
		slog.Error("store_link_refresh_item: process", "err", err, "item_id", job.Args.JobItemID)
		return err
	}
	return nil
}

type storeLinkItemMeta struct {
	Storefront string `json:"storefront"`
	Force      bool   `json:"force"`
}

// ProcessItem resolves store_link for one (user, storefront) group. It guarantees
// the job_item always reaches a terminal state: a deferred finalizer marks it
// failed if no explicit terminal mark was made (e.g. shutdown cut the work loop
// short), so an item can never be orphaned in 'pending' (which would wedge the
// parent job and the dispatch guard). All bookkeeping writes use a
// cancellation-free context so a cancelled job context cannot orphan the item.
// Returns a non-nil error only when the job_item itself could not be loaded.
func (w *StoreLinkRefreshItemWorker) ProcessItem(ctx context.Context, jobItemID string) error {
	var item models.JobItem
	if err := w.DB.NewSelect().Model(&item).Where("id = ?", jobItemID).Scan(ctx); err != nil {
		return fmt.Errorf("load job_item: %w", err)
	}

	bookCtx := context.WithoutCancel(ctx)
	finalized := false
	defer func() {
		if !finalized {
			markItemFailed(bookCtx, w.DB, &item, "interrupted before completion", "store_link_refresh: markItemFailed")
		}
		storeLinkCheckJobCompletion(bookCtx, w.DB, item.JobID)
	}()

	var meta storeLinkItemMeta
	if err := json.Unmarshal(item.SourceMetadata, &meta); err != nil {
		markItemFailed(bookCtx, w.DB, &item, fmt.Sprintf("parse source_metadata: %v", err), "store_link_refresh: markItemFailed")
		finalized = true
		return nil
	}

	resolver, err := w.ResolverFor(ctx, meta.Storefront, item.UserID)
	if err != nil {
		markItemFailed(bookCtx, w.DB, &item, fmt.Sprintf("resolver: %v", err), "store_link_refresh: markItemFailed")
		finalized = true
		return nil
	}

	var rows []struct {
		ID             string          `bun:"id"`
		ExternalID     string          `bun:"external_id"`
		SourceMetadata json.RawMessage `bun:"source_metadata"`
	}
	q := `SELECT id, external_id, source_metadata FROM external_games
	      WHERE user_id = ? AND storefront = ? AND is_available = true`
	if !meta.Force {
		q += ` AND store_link IS NULL`
	}
	if err := w.DB.NewRaw(q, item.UserID, meta.Storefront).Scan(ctx, &rows); err != nil {
		markItemFailed(bookCtx, w.DB, &item, fmt.Sprintf("select target rows: %v", err), "store_link_refresh: markItemFailed")
		finalized = true
		return nil
	}

	for _, r := range rows {
		if ctx.Err() != nil {
			// Shutdown mid-loop: stop now. The deferred finalizer marks the item
			// failed; the next incremental refresh re-resolves the rows still null.
			slog.Info("store_link_refresh: interrupted mid-group, remaining rows resolve on next refresh",
				"storefront", meta.Storefront, "item_id", jobItemID)
			return nil
		}
		var sm map[string]string
		if len(r.SourceMetadata) > 0 {
			_ = json.Unmarshal(r.SourceMetadata, &sm) //nolint:errcheck // best-effort; nil map is fine
		}
		link, rerr := resolver.Resolve(ctx, r.ExternalID, sm)
		if rerr != nil {
			slog.Warn("store_link_refresh: resolve failed", "storefront", meta.Storefront, "external_id", r.ExternalID, "err", rerr)
			continue
		}
		if link == "" {
			continue
		}
		if _, e := w.DB.NewRaw(
			`UPDATE external_games SET store_link = ?, updated_at = now() WHERE id = ?`, link, r.ID,
		).Exec(bookCtx); e != nil {
			slog.Error("store_link_refresh: update store_link", "err", e, "id", r.ID)
		}
	}

	markItemCompleted(bookCtx, w.DB, &item, "store_link_refresh: markItemCompleted")
	finalized = true
	return nil
}

func storeLinkCheckJobCompletion(ctx context.Context, db *bun.DB, jobID string) {
	remaining, ok := countJobItems(ctx, db, jobID, "status NOT IN ('completed','failed','skipped')", "store_link_refresh: check job completion")
	if !ok || remaining > 0 {
		return
	}
	finalizeJobCompleted(ctx, db, jobID, "store_link_refresh: finalize", false)
}

// storeLinkReapMinAge keeps the reaper from failing a just-dispatched item in
// the brief window between its job_items insert and its River enqueue.
const storeLinkReapMinAge = 2 * time.Minute

// reapStuckStoreLinkJobs finalizes store_link_refresh jobs wedged by an orphaned
// item — one left in pending/processing with no non-terminal River job backing
// it (e.g. a worker killed mid-run before the finalize fix landed). It marks such
// items failed and runs the completion check so the parent job finalizes and
// stops blocking the dispatch guard. Self-healing on every dispatch (sync
// completion or admin trigger), so no manual intervention is needed on any
// instance. The next incremental refresh re-resolves the failed items' null rows.
func reapStuckStoreLinkJobs(ctx context.Context, db *bun.DB) {
	var stuck []struct {
		ItemID string `bun:"id"`
		JobID  string `bun:"job_id"`
	}
	if err := db.NewRaw(`
		SELECT ji.id, ji.job_id
		FROM job_items ji
		JOIN jobs j ON j.id = ji.job_id
		WHERE j.job_type = ?
		  AND j.status IN ('pending','processing')
		  AND ji.status IN ('pending','processing')
		  AND ji.created_at < now() - (?::text || ' seconds')::interval
		  AND NOT EXISTS (
		    SELECT 1 FROM river_job rj
		    WHERE rj.args->>'job_item_id' = ji.id
		      AND rj.state NOT IN ('completed','discarded','cancelled')
		  )`,
		models.JobTypeStoreLinkRefresh, int64(storeLinkReapMinAge.Seconds()),
	).Scan(ctx, &stuck); err != nil {
		slog.Error("store_link_refresh_dispatch: reap query", "err", err)
		return
	}
	if len(stuck) == 0 {
		return
	}
	jobIDs := make(map[string]struct{})
	for _, s := range stuck {
		var item models.JobItem
		if err := db.NewSelect().Model(&item).Where("id = ?", s.ItemID).Scan(ctx); err != nil {
			slog.Error("store_link_refresh_dispatch: reap load item", "err", err, "item_id", s.ItemID)
			continue
		}
		markItemFailed(ctx, db, &item, "orphaned: no active worker (reaped)", "store_link_refresh: reap")
		jobIDs[s.JobID] = struct{}{}
	}
	for jobID := range jobIDs {
		storeLinkCheckJobCompletion(ctx, db, jobID)
	}
	slog.Info("store_link_refresh_dispatch: reaped orphaned items", "items", len(stuck), "jobs", len(jobIDs))
}
