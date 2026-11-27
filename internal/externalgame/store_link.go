package externalgame

import (
	"context"

	"github.com/uptrace/bun"
)

// SetStoreLink writes the store link of the External game id: the identifier
// a storefront's product URL is built from. Only the store-link refresh
// writes it; a sync never does, so a re-sync cannot clear a resolved link.
func SetStoreLink(ctx context.Context, db bun.IDB, id, link string) error {
	_, err := db.NewRaw(
		`UPDATE external_games SET store_link = ?, updated_at = now() WHERE id = ?`, link, id,
	).Exec(ctx)
	return err
}
