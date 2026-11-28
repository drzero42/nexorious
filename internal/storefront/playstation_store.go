package storefront

import (
	"context"
	"errors"

	playstationstoresvc "github.com/drzero42/nexorious/internal/services/playstationstore"
)

// PlayStationStoreClient is the PSN call connecting verifies an NPSSO token
// with. *playstationstoresvc.Client implements it.
type PlayStationStoreClient interface {
	GetAccountInfo(ctx context.Context, npssoToken string) (*playstationstoresvc.PSNAccountInfo, error)
}

// The ways ConnectPlayStationStore rejects a sign-in. Nothing is stored after
// either.
var (
	// ErrNPSSOTokenLength: the token is not 64 characters.
	ErrNPSSOTokenLength = errors.New("playstation store: npsso token must be 64 characters")
	// ErrInvalidNPSSOToken: PSN did not accept the token.
	ErrInvalidNPSSOToken = errors.New("playstation store: invalid npsso token")
)

const npssoTokenLength = 64

// PlayStationStoreAccount is the PSN account a connected token belongs to.
type PlayStationStoreAccount struct {
	OnlineID  string
	AccountID string
}

// ConnectPlayStationStore verifies an NPSSO token against PSN and stores it as
// the user's PlayStation Store Connection.
func (c *Connections) ConnectPlayStationStore(ctx context.Context, userID, npssoToken string) (PlayStationStoreAccount, error) {
	if len(npssoToken) != npssoTokenLength {
		return PlayStationStoreAccount{}, ErrNPSSOTokenLength
	}
	info, err := c.clients.PlayStationStore.GetAccountInfo(ctx, npssoToken)
	if err != nil {
		// Any failure to sign in with the token is a rejection of it.
		return PlayStationStoreAccount{}, ErrInvalidNPSSOToken
	}
	creds := PlayStationStoreCredentials{NPSSOToken: npssoToken, OnlineID: info.OnlineID}
	// PSN accepted the sign-in; storing it does not depend on the caller
	// still waiting.
	if err := c.Store(context.WithoutCancel(ctx), userID, "playstation-store", creds); err != nil {
		return PlayStationStoreAccount{}, err
	}
	return PlayStationStoreAccount{OnlineID: info.OnlineID, AccountID: info.AccountID}, nil
}
