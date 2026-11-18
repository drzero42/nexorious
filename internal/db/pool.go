package db

import "github.com/uptrace/bun"

// MaxIdleConns is the server pool's idle-connection limit. DropIdleConns
// restores it after flushing, so the two must agree.
const MaxIdleConns = 5

// DropIdleConns closes every idle connection in db's pool, then restores the
// MaxIdleConns limit. Use it after something outside the pool has terminated
// its backends (a restore runs pg_terminate_backend). Otherwise each stale
// idle connection fails the next query that picks it up with
// "FATAL: terminating connection due to administrator command (57P01)":
// pgdriver discards the connection only after that query has failed, and
// database/sql does not retry it (#1284).
func DropIdleConns(db *bun.DB) {
	db.SetMaxIdleConns(0) // n <= 0 retains no idle conns: closes the current ones
	db.SetMaxIdleConns(MaxIdleConns)
}
