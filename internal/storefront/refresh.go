package storefront

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"

	"github.com/drzero42/nexorious/internal/logging"
	"github.com/drzero42/nexorious/internal/services/storefrontadapter"
)

// storedCredentialsWriter saves a sign-in a sync refreshed (GOG's new refresh
// token, Epic's new snapshot, or Amazon's updated user data) while the stored
// sign-in still matches the one the sync last read or wrote. A reconnect's
// newer sign-in wins and the old refresh is dropped.
type storedCredentialsWriter struct {
	conns      *Connections
	userID     string
	storefront string

	mu      sync.Mutex
	current string // the ciphertext this sync last read or wrote
}

func newStoredCredentialsWriter(conns *Connections, userID, storefront, loaded string) *storedCredentialsWriter {
	return &storedCredentialsWriter{conns: conns, userID: userID, storefront: storefront, current: loaded}
}

// bind ties a credentials failure from inner to the sign-in the sync last
// read or saved, read at failure time: Epic and Amazon can save a refresh
// before reporting the failure. A reconnect during the sync has a different
// ciphertext, so the worker leaves that Connection alone.
func (w *storedCredentialsWriter) bind(inner storefrontadapter.Adapter) storefrontadapter.Adapter {
	return credentialsBoundAdapter{inner: inner, w: w}
}

func (w *storedCredentialsWriter) ciphertext() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.current
}

// save encrypts plain and writes it.
func (w *storedCredentialsWriter) save(ctx context.Context, plain []byte) error {
	ciphertext, err := w.conns.encrypter.Encrypt(plain)
	if err != nil {
		return fmt.Errorf("save refreshed %s sign-in: encrypt: %w", w.storefront, err)
	}
	return w.write(ctx, ciphertext)
}

func (w *storedCredentialsWriter) write(ctx context.Context, ciphertext string) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	res, err := w.conns.db.NewRaw(
		`UPDATE user_sync_configs SET storefront_credentials = ?, updated_at = now()
		 WHERE user_id = ? AND storefront = ? AND storefront_credentials = ?`,
		ciphertext, w.userID, w.storefront, w.current,
	).Exec(ctx)
	if err != nil {
		return fmt.Errorf("save refreshed %s sign-in: %w", w.storefront, err)
	}
	if n, _ := res.RowsAffected(); n == 0 { //nolint:errcheck // RowsAffected never errors for this driver; 0 reads as "replaced"
		slog.InfoContext(ctx, "sync: refreshed sign-in dropped: the store was reconnected during the sync", logging.KeySource, w.storefront)
		return nil
	}
	w.current = ciphertext
	return nil
}

// credentialsBoundAdapter returns a credentials failure as a
// CredentialsErrorFor naming the sign-in its writer last read or wrote.
type credentialsBoundAdapter struct {
	inner storefrontadapter.Adapter
	w     *storedCredentialsWriter
}

func (a credentialsBoundAdapter) GetLibrary(ctx context.Context, batchSize int, onBatch func([]storefrontadapter.ExternalGameEntry) error) error {
	err := a.inner.GetLibrary(ctx, batchSize, onBatch)
	if errors.Is(err, storefrontadapter.ErrCredentials) {
		ciphertext := a.w.ciphertext()
		return &storefrontadapter.CredentialsErrorFor{Credentials: &ciphertext, Err: err}
	}
	return err
}
