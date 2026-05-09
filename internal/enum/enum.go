package enum

// PlayStatus represents valid play_status values for user_games.
type PlayStatus string

const (
	PlayStatusNotStarted PlayStatus = "not_started"
	PlayStatusInProgress PlayStatus = "in_progress"
	PlayStatusCompleted  PlayStatus = "completed"
	PlayStatusMastered   PlayStatus = "mastered"
	PlayStatusDominated  PlayStatus = "dominated"
	PlayStatusShelved    PlayStatus = "shelved"
	PlayStatusDropped    PlayStatus = "dropped"
	PlayStatusReplay     PlayStatus = "replay"
)

var validPlayStatuses = map[PlayStatus]bool{
	PlayStatusNotStarted: true,
	PlayStatusInProgress: true,
	PlayStatusCompleted:  true,
	PlayStatusMastered:   true,
	PlayStatusDominated:  true,
	PlayStatusShelved:    true,
	PlayStatusDropped:    true,
	PlayStatusReplay:     true,
}

// Valid reports whether s is a recognised play status.
func (s PlayStatus) Valid() bool {
	return validPlayStatuses[s]
}

// OwnershipStatus represents valid ownership_status values for user_game_platforms.
type OwnershipStatus string

const (
	OwnershipOwned         OwnershipStatus = "owned"
	OwnershipBorrowed      OwnershipStatus = "borrowed"
	OwnershipRented        OwnershipStatus = "rented"
	OwnershipSubscription  OwnershipStatus = "subscription"
	OwnershipNoLongerOwned OwnershipStatus = "no_longer_owned"
)

var validOwnershipStatuses = map[OwnershipStatus]bool{
	OwnershipOwned:         true,
	OwnershipBorrowed:      true,
	OwnershipRented:        true,
	OwnershipSubscription:  true,
	OwnershipNoLongerOwned: true,
}

// Valid reports whether s is a recognised ownership status.
func (s OwnershipStatus) Valid() bool {
	return validOwnershipStatuses[s]
}
