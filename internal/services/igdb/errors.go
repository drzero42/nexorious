package igdb

import "errors"

var (
	ErrIGDBNotConfigured = errors.New("IGDB credentials not configured")
	ErrGameNotFound      = errors.New("game not found in IGDB")
	ErrTwitchAuth        = errors.New("twitch authentication failed")
)

// IsAuthError reports whether err is (or wraps) an authentication failure,
// meaning bad credentials or an HTTP 4xx from Twitch. Network errors
// (timeouts, DNS failures) do NOT satisfy IsAuthError.
func IsAuthError(err error) bool {
	return err != nil && errors.Is(err, ErrTwitchAuth)
}
