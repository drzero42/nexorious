package tasks

import (
	"context"

	"github.com/uptrace/bun"
)

// jobDedupKey builds the key identifying an active job for dedup purposes,
// shared by the advisory lock and the "already active?" guard. userID is "" for
// non-user-scoped (global) dedup.
func jobDedupKey(jobType, source, userID string) string {
	return jobType + "|" + source + "|" + userID
}

// AcquireJobDedupLock takes a transaction-scoped PostgreSQL advisory lock that
// serializes concurrent creation of jobs sharing the same (jobType, source,
// userID) dedup key. The lock is held until the surrounding transaction commits
// or rolls back.
//
// Callers MUST run their "already active?" guard SELECT and the INSERT inside
// this same transaction, after calling this — that makes the guard and insert
// atomic with respect to any other transaction holding the same key, closing
// the READ COMMITTED TOCTOU window where two transactions both pass the guard
// (neither seeing the other's uncommitted insert) and both insert a duplicate
// active row. userID is "" for global (not user-scoped) dedup.
func AcquireJobDedupLock(ctx context.Context, tx bun.Tx, jobType, source, userID string) error {
	_, err := tx.NewRaw(`SELECT pg_advisory_xact_lock(hashtext(?))`, jobDedupKey(jobType, source, userID)).Exec(ctx)
	return err
}
