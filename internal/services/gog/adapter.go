package gog

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/drzero42/nexorious/internal/services/storefrontadapter"
)

// clientInterface is the subset of *Client that Adapter depends on.
// Tests inject a fake without making real HTTP calls.
type clientInterface interface {
	RefreshToken(ctx context.Context, refreshToken string) (*TokenResponse, error)
	GetLibrary(ctx context.Context, accessToken string, batchSize int, onBatch func([]ExternalGameEntry) error) error
}

// Adapter wraps a Client and implements storefrontadapter.Adapter.
// It refreshes the OAuth2 access token before each fetch, delegating
// token persistence to the onNewTokens callback.
type Adapter struct {
	client       clientInterface
	refreshToken string
	onNewTokens  func(accessToken, refreshToken string) error
}

// NewAdapter returns a storefrontadapter.Adapter for GOG.
// refreshToken is the OAuth2 refresh token loaded from user_sync_configs.
// onNewTokens is called with the new access/refresh pair after a successful
// token refresh; the factory wires the DB write here. onNewTokens may be nil.
func NewAdapter(
	client clientInterface,
	refreshToken string,
	onNewTokens func(accessToken, refreshToken string) error,
) storefrontadapter.Adapter {
	return &Adapter{client: client, refreshToken: refreshToken, onNewTokens: onNewTokens}
}

// GetLibrary implements storefrontadapter.Adapter.
func (a *Adapter) GetLibrary(ctx context.Context, batchSize int, onBatch func([]storefrontadapter.ExternalGameEntry) error) error {
	tok, err := a.client.RefreshToken(ctx, a.refreshToken)
	if errors.Is(err, ErrGOGAuthExpired) {
		return fmt.Errorf("%w: gog token refresh", storefrontadapter.ErrCredentials)
	}
	if err != nil {
		return err
	}

	if a.onNewTokens != nil {
		if err := a.onNewTokens(tok.AccessToken, tok.RefreshToken); err != nil {
			slog.Error("gog: persist refreshed tokens failed", "err", err)
		}
	}

	return a.client.GetLibrary(ctx, tok.AccessToken, batchSize, func(entries []ExternalGameEntry) error {
		mapped := make([]storefrontadapter.ExternalGameEntry, 0, len(entries))
		for _, e := range entries {
			mapped = append(mapped, storefrontadapter.ExternalGameEntry{
				ExternalID:      e.ExternalID,
				Title:           e.Title,
				PlaytimeHours:   e.PlaytimeHours,
				Platforms:       e.Platforms,
				OwnershipStatus: e.OwnershipStatus,
				IsSubscription:  e.IsSubscription,
			})
		}
		return onBatch(mapped)
	})
}
