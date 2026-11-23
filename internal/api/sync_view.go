package api

// SQL fragments for a storefront's External game view, shared by the list
// (HandleListExternalGames) and the review badge (HandlePendingReviewCount) so
// the two agree (#1338). Each fragment expects the external game aliased eg;
// the newest-item fragments also expect syncNewestItemJoin.
//
// A game's status comes from its newest item only, and only while nothing has
// written the game to the library since (library_written_at): a rematch or a
// later write supersedes an old failed or in-review item without touching it.

// syncNewestItemJoin joins the game's newest item as ni and its Job as nj.
const syncNewestItemJoin = `
	LEFT JOIN LATERAL (
		SELECT ji.id, ji.job_id, ji.status, ji.created_at, ji.processed_at
		FROM job_items ji
		WHERE ji.external_game_id = eg.id
		ORDER BY ji.created_at DESC, ji.id DESC
		LIMIT 1
	) ni ON true
	LEFT JOIN jobs nj ON nj.id = ni.job_id`

// syncNeedsReview: the newest item is in Pending review and no write happened
// since it was created, or any item of the game is in Pending review in an
// active Job (an older Job that "retry failed" reopened: it blocks new syncs
// until the game is reviewed).
const syncNeedsReview = `(
	COALESCE(ni.status = 'pending_review'
	         AND (eg.library_written_at IS NULL OR eg.library_written_at < ni.created_at), false)
	OR EXISTS (
		SELECT 1 FROM job_items r JOIN jobs rj ON rj.id = r.job_id
		WHERE r.external_game_id = eg.id AND r.status = 'pending_review'
		  AND rj.status IN ('pending', 'processing')))`

// syncFailed: the newest item failed and no write happened since.
const syncFailed = `COALESCE(ni.status = 'failed'
	AND (eg.library_written_at IS NULL OR eg.library_written_at < COALESCE(ni.processed_at, ni.created_at)), false)`

// syncInFlight: the newest item is unfinished and being worked, by the same
// test as rematch's 409: its Job is active, or a River job naming it can still
// run. The River part is one uncorrelated subquery (a per-row lookup on args
// cannot use an index, and the list is polled during a sync); IS NOT NULL keeps
// a NULL out of the list, which would make NOT IN drop every row.
const syncInFlight = `COALESCE(ni.status IN ('pending', 'processing') AND (
	nj.status IN ('pending', 'processing')
	OR ni.id IN (SELECT args->>'job_item_id' FROM river_job
	             WHERE ` + liveRiverJobPredicate + ` AND args->>'job_item_id' IS NOT NULL)), false)`
