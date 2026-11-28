package storefront

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/drzero42/nexorious/internal/logging"
	amazongamessvc "github.com/drzero42/nexorious/internal/services/amazongames"
)

// AmazonGamesClient is the nile calls Amazon Games needs: the library a sync
// reads, and the sign-in, registration and device sign-out connecting and
// disconnecting run. *amazongamessvc.Client implements it.
type AmazonGamesClient interface {
	amazongamessvc.LibraryClient
	// LoginURL starts a sign-in and returns its URL and the values nile needs
	// to finish it, which must stay server-side.
	LoginURL(ctx context.Context) (*amazongamessvc.Login, error)
	// Register finishes the sign-in with the authorization code and returns
	// the account and nile's user data.
	Register(ctx context.Context, code string, login amazongamessvc.Login) (*amazongamessvc.Account, json.RawMessage, error)
	// Logout deregisters the device a stored sign-in (nile's user data)
	// belongs to.
	Logout(ctx context.Context, userData json.RawMessage) error
}

// The ways StartAmazonGamesSignIn and ConnectAmazonGames reject a sign-in.
// Nothing is stored after any of them.
var (
	// ErrAmazonGamesNotInstalled: nile is not on PATH, so nothing can sign in
	// to Amazon.
	ErrAmazonGamesNotInstalled = errors.New("amazon games: nile is not installed")
	// ErrAmazonGamesStartFailed: nile could not start a sign-in. The error
	// wraps the cause.
	ErrAmazonGamesStartFailed = errors.New("amazon games: could not start a sign-in")
	// ErrAmazonGamesAttemptIDMissing: no attempt id was given.
	ErrAmazonGamesAttemptIDMissing = errors.New("amazon games: attempt id is required")
	// ErrInvalidAmazonGamesRedirect: the landing address carries no
	// authorization code. The error's message is only the reason, worded for
	// the user.
	ErrInvalidAmazonGamesRedirect = errors.New("amazon games: invalid redirect url")
	// ErrAmazonGamesAttemptUnusable: the attempt id does not decrypt with this
	// server's key, is not an Amazon sign-in attempt, was started by another
	// user, is incomplete or has expired.
	ErrAmazonGamesAttemptUnusable = errors.New("amazon games: sign-in attempt expired or unusable")
	// ErrAmazonGamesSignInRejected: Amazon did not register the device. The
	// error wraps the cause.
	ErrAmazonGamesSignInRejected = errors.New("amazon games: sign-in rejected")
)

// amazonLoginTTL bounds how long a started Amazon sign-in can be finished.
const amazonLoginTTL = 15 * time.Minute

// amazonLoginKind marks an attempt id as an Amazon sign-in attempt, so no
// other ciphertext made with the same key (a stored credential) is ever read
// as one.
const amazonLoginKind = "amazon-games-login"

// maxAmazonAttemptIDLen bounds the attempt id the finish will decrypt. A real
// one is about 450 characters; the cap keeps a pasted multi-kilobyte value
// from being decoded at all.
const maxAmazonAttemptIDLen = 2048

// amazonSignOutLimit bounds the best-effort device sign-out a disconnect
// starts.
const amazonSignOutLimit = 20 * time.Second

// amazonLoginAttempt is everything finishing an Amazon sign-in needs: the
// client id, code verifier and serial nile generated, who started it and until
// when it can be finished. The browser holds it encrypted with the server's
// key as the attempt id, so a start and a finish may reach different servers,
// or one server either side of a restart. Starting again does not cancel an
// earlier attempt; whichever finishes last is the stored sign-in.
type amazonLoginAttempt struct {
	Kind         string    `json:"kind"`
	ClientID     string    `json:"client_id"`
	CodeVerifier string    `json:"code_verifier"`
	Serial       string    `json:"serial"`
	UserID       string    `json:"user_id"`
	ExpiresAt    time.Time `json:"expires_at"`
}

// AmazonGamesSignIn is a started Amazon sign-in: the address to open, and the
// attempt id to finish it with.
type AmazonGamesSignIn struct {
	LoginURL  string
	AttemptID string
}

// AmazonGamesConfigured reports whether Amazon can be signed in to and
// synced: there is an Amazon client and nile is on PATH.
func (c *Connections) AmazonGamesConfigured() bool {
	return c.clients.Amazon != nil && c.clients.Amazon.Configured()
}

// StartAmazonGamesSignIn starts an Amazon sign-in for userID. The values nile
// needs to finish it travel sealed in the attempt id, bound to userID and
// valid for 15 minutes.
func (c *Connections) StartAmazonGamesSignIn(ctx context.Context, userID string) (AmazonGamesSignIn, error) {
	if !c.AmazonGamesConfigured() {
		return AmazonGamesSignIn{}, ErrAmazonGamesNotInstalled
	}
	login, err := c.clients.Amazon.LoginURL(ctx)
	if err != nil {
		return AmazonGamesSignIn{}, fmt.Errorf("%w: %w", ErrAmazonGamesStartFailed, err)
	}
	attemptID, err := c.sealAmazonLogin(userID, *login)
	if err != nil {
		return AmazonGamesSignIn{}, fmt.Errorf("seal amazon sign-in attempt: %w", err)
	}
	return AmazonGamesSignIn{LoginURL: login.URL, AttemptID: attemptID}, nil
}

