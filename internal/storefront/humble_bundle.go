package storefront

import (
	"context"
	"errors"
	"fmt"
	"strings"

	humblesvc "github.com/drzero42/nexorious/internal/services/humble"
)

// HumbleBundleClient is the Humble Bundle call connecting verifies a session
// cookie with. *humblesvc.Client implements it.
type HumbleBundleClient interface {
	Verify(ctx context.Context, sessionCookie string) error
}

// The ways ConnectHumbleBundle rejects a sign-in. Nothing is stored after any
// of them.
var (
	// ErrHumbleBundleCookieMissing: the cookie is empty once trimmed.
	ErrHumbleBundleCookieMissing = errors.New("humble bundle: session cookie is required")
	// ErrInvalidHumbleBundleCookie: Humble Bundle did not accept the cookie.
	ErrInvalidHumbleBundleCookie = errors.New("humble bundle: invalid session cookie")
	// ErrHumbleBundleUnavailable: Humble Bundle could not be reached or gave no
	// usable answer. The error wraps the cause.
	ErrHumbleBundleUnavailable = errors.New("humble bundle: unavailable")
)

// ConnectHumbleBundle verifies a pasted session cookie against Humble
// Bundle's order API and stores it, trimmed, as the user's Humble Bundle
// Connection.
func (c *Connections) ConnectHumbleBundle(ctx context.Context, userID, sessionCookie string) error {
	sessionCookie = strings.TrimSpace(sessionCookie)
	if sessionCookie == "" {
		return ErrHumbleBundleCookieMissing
	}
	err := c.clients.HumbleBundle.Verify(ctx, sessionCookie)
	switch {
	case errors.Is(err, humblesvc.ErrCredentials):
		return ErrInvalidHumbleBundleCookie
	case err != nil:
		// The cookie travels in a header, so the cause does not carry it.
		return fmt.Errorf("%w: %w", ErrHumbleBundleUnavailable, err)
	}
	// Humble Bundle accepted the sign-in; storing it does not depend on the
	// caller still waiting.
	return c.Store(context.WithoutCancel(ctx), userID, "humble-bundle", HumbleBundleCredentials{SessionCookie: sessionCookie})
}
