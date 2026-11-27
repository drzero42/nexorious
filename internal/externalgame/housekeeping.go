package externalgame

import (
	"context"
	"fmt"

	"github.com/uptrace/bun"
)

// SetStoreLink writes the store link of the External game id: the identifier
// a storefront's product URL is built from. Only the store-link refresh
// writes it; a sync never does, so a re-sync cannot clear a resolved link.
func SetStoreLink(ctx context.Context, db bun.IDB, id, link string) error {
	if _, err := db.NewRaw(
		`UPDATE external_games SET store_link = ?, updated_at = now() WHERE id = ?`, link, id,
	).Exec(ctx); err != nil {
		return fmt.Errorf("set store link: %w", err)
	}
	return nil
}

// DeleteForStorefront deletes every External game of the user on the
// storefront, for a reset of that storefront's sync data. Their platform rows
// go by the foreign key's cascade.
func DeleteForStorefront(ctx context.Context, db bun.IDB, userID, storefront string) error {
	if _, err := db.NewRaw(
		`DELETE FROM external_games WHERE user_id = ? AND storefront = ?`, userID, storefront,
	).Exec(ctx); err != nil {
		return fmt.Errorf("delete external games for storefront: %w", err)
	}
	return nil
}

// DeleteAll deletes every External game of every user, for the admin reset.
// Their platform rows go by the foreign key's cascade.
func DeleteAll(ctx context.Context, db bun.IDB) error {
	if _, err := db.NewRaw(`DELETE FROM external_games`).Exec(ctx); err != nil {
		return fmt.Errorf("delete all external games: %w", err)
	}
	return nil
}
