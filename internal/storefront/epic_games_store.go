package storefront

import (
	"context"
	"errors"
	"log/slog"

	"github.com/drzero42/nexorious/internal/logging"
)

// The ways ConnectEpicGamesStore rejects a sign-in. Nothing is stored after
// any of them.
var (
	// ErrEpicGamesStoreNotConfigured: LEGENDARY_WORK_DIR is unset, so nothing
	// can sign in to Epic.
	ErrEpicGamesStoreNotConfigured = errors.New("epic games store: legendary is not configured")
	// ErrEpicGamesStoreAuthCodeMissing: no auth code was given.
	ErrEpicGamesStoreAuthCodeMissing = errors.New("epic games store: auth code is required")
	// ErrEpicGamesStoreSignInFailed: legendary did not sign in with the code.
	// The error's message is only legendary's reason.
	ErrEpicGamesStoreSignInFailed = errors.New("epic games store: sign-in failed")
	// ErrEpicGamesStoreNoAccount: legendary signed in but left no account
	// behind (no user.json).
	ErrEpicGamesStoreNoAccount = errors.New("epic games store: no account after sign-in")
)

// EpicGamesStoreAccount is the Epic account a connected sign-in belongs to.
type EpicGamesStoreAccount struct {
	DisplayName string
	AccountID   string
}

// EpicGamesStoreConfigured reports whether Epic can be signed in to and
// synced: there is an Epic client and LEGENDARY_WORK_DIR is set.
func (c *Connections) EpicGamesStoreConfigured() bool {
	return c.clients.Epic != nil && c.clients.Epic.Configured()
}

// ConnectEpicGamesStore signs in to Epic through legendary with an auth code
// and stores legendary's state snapshot as the user's Epic Games Store
// Connection.
func (c *Connections) ConnectEpicGamesStore(ctx context.Context, userID, authCode string) (EpicGamesStoreAccount, error) {
	if !c.EpicGamesStoreConfigured() {
		return EpicGamesStoreAccount{}, ErrEpicGamesStoreNotConfigured
	}
	if authCode == "" {
		return EpicGamesStoreAccount{}, ErrEpicGamesStoreAuthCodeMissing
	}
	info, snapshot, err := c.clients.Epic.Authenticate(ctx, authCode)
	if err != nil {
		return EpicGamesStoreAccount{}, reasonError{sentinel: ErrEpicGamesStoreSignInFailed, reason: err}
	}
	if info == nil {
		return EpicGamesStoreAccount{}, ErrEpicGamesStoreNoAccount
	}
	// Epic accepted the sign-in; storing it does not depend on the caller
	// still waiting.
	if err := c.Store(context.WithoutCancel(ctx), userID, "epic-games-store", snapshot); err != nil {
		return EpicGamesStoreAccount{}, err
	}
	return EpicGamesStoreAccount{DisplayName: info.DisplayName, AccountID: info.AccountID}, nil
}

// DisconnectEpicGamesStore removes the user's stored Epic sign-in, then the
// legendary files versions before per-operation sessions left on disk for
// them. A failure to remove those is logged as a warning and otherwise
// ignored: the stored sign-in is already gone, so the disconnect itself
// succeeded.
func (c *Connections) DisconnectEpicGamesStore(ctx context.Context, userID string) error {
	// Removing a sign-in does not depend on the caller still waiting.
	ctx = context.WithoutCancel(ctx)
	if err := c.Disconnect(ctx, userID, "epic-games-store"); err != nil {
		return err
	}
	if c.clients.Epic == nil {
		return nil
	}
	if err := c.clients.Epic.Cleanup(ctx, userID); err != nil {
		slog.WarnContext(ctx, "epic: cleanup failed", logging.KeyErr, err, logging.KeySource, "epic-games-store")
	}
	return nil
}
