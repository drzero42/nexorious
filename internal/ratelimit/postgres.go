package ratelimit

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/uptrace/bun"
)

// PostgresLimiter implements a distributed token-bucket rate limiter backed by
// a PostgreSQL row in the rate_limiter_tokens table. It is safe for use across
// multiple processes sharing the same database.
type PostgresLimiter struct {
	db    *bun.DB
	key   string
	rps   float64
	burst float64
}

// NewPostgres creates a PostgresLimiter for the given key (e.g. "igdb").
// rps is the refill rate (tokens per second); burst is the maximum token capacity.
// rps must be positive; burst must be >= 1.
func NewPostgres(db *bun.DB, key string, rps float64, burst float64) *PostgresLimiter {
	if rps <= 0 {
		rps = 1
	}
	if burst < 1 {
		burst = 1
	}
	return &PostgresLimiter{db: db, key: key, rps: rps, burst: burst}
}

// Wait blocks until a token is available or the context is cancelled.
// It retries with a short sleep when tokens are exhausted.
func (p *PostgresLimiter) Wait(ctx context.Context) error {
	for {
		if ctx.Err() != nil {
			return fmt.Errorf("ratelimit: %w", ctx.Err())
		}

		ok, err := p.tryConsume(ctx)
		if err != nil {
			return fmt.Errorf("ratelimit: %w", err)
		}
		if ok {
			return nil
		}

		// Sleep for roughly the time it takes to refill one token.
		sleep := time.Duration(float64(time.Second) / p.rps)
		t := time.NewTimer(sleep)
		select {
		case <-ctx.Done():
			t.Stop()
			return fmt.Errorf("ratelimit: %w", ctx.Err())
		case <-t.C:
		}
	}
}

// tryConsume attempts to consume one token. Returns (true, nil) on success,
// (false, nil) when no tokens are available, or (false, err) on DB error.
func (p *PostgresLimiter) tryConsume(ctx context.Context) (bool, error) {
	tx, err := p.db.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback() }()

	// Ensure the row exists before locking it.
	_, err = tx.NewRaw(
		`INSERT INTO rate_limiter_tokens (key, tokens, last_refill)
		 VALUES (?, ?, now())
		 ON CONFLICT (key) DO NOTHING`,
		p.key, p.burst,
	).Exec(ctx)
	if err != nil {
		return false, err
	}

	var tokens float64
	var lastRefill time.Time
	err = tx.NewRaw(
		`SELECT tokens, last_refill FROM rate_limiter_tokens WHERE key = ? FOR UPDATE`,
		p.key,
	).Scan(ctx, &tokens, &lastRefill)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		return false, err
	}

	// Refill tokens based on elapsed time.
	now := time.Now()
	elapsed := now.Sub(lastRefill).Seconds()
	tokens = math.Min(p.burst, tokens+elapsed*p.rps)

	if tokens < 1.0 {
		return false, nil
	}

	// Consume one token.
	tokens--
	_, err = tx.NewRaw(
		`UPDATE rate_limiter_tokens SET tokens = ?, last_refill = ? WHERE key = ?`,
		tokens, now, p.key,
	).Exec(ctx)
	if err != nil {
		return false, err
	}

	if err := tx.Commit(); err != nil {
		return false, err
	}
	return true, nil
}
