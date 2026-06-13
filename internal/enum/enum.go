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

// FinishedPlayStatuses are the play statuses that remove a game from every pool
// and exclude it from pool suggestions (#955). dropped is included deliberately:
// it is the strongest "not next" signal, so it leaves the plan like a completion.
var FinishedPlayStatuses = []PlayStatus{
	PlayStatusCompleted,
	PlayStatusMastered,
	PlayStatusDominated,
	PlayStatusDropped,
}

// FinishedPlayStatusStrings returns the finished set as plain strings, for use
// in SQL `IN (?)` clauses via bun.List.
func FinishedPlayStatusStrings() []string {
	out := make([]string, len(FinishedPlayStatuses))
	for i, s := range FinishedPlayStatuses {
		out[i] = string(s)
	}
	return out
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
