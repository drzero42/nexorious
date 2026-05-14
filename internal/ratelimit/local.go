package ratelimit

import (
	"context"
	"time"

	"golang.org/x/time/rate"
)

// LocalLimiter wraps golang.org/x/time/rate for single-process rate limiting.
type LocalLimiter struct {
	r *rate.Limiter
}

// NewLocal creates a LocalLimiter with the given requests-per-second rate and burst capacity.
// rps must be positive; burst must be >= 1.
func NewLocal(rps float64, burst int) *LocalLimiter {
	if rps <= 0 {
		rps = 1
	}
	if burst < 1 {
		burst = 1
	}
	interval := time.Duration(float64(time.Second) / rps)
	return &LocalLimiter{r: rate.NewLimiter(rate.Every(interval), burst)}
}

// Wait blocks until the limiter permits one event or the context is cancelled.
func (l *LocalLimiter) Wait(ctx context.Context) error {
	return l.r.Wait(ctx)
}
