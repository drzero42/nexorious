package storefront

// The stored credential shape of each storefront with a typed Connection. The
// JSON field names are the stored format: renaming one breaks every existing
// Connection.

// SteamCredentials is a stored Steam Connection.
type SteamCredentials struct {
	WebAPIKey   string `json:"web_api_key"`
	SteamID     string `json:"steam_id"`
	DisplayName string `json:"display_name"`
}

// PlayStationStoreCredentials is a stored PlayStation Store Connection. Older
// rows also carry account_id, region, is_verified and token_expired_at, which
// nothing reads; decoding ignores them.
type PlayStationStoreCredentials struct {
	NPSSOToken string `json:"npsso_token"`
	OnlineID   string `json:"online_id"`
}

// GOGCredentials is a stored GOG Connection. A sync replaces RefreshToken
// with the one GOG returns and keeps every other field.
type GOGCredentials struct {
	RefreshToken string `json:"refresh_token"`
	Username     string `json:"username"`
}

// HumbleBundleCredentials is a stored Humble Bundle Connection.
type HumbleBundleCredentials struct {
	SessionCookie string `json:"session_cookie"`
}
