package tasks

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/riverqueue/river"
	"github.com/uptrace/bun"

	"github.com/drzero42/nexorious/internal/db/models"
	"github.com/drzero42/nexorious/internal/joblifecycle"
	"github.com/drzero42/nexorious/internal/logging"
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
	// JobID, when set, names the Job an admin created (internal/joblifecycle,
	// with this River job as its dispatch owner); empty means the dispatch
	// runs before any Job exists (after a sync) and creates its Job with all
	// items.
	JobID string `json:"job_id,omitempty"`
}

func (StoreLinkRefreshDispatchArgs) Kind() string { return "store_link_refresh_dispatch" }

func (StoreLinkRefreshDispatchArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{MaxAttempts: 5, Priority: 3}
}

type StoreLinkRefreshDispatchWorker struct {
	river.WorkerDefaults[StoreLinkRefreshDispatchArgs]
	DB *bun.DB
	LC *joblifecycle.Module
}

func (w *StoreLinkRefreshDispatchWorker) Timeout(*river.Job[StoreLinkRefreshDispatchArgs]) time.Duration {
	return dispatchTimeout
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

func storeLinkItems(groups []storeLinkGroup, force bool) []joblifecycle.NewItem {
	items := make([]joblifecycle.NewItem, len(groups))
	for i, g := range groups {
		meta, _ := json.Marshal(storeLinkItemMeta{Storefront: g.Storefront, Force: force}) //nolint:errcheck // fixed struct
		items[i] = joblifecycle.NewItem{UserID: g.UserID, ItemKey: g.Storefront, SourceTitle: g.Storefront, SourceMetadata: meta}
	}
	return items
}

func (w *StoreLinkRefreshDispatchWorker) Work(ctx context.Context, job *river.Job[StoreLinkRefreshDispatchArgs]) error {
	if job.Args.JobID != "" {
		return w.dispatchJob(ctx, job)
	}
	return w.dispatchWithoutJob(ctx, job)
}

// dispatchJob fills a Job an admin created. Its args come from the Job row
// (DispatchArgs of the kind), so a re-run dispatch is the same dispatch.
func (w *StoreLinkRefreshDispatchWorker) dispatchJob(ctx context.Context, rj *river.Job[StoreLinkRefreshDispatchArgs]) error {
	jobID := rj.Args.JobID
	ctx = logging.WithJobID(ctx, jobID)
	if _, err := w.LC.StartDispatch(ctx, jobID, rj.ID); err != nil {
		return exitOrRetry(ctx, err)
	}
	groups, total, err := w.SelectGroups(ctx, rj.Args)
	if err != nil {
		if ctx.Err() == nil && lastAttempt(rj.JobRow) {
			return failMaintDispatch(ctx, w.LC, jobID, rj.JobRow, "store_link_refresh_dispatch", err.Error())
		}
		return exitOrRetry(ctx, err)
	}
	n := len(groups)
	err = w.LC.MarkDispatched(ctx, jobID, rj.ID, joblifecycle.Dispatched{Items: storeLinkItems(groups, rj.Args.Force), TotalItems: &n})
	if err != nil {
		if ctx.Err() == nil && !joblifecycle.IsRefusal(err) && lastAttempt(rj.JobRow) {
			return failMaintDispatch(ctx, w.LC, jobID, rj.JobRow, "store_link_refresh_dispatch", err.Error())
		}
		return exitOrRetry(ctx, err)
	}
	slog.InfoContext(ctx, "store_link_refresh_dispatch: dispatched", "groups", n, "rows", total)
	return nil
}

// dispatchWithoutJob runs before any Job exists (the refresh after a sync):
// it creates the Job with all its items, or nothing when there is nothing to
// do or an equivalent Job is active.
func (w *StoreLinkRefreshDispatchWorker) dispatchWithoutJob(ctx context.Context, rj *river.Job[StoreLinkRefreshDispatchArgs]) error {
	args := rj.Args
	source := models.JobSourceSystem
	if args.Storefront != "" {
		source = args.Storefront
	}
	groups, total, err := w.SelectGroups(ctx, args)
	if err != nil {
		return maintDispatchError(ctx, w.DB, rj.JobRow, "store_link_refresh_dispatch", err)
	}
	if len(groups) == 0 {
		return nil
	}
	ownerID := args.UserID
	if ownerID == "" {
		err := w.DB.NewRaw(`SELECT id FROM users WHERE is_admin = true ORDER BY created_at LIMIT 1`).Scan(ctx, &ownerID)
		if errors.Is(err, sql.ErrNoRows) {
			slog.WarnContext(ctx, "store_link_refresh_dispatch: no admin user, skipping")
			return nil
		}
		if err != nil {
			return maintDispatchError(ctx, w.DB, rj.JobRow, "store_link_refresh_dispatch", fmt.Errorf("query admin user: %w", err))
		}
	}
	c, err := w.LC.CreateWithItems(ctx, joblifecycle.NewJob{
		UserID: ownerID, JobType: models.JobTypeStoreLinkRefresh, Source: source,
		Priority: models.JobPriorityLow, TotalItems: len(groups),
	}, storeLinkItems(groups, args.Force), nil)
	if err != nil {
		return maintDispatchError(ctx, w.DB, rj.JobRow, "store_link_refresh_dispatch", err)
	}
	if !c.Created {
		slog.InfoContext(ctx, "store_link_refresh_dispatch: equivalent job active, skipping", "existing", c.JobID)
		return nil
	}
	slog.InfoContext(ctx, "store_link_refresh_dispatch: job created", logging.KeyJobID, c.JobID, "groups", len(groups), "rows", total)
	return nil
}

// ── Item worker ───────────────────────────────────────────────────────────────

type StoreLinkRefreshItemArgs struct {
	JobItemID string `json:"job_item_id"`
}

func (StoreLinkRefreshItemArgs) Kind() string { return "store_link_refresh_item" }

func (StoreLinkRefreshItemArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{MaxAttempts: 5, Priority: 3}
}

type StoreLinkRefreshItemWorker struct {
	river.WorkerDefaults[StoreLinkRefreshItemArgs]
	DB          *bun.DB
	LC          *joblifecycle.Module
	ResolverFor resolverFactory
}

// Timeout overrides River's 1-minute default: an item resolves a whole
// (user, storefront) group, which for a large rate-limited storefront (PSN
// makes one ~200ms-throttled call per title) easily exceeds a minute. The bound
// is generous rather than disabled (-1) so a hung upstream still hits the
// deadline; the attempt then counts as failed and River retries it, and after
// the last one recovery fails the item.
func (w *StoreLinkRefreshItemWorker) Timeout(*river.Job[StoreLinkRefreshItemArgs]) time.Duration {
	return 30 * time.Minute
}

type storeLinkItemMeta struct {
	Storefront string `json:"storefront"`
	Force      bool   `json:"force"`
}

// Work resolves store_link for one (user, storefront) group. Each link is
// saved as it is resolved (a progress write, outside the transition), so a
// second attempt only has the rows still without one left to do; only the end
// of the group is the item's transition. An interrupted item records nothing
// and is run again.
func (w *StoreLinkRefreshItemWorker) Work(ctx context.Context, job *river.Job[StoreLinkRefreshItemArgs]) error {
	item, done, err := loadOwnedItem(ctx, w.DB, w.LC, job.Args.JobItemID, job.ID)
	if done || err != nil {
		return err
	}
	ctx = logging.WithJobID(ctx, item.JobID)
	fail := func(msg string) error { return failItem(ctx, w.LC, item.ID, job.ID, msg) }

	var meta storeLinkItemMeta
	if err := json.Unmarshal(item.SourceMetadata, &meta); err != nil {
		return fail(fmt.Sprintf("parse source_metadata: %v", err))
	}

	resolver, err := w.ResolverFor(ctx, meta.Storefront, item.UserID)
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return fail(fmt.Sprintf("resolver: %v", err))
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
		if ctx.Err() == nil && lastAttempt(job.JobRow) {
			return fail(fmt.Sprintf("select target rows: %v", err))
		}
		return exitOrRetry(ctx, fmt.Errorf("select target rows: %w", err))
	}

	for _, r := range rows {
		if ctx.Err() != nil {
			slog.InfoContext(ctx, "store_link_refresh: interrupted mid-group, it runs again",
				logging.KeySource, meta.Storefront, logging.KeyJobItemID, item.ID)
			return ctx.Err()
		}
		var sm map[string]string
		if len(r.SourceMetadata) > 0 {
			_ = json.Unmarshal(r.SourceMetadata, &sm) //nolint:errcheck // best-effort; nil map is fine
		}
		link, rerr := resolver.Resolve(ctx, r.ExternalID, sm)
		if rerr != nil {
			slog.WarnContext(ctx, "store_link_refresh: resolve failed", logging.KeySource, meta.Storefront, "external_id", r.ExternalID, logging.KeyErr, rerr, logging.Cat(logging.CategoryExternalAPI))
			continue
		}
		if link == "" {
			continue
		}
		if _, e := w.DB.NewRaw(
			`UPDATE external_games SET store_link = ?, updated_at = now() WHERE id = ?`, link, r.ID,
		).Exec(ctx); e != nil {
			slog.WarnContext(ctx, "store_link_refresh: update store_link", logging.KeyErr, e, "id", r.ID, logging.Cat(logging.CategoryDB))
		}
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}

	return exitOrRetry(ctx, w.LC.FinishItem(ctx, item.ID, job.ID, joblifecycle.ItemResult{Status: models.JobItemStatusCompleted}))
}
