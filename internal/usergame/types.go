package usergame

import "time"

type AcquireMode int

const (
	// ModeCreate requires the user_games row not to exist; a pre-existing row
	// (or duplicate platform) yields ErrConflict. Used by the REST create path.
	ModeCreate AcquireMode = iota
	// ModeUpsert finds-or-creates idempotently and merges platforms. Used by
	// sync and import.
	ModeUpsert
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
	ExternalGameID  *string
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
}

type Result struct {
	UserGameID      string
	Created         bool
	PlatformChanges []PlatformChange
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
