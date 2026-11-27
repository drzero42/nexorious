package externalgame

import (
	"context"

	"github.com/uptrace/bun"
)

// DeleteForStorefront deletes every External game of the user on the
// storefront, for a reset of that storefront's sync data. Their platform rows
// go by the foreign key's cascade.
func DeleteForStorefront(ctx context.Context, db bun.IDB, userID, storefront string) error {
	_, err := db.NewRaw(
		`DELETE FROM external_games WHERE user_id = ? AND storefront = ?`, userID, storefront,
	).Exec(ctx)
	return err
}

// DeleteForAllUsers deletes every External game of every user, for the admin
// reset. Their platform rows go by the foreign key's cascade.
func DeleteForAllUsers(ctx context.Context, db bun.IDB) error {
	_, err := db.NewRaw(`DELETE FROM external_games`).Exec(ctx)
	return err
}
