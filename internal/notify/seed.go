package notify

import (
	"context"

	"github.com/uptrace/bun"
)

// SeedDefaultSubscriptions inserts the default-on event types for a user.
// Idempotent (ON CONFLICT DO NOTHING). Accepts *bun.DB or bun.Tx via bun.IDB.
func SeedDefaultSubscriptions(ctx context.Context, db bun.IDB, userID string) error {
	for _, eventType := range DefaultSubscriptions() {
		if _, err := db.NewRaw(
			`INSERT INTO notification_subscriptions (user_id, event_type, created_at)
			 VALUES (?, ?, now()) ON CONFLICT (user_id, event_type) DO NOTHING`,
			userID, eventType,
		).Exec(ctx); err != nil {
			return err
		}
	}
	return nil
}
