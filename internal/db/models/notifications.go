package models

import (
	"encoding/json"
	"time"

	"github.com/uptrace/bun"
)

// Event mirrors the append-only events outbox table.
type Event struct {
	bun.BaseModel `bun:"table:events"`

	ID          string          `bun:"id,pk"             json:"id"`
	Type        string          `bun:"type,notnull"      json:"type"`
	Scope       string          `bun:"scope,notnull"     json:"scope"`
	ActorUserID *string         `bun:"actor_user_id"     json:"actor_user_id"`
	Payload     json.RawMessage `bun:"payload,notnull"   json:"payload"`
	DedupKey    *string         `bun:"dedup_key"         json:"dedup_key"`
	OccurredAt  time.Time       `bun:"occurred_at,notnull" json:"occurred_at"`
}

// NotificationChannel mirrors notification_channels. EncryptedURL is never
// exposed in API responses (json:"-").
type NotificationChannel struct {
	bun.BaseModel `bun:"table:notification_channels"`

	ID           string    `bun:"id,pk"                json:"id"`
	UserID       string    `bun:"user_id,notnull"      json:"user_id"`
	Name         string    `bun:"name,notnull"         json:"name"`
	EncryptedURL string    `bun:"encrypted_url,notnull" json:"-"`
	CreatedAt    time.Time `bun:"created_at,notnull"   json:"created_at"`
}

// NotificationSubscription mirrors notification_subscriptions (pure presence).
type NotificationSubscription struct {
	bun.BaseModel `bun:"table:notification_subscriptions"`

	UserID    string    `bun:"user_id,pk"     json:"user_id"`
	EventType string    `bun:"event_type,pk"  json:"event_type"`
	CreatedAt time.Time `bun:"created_at,notnull" json:"created_at"`
}
