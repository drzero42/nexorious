package igdb

import "errors"

var (
	ErrIGDBNotConfigured = errors.New("IGDB credentials not configured")
	ErrGameNotFound      = errors.New("game not found in IGDB")
	// ErrTwitchAuth wraps every non-200 answer from Twitch's token endpoint.
	ErrTwitchAuth = errors.New("twitch authentication failed")
	// ErrTwitchCredentialsRejected is wrapped as well when Twitch answered
	// 400, 401 or 403: the configured credentials were rejected, which does
	// not go away by trying again. Other answers (5xx, 429) are transient.
	ErrTwitchCredentialsRejected = errors.New("IGDB credentials rejected")
)

// IsAuthError reports whether err means the configured IGDB credentials were
// rejected (Twitch answered 400, 401 or 403). Network errors and transient
// answers (5xx, 429) do NOT satisfy IsAuthError.
func IsAuthError(err error) bool {
	return err != nil && errors.Is(err, ErrTwitchCredentialsRejected)
}
