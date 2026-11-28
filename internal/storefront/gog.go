package storefront

import (
	"context"
	"errors"

	gogsvc "github.com/drzero42/nexorious/internal/services/gog"
)

// The ways ConnectGOG rejects a sign-in. Nothing is stored after any of them.
var (
	// ErrGOGAuthCodeMissing: no code or login URL was given.
	ErrGOGAuthCodeMissing = errors.New("gog: auth code is required")
	// ErrInvalidGOGAuthCode: the input is a URL that is not GOG's login
	// redirect, or carries no code. The error's message is only the reason,
	// worded for the user.
	ErrInvalidGOGAuthCode = errors.New("gog: invalid auth code")
	// ErrGOGSignInFailed: GOG did not exchange the code for tokens. The
	// error's message is only GOG's answer.
	ErrGOGSignInFailed = errors.New("gog: sign-in failed")
)

// GOGSignInURL is the GOG login page whose redirect carries the code
// ConnectGOG takes; empty when there is no GOG client.
func (c *Connections) GOGSignInURL() string {
	if c.clients.GOG == nil {
		return ""
	}
	return c.clients.GOG.BuildAuthURL()
}

// ConnectGOG exchanges a GOG authorization code, bare or inside the login
// redirect URL, for tokens and stores the refresh token as the user's GOG
// Connection. It returns the GOG username.
func (c *Connections) ConnectGOG(ctx context.Context, userID, authCode string) (string, error) {
	if authCode == "" {
		return "", ErrGOGAuthCodeMissing
	}
	code, err := gogsvc.ParseAuthCode(authCode)
	if err != nil {
		return "", reasonError{sentinel: ErrInvalidGOGAuthCode, reason: err}
	}
	tok, err := c.clients.GOG.ExchangeCode(ctx, code)
	if err != nil {
		return "", reasonError{sentinel: ErrGOGSignInFailed, reason: err}
	}
	creds := GOGCredentials{RefreshToken: tok.RefreshToken, Username: tok.Username}
	// GOG accepted the sign-in; storing it does not depend on the caller
	// still waiting.
	if err := c.Store(context.WithoutCancel(ctx), userID, "gog", creds); err != nil {
		return "", err
	}
	return tok.Username, nil
}
