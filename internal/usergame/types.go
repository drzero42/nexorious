package usergame

import "time"

type AcquireMode int

const (
	// ModeCreate requires the user_games row not to exist; a pre-existing row
	// (or duplicate platform) yields ErrConflict. Used by the REST create path.
	ModeCreate AcquireMode = iota
	// ModeUpsert finds-or-creates idempotently and merges platforms, bumping
	// updated_at on conflict. Used by sync.
	ModeUpsert
	// ModeImport finds-or-creates idempotently and merges platforms, but persists
	// the meta fields (and caller-supplied created_at/updated_at) on a fresh
	// insert while leaving an existing row — including its updated_at — fully
	// intact on conflict. Used by the import workers.
	ModeImport
)

type TagMode int

const (
	// TagMerge adds the supplied tags without removing existing ones (sync/import).
	TagMerge TagMode = iota
	// TagReplace reconciles to exactly the supplied set (explicit REST replace).
	TagReplace
)

type PlatformInput struct {
	Platform        *string
	Storefront      *string
	HoursPlayed     *float64
	OwnershipStatus *string
	IsAvailable     *bool
	AcquiredDate    *time.Time
	// ClearAcquiredDate instructs UpdatePlatform to set acquired_date to NULL.
	// It is distinct from AcquiredDate==nil (which means "leave unchanged") and
	// is used when the caller explicitly supplies an empty string for the field.
	ClearAcquiredDate bool
	ExternalGameID    *string
	// SyncFromSource marks the platform row as storefront-synced. Set to true
	// only in the sync worker; REST-create and import callers leave it false.
	SyncFromSource bool
	// AchievementsUnlocked / AchievementsTotal carry Steam achievement counts on
	// the sync path. Nil leaves the columns NULL on insert and unchanged on update.
	AchievementsUnlocked *int
	AchievementsTotal    *int
}

type TagInput struct {
	Name  string
	Color *string
}

type AcquireParams struct {
	UserID    string
	GameID    int32
	Mode      AcquireMode
	Platforms []PlatformInput
	Tags      []TagInput
	TagMode   TagMode

	// Meta fields — used by ModeCreate and ModeImport to initialise the
	// user_games row on a fresh insert. Ignored on ModeUpsert (sync creates the
	// row with column defaults) and on the conflict path (the existing row wins).
	PlayStatus     *string
	PersonalRating *int32
	IsLoved        bool
	PersonalNotes  *string
	IsWishlisted   bool

	// CreatedAt / UpdatedAt seed the timestamps on a ModeImport insert so
	// imported rows round-trip their original timestamps. Nil falls through to
	// now(). Ignored by ModeCreate (always now()) and ModeUpsert.
	CreatedAt *time.Time
	UpdatedAt *time.Time
}

type Result struct {
	UserGameID      string
	Created         bool
	PlatformChanges []PlatformChange
	// PlatformID is populated by AddPlatform with the newly inserted platform row ID.
	PlatformID string
}

type PlatformChange struct {
	Platform          string
	Storefront        string
	Created           bool
	OwnershipUpgraded bool
	OldOwnership      *string
	NewOwnership      *string
}

// ownershipRank returns a numeric rank for an ownership status string.
// Higher = better. Used to avoid downgrading ownership on update.
func ownershipRank(status string) int {
	switch status {
	case "owned":
		return 4
	case "borrowed", "rented":
		return 3
	case "subscription":
		return 2
	case "no_longer_owned":
		return 1
	default:
		return 0
	}
}
