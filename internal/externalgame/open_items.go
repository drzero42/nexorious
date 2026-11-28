package externalgame

import (
	"context"
	"fmt"

	"github.com/uptrace/bun"

	"github.com/drzero42/nexorious/internal/db/models"
)

// The open items of a set of editions in sync Jobs. These are reads only: the
// items' status belongs to internal/joblifecycle, and callers hand the ids to
// it. Each lookup is built from the edition helpers, so it finds the same set
// the lock does.

// EditionsOpenItemJobs returns the active Jobs in which the user's External
// game id or one of its editions has an open item (pending or in Pending
// review): the Job holding the game's own item first, then the newest. It is
// a plain read, without locks.
func EditionsOpenItemJobs(ctx context.Context, db bun.IDB, userID, id string) ([]string, error) {
	return openItemJobs(ctx, db, userID, id, SameEdition("eg", "g"))
}

// OwnOpenItemJobs returns the active Jobs in which the user's External game id
// itself has an open item (pending or in Pending review), newest first: the
// Jobs a decision on that edition alone touches (#1452). It is a plain read,
// without locks.
func OwnOpenItemJobs(ctx context.Context, db bun.IDB, userID, id string) ([]string, error) {
	return openItemJobs(ctx, db, userID, id, "eg.id = g.id")
}

// openItemJobs returns the active Jobs with an open item of an External game
// eg that meets cond against the user's game g, the Job holding g's own item
// first, then the newest.
func openItemJobs(ctx context.Context, db bun.IDB, userID, id, cond string) ([]string, error) {
	var jobIDs []string
	err := db.NewRaw(`
		SELECT ji.job_id FROM job_items ji
		JOIN jobs j ON j.id = ji.job_id
		JOIN external_games eg ON eg.id = ji.external_game_id
		JOIN external_games g ON g.id = ? AND g.user_id = ?
		WHERE `+cond+`
		  AND ji.status IN (?)
		  AND j.status IN (?)
		GROUP BY ji.job_id
		ORDER BY bool_or(eg.id = g.id) DESC, max(j.created_at) DESC`,
		id, userID,
		bun.List(models.OpenJobItemStatuses()),
		bun.List(models.ActiveJobStatuses()),
	).Scan(ctx, &jobIDs)
	if err != nil {
		return nil, fmt.Errorf("find the Jobs with open items: %w", err)
	}
	return jobIDs, nil
}

// OpenItem is an open item of a set of editions in one Job.
type OpenItem struct {
	ID             string               `bun:"id"`
	ExternalGameID string               `bun:"external_game_id"`
	Status         models.JobItemStatus `bun:"status"`
}

// EditionsOpenItems returns the open items (pending or in Pending review) of
// the External games editions in the Job jobID, in id order.
func EditionsOpenItems(ctx context.Context, db bun.IDB, jobID string, editions []string) ([]OpenItem, error) {
	var items []OpenItem
	if err := db.NewRaw(`
		SELECT id, external_game_id, status FROM job_items
		WHERE job_id = ? AND external_game_id IN (?) AND status IN (?)
		ORDER BY id`,
		jobID, bun.List(editions), bun.List(models.OpenJobItemStatuses()),
	).Scan(ctx, &items); err != nil {
		return nil, fmt.Errorf("load the editions' open items: %w", err)
	}
	return items, nil
}

// InReview is a same-title game of an External game, with its item in Pending
// review in one Job.
type InReview struct {
	GameID string `bun:"game_id"`
	ItemID string `bun:"item_id"`
}

// UndecidedSameTitleInReview returns the same-title games of the External game
// id that have no decision and whose item in the Job jobID is in Pending
// review, in game id order: the games a confident match of id lets take that
// match (the settle rule of ADR 0008).
func UndecidedSameTitleInReview(ctx context.Context, db bun.IDB, jobID, id string) ([]InReview, error) {
	var games []InReview
	if err := db.NewRaw(`
		SELECT o.id AS game_id, ji.id AS item_id
		FROM external_games o
		JOIN external_games me ON me.id = ?
		JOIN job_items ji ON ji.external_game_id = o.id AND ji.job_id = ? AND ji.status = ?
		WHERE `+SameTitle("o", "me")+` AND o.id <> me.id AND `+Undecided("o")+`
		ORDER BY o.id, ji.id`,
		id, jobID, models.JobItemStatusPendingReview,
	).Scan(ctx, &games); err != nil {
		return nil, fmt.Errorf("find undecided same-title games in review: %w", err)
	}
	return games, nil
}
