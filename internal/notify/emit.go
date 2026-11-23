package notify

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sync"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
	"github.com/uptrace/bun"

	"github.com/drzero42/nexorious/internal/logging"
)

// EmitParams describes one event to emit.
type EmitParams struct {
	Type        string
	Scope       string
	ActorUserID string
	Payload     any
	DedupKey    string
}

var (
	rcMu        sync.RWMutex
	riverClient *river.Client[pgx.Tx]
	txClient    *river.Client[*sql.Tx]
)

// SetRiverClient registers (or replaces) the River client used to enqueue
// NotifyWorker jobs. Called once at startup.
func SetRiverClient(rc *river.Client[pgx.Tx]) {
	rcMu.Lock()
	defer rcMu.Unlock()
	riverClient = rc
}

// SetTxRiverClient registers the insert-only River client (on Bun's *sql.DB)
// EmitTx inserts delivery jobs through, inside the caller's transaction.
// Called once at startup.
func SetTxRiverClient(rc *river.Client[*sql.Tx]) {
	rcMu.Lock()
	defer rcMu.Unlock()
	txClient = rc
}

// Emit writes an event row (idempotent on DedupKey) via db, then best-effort
// enqueues a NotifyWorker to deliver it. Never returns an error: emission is
// fire-and-forget and must not break the caller's primary work.
func Emit(ctx context.Context, db *bun.DB, p EmitParams) {
	id, err := insertEvent(ctx, db, p)
	if err != nil {
		slog.ErrorContext(ctx, "notify: insert event", "type", p.Type, logging.KeyErr, err, logging.Cat(logging.CategoryDB))
		return
	}
	if id != "" {
		enqueueNotify(ctx, id)
	}
}

// EmitTx writes the event row and the River job that delivers it through
// tx, so both commit or roll back with the caller's other writes: a
// notification is never recorded without its delivery. Unlike Emit it returns
// errors (including a cancelled context's), so the caller can roll back and
// retry. An event whose DedupKey already exists inserts nothing.
func EmitTx(ctx context.Context, tx bun.Tx, p EmitParams) error {
	id, err := insertEvent(ctx, tx, p)
	if err != nil {
		return err
	}
	if id == "" {
		return nil
	}
	rcMu.RLock()
	rc := txClient
	rcMu.RUnlock()
	if rc == nil {
		slog.DebugContext(ctx, "notify: tx river client unset; event persisted but not enqueued", "event_id", id)
		return nil
	}
	if _, err := rc.InsertTx(ctx, tx.Tx, NotifyArgs{EventID: id}, nil); err != nil {
		return fmt.Errorf("enqueue notify: %w", err)
	}
	return nil
}

// insertEvent writes the event row and returns its id, or "" when an event
// with the same DedupKey already exists.
func insertEvent(ctx context.Context, db bun.IDB, p EmitParams) (string, error) {
	if p.Payload == nil {
		p.Payload = map[string]any{}
	}
	payloadJSON, err := json.Marshal(p.Payload)
	if err != nil {
		slog.ErrorContext(ctx, "notify: marshal payload", "type", p.Type, logging.KeyErr, err)
		payloadJSON = []byte("{}")
	}

	var actor *string
	if p.ActorUserID != "" {
		actor = &p.ActorUserID
	}
	var dedup *string
	if p.DedupKey != "" {
		dedup = &p.DedupKey
	}

	id := uuid.NewString()
	var insertedID string
	err = db.NewRaw(
		`INSERT INTO events (id, type, scope, actor_user_id, payload, dedup_key, occurred_at)
		 VALUES (?, ?, ?, ?, ?, ?, now())
		 ON CONFLICT (dedup_key) DO NOTHING
		 RETURNING id`,
		id, p.Type, p.Scope, actor, json.RawMessage(payloadJSON), dedup,
	).Scan(ctx, &insertedID)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return insertedID, nil
}

func enqueueNotify(ctx context.Context, eventID string) {
	rcMu.RLock()
	rc := riverClient
	rcMu.RUnlock()
	if rc == nil {
		slog.DebugContext(ctx, "notify: river client unset; event persisted but not enqueued", "event_id", eventID)
		return
	}
	if _, err := rc.Insert(ctx, NotifyArgs{EventID: eventID}, nil); err != nil {
		slog.WarnContext(ctx, "notify: enqueue NotifyWorker", "event_id", eventID, logging.KeyErr, err)
	}
}

// NotifyArgs is the River job payload for delivering one event.
type NotifyArgs struct {
	EventID string `json:"event_id"`
}

// Kind implements river.JobArgs.
func (NotifyArgs) Kind() string { return "notify" }

// InsertOpts limits retries — delivery is fire-and-forget; infra retry only.
func (NotifyArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{MaxAttempts: 1, Priority: 3}
}
