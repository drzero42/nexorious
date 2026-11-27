// Package usergame owns every user-game mutation (#1056): Acquire (the
// keystone), adding, updating and removing platforms, setting the status,
// recording progress, deleting a game and clearing the library. The API, sync
// and import all go through it.
//
// Each operation is a free function that runs in a transaction (RunInTx on the
// database or transaction it is given), so the invariant tail (clear the
// wishlist flag, promote a played game, remove a finished game from pools,
// reconcile tags) commits with the change. The helpers for that tail are
// unexported: callers ask for the operation, never chain platform inserts and
// invariants themselves.
//
// Boundary errors are ErrNotFound, ErrConflict, ErrValidation and
// ErrUnprocessable, which the API maps to HTTP statuses (httpError). Nullable
// PATCH fields use Clear* bool sentinels to tell "omitted" from "clear it".
package usergame
