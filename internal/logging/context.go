package logging

import "context"

// ctxKey is an unexported type so keys never collide with other packages'.
type ctxKey int

const (
	requestIDKey ctxKey = iota
	jobIDKey
	userIDKey
)

// WithRequestID returns a ctx carrying the HTTP request id.
func WithRequestID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, requestIDKey, id)
}

// WithJobID returns a ctx carrying the River job id.
func WithJobID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, jobIDKey, id)
}

// WithUserID returns a ctx carrying the authenticated user id.
func WithUserID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, userIDKey, id)
}

func requestID(ctx context.Context) string { s, _ := ctx.Value(requestIDKey).(string); return s }
func jobID(ctx context.Context) string     { s, _ := ctx.Value(jobIDKey).(string); return s }
func userID(ctx context.Context) string    { s, _ := ctx.Value(userIDKey).(string); return s }

// RequestIDForTest exposes the ctx-carried request id for tests in other packages.
func RequestIDForTest(ctx context.Context) string { return requestID(ctx) }
