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

// credentialsBoundAdapter ties a credentials failure to the sign-in the
// sync last read or saved. A reconnect during the sync has a different
// ciphertext, so the worker leaves that connection alone.
type credentialsBoundAdapter struct {
	inner      tasks.StorefrontAdapter
	ciphertext func() string
}

func boundToCredentials(inner tasks.StorefrontAdapter, loaded string) tasks.StorefrontAdapter {
	return credentialsBoundAdapter{inner: inner, ciphertext: func() string { return loaded }}
}

func (a credentialsBoundAdapter) GetLibrary(ctx context.Context, batchSize int, onBatch func([]tasks.ExternalGameEntry) error) error {
	err := a.inner.GetLibrary(ctx, batchSize, onBatch)
	if errors.Is(err, tasks.ErrCredentials) {
		ciphertext := a.ciphertext()
		return &tasks.CredentialsErrorFor{Credentials: &ciphertext, Err: err}
	}
	return err
}

// storedCredentialsWriter saves a sign-in a sync refreshed (GOG's new refresh
// token, Epic's new snapshot, or Amazon's updated user data) while the stored
// sign-in still matches the one the sync last read or wrote. A reconnect's
// newer sign-in wins and the old refresh is dropped.
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

// bind reads the last saved value at failure time. Epic and Amazon can
// persist a refresh before reporting the failure.
func (w *storedCredentialsWriter) bind(inner tasks.StorefrontAdapter) tasks.StorefrontAdapter {
	return credentialsBoundAdapter{
		inner: inner,
		ciphertext: func() string {
			w.mu.Lock()
			defer w.mu.Unlock()
			return w.current
		},
	}
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
