package usergame

import "errors"

var (
	// ErrNotFound is returned when the target user game (or platform) does not
	// exist for the given user. Maps to HTTP 404.
	ErrNotFound = errors.New("user game not found")
	// ErrConflict is returned when an operation would violate uniqueness
	// (game already in collection / duplicate platform). Maps to HTTP 409.
	ErrConflict = errors.New("conflict")
	// ErrValidation is returned for invalid input (bad ownership status, empty
	// platform set where one is required, etc.). Maps to HTTP 400.
	ErrValidation = errors.New("validation")
)
