package storefront

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"

	"github.com/drzero42/nexorious/internal/crypto"
	"github.com/drzero42/nexorious/internal/db/models"
	"github.com/drzero42/nexorious/internal/logging"
)

// Connections stores and loads users' storefront Connections.
type Connections struct {
	db        *bun.DB
	encrypter *crypto.Encrypter
}

// NewConnections returns the Connections backed by db, encrypting stored
// sign-ins with encrypter.
func NewConnections(db *bun.DB, encrypter *crypto.Encrypter) *Connections {
	return &Connections{db: db, encrypter: encrypter}
}

// Store saves creds as the user's Connection to the storefront, replacing any
// stored one, and clears credentials_error. creds is the storefront's typed
// credentials (SteamCredentials, ...), or for Epic and Amazon the opaque value
// their service package returned.
func (c *Connections) Store(ctx context.Context, userID, storefront string, creds any) error {
	credsJSON, err := json.Marshal(creds)
	if err != nil {
		return fmt.Errorf("store %s connection: marshal: %w", storefront, err)
	}
	ciphertext, err := c.encrypter.Encrypt(credsJSON)
	if err != nil {
		return fmt.Errorf("store %s connection: encrypt: %w", storefront, err)
	}
	now := time.Now().UTC()
	row := &models.UserSyncConfig{
		ID:                    uuid.NewString(),
		UserID:                userID,
		Storefront:            storefront,
		Frequency:             "manual",
		StorefrontCredentials: &ciphertext,
		CreatedAt:             now,
		UpdatedAt:             now,
	}
	// credentials_error belongs to storefront health (#1410); until that has
	// an owner, connecting clears it here.
	_, err = c.db.NewInsert().Model(row).
		On("CONFLICT (user_id, storefront) DO UPDATE SET storefront_credentials = EXCLUDED.storefront_credentials, credentials_error = false, updated_at = EXCLUDED.updated_at").
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("store %s connection: %w", storefront, err)
	}
	return nil
}

// Clear removes the user's stored sign-in for the storefront. Clearing an
// absent Connection is a no-op.
func (c *Connections) Clear(ctx context.Context, userID, storefront string) error {
	_, err := c.db.NewRaw(
		`UPDATE user_sync_configs SET storefront_credentials = NULL, updated_at = now() WHERE user_id = ? AND storefront = ?`,
		userID, storefront,
	).Exec(ctx)
	if err != nil {
		return fmt.Errorf("clear %s connection: %w", storefront, err)
	}
	return nil
}

// Take clears the user's stored sign-in for the storefront and returns the
// ciphertext it cleared, in one statement, so nothing stored after it can be
// returned. It returns nil when nothing was stored.
func (c *Connections) Take(ctx context.Context, userID, storefront string) (*string, error) {
	var removed *string
	err := c.db.NewRaw(
		`UPDATE user_sync_configs u SET storefront_credentials = NULL, updated_at = now()
		 FROM (SELECT id, storefront_credentials FROM user_sync_configs WHERE user_id = ? AND storefront = ? FOR UPDATE) old
		 WHERE u.id = old.id
		 RETURNING old.storefront_credentials`,
		userID, storefront,
	).Scan(ctx, &removed)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("take %s connection: %w", storefront, err)
	}
	return removed, nil
}

// Loaded is the state of a user's stored Connection.
type Loaded struct {
	Connected        bool   // a sign-in is stored
	CredentialsError bool   // the sign-in does not decrypt, or a sync flagged it
	Plaintext        []byte // the decrypted sign-in; nil unless Connected and it decrypts
}

// Load reads and decrypts the user's stored sign-in for the storefront. A
// non-nil error is a database failure; every other state is in Loaded.
// Callers decode Plaintext into the storefront's credentials type.
func (c *Connections) Load(ctx context.Context, userID, storefront string) (Loaded, error) {
	var row models.UserSyncConfig
	err := c.db.NewSelect().Model(&row).
		Where("user_id = ? AND storefront = ?", userID, storefront).
		Scan(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return Loaded{}, nil
	}
	if err != nil {
		return Loaded{}, fmt.Errorf("load %s connection: %w", storefront, err)
	}
	if row.StorefrontCredentials == nil {
		return Loaded{}, nil
	}
	plain, err := c.encrypter.Decrypt(*row.StorefrontCredentials)
	if err != nil {
		slog.WarnContext(logging.WithUserID(ctx, userID), "storefront: stored sign-in does not decrypt", logging.KeySource, storefront, logging.KeyErr, err, logging.Cat(logging.CategoryAuth))
		return Loaded{Connected: true, CredentialsError: true}, nil
	}
	return Loaded{Connected: true, CredentialsError: row.CredentialsError, Plaintext: plain}, nil
}
