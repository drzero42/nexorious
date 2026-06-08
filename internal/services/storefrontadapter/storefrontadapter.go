package storefrontadapter

import (
	"context"
	"errors"
)

// ExternalGameEntry is the normalised game representation yielded by any storefront adapter.
type ExternalGameEntry struct {
	ExternalID      string
	Title           string
	PlaytimeHours   float64  // 0 when the storefront does not provide playtime; fractional when available (Steam, PSN)
	Platforms       []string // storefront-specific names; canonicalised to slugs by the worker
	OwnershipStatus string   // "owned", "subscription", etc.
	IsSubscription  bool
	// SourceMetadata carries per-source resolution inputs captured at sync time
	// (e.g. Epic's namespace). Persisted to external_games.source_metadata; never
	// used to render store_link directly. Nil/empty for stores that need nothing.
	SourceMetadata map[string]string
}

// Adapter is the interface every storefront adapter must satisfy.
type Adapter interface {
	GetLibrary(ctx context.Context, batchSize int, onBatch func([]ExternalGameEntry) error) error
}

// ErrCredentials is returned by an adapter when credentials are invalid,
// expired, or cannot be decrypted. DispatchSyncWorker marks the job failed on this error.
var ErrCredentials = errors.New("credentials error")
