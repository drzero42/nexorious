package epicgamesstore

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/drzero42/nexorious/internal/logging"
	"github.com/drzero42/nexorious/internal/services/storefrontadapter"
)

// clientInterface is the subset of *Client that Adapter depends on.
// Tests inject a fake without invoking the real legendary subprocess.
type clientInterface interface {
	Configured() bool
	newSession() (librarySession, error)
}

// librarySession is one sync's private legendary config directory.
type librarySession interface {
	RestoreSnapshot(snapshot map[string]string) error
	GetLibrary(ctx context.Context, onBatch func([]ExternalGameEntry) error) error
	CaptureSnapshot() (map[string]string, error)
	Close() error
}

// newSession satisfies clientInterface for the real client.
func (c *Client) newSession() (librarySession, error) {
	s, err := c.openSession()
	if err != nil {
		return nil, err
	}
	return s, nil
}

// Adapter implements storefrontadapter.Adapter for the Epic Games Store via
// the Legendary CLI. Each GetLibrary runs in its own session directory: it
// restores the stored snapshot there, fetches, captures the updated snapshot
// (delegating persistence to onSnapshot), and removes the directory.
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
func (a *Adapter) GetLibrary(ctx context.Context, batchSize int, onBatch func([]storefrontadapter.ExternalGameEntry) error) error {
	if !a.client.Configured() {
		return fmt.Errorf("epic: legendary not configured (LEGENDARY_WORK_DIR unset)")
	}

	sess, err := a.client.newSession()
	if err != nil {
		return err
	}
	defer closeSession(ctx, sess)

	if err := sess.RestoreSnapshot(a.snapshot); err != nil {
		return fmt.Errorf("epic: restore snapshot: %w", err)
	}

	// Per docs/sync.md § Epic, the adapter chunks the client's single big
	// batch into batches of ≤10 before invoking the outer onBatch.
	chunkSize := batchSize
	if chunkSize <= 0 || chunkSize > 10 {
		chunkSize = 10
	}

	fetchErr := sess.GetLibrary(ctx, func(batch []ExternalGameEntry) error {
		for start := 0; start < len(batch); start += chunkSize {
			end := min(start+chunkSize, len(batch))
			mapped := make([]storefrontadapter.ExternalGameEntry, 0, end-start)
			for _, e := range batch[start:end] {
				var sourceMeta map[string]string
				if e.Namespace != "" {
					sourceMeta = map[string]string{"namespace": e.Namespace}
				}
				mapped = append(mapped, storefrontadapter.ExternalGameEntry{
					ExternalID:      e.ExternalID,
					Title:           e.Title,
					PlaytimeHours:   0,
					Platforms:       []string{"pc-windows"},
					OwnershipStatus: e.OwnershipStatus,
					IsSubscription:  false,
					SourceMetadata:  sourceMeta,
				})
			}
			if err := onBatch(mapped); err != nil {
				return err
			}
		}
		return nil
	})

	// Capture updated snapshot regardless of fetch error.
	newSnapshot, captureErr := sess.CaptureSnapshot()
	if captureErr != nil {
		slog.WarnContext(ctx, "epic: capture snapshot failed",
			logging.KeyUserID, a.userID, logging.KeyErr, captureErr,
			logging.Cat(logging.CategoryExternalAPI))
	} else if len(newSnapshot) > 0 && a.onSnapshot != nil {
		if err := a.onSnapshot(newSnapshot); err != nil {
			slog.ErrorContext(ctx, "epic: persist updated snapshot failed",
				logging.KeyUserID, a.userID, logging.KeyErr, err,
				logging.Cat(logging.CategoryDB))
		}
	}

	if errors.Is(fetchErr, ErrAuthFailed) {
		return fmt.Errorf("%w: epic legendary auth failure", storefrontadapter.ErrCredentials)
	}
	return fetchErr
}
