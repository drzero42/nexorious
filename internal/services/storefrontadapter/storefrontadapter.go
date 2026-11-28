package storefrontadapter

import (
	"context"
	"errors"

	"github.com/drzero42/nexorious/internal/enum"
)

// ExternalGameEntry is the normalised game representation yielded by any storefront adapter.
type ExternalGameEntry struct {
	ExternalID      string
	Title           string
	PlaytimeHours   float64  // 0 when the storefront does not provide playtime; fractional when available (Steam, PSN)
	Platforms       []string // storefront-specific names; canonicalised to slugs by the worker
	OwnershipStatus enum.OwnershipStatus
	// AchievementsUnlocked / AchievementsTotal hold per-game achievement counts
	// when the source provides them (Steam only in v1). Nil = not fetched / not
	// available. Carried onto the first platform row, like PlaytimeHours.
	AchievementsUnlocked *int
	AchievementsTotal    *int
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

// IncompleteLibraryError indicates that a storefront returned a library that
// cannot safely be treated as complete. Message explains how to retry.
type IncompleteLibraryError struct {
	Message string
}

func (e *IncompleteLibraryError) Error() string { return e.Message }

// CredentialsErrorFor is a credentials failure that belongs to one stored
// sign-in: Credentials is the user_sync_configs ciphertext the sync loaded.
// The worker flags credentials_error, and notifies, only while that sign-in is
// still the stored one, in the same UPDATE that sets the flag, so a reconnect
// committed while the sync ran is never marked expired by it.
type CredentialsErrorFor struct {
	Credentials *string // nil: the sync loaded no stored sign-in
	Err         error
}

func (e *CredentialsErrorFor) Error() string { return e.Err.Error() }
func (e *CredentialsErrorFor) Unwrap() error { return e.Err }