// ConnectAmazonGames finishes the sign-in attemptID carries with the landing
// address (or bare code), registers the device through nile, and stores nile's
// user data as the user's Amazon Games Connection. It returns the account's
// name.
func (c *Connections) ConnectAmazonGames(ctx context.Context, userID, attemptID, redirectURL string) (string, error) {
	if !c.AmazonGamesConfigured() {
		return "", ErrAmazonGamesNotInstalled
	}
	if attemptID == "" {
		return "", ErrAmazonGamesAttemptIDMissing
	}
	code, err := amazongamessvc.ParseAuthCode(redirectURL)
	if err != nil {
		return "", reasonError{sentinel: ErrInvalidAmazonGamesRedirect, reason: err}
	}
	login, ok := c.openAmazonLogin(userID, attemptID)
	if !ok {
		return "", ErrAmazonGamesAttemptUnusable
	}
	account, userData, err := c.clients.Amazon.Register(ctx, code, login)
	if err != nil {
		return "", fmt.Errorf("%w: %w", ErrAmazonGamesSignInRejected, err)
	}
	// Amazon registered the device; storing it does not depend on the caller
	// still waiting.
	if err := c.Store(context.WithoutCancel(ctx), userID, "amazon-games", userData); err != nil {
		return "", err
	}
	return account.Name, nil
}

// DisconnectAmazonGames removes the user's stored Amazon sign-in, then signs
// that device out of Amazon in the background, best effort and under a time
// limit, so reconnecting does not leave another registered device on the
// account; a sign-out failure is only logged. The sign-out uses only the
// sign-in this disconnect removed: a reconnect stored meanwhile is never
// touched, and a removal that failed signs nothing out.
func (c *Connections) DisconnectAmazonGames(ctx context.Context, userID string) error {
	// Detached from the caller so neither the removal nor the sign-out waits
	// on it, keeping its logging values.
	ctx = context.WithoutCancel(ctx)
	removed, err := c.take(ctx, userID, "amazon-games")
	if err != nil {
		return err
	}
	if removed != nil && c.AmazonGamesConfigured() {
		c.amazonSignOuts.Go(func() { c.signOutAmazonDevice(ctx, *removed) })
	}
	return nil
}

// signOutAmazonDevice runs nile's logout with a removed Amazon sign-in, under
// a time limit; any failure is logged and otherwise ignored.
func (c *Connections) signOutAmazonDevice(ctx context.Context, ciphertext string) {
	limit := amazonSignOutLimit
	if c.amazonSignOutTimeout > 0 {
		limit = c.amazonSignOutTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, limit)
	defer cancel()
	userData, err := c.encrypter.Decrypt(ciphertext)
	if err != nil {
		slog.WarnContext(ctx, "amazon: device sign-out skipped: the removed sign-in does not decrypt", logging.KeyErr, err, logging.KeySource, "amazon-games", logging.Cat(logging.CategoryAuth))
		return
	}
	if err := c.clients.Amazon.Logout(ctx, userData); err != nil {
		slog.WarnContext(ctx, "amazon: device sign-out after disconnect failed", logging.KeyErr, err, logging.KeySource, "amazon-games", logging.Cat(logging.CategoryExternalAPI))
		return
	}
	slog.InfoContext(ctx, "amazon: device signed out after disconnect", logging.KeySource, "amazon-games")
}

// sealAmazonLogin returns the attempt id for a sign-in userID just started.
func (c *Connections) sealAmazonLogin(userID string, login amazongamessvc.Login) (string, error) {
	raw, err := json.Marshal(amazonLoginAttempt{
		Kind:         amazonLoginKind,
		ClientID:     login.ClientID,
		CodeVerifier: login.CodeVerifier,
		Serial:       login.Serial,
		UserID:       userID,
		ExpiresAt:    c.amazonLoginNow().Add(amazonLoginTTL),
	})
	if err != nil {
		return "", err
	}
	return c.encrypter.Encrypt(raw)
}

// openAmazonLogin returns the sign-in an attempt id carries, when it decrypts
// with this server's key, is an Amazon sign-in attempt, was started by userID,
// is complete and has not expired.
func (c *Connections) openAmazonLogin(userID, attemptID string) (amazongamessvc.Login, bool) {
	if len(attemptID) > maxAmazonAttemptIDLen {
		return amazongamessvc.Login{}, false
	}
	raw, err := c.encrypter.Decrypt(attemptID)
	if err != nil {
		return amazongamessvc.Login{}, false
	}
	var a amazonLoginAttempt
	if err := json.Unmarshal(raw, &a); err != nil {
		return amazongamessvc.Login{}, false
	}
	if a.Kind != amazonLoginKind || a.UserID != userID ||
		a.ClientID == "" || a.CodeVerifier == "" || a.Serial == "" ||
		a.ExpiresAt.IsZero() || !c.amazonLoginNow().Before(a.ExpiresAt) {
		return amazongamessvc.Login{}, false
	}
	return amazongamessvc.Login{ClientID: a.ClientID, CodeVerifier: a.CodeVerifier, Serial: a.Serial}, true
}

func (c *Connections) amazonLoginNow() time.Time {
	if c.amazonLoginClock != nil {
		return c.amazonLoginClock()
	}
	return time.Now()
}

// SetAmazonLoginClockForTest replaces the clock Amazon sign-in attempts are
// stamped and expire by.
func (c *Connections) SetAmazonLoginClockForTest(now func() time.Time) {
	c.amazonLoginClock = now
}

// SetAmazonSignOutTimeoutForTest replaces amazonSignOutLimit.
func (c *Connections) SetAmazonSignOutTimeoutForTest(d time.Duration) {
	c.amazonSignOutTimeout = d
}

// WaitForAmazonSignOutsForTest waits for every device sign-out a disconnect
// started.
func (c *Connections) WaitForAmazonSignOutsForTest() {
	c.amazonSignOuts.Wait()
}
