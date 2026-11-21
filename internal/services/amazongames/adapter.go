package amazongames

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/drzero42/nexorious/internal/logging"
	"github.com/drzero42/nexorious/internal/services/storefrontadapter"
)

// LibraryClient is the subset of *Client that Adapter depends on. Tests inject
// a fake without invoking the real nile subprocess.
type LibraryClient interface {
	Configured() bool
	Library(ctx context.Context, userData json.RawMessage) ([]Entitlement, json.RawMessage, error)
}

// Adapter implements storefrontadapter.Adapter for Amazon Games via nile.
type Adapter struct {
	client     LibraryClient
	userData   json.RawMessage
	onUserData func(json.RawMessage) error
}

// NewAdapter returns a storefrontadapter.Adapter for Amazon Games. userData is
// the decrypted nile user data loaded from user_sync_configs. onUserData is
// called with nile's user data as it was after the run (a refreshed access
// token), before any batch is delivered and even when the fetch fails; the
// factory wires the DB write here. onUserData may be nil.
func NewAdapter(client LibraryClient, userData json.RawMessage, onUserData func(json.RawMessage) error) storefrontadapter.Adapter {
	return &Adapter{client: client, userData: userData, onUserData: onUserData}
}

// GetLibrary implements storefrontadapter.Adapter. The whole library is read
// before the first batch is delivered, so a failure on any page returns
// before onBatch runs and the worker's removal sweep never sees a partial
// library.
func (a *Adapter) GetLibrary(ctx context.Context, batchSize int, onBatch func([]storefrontadapter.ExternalGameEntry) error) error {
	if !a.client.Configured() {
		return errors.New("amazon: nile not installed")
	}

	entitlements, refreshed, err := a.client.Library(ctx, a.userData)
	if refreshed != nil && a.onUserData != nil {
		if perr := a.onUserData(refreshed); perr != nil {
			slog.ErrorContext(ctx, "amazon: persist refreshed sign-in failed",
				logging.KeyErr, perr, logging.Cat(logging.CategoryDB))
		}
	}
	if errors.Is(err, ErrAuthFailed) {
		return fmt.Errorf("%w: amazon sign-in rejected", storefrontadapter.ErrCredentials)
	}
	if err != nil {
		return err
	}

	entries := toEntries(entitlements)
	slog.InfoContext(ctx, "amazon: library read", "entitlements", len(entitlements), "games", len(entries))

	if batchSize <= 0 {
		batchSize = len(entries)
	}
	for start := 0; start < len(entries); start += batchSize {
		end := min(start+batchSize, len(entries))
		if err := onBatch(entries[start:end]); err != nil {
			return err
		}
	}
	return nil
}

// toEntries maps entitlements to external games: one per Amazon product id,
// in nile's order. A product with no title is still sent, under a title that
// names its id, so it lands in review rather than being swept as removed.
func toEntries(entitlements []Entitlement) []storefrontadapter.ExternalGameEntry {
	seen := make(map[string]bool, len(entitlements))
	out := make([]storefrontadapter.ExternalGameEntry, 0, len(entitlements))
	for _, e := range entitlements {
		if e.ProductID == "" || seen[e.ProductID] {
			continue
		}
		seen[e.ProductID] = true
		title := strings.TrimSpace(e.Title)
		if title == "" {
			title = "Amazon product " + e.ProductID
		}
		out = append(out, storefrontadapter.ExternalGameEntry{
			ExternalID:      e.ProductID,
			Title:           title,
			Platforms:       []string{"pc-windows"},
			OwnershipStatus: "owned",
		})
	}
	return out
}
