package main

import (
	"context"
	"errors"
	"log/slog"
	"sync"

	"github.com/uptrace/bun"

	"github.com/drzero42/nexorious/internal/logging"
	"github.com/drzero42/nexorious/internal/worker/tasks"
)

// credentialsBoundAdapter ties an adapter's credentials failures to the stored
// sign-in the sync loaded (tasks.CredentialsErrorFor), so the worker does not
// flag a sign-in the user replaced while the sync ran. It wraps every store
// whose credentials failure is reported before its adapter writes the stored
// sign-in: Steam, PlayStation and Humble never write it, and GOG fails on a
// refused refresh before storing the new token. Not Epic, whose adapter
// rewrites its snapshot on every run, failed ones included, so the loaded
// ciphertext is stale by the time its failure is reported and binding to it
// would hide a real expiry.
type credentialsBoundAdapter struct {
	inner  tasks.StorefrontAdapter
	loaded string
}

func boundToCredentials(inner tasks.StorefrontAdapter, loaded string) tasks.StorefrontAdapter {
	return credentialsBoundAdapter{inner: inner, loaded: loaded}
}

func (a credentialsBoundAdapter) GetLibrary(ctx context.Context, batchSize int, onBatch func([]tasks.ExternalGameEntry) error) error {
	err := a.inner.GetLibrary(ctx, batchSize, onBatch)
	if errors.Is(err, tasks.ErrCredentials) {
		return &tasks.CredentialsErrorFor{Credentials: &a.loaded, Err: err}
	}
	return err
}

// storedCredentialsWriter saves a sign-in a sync refreshed (GOG's new refresh
// token, Epic's new snapshot) only while the stored sign-in is still the one
// this sync last read or wrote. A reconnect committed while the sync ran is
// never overwritten by the older sign-in's refreshed tokens; the refreshed
// value is dropped instead.
type storedCredentialsWriter struct {
	db         *bun.DB
	userID     string
	storefront string

	mu      sync.Mutex
	current string // the ciphertext this sync last read or wrote
}

func newStoredCredentialsWriter(db *bun.DB, userID, storefront, loaded string) *storedCredentialsWriter {
	return &storedCredentialsWriter{db: db, userID: userID, storefront: storefront, current: loaded}
}

func (w *storedCredentialsWriter) write(ctx context.Context, ciphertext string) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	res, err := w.db.NewRaw(
		`UPDATE user_sync_configs SET storefront_credentials = ?, updated_at = now()
		 WHERE user_id = ? AND storefront = ? AND storefront_credentials = ?`,
		ciphertext, w.userID, w.storefront, w.current,
	).Exec(ctx)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 { //nolint:errcheck // RowsAffected never errors for this driver; 0 reads as "replaced"
		slog.InfoContext(ctx, "sync: refreshed sign-in dropped: the store was reconnected during the sync", logging.KeySource, w.storefront)
		return nil
	}
	w.current = ciphertext
	return nil
}
