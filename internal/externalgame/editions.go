package externalgame

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/uptrace/bun"

	"github.com/drzero42/nexorious/internal/db/models"
)

// Editions (#1449, ADR 0008). The editions of an External game are the
// External games of the same user and storefront that have the same title, the
// same match and the same skip. Nothing stores that: two games are editions for
// as long as they agree.
//
// The key is defined here once, as columns, and every query that needs it (the
// lock below, the storefront list, the review badge, the lookup of a set's open
// items) is built from these helpers, so they cannot disagree on which games
// form a set.

// titleColumns make two External games same-title games.
var titleColumns = []string{"user_id", "storefront", "title"}

// decisionColumns hold a game's decision: its match and its skip.
var decisionColumns = []string{"resolved_igdb_id", "is_skipped"}

// SameTitle is the SQL condition that the External games aliased a and b are
// same-title games: same user, storefront and title.
func SameTitle(a, b string) string {
	conds := make([]string, 0, len(titleColumns))
	for _, c := range titleColumns {
		conds = append(conds, fmt.Sprintf("%s.%s = %s.%s", a, c, b, c))
	}
	return "(" + strings.Join(conds, " AND ") + ")"
}

// SameEdition is the SQL condition that the External games aliased a and b are
// editions of each other: same-title games with the same match and skip.
func SameEdition(a, b string) string {
	conds := []string{SameTitle(a, b)}
	for _, c := range decisionColumns {
		conds = append(conds, fmt.Sprintf("%s.%s IS NOT DISTINCT FROM %s.%s", a, c, b, c))
	}
	return "(" + strings.Join(conds, " AND ") + ")"
}

// EditionKey is the column list that groups the External games aliased a into
// sets of editions, for GROUP BY and PARTITION BY (both treat NULLs as equal,
// as SameEdition does).
func EditionKey(a string) string {
	cols := make([]string, 0, len(titleColumns)+len(decisionColumns))
	for _, c := range slices.Concat(titleColumns, decisionColumns) {
		cols = append(cols, a+"."+c)
	}
	return strings.Join(cols, ", ")
}

// ErrEditionsChanged: the game's set of editions kept changing while
// LockEditions tried to lock it. Callers answer it with 409 and ask the user to
// try again.
var ErrEditionsChanged = errors.New("the game's editions changed while they were being locked; try again")

// lockEditionsTries bounds how often LockEditions tries again.
const lockEditionsTries = 3

// editionState is what makes a game a member of its set of editions.
type editionState struct {
	UserID         string `bun:"user_id"`
	Storefront     string `bun:"storefront"`
	Title          string `bun:"title"`
	ResolvedIGDBID *int32 `bun:"resolved_igdb_id"`
	IsSkipped      bool   `bun:"is_skipped"`
}

// stateRow binds a state as a one-row relation aliased k, for SameEdition.
const stateRow = `(SELECT ?::text AS user_id, ?::text AS storefront, ?::text AS title,
	?::integer AS resolved_igdb_id, ?::boolean AS is_skipped) k`

func (s editionState) args() []any {
	return []any{s.UserID, s.Storefront, s.Title, s.ResolvedIGDBID, s.IsSkipped}
}

// errRelock rolls back a try of LockEditions.
var errRelock = errors.New("editions changed; lock again")

