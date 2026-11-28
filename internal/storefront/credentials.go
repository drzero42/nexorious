package storefront

import (
	"encoding/json"
	"fmt"
)

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

// The account display name each storefront's status reports, decoded from its
// stored sign-in.

func steamAccount(plain []byte) (string, error) {
	var creds SteamCredentials
	err := json.Unmarshal(plain, &creds)
	return creds.DisplayName, err
}

func playStationStoreAccount(plain []byte) (string, error) {
	var creds PlayStationStoreCredentials
	err := json.Unmarshal(plain, &creds)
	return creds.OnlineID, err
}

func gogAccount(plain []byte) (string, error) {
	var creds GOGCredentials
	err := json.Unmarshal(plain, &creds)
	return creds.Username, err
}

// epicGamesStoreAccount reads displayName from the user.json in legendary's
// state snapshot (a map of relative path to file content). A snapshot without
// user.json has no name.
func epicGamesStoreAccount(plain []byte) (string, error) {
	var snapshot map[string]string
	if err := json.Unmarshal(plain, &snapshot); err != nil {
		return "", err
	}
	userJSON, ok := snapshot["user.json"]
	if !ok {
		return "", nil
	}
	var user struct {
		DisplayName string `json:"displayName"`
	}
	if err := json.Unmarshal([]byte(userJSON), &user); err != nil {
		return "", fmt.Errorf("user.json: %w", err)
	}
	return user.DisplayName, nil
}

// amazonGamesAccount reads customer_info.given_name from nile's user data.
func amazonGamesAccount(plain []byte) (string, error) {
	var userData struct {
		Extensions struct {
			CustomerInfo struct {
				GivenName string `json:"given_name"`
			} `json:"customer_info"`
		} `json:"extensions"`
	}
	err := json.Unmarshal(plain, &userData)
	return userData.Extensions.CustomerInfo.GivenName, err
}
