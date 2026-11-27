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
// lock below, the storefront list, the review badge, the ignored list, the
// sync status count, the lookup of a set's open items) is built from these helpers, so they cannot disagree on which games
// form a set.

// keyColumn is a column of the key, with its SQL type.
type keyColumn struct{ name, sqlType string }

// titleColumns make two External games same-title games.
var titleColumns = []keyColumn{{"user_id", "text"}, {"storefront", "text"}, {"title", "text"}}

// decisionColumns hold a game's decision: its match and its skip.
var decisionColumns = []keyColumn{{"resolved_igdb_id", "integer"}, {"is_skipped", "boolean"}}

// editionColumns is the whole key: the title columns, then the decision.
var editionColumns = slices.Concat(titleColumns, decisionColumns)

// SameTitle is the SQL condition that the External games aliased a and b are
// same-title games: same user, storefront and title.
func SameTitle(a, b string) string {
	conds := make([]string, 0, len(titleColumns))
	for _, c := range titleColumns {
		conds = append(conds, fmt.Sprintf("%s.%s = %s.%s", a, c.name, b, c.name))
	}
	return "(" + strings.Join(conds, " AND ") + ")"
}

// SameEdition is the SQL condition that the External games aliased a and b are
// editions of each other: same-title games with the same match and skip.
func SameEdition(a, b string) string {
	conds := []string{SameTitle(a, b)}
	for _, c := range decisionColumns {
		conds = append(conds, fmt.Sprintf("%s.%s IS NOT DISTINCT FROM %s.%s", a, c.name, b, c.name))
	}
	return "(" + strings.Join(conds, " AND ") + ")"
}

// EditionKey is the column list that groups the External games aliased a into
// sets of editions, for GROUP BY and PARTITION BY (both treat NULLs as equal,
// as SameEdition does).
func EditionKey(a string) string {
	cols := make([]string, 0, len(editionColumns))
	for _, c := range editionColumns {
		cols = append(cols, a+"."+c.name)
	}
	return strings.Join(cols, ", ")
}

// Undecided is the SQL condition that the External game aliased a has no
// decision: it is neither matched nor skipped.
func Undecided(a string) string {
	return fmt.Sprintf("(%s.resolved_igdb_id IS NULL AND NOT %s.is_skipped)", a, a)
}

// SameGame is the SQL condition that the External games aliased a and b count
// as the same game in a storefront's change lists (#1443): editions of each
// other, or same-title games one of which has no decision. A game without a
// decision takes the oldest same-title decision as soon as its match worker
// runs, so it is about to be an edition of the set.
func SameGame(a, b string) string {
	return "(" + SameEdition(a, b) + " OR (" + SameTitle(a, b) + " AND (" + Undecided(a) + " OR " + Undecided(b) + ")))"
}

// ErrEditionsChanged: the game's set of editions kept changing while
// LockEditions tried to lock it. Callers answer it with 409 and ask the user to
// try again.
var ErrEditionsChanged = errors.New("the game's editions changed while they were being locked; try again")

// lockEditionsTries bounds how often LockEditions tries again.
const lockEditionsTries = 3

// editionState is what makes a game a member of its set of editions: its
// values of editionColumns, in order, read as text (NULL stays NULL).
type editionState []sql.NullString

// readEditionState reads the state of the user's External game id, without a
// lock.
func readEditionState(ctx context.Context, db bun.IDB, userID, id string) (editionState, error) {
	cols := make([]string, len(editionColumns))
	st := make(editionState, len(editionColumns))
	dest := make([]any, len(editionColumns))
	for i, c := range editionColumns {
		cols[i] = c.name + "::text"
		dest[i] = &st[i]
	}
	err := db.NewRaw(
		`SELECT `+strings.Join(cols, ", ")+` FROM external_games WHERE id = ? AND user_id = ?`, id, userID,
	).Scan(ctx, dest...)
	return st, err
}

// args binds the state to the placeholders of editionsOfStateFrom.
func (s editionState) args() []any {
	args := make([]any, len(s))
	for i, v := range s {
		if v.Valid {
			args[i] = v.String
		}
	}
	return args
}

