package epic

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/drzero42/nexorious/internal/services/storefrontadapter"
)

// clientInterface is the subset of *Client that Adapter depends on.
// Tests inject a fake without invoking the real legendary subprocess.
type clientInterface interface {
	Configured() bool
	RestoreSnapshot(userID string, snapshot map[string]string) error
	GetLibrary(ctx context.Context, userID string, onBatch func([]ExternalGameEntry) error) error
	CaptureSnapshot(userID string) (map[string]string, error)
}

// Adapter implements storefrontadapter.Adapter for the Epic Games Store via
// the Legendary CLI. It restores a session snapshot before fetching and
// captures the updated snapshot afterward, delegating persistence to onSnapshot.
type Adapter struct {
	client     clientInterface
	userID     string
	snapshot   map[string]string
	onSnapshot func(map[string]string) error
}

// NewAdapter returns a storefrontadapter.Adapter for Epic Games Store.
// snapshot is the decrypted legendary state loaded from user_sync_configs.
// onSnapshot is called with the updated snapshot after CaptureSnapshot; the
// factory wires the DB write here. onSnapshot may be nil.
func NewAdapter(
	client clientInterface,
	userID string,
	snapshot map[string]string,
	onSnapshot func(map[string]string) error,
) storefrontadapter.Adapter {
	return &Adapter{
		client:     client,
		userID:     userID,
		snapshot:   snapshot,
		onSnapshot: onSnapshot,
	}
}

// GetLibrary implements storefrontadapter.Adapter.
func (a *Adapter) GetLibrary(ctx context.Context, _ int, onBatch func([]storefrontadapter.ExternalGameEntry) error) error {
	if !a.client.Configured() {
		return fmt.Errorf("epic: legendary not configured (LEGENDARY_WORK_DIR unset)")
	}

	if err := a.client.RestoreSnapshot(a.userID, a.snapshot); err != nil {
		return fmt.Errorf("epic: restore snapshot: %w", err)
	}

	fetchErr := a.client.GetLibrary(ctx, a.userID, func(batch []ExternalGameEntry) error {
		mapped := make([]storefrontadapter.ExternalGameEntry, 0, len(batch))
		for _, e := range batch {
			mapped = append(mapped, storefrontadapter.ExternalGameEntry{
				ExternalID:      e.ExternalID,
				Title:           e.Title,
				PlaytimeHours:   0,
				Platforms:       []string{"pc-windows"},
				OwnershipStatus: e.OwnershipStatus,
				IsSubscription:  false,
			})
		}
		return onBatch(mapped)
	})

	// Capture updated snapshot regardless of fetch error.
	newSnapshot, captureErr := a.client.CaptureSnapshot(a.userID)
	if captureErr != nil {
		slog.Error("epic: capture snapshot failed", "user_id", a.userID, "err", captureErr)
	} else if len(newSnapshot) > 0 && a.onSnapshot != nil {
		if err := a.onSnapshot(newSnapshot); err != nil {
			slog.Error("epic: persist updated snapshot failed", "user_id", a.userID, "err", err)
		}
	}

	if errors.Is(fetchErr, ErrAuthFailed) {
		return fmt.Errorf("%w: epic legendary auth failure", storefrontadapter.ErrCredentials)
	}
	return fetchErr
}
