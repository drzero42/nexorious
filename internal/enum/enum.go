package enum

import "slices"

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

// allPlayStatuses lists every valid play status in display order (the order a
// game progresses through, not alphabetical).
var allPlayStatuses = []PlayStatus{
	PlayStatusNotStarted,
	PlayStatusInProgress,
	PlayStatusCompleted,
	PlayStatusMastered,
	PlayStatusDominated,
	PlayStatusShelved,
	PlayStatusDropped,
	PlayStatusReplay,
}

// AllPlayStatuses returns every valid play status as plain strings, in display
// order. These are stable enum values, so clients may rely on them directly.
func AllPlayStatuses() []string {
	return playStatusStrings(allPlayStatuses)
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
	return playStatusStrings(FinishedPlayStatuses)
}

// playStatusStrings converts play statuses to plain strings, keeping their order.
func playStatusStrings(statuses []PlayStatus) []string {
	out := make([]string, len(statuses))
	for i, s := range statuses {
		out[i] = string(s)
	}
	return out
}

// BeatenPlayStatuses are the Finished play statuses the user played through:
// every Finished status except dropped (see GLOSSARY "Beaten").
var BeatenPlayStatuses = []PlayStatus{
	PlayStatusCompleted,
	PlayStatusMastered,
	PlayStatusDominated,
}

// BeatenPlayStatusStrings returns the beaten set as plain strings, for use in
// SQL `IN (?)` clauses via bun.List.
func BeatenPlayStatusStrings() []string {
	return playStatusStrings(BeatenPlayStatuses)
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

// Rank returns how strong an ownership status is; higher is stronger. Owned
// ranks above borrowed and rented (equal), then subscription, then
// no_longer_owned; an unknown or empty status ranks 0.
func (s OwnershipStatus) Rank() int {
	switch s {
	case OwnershipOwned:
		return 4
	case OwnershipBorrowed, OwnershipRented:
		return 3
	case OwnershipSubscription:
		return 2
	case OwnershipNoLongerOwned:
		return 1
	default:
		return 0
	}
}

// allOwnershipStatuses lists every valid ownership status in display order.
var allOwnershipStatuses = []OwnershipStatus{
	OwnershipOwned,
	OwnershipBorrowed,
	OwnershipRented,
	OwnershipSubscription,
	OwnershipNoLongerOwned,
}

// AllOwnershipStatuses returns every valid ownership status as plain strings,
// in display order. These are stable enum values, so clients may rely on them
// directly.
func AllOwnershipStatuses() []string {
	out := make([]string, len(allOwnershipStatuses))
	for i, s := range allOwnershipStatuses {
		out[i] = string(s)
	}
	return out
}

// ChangeType represents valid change_type values for the changes table: one
// row per library outcome of a sync or import Job.
type ChangeType string

const (
	ChangeAdded            ChangeType = "added"
	ChangeUpdated          ChangeType = "updated"
	ChangeRemoved          ChangeType = "removed"
	ChangeStatusChanged    ChangeType = "status_changed"
	ChangeSkipped          ChangeType = "skipped"
	ChangeAlreadyInLibrary ChangeType = "already_in_library"
)

// LibraryChangeTypes are the change types that record a write to a library
// entry (sync and import both write them). A sync's diff lists them once per
// library entry (#1443).
var LibraryChangeTypes = []ChangeType{
	ChangeAdded,
	ChangeAlreadyInLibrary,
	ChangeStatusChanged,
}

// IsLibraryEntry reports whether t is one of LibraryChangeTypes.
func (t ChangeType) IsLibraryEntry() bool {
	return slices.Contains(LibraryChangeTypes, t)
}

// LibraryChangeTypeStrings returns LibraryChangeTypes as plain strings, for
// use in SQL `IN (?)` clauses via bun.List.
func LibraryChangeTypeStrings() []string {
	out := make([]string, len(LibraryChangeTypes))
	for i, t := range LibraryChangeTypes {
		out[i] = string(t)
	}
	return out
}
