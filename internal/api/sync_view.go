package api

import (
	"fmt"

	"github.com/drzero42/nexorious/internal/db/models"
	"github.com/drzero42/nexorious/internal/enum"
	"github.com/drzero42/nexorious/internal/externalgame"
)

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

// activeJobStatusList is models.ActiveJobStatuses as an SQL list, for the
// constant fragments below, which cannot bind it (TestActiveJobStatusList).
const activeJobStatusList = `'` + string(models.JobStatusPending) + `', '` + string(models.JobStatusProcessing) + `'`

// syncNeedsReview: the newest item is in Pending review and no write happened
// since it was created, or any item of the game is in Pending review in an
// active Job (an older Job that "retry failed" reopened: it blocks new syncs
// until the game is reviewed).
const syncNeedsReview = `(
	COALESCE(ni.status = '` + string(models.JobItemStatusPendingReview) + `'
	         AND (eg.library_written_at IS NULL OR eg.library_written_at < ni.created_at), false)
	OR EXISTS (
		SELECT 1 FROM job_items r JOIN jobs rj ON rj.id = r.job_id
		WHERE r.external_game_id = eg.id AND r.status = '` + string(models.JobItemStatusPendingReview) + `'
		  AND rj.status IN (` + activeJobStatusList + `)))`

// syncFailed: the newest item failed and no write happened since.
const syncFailed = `COALESCE(ni.status = '` + string(models.JobItemStatusFailed) + `'
	AND (eg.library_written_at IS NULL OR eg.library_written_at < COALESCE(ni.processed_at, ni.created_at)), false)`

// syncRetryable: the game shows as failed and Retry could work: the newest
// item's Job is active or completed. Retry never reopens a failed or
// cancelled Job, so the view offers no retry there.
const syncRetryable = `(` + syncFailed + ` AND NOT ` + syncNeedsReview + `
	AND COALESCE(nj.status IN (` + activeJobStatusList + `, '` + string(models.JobStatusCompleted) + `'), false))`

// syncInFlight: the newest item is pending in an active Job, so a sync worker
// still acts on it. No item is open in an ended Job (the lifecycle
// module cancels them with it).
const syncInFlight = `COALESCE(ni.status IN ('` + string(models.JobItemStatusPending) + `', '` + string(models.JobItemStatusProcessing) + `')
	AND nj.status IN (` + activeJobStatusList + `), false)`

// syncRepresentativeOrder orders the editions of a set (External games aliased
// a) so that the first is the set's representative: the oldest edition still
// in the storefront library, else the oldest. The storefront list, the review
// badge and the ignored list all pick their row by it.
func syncRepresentativeOrder(a string) string {
	return fmt.Sprintf("NOT %[1]s.is_available, %[1]s.created_at, %[1]s.id", a)
}

// syncEditionSets is the WITH clause shared by the list and the badge, so the
// two group and flag the sets of editions the same way. edition_rows has one
// row per External game eg of the user that matches where (SQL with its own
// placeholders), with its view flags, its set's representative rep_id (the
// oldest edition still in the storefront library, else the oldest) and its
// rank_in_set. edition_sets has one row per set of editions
// (externalgame.EditionKey): it needs review, has failed or is in flight when
// any edition does, and is available when any edition is. Callers select
// FROM edition_sets s JOIN edition_rows r ON r.id = s.rep_id.
func syncEditionSets(where string) string {
	return `
	WITH edition_rows AS (
		SELECT eg.id, eg.storefront, eg.external_id, eg.title, eg.resolved_igdb_id,
		       eg.is_skipped, eg.is_available, eg.store_link,
		       COALESCE(eg.ownership_status = '` + string(enum.OwnershipSubscription) + `', false) AS is_subscription,
		       first_value(eg.id) OVER w AS rep_id,
		       row_number() OVER w AS rank_in_set,
		       ni.id AS newest_item_id,
		       ` + syncNeedsReview + ` AS needs_review,
		       ` + syncFailed + ` AS failed,
		       ` + syncRetryable + ` AS retryable,
		       ` + syncInFlight + ` AS in_flight
		FROM external_games eg
		` + syncNewestItemJoin + `
		WHERE ` + where + `
		WINDOW w AS (PARTITION BY ` + externalgame.EditionKey("eg") + `
		             ORDER BY ` + syncRepresentativeOrder("eg") + `)
	), edition_sets AS (
		SELECT rep_id, array_agg(id) AS ids,
		       bool_or(is_available) AS is_available,
		       bool_or(needs_review) AS needs_review,
		       bool_or(failed) AS failed,
		       bool_or(in_flight) AS in_flight,
		       (array_agg(newest_item_id ORDER BY rank_in_set) FILTER (WHERE retryable))[1] AS retry_item_id
		FROM edition_rows GROUP BY rep_id
	)`
}

// syncSetListed: the set (edition_sets aliased s) is listed, as no edition is in flight.
const syncSetListed = `NOT s.in_flight`

// syncSetInReview: the set (edition_sets aliased s) is listed as needing review.
const syncSetInReview = syncSetListed + ` AND s.needs_review`