// editionsOfStateFrom is the FROM and WHERE clauses that select, as eg, the
// External games in the set of editions of a bound editionState.
var editionsOfStateFrom = func() string {
	cols := make([]string, len(editionColumns))
	for i, c := range editionColumns {
		cols[i] = "?::" + c.sqlType + " AS " + c.name
	}
	return `FROM external_games eg, (SELECT ` + strings.Join(cols, ", ") + `) k
		WHERE ` + SameEdition("eg", "k")
}()

// errRelock rolls back a try of LockEditions.
var errRelock = errors.New("editions changed; lock again")

// LockEditions locks the user's External game id and its editions FOR UPDATE,
// in id order, and returns the locked set, on which a decision is made
// (Rematch, Skip, Unskip). A missing game, or another user's, is ErrNotFound;
// a set that kept changing is ErrEditionsChanged.
//
// It reads the game's state without a lock, locks the set by that state, then
// reads the set's ids again in a new statement. The lock holds when the game
// is in the locked set and the second read gives exactly the locked ids:
// otherwise a decision committed in between (the game left the set), or a match
// worker that was taking precedent from a locked game added an edition the
// locking statement could not see. Each try runs in a savepoint and is rolled
// back before the next, so locks are always taken in id order.
func LockEditions(ctx context.Context, tx bun.Tx, userID, id string) (*Editions, error) {
	for range lockEditionsTries {
		st, err := readEditionState(ctx, tx, userID, id)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return nil, ErrNotFound
			}
			return nil, fmt.Errorf("read external game: %w", err)
		}

		var set []models.ExternalGame
		err = tx.RunInTx(ctx, nil, func(ctx context.Context, sp bun.Tx) error {
			set = nil
			if err := sp.NewRaw(`
				SELECT eg.* `+editionsOfStateFrom+`
				ORDER BY eg.id
				FOR UPDATE OF eg`,
				st.args()...,
			).Scan(ctx, &set); err != nil && !errors.Is(err, sql.ErrNoRows) {
				return fmt.Errorf("lock editions: %w", err)
			}
			var again []string
			if err := sp.NewRaw(`
				SELECT eg.id `+editionsOfStateFrom+`
				ORDER BY eg.id`,
				st.args()...,
			).Scan(ctx, &again); err != nil && !errors.Is(err, sql.ErrNoRows) {
				return fmt.Errorf("read editions again: %w", err)
			}
			locked := GameIDs(set)
			if !slices.Contains(locked, id) || !slices.Equal(locked, again) {
				return errRelock
			}
			return nil
		})
		if errors.Is(err, errRelock) {
			continue
		}
		if err != nil {
			return nil, err
		}
		for _, eg := range set {
			if eg.ID == id {
				return &Editions{tx: tx, userID: userID, game: eg, set: set}, nil
			}
		}
	}
	return nil, ErrEditionsChanged
}

// GameIDs returns the ids of the External games games, in order.
func GameIDs(games []models.ExternalGame) []string {
	ids := make([]string, len(games))
	for i := range games {
		ids[i] = games[i].ID
	}
	return ids
}

// precedentFrom selects the candidates for the precedent of the game aliased
// me: the other same-title games that have a decision. The precedent is the
// oldest of them (created_at, then id).
var precedentFrom = `
	FROM external_games s
	JOIN external_games me ON me.id = ?
	WHERE ` + SameTitle("s", "me") + ` AND s.id <> me.id
	  AND NOT ` + Undecided("s")

// HasPrecedent reports, without a lock, whether a same-title game of the
// External game id has a decision. It saves a transaction for the common game
// that has no same-title game at all; TakePrecedent decides under its locks.
func HasPrecedent(ctx context.Context, db bun.IDB, id string) (bool, error) {
	var exists bool
	if err := db.NewRaw(
		`SELECT EXISTS (SELECT 1 `+precedentFrom+`)`, id,
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
// precedent is a skip the game is left as it is and false is returned. The match
// worker uses it to complete same-title reviews, so a game is never left
// skipped with its item in review.
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
			`+precedentFrom+`
			ORDER BY s.created_at, s.id
			LIMIT 1
			FOR SHARE OF s
		) p
		WHERE e.id = ? AND `+Undecided("e")+cond,
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
