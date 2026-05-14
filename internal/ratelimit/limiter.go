package ratelimit

import "context"

// Limiter is the rate limiting abstraction used by API clients.
type Limiter interface {
	Wait(ctx context.Context) error
}
