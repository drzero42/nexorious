package notify

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log/slog"
	"sync"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
	"github.com/uptrace/bun"
)

// EmitParams describes one event to emit.
type EmitParams struct {
	Type        string
	Scope       string
	ActorUserID string
	Payload     map[string]any
	DedupKey    string
}

var (
	rcMu        sync.RWMutex
	riverClient *river.Client[pgx.Tx]
)

// SetRiverClient registers (or replaces) the River client used to enqueue
// NotifyWorker jobs. Called once at startup and again in RebuildServices.
func SetRiverClient(rc *river.Client[pgx.Tx]) {
	rcMu.Lock()
	defer rcMu.Unlock()
	riverClient = rc
}

// Emit writes an event row (idempotent on DedupKey) via db, then best-effort
// enqueues a NotifyWorker to deliver it. Never returns an error: emission is
// fire-and-forget and must not break the caller's primary work.
func Emit(ctx context.Context, db *bun.DB, p EmitParams) {
	if p.Payload == nil {
		p.Payload = map[string]any{}
	}
	payloadJSON, err := json.Marshal(p.Payload)
	if err != nil {
		slog.Error("notify: marshal payload", "type", p.Type, "err", err)
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
		return
	}
	if err != nil {
		slog.Error("notify: insert event", "type", p.Type, "err", err)
		return
	}

	enqueueNotify(ctx, insertedID)
}

func enqueueNotify(ctx context.Context, eventID string) {
	rcMu.RLock()
	rc := riverClient
	rcMu.RUnlock()
	if rc == nil {
		slog.Debug("notify: river client unset; event persisted but not enqueued", "event_id", eventID)
		return
	}
	if _, err := rc.Insert(ctx, NotifyArgs{EventID: eventID}, nil); err != nil {
		slog.Warn("notify: enqueue NotifyWorker", "event_id", eventID, "err", err)
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
