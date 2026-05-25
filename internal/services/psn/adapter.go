package psn

import (
	"context"
	"errors"
	"fmt"

	"github.com/drzero42/nexorious/internal/services/storefrontadapter"
)

// Adapter wraps a Client with a pre-configured NPSSO token and implements storefrontadapter.Adapter.
type Adapter struct {
	client     *Client
	npssoToken string
}

// NewAdapter returns a storefrontadapter.Adapter for PSN.
func NewAdapter(client *Client, npssoToken string) storefrontadapter.Adapter {
	return &Adapter{client: client, npssoToken: npssoToken}
}

func (a *Adapter) GetLibrary(ctx context.Context, batchSize int, onBatch func([]storefrontadapter.ExternalGameEntry) error) error {
	err := a.client.GetLibrary(ctx, a.npssoToken, batchSize, func(entries []ExternalGameEntry) error {
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
	if errors.Is(err, ErrInvalidNPSSOToken) || errors.Is(err, ErrPSNGraphQLSchemaChanged) {
		return fmt.Errorf("%w: %v", storefrontadapter.ErrCredentials, err)
	}
	return err
}
