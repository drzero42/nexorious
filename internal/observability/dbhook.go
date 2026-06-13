package observability

import (
	"context"
	"database/sql"
	"errors"

	"github.com/uptrace/bun"
)

// dbErrorHook is a bun.QueryHook that counts failed queries into
// nexorious_db_errors_total. bunotel emits query timing but no error signal, so
// this fills the gap for DB-error alerting (#913). It is intentionally minimal:
// no spans, no extra labels beyond the SQL operation.
type dbErrorHook struct{}

// NewDBErrorHook returns a bun.QueryHook recording DB query failures as the
// nexorious_db_errors_total metric. Register it alongside the bunotel hook.
func NewDBErrorHook() bun.QueryHook { return dbErrorHook{} }

func (dbErrorHook) BeforeQuery(ctx context.Context, _ *bun.QueryEvent) context.Context {
	return ctx
}

func (dbErrorHook) AfterQuery(ctx context.Context, event *bun.QueryEvent) {
	if event.Err == nil {
		return
	}
	// ErrNoRows is an expected "not found" result, not a failure; context
	// cancellation is shutdown noise, not a DB fault. Timeouts
	// (context.DeadlineExceeded) are deliberately NOT excluded — a query that
	// runs past its deadline usually signals a real DB problem worth counting.
	if errors.Is(event.Err, sql.ErrNoRows) || errors.Is(event.Err, context.Canceled) {
		return
	}
	RecordDBError(ctx, event.Operation())
}