// LockEditions locks the user's External game id and its editions FOR UPDATE,
// in id order, and returns the game and the whole set (the game included,
// ordered by id). A missing game, or another user's, is ErrNotFound; a set
// that kept changing is ErrEditionsChanged.
//
// It reads the game's state without a lock, locks the set by that state, then
// reads the set's ids again in a new statement. The lock holds when the game
// is in the locked set and the second read gives exactly the locked ids:
// otherwise a decision committed in between (the game left the set), or a match
// worker that was taking precedent from a locked game added an edition the
// locking statement could not see. Each try runs in a savepoint and is rolled
// back before the next, so locks are always taken in id order.
func LockEditions(ctx context.Context, tx bun.Tx, userID, id string) (models.ExternalGame, []models.ExternalGame, error) {
	for range lockEditionsTries {
		var st editionState
		if err := tx.NewRaw(
			`SELECT user_id, storefront, title, resolved_igdb_id, is_skipped FROM external_games WHERE id = ? AND user_id = ?`,
			id, userID,
		).Scan(ctx, &st); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return models.ExternalGame{}, nil, ErrNotFound
			}
			return models.ExternalGame{}, nil, fmt.Errorf("read external game: %w", err)
		}

		var set []models.ExternalGame
		err := tx.RunInTx(ctx, nil, func(ctx context.Context, sp bun.Tx) error {
			set = nil
			if err := sp.NewRaw(`
				SELECT eg.* FROM external_games eg, `+stateRow+`
				WHERE `+SameEdition("eg", "k")+`
				ORDER BY eg.id
				FOR UPDATE OF eg`,
				st.args()...,
			).Scan(ctx, &set); err != nil && !errors.Is(err, sql.ErrNoRows) {
				return fmt.Errorf("lock editions: %w", err)
			}
			var again []string
			if err := sp.NewRaw(`
				SELECT eg.id FROM external_games eg, `+stateRow+`
				WHERE `+SameEdition("eg", "k")+`
				ORDER BY eg.id`,
				st.args()...,
			).Scan(ctx, &again); err != nil && !errors.Is(err, sql.ErrNoRows) {
				return fmt.Errorf("read editions again: %w", err)
			}
			locked := make([]string, len(set))
			for i := range set {
				locked[i] = set[i].ID
			}
			if !slices.Contains(locked, id) || !slices.Equal(locked, again) {
				return errRelock
			}
			return nil
		})
		if errors.Is(err, errRelock) {
			continue
		}
		if err != nil {
			return models.ExternalGame{}, nil, err
		}
		for _, eg := range set {
			if eg.ID == id {
				return eg, set, nil
			}
		}
	}
	return models.ExternalGame{}, nil, ErrEditionsChanged
}

// IDs returns the ids of games, in order.
func IDs(games []models.ExternalGame) []string {
	ids := make([]string, len(games))
	for i := range games {
		ids[i] = games[i].ID
	}
	return ids
}

// precedentFrom selects the precedent of the game aliased me: the oldest other
// same-title game that has a decision (created_at, then id).
const precedentFrom = `
	FROM external_games s
	JOIN external_games me ON me.id = ?
	WHERE %s AND s.id <> me.id
	  AND (s.resolved_igdb_id IS NOT NULL OR s.is_skipped)`

// HasPrecedent reports, without a lock, whether a same-title game of the
// External game id has a decision. It saves a transaction for the common game
// that has no same-title game at all; TakePrecedent decides under its locks.
func HasPrecedent(ctx context.Context, db bun.IDB, id string) (bool, error) {
	var exists bool
	if err := db.NewRaw(
		`SELECT EXISTS (SELECT 1 `+fmt.Sprintf(precedentFrom, SameTitle("s", "me"))+`)`, id,
	).Scan(ctx, &exists); err != nil {
		return false, fmt.Errorf("check precedent: %w", err)
	}
	return exists, nil
}

// TakePrecedent gives the External game id, when it has no decision (unmatched
// and not skipped), the match and the skip of its precedent, the oldest
// same-title game that has one, and reports whether it did. The precedent is
// read FOR SHARE, so a Rematch, Skip or Unskip in progress on it is waited for.
func TakePrecedent(ctx context.Context, db bun.IDB, id string) (bool, error) {
	return takePrecedent(ctx, db, id, false)
}

// TakeMatchPrecedent is TakePrecedent that only takes a match: when the
// precedent is a skip the game is left as it is and false is returned. Settle
// uses it, so a game is never left skipped with its item in review.
func TakeMatchPrecedent(ctx context.Context, db bun.IDB, id string) (bool, error) {
	return takePrecedent(ctx, db, id, true)
}

func takePrecedent(ctx context.Context, db bun.IDB, id string, matchOnly bool) (bool, error) {
	cond := ""
	if matchOnly {
		cond = ` AND NOT p.is_skipped`
	}
	res, err := db.NewRaw(`
		UPDATE external_games e
		SET resolved_igdb_id = p.resolved_igdb_id, is_skipped = p.is_skipped, updated_at = now()
		FROM (
			SELECT s.resolved_igdb_id, s.is_skipped
			`+fmt.Sprintf(precedentFrom, SameTitle("s", "me"))+`
			ORDER BY s.created_at, s.id
			LIMIT 1
			FOR SHARE OF s
		) p
		WHERE e.id = ? AND e.resolved_igdb_id IS NULL AND NOT e.is_skipped`+cond,
		id, id,
	).Exec(ctx)
	if err != nil {
		return false, fmt.Errorf("take precedent: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("take precedent: rows affected: %w", err)
	}
	return n > 0, nil
}
