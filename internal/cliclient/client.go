// Package cliclient is a thin HTTP client over the Nexorious /api/auth/*,
// /api/migrate/*, and /health endpoints used by the CLI to bootstrap an admin,
// run migrations, and manage an API key.
package cliclient

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const sessionCookieName = "session_id"

// defaultScopes is the scope requested for CLI-minted keys. The server accepts
// "read" or "write"; the CLI always mints write-scoped keys.
const defaultScopes = "write"

// Client talks to one Nexorious server.
type Client struct {
	baseURL string
	hc      *http.Client
}

// New returns a Client for the given base URL (trailing slash trimmed). The
// client does not follow redirects: a gate's 302 is an observable response, so
// callers (e.g. setup) can read its Location instead of silently chasing it.
func New(baseURL string) *Client {
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		hc: &http.Client{
			Timeout: 30 * time.Second,
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
	}
}

// errorBody decodes both error-body shapes the server emits: Echo's default
// `{"message":"…"}` and the hand-written handlers' `{"error":"…"}` (backup,
// admin, settings, notifications). msg() returns whichever is set.
type errorBody struct {
	Message  string `json:"message"`
	Error    string `json:"error"`
	AppState string `json:"app_state"`
}

func (eb errorBody) msg() string {
	if eb.Message != "" {
		return eb.Message
	}
	return eb.Error
}

// httpError decodes an error response (`{"message":…}` or `{"error":…}`) into a
// readable error including the status code.
func httpError(resp *http.Response) error {
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if err != nil {
		return fmt.Errorf("server returned %d (failed reading body: %w)", resp.StatusCode, err)
	}
	var eb errorBody
	if json.Unmarshal(body, &eb) == nil {
		if m := eb.msg(); m != "" {
			return fmt.Errorf("server returned %d: %s", resp.StatusCode, m)
		}
		// App-state gates (issue #771) answer with {"app_state":…} and no
		// error/message field; surface the state so the operator knows the
		// instance is not ready (e.g. migrations pending) rather than seeing a
		// bare status code.
		if eb.AppState != "" {
			return fmt.Errorf("server returned %d: instance not ready (state: %s)", resp.StatusCode, eb.AppState)
		}
	}
	return fmt.Errorf("server returned %d", resp.StatusCode)
}

// Login posts credentials and returns the raw session_id cookie value. The value
// is read straight off the response (not via a cookie jar) so a Secure-flagged
// cookie issued over http://localhost is still usable for the follow-up calls.
func (c *Client) Login(username, password string) (string, error) {
	payload, err := json.Marshal(map[string]string{"username": username, "password": password})
	if err != nil {
		return "", fmt.Errorf("marshal login: %w", err)
	}
	req, err := http.NewRequest(http.MethodPost, c.baseURL+"/api/auth/login", bytes.NewReader(payload))
	if err != nil {
		return "", fmt.Errorf("build login request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.hc.Do(req)
	if err != nil {
		return "", fmt.Errorf("login request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return "", httpError(resp)
	}
	for _, ck := range resp.Cookies() {
		if ck.Name == sessionCookieName {
			return ck.Value, nil
		}
	}
	return "", fmt.Errorf("login succeeded but no %s cookie was returned", sessionCookieName)
}

type createAPIKeyResp struct {
	ID  string `json:"id"`
	Key string `json:"key"`
}

// CreateAPIKey mints a write-scoped key named `name`, authenticating with the
// session cookie. Returns the raw key and its server-side id.
func (c *Client) CreateAPIKey(sessionID, name string) (string, string, error) {
	payload, err := json.Marshal(map[string]string{"name": name, "scopes": defaultScopes})
	if err != nil {
		return "", "", fmt.Errorf("marshal create key: %w", err)
	}
	req, err := http.NewRequest(http.MethodPost, c.baseURL+"/api/auth/api-keys", bytes.NewReader(payload))
	if err != nil {
		return "", "", fmt.Errorf("build create key request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: sessionID}) //nolint:gosec // outbound request cookie from a CLI client; Secure/HttpOnly are response-cookie attributes that don't apply here

	resp, err := c.hc.Do(req)
	if err != nil {
		return "", "", fmt.Errorf("create key request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return "", "", httpError(resp)
	}
	var out createAPIKeyResp
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", "", fmt.Errorf("decode create key response: %w", err)
	}
	return out.Key, out.ID, nil
}

// APIKey describes one API key as returned by the /api/auth/api-keys endpoints.
// Key is only populated by CreateAPIKeyWithBearer (the raw value is shown once at
// creation); list responses never include it.
type APIKey struct {
	ID         string     `json:"id"`
	Name       string     `json:"name"`
	Scopes     string     `json:"scopes"`
	Key        string     `json:"key,omitempty"`
	LastUsedAt *time.Time `json:"last_used_at"`
	CreatedAt  time.Time  `json:"created_at"`
	ExpiresAt  *time.Time `json:"expires_at"`
}

// ListAPIKeys returns the caller's non-revoked API keys, authenticating with the
// key itself as a Bearer token.
func (c *Client) ListAPIKeys(key string) ([]APIKey, error) {
	req, err := http.NewRequest(http.MethodGet, c.baseURL+"/api/auth/api-keys", nil)
	if err != nil {
		return nil, fmt.Errorf("build list keys request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+key)

	resp, err := c.hc.Do(req)
	if err != nil {
		return nil, fmt.Errorf("list keys request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, httpError(resp)
	}
	var out []APIKey
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("decode list keys response: %w", err)
	}
	return out, nil
}

// CreateAPIKeyWithBearer mints a key authenticating with an existing key as a
// Bearer token (used by `api-key generate`). When expiresAt is non-nil it is sent
// as the request's expires_at (the server validates the RFC3339 format). The
// returned APIKey includes the raw Key, shown exactly once.
func (c *Client) CreateAPIKeyWithBearer(key, name, scopes string, expiresAt *string) (APIKey, error) {
	body := map[string]string{"name": name, "scopes": scopes}
	if expiresAt != nil {
		body["expires_at"] = *expiresAt
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return APIKey{}, fmt.Errorf("marshal create key: %w", err)
	}
	req, err := http.NewRequest(http.MethodPost, c.baseURL+"/api/auth/api-keys", bytes.NewReader(payload))
	if err != nil {
		return APIKey{}, fmt.Errorf("build create key request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+key)

	resp, err := c.hc.Do(req)
	if err != nil {
		return APIKey{}, fmt.Errorf("create key request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return APIKey{}, httpError(resp)
	}
	var out APIKey
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return APIKey{}, fmt.Errorf("decode create key response: %w", err)
	}
	return out, nil
}

// revoke issues DELETE /api/auth/api-keys/:id with caller-supplied auth.
func (c *Client) revoke(keyID string, auth func(*http.Request)) error {
	req, err := http.NewRequest(http.MethodDelete, c.baseURL+"/api/auth/api-keys/"+keyID, nil)
	if err != nil {
		return fmt.Errorf("build revoke request: %w", err)
	}
	auth(req)

	resp, err := c.hc.Do(req)
	if err != nil {
		return fmt.Errorf("revoke request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusNoContent {
		return httpError(resp)
	}
	return nil
}

// RevokeAPIKeyWithCookie revokes a key using a session cookie (used during
// login rotation, before the new key exists).
func (c *Client) RevokeAPIKeyWithCookie(sessionID, keyID string) error {
	return c.revoke(keyID, func(r *http.Request) {
		r.AddCookie(&http.Cookie{Name: sessionCookieName, Value: sessionID}) //nolint:gosec // outbound request cookie from a CLI client; Secure/HttpOnly are response-cookie attributes that don't apply here
	})
}

// RevokeAPIKeyWithBearer revokes a key using the key itself as a Bearer token
// (used by logout).
func (c *Client) RevokeAPIKeyWithBearer(key, keyID string) error {
	return c.revoke(keyID, func(r *http.Request) {
		r.Header.Set("Authorization", "Bearer "+key)
	})
}

type healthResp struct {
	Status string `json:"status"`
}

// Health performs the GET /health preflight and returns the reported status
// ("ok" when the server is ready, otherwise the app-state name such as
// "needs_migration" or "db_unavailable").
func (c *Client) Health() (string, error) {
	req, err := http.NewRequest(http.MethodGet, c.baseURL+"/health", nil)
	if err != nil {
		return "", fmt.Errorf("build health request: %w", err)
	}
	resp, err := c.hc.Do(req)
	if err != nil {
		return "", fmt.Errorf("health request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return "", httpError(resp)
	}
	var out healthResp
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", fmt.Errorf("decode health response: %w", err)
	}
	return out.Status, nil
}

// ServerVersionInfo is the GET /api/version response: the server's own build
// version/commit plus its update-check verdict.
type ServerVersionInfo struct {
	Version         string `json:"version"`
	Commit          string `json:"commit"`
	UpdateAvailable bool   `json:"update_available"`
	LatestVersion   string `json:"latest_version"`
	ReleaseURL      string `json:"release_url"`
}

// ServerVersion fetches GET /api/version. The endpoint requires authentication
// (issue #1108), so the key (when non-empty) is sent as a bearer token; an empty
// key sends no auth header and the server responds 401, surfaced to the caller.
func (c *Client) ServerVersion(key string) (*ServerVersionInfo, error) {
	req, err := http.NewRequest(http.MethodGet, c.baseURL+"/api/version", nil)
	if err != nil {
		return nil, fmt.Errorf("build version request: %w", err)
	}
	if key != "" {
		req.Header.Set("Authorization", "Bearer "+key)
	}
	resp, err := c.hc.Do(req)
	if err != nil {
		return nil, fmt.Errorf("version request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, httpError(resp)
	}
	var out ServerVersionInfo
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("decode version response: %w", err)
	}
	return &out, nil
}

// SetupResult is the interpreted outcome of a setup-admin attempt. The caller
// maps StatusCode (and Location for a 3xx redirect) to a message and exit code.
type SetupResult struct {
	StatusCode int
	Location   string // Location header, set when StatusCode is a 3xx redirect
	Message    string // server {"message":...}, set for 4xx when present
}

// SetupAdmin posts the first-admin credentials to POST /api/auth/setup/admin.
// It returns a SetupResult for any HTTP response (including 3xx/4xx) so the
// caller can map the outcome; it returns a non-nil error only for transport
// failures (e.g. the server is unreachable). Redirects are not followed, so a
// gate's 302 is observable via Location.
func (c *Client) SetupAdmin(username, password string) (*SetupResult, error) {
	payload, err := json.Marshal(map[string]string{"username": username, "password": password})
	if err != nil {
		return nil, fmt.Errorf("marshal setup: %w", err)
	}
	req, err := http.NewRequest(http.MethodPost, c.baseURL+"/api/auth/setup/admin", bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("build setup request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.hc.Do(req)
	if err != nil {
		return nil, fmt.Errorf("setup request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	res := &SetupResult{StatusCode: resp.StatusCode}
	if resp.StatusCode >= 300 && resp.StatusCode < 400 {
		res.Location = resp.Header.Get("Location")
		return res, nil
	}
	if resp.StatusCode >= 400 {
		body, readErr := io.ReadAll(io.LimitReader(resp.Body, 4096))
		if readErr == nil {
			var eb errorBody
			if json.Unmarshal(body, &eb) == nil {
				res.Message = eb.msg()
			}
		}
	}
	return res, nil
}

// Logout drops the throwaway session created during login.
func (c *Client) Logout(sessionID string) error {
	req, err := http.NewRequest(http.MethodPost, c.baseURL+"/api/auth/logout", nil)
	if err != nil {
		return fmt.Errorf("build logout request: %w", err)
	}
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: sessionID}) //nolint:gosec // outbound request cookie from a CLI client; Secure/HttpOnly are response-cookie attributes that don't apply here

	resp, err := c.hc.Do(req)
	if err != nil {
		return fmt.Errorf("logout request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return httpError(resp)
	}
	return nil
}

type meResp struct {
	Username string `json:"username"`
}

// Me returns the authenticated username for the given API key.
func (c *Client) Me(key string) (string, error) {
	req, err := http.NewRequest(http.MethodGet, c.baseURL+"/api/auth/me", nil)
	if err != nil {
		return "", fmt.Errorf("build me request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+key)

	resp, err := c.hc.Do(req)
	if err != nil {
		return "", fmt.Errorf("me request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return "", httpError(resp)
	}
	var out meResp
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", fmt.Errorf("decode me response: %w", err)
	}
	return out.Username, nil
}

// RunMigrations triggers POST /api/migrate/run on the running server, so the
// server's own migrator applies pending migrations and its in-memory state
// transitions to ready. 202 ("migration started"), 400 ("already up to date"),
// and 409 ("in progress") are all treated as success (nil) — the caller then
// polls MigrationStatus to learn the outcome. Other responses return an error.
func (c *Client) RunMigrations() error {
	req, err := http.NewRequest(http.MethodPost, c.baseURL+"/api/migrate/run", nil)
	if err != nil {
		return fmt.Errorf("build migrate request: %w", err)
	}
	resp, err := c.hc.Do(req)
	if err != nil {
		return fmt.Errorf("migrate request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	switch resp.StatusCode {
	case http.StatusAccepted, http.StatusBadRequest, http.StatusConflict:
		return nil
	default:
		return httpError(resp)
	}
}

type migrationStatusResp struct {
	State        string `json:"state"`
	PendingCount int    `json:"pending_count"`
	Error        string `json:"error"`
}

// MigrationStatus returns the server's migration state from
// GET /api/migrate/status ("needs_migration", "migrating", "ready",
// "migration_failed", or "db_unavailable") along with the server's failure
// detail (the "error" field, populated only in the "migration_failed" state;
// empty otherwise).
func (c *Client) MigrationStatus() (state, detail string, err error) {
	req, err := http.NewRequest(http.MethodGet, c.baseURL+"/api/migrate/status", nil)
	if err != nil {
		return "", "", fmt.Errorf("build status request: %w", err)
	}
	resp, err := c.hc.Do(req)
	if err != nil {
		return "", "", fmt.Errorf("status request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return "", "", httpError(resp)
	}
	var out migrationStatusResp
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", "", fmt.Errorf("decode status response: %w", err)
	}
	return out.State, out.Error, nil
}

// doBearer performs an authenticated JSON request. A non-nil body is JSON-encoded
// as the request body; on a 2xx response a non-nil out is decoded from the body
// (skipped for 204). Non-2xx responses become an httpError.
func (c *Client) doBearer(method, path, key string, body, out any) error {
	var rdr io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("marshal request: %w", err)
		}
		rdr = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, c.baseURL+path, rdr)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if key != "" {
		req.Header.Set("Authorization", "Bearer "+key)
	}

	resp, err := c.hc.Do(req)
	if err != nil {
		return fmt.Errorf("request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return httpError(resp)
	}
	if out != nil && resp.StatusCode != http.StatusNoContent {
		if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
			return fmt.Errorf("decode response: %w", err)
		}
	}
	return nil
}

// IGDBCandidate is one IGDB search hit. UserGameID is non-nil when the game is
// already in the caller's library.
type IGDBCandidate struct {
	IgdbID               int      `json:"igdb_id"`
	IgdbSlug             string   `json:"igdb_slug"`
	Title                string   `json:"title"`
	ReleaseDate          string   `json:"release_date"`
	CoverArtURL          string   `json:"cover_art_url"`
	Description          string   `json:"description"`
	Platforms            []string `json:"platforms"`
	HowLongToBeatMain    *float64 `json:"howlongtobeat_main"`
	UserGameID           *string  `json:"user_game_id"`
	UserGameIsWishlisted *bool    `json:"user_game_is_wishlisted"`
}

// IGDBSearchResponse is the result of an IGDB search or single-game lookup.
type IGDBSearchResponse struct {
	Games []IGDBCandidate `json:"games"`
	Total int             `json:"total"`
}

// Game is the minimal local game record returned by igdb-import.
type Game struct {
	ID    int    `json:"id"`
	Title string `json:"title"`
}

// SearchIGDB searches the IGDB catalog for the given query (limit 1–50).
func (c *Client) SearchIGDB(key, query string, limit int) (*IGDBSearchResponse, error) {
	var out IGDBSearchResponse
	body := map[string]any{"query": query, "limit": limit}
	if err := c.doBearer(http.MethodPost, "/api/games/search/igdb", key, body, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ImportIGDBGame imports IGDB metadata for igdbID into the local DB, returning
// the local game record.
func (c *Client) ImportIGDBGame(key string, igdbID int) (*Game, error) {
	var out Game
	body := map[string]any{"igdb_id": igdbID, "download_cover_art": true}
	if err := c.doBearer(http.MethodPost, "/api/games/igdb-import", key, body, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// GameRef is the minimal game metadata embedded in a user-game (the title source).
type GameRef struct {
	ID    int    `json:"id"`
	Title string `json:"title"`
}

// Tag is a user tag.
type Tag struct {
	ID        string  `json:"id"`
	Name      string  `json:"name"`
	Color     *string `json:"color"`
	GameCount int64   `json:"game_count"`
}

// UserGamePlatform is one platform row on a user-game.
type UserGamePlatform struct {
	ID              string   `json:"id"`
	Platform        *string  `json:"platform"`
	Storefront      *string  `json:"storefront"`
	HoursPlayed     *float64 `json:"hours_played"`
	OwnershipStatus *string  `json:"ownership_status"`
}

// UserGame is one collection entry (subset of the API response).
type UserGame struct {
	ID             string             `json:"id"`
	GameID         int                `json:"game_id"`
	PlayStatus     *string            `json:"play_status"`
	PersonalRating *int               `json:"personal_rating"`
	IsLoved        bool               `json:"is_loved"`
	IsWishlisted   bool               `json:"is_wishlisted"`
	PersonalNotes  *string            `json:"personal_notes"`
	Game           *GameRef           `json:"game"`
	HoursPlayed    float64            `json:"hours_played"`
	Platforms      []UserGamePlatform `json:"platforms"`
	Tags           []Tag              `json:"tags"`
}

// Title returns the game's title, or "" if the relation is absent.
func (u *UserGame) Title() string {
	if u.Game == nil {
		return ""
	}
	return u.Game.Title
}

// UserGameListResponse is the paged list payload.
type UserGameListResponse struct {
	UserGames []UserGame `json:"user_games"`
	Total     int        `json:"total"`
	Page      int        `json:"page"`
	PerPage   int        `json:"per_page"`
	Pages     int        `json:"pages"`
}

// ListUserGames returns user-games filtered by the given query params.
func (c *Client) ListUserGames(key string, params url.Values) (*UserGameListResponse, error) {
	path := "/api/user-games"
	if enc := params.Encode(); enc != "" {
		path += "?" + enc
	}
	var out UserGameListResponse
	if err := c.doBearer(http.MethodGet, path, key, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// GetUserGame fetches a single user-game by id.
func (c *Client) GetUserGame(key, id string) (*UserGame, error) {
	var out UserGame
	if err := c.doBearer(http.MethodGet, "/api/user-games/"+url.PathEscape(id), key, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// CollectionStats mirrors the GET /api/user-games/stats response.
type CollectionStats struct {
	TotalGames       int            `json:"total_games"`
	CompletionStats  map[string]int `json:"completion_stats"`
	OwnershipStats   map[string]int `json:"ownership_stats"`
	PlatformStats    map[string]int `json:"platform_stats"`
	GenreStats       map[string]int `json:"genre_stats"`
	PileOfShame      int            `json:"pile_of_shame"`
	CompletionRate   float64        `json:"completion_rate"`
	AverageRating    *float64       `json:"average_rating"`
	TotalHoursPlayed float64        `json:"total_hours_played"`
}

// GetCollectionStats fetches aggregate collection statistics for the user.
func (c *Client) GetCollectionStats(key string) (*CollectionStats, error) {
	var out CollectionStats
	if err := c.doBearer(http.MethodGet, "/api/user-games/stats", key, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// PlatformInput is a platform row for create / move-to-library / add-platform.
type PlatformInput struct {
	Platform        string   `json:"platform,omitempty"`
	Storefront      string   `json:"storefront,omitempty"`
	OwnershipStatus string   `json:"ownership_status,omitempty"`
	HoursPlayed     *float64 `json:"hours_played,omitempty"`
}

// CreateUserGameInput is the body for creating a collection entry.
type CreateUserGameInput struct {
	GameID         int             `json:"game_id"`
	PlayStatus     string          `json:"play_status,omitempty"`
	PersonalRating *int            `json:"personal_rating,omitempty"`
	IsLoved        bool            `json:"is_loved,omitempty"`
	PersonalNotes  *string         `json:"personal_notes,omitempty"`
	IsWishlisted   bool            `json:"is_wishlisted,omitempty"`
	Platforms      []PlatformInput `json:"platforms,omitempty"`
}

// CreateUserGame adds a game to the collection.
func (c *Client) CreateUserGame(key string, in CreateUserGameInput) (*UserGame, error) {
	var out UserGame
	if err := c.doBearer(http.MethodPost, "/api/user-games", key, in, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// MoveToLibrary promotes a wishlisted user-game to the library with platforms.
func (c *Client) MoveToLibrary(key, id string, platforms []PlatformInput) (*UserGame, error) {
	var out UserGame
	body := map[string]any{"platforms": platforms}
	if err := c.doBearer(http.MethodPost, "/api/user-games/"+url.PathEscape(id)+"/move-to-library", key, body, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// UpdateUserGame applies a partial field update (play_status, personal_rating,
// is_loved, personal_notes). A nil map value clears the field server-side.
func (c *Client) UpdateUserGame(key, id string, fields map[string]any) (*UserGame, error) {
	var out UserGame
	if err := c.doBearer(http.MethodPut, "/api/user-games/"+url.PathEscape(id), key, fields, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// UpdateProgress sets play_status via the dedicated progress endpoint.
func (c *Client) UpdateProgress(key, id, playStatus string) (*UserGame, error) {
	var out UserGame
	body := map[string]any{"play_status": playStatus}
	if err := c.doBearer(http.MethodPut, "/api/user-games/"+url.PathEscape(id)+"/progress", key, body, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// AddPlatform creates a platform row on a user-game.
func (c *Client) AddPlatform(key, id string, p PlatformInput) error {
	return c.doBearer(http.MethodPost, "/api/user-games/"+url.PathEscape(id)+"/platforms", key, p, nil)
}

// UpdatePlatform applies a partial update to one platform row.
func (c *Client) UpdatePlatform(key, id, platformID string, fields map[string]any) error {
	return c.doBearer(http.MethodPut, "/api/user-games/"+url.PathEscape(id)+"/platforms/"+url.PathEscape(platformID), key, fields, nil)
}

// DeletePlatform removes a platform row.
func (c *Client) DeletePlatform(key, id, platformID string) error {
	return c.doBearer(http.MethodDelete, "/api/user-games/"+url.PathEscape(id)+"/platforms/"+url.PathEscape(platformID), key, nil, nil)
}

// ReplaceTags sets the complete tag set (by name) on a user-game.
func (c *Client) ReplaceTags(key, id string, tags []string) (*UserGame, error) {
	var out UserGame
	body := map[string]any{"tags": tags}
	if err := c.doBearer(http.MethodPut, "/api/user-games/"+url.PathEscape(id)+"/tags", key, body, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// DeleteUserGame removes a user-game.
func (c *Client) DeleteUserGame(key, id string) error {
	return c.doBearer(http.MethodDelete, "/api/user-games/"+url.PathEscape(id), key, nil, nil)
}

// ListTags returns the caller's tags (used to resolve --tag NAME to an id).
func (c *Client) ListTags(key string) ([]Tag, error) {
	var out []Tag
	if err := c.doBearer(http.MethodGet, "/api/tags", key, nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// CreateTag creates a tag with an optional color.
func (c *Client) CreateTag(key, name string, color *string) (*Tag, error) {
	body := map[string]any{"name": name}
	if color != nil {
		body["color"] = *color
	}
	var out Tag
	if err := c.doBearer(http.MethodPost, "/api/tags", key, body, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// UpdateTag updates a tag's name and/or color (only non-nil fields are sent).
func (c *Client) UpdateTag(key, id string, name, color *string) (*Tag, error) {
	body := map[string]any{}
	if name != nil {
		body["name"] = *name
	}
	if color != nil {
		body["color"] = *color
	}
	var out Tag
	if err := c.doBearer(http.MethodPut, "/api/tags/"+url.PathEscape(id), key, body, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// DeleteTag removes a tag.
func (c *Client) DeleteTag(key, id string) error {
	return c.doBearer(http.MethodDelete, "/api/tags/"+url.PathEscape(id), key, nil, nil)
}

// Pool is a play-planning pool's metadata.
type Pool struct {
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Color     *string         `json:"color"`
	Position  int             `json:"position"`
	Filter    json.RawMessage `json:"filter"`
	HasFilter bool            `json:"has_filter"`
}

// PoolListItem is one row of the pool list.
type PoolListItem struct {
	ID             string  `json:"id"`
	Name           string  `json:"name"`
	Color          *string `json:"color"`
	Position       int     `json:"position"`
	HasFilter      bool    `json:"has_filter"`
	QueueCount     int64   `json:"queue_count"`
	CandidateCount int64   `json:"candidate_count"`
}

// PoolDetail is a pool plus its ordered queue and candidate user-games.
type PoolDetail struct {
	Pool
	Queue      []UserGame `json:"queue"`
	Candidates []UserGame `json:"candidates"`
}

// ListPools returns the caller's pools ordered by position.
func (c *Client) ListPools(key string) ([]PoolListItem, error) {
	var out []PoolListItem
	if err := c.doBearer(http.MethodGet, "/api/pools", key, nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// GetPool returns a pool with its queue and candidates.
func (c *Client) GetPool(key, id string) (*PoolDetail, error) {
	var out PoolDetail
	if err := c.doBearer(http.MethodGet, "/api/pools/"+url.PathEscape(id), key, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// CreatePool creates a pool with optional color and filter (raw JSON, server-validated).
func (c *Client) CreatePool(key, name string, color *string, filter json.RawMessage) (*Pool, error) {
	body := map[string]any{"name": name}
	if color != nil {
		body["color"] = *color
	}
	if filter != nil {
		body["filter"] = filter
	}
	var out Pool
	if err := c.doBearer(http.MethodPost, "/api/pools", key, body, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// UpdatePool applies a partial update (name/color/filter).
func (c *Client) UpdatePool(key, id string, fields map[string]any) (*Pool, error) {
	var out Pool
	if err := c.doBearer(http.MethodPut, "/api/pools/"+url.PathEscape(id), key, fields, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// DeletePool removes a pool.
func (c *Client) DeletePool(key, id string) error {
	return c.doBearer(http.MethodDelete, "/api/pools/"+url.PathEscape(id), key, nil, nil)
}

// AddPoolGame adds one game (as a candidate) to a pool.
func (c *Client) AddPoolGame(key, poolID, userGameID string) error {
	body := map[string]any{"user_game_id": userGameID}
	return c.doBearer(http.MethodPost, "/api/pools/"+url.PathEscape(poolID)+"/games", key, body, nil)
}

// BulkAddPoolGames adds multiple games (as candidates); returns the count newly inserted.
func (c *Client) BulkAddPoolGames(key, poolID string, userGameIDs []string) (int64, error) {
	body := map[string]any{"user_game_ids": userGameIDs}
	var out struct {
		Added int64 `json:"added"`
	}
	if err := c.doBearer(http.MethodPost, "/api/pools/"+url.PathEscape(poolID)+"/games/bulk", key, body, &out); err != nil {
		return 0, err
	}
	return out.Added, nil
}

// RemovePoolGame removes a game from a pool.
func (c *Client) RemovePoolGame(key, poolID, userGameID string) error {
	return c.doBearer(http.MethodDelete, "/api/pools/"+url.PathEscape(poolID)+"/games/"+url.PathEscape(userGameID), key, nil, nil)
}

// SetQueue declaratively sets the pool's ordered queue (ids must already be members;
// an empty slice clears the queue). Members not listed become candidates.
func (c *Client) SetQueue(key, poolID string, userGameIDs []string) error {
	body := map[string]any{"ids": userGameIDs}
	return c.doBearer(http.MethodPut, "/api/pools/"+url.PathEscape(poolID)+"/queue", key, body, nil)
}

// ReorderPools sets pool positions by the given order.
func (c *Client) ReorderPools(key string, poolIDs []string) error {
	body := map[string]any{"ids": poolIDs}
	return c.doBearer(http.MethodPost, "/api/pools/reorder", key, body, nil)
}

// SyncConfig is the per-storefront sync configuration returned by
// GET /api/sync/config and GET/PUT /api/sync/config/:storefront.
type SyncConfig struct {
	ID           string  `json:"id"`
	Storefront   string  `json:"storefront"`
	Frequency    string  `json:"frequency"`
	LastSyncedAt *string `json:"last_synced_at"`
	IsConfigured bool    `json:"is_configured"`
	CreatedAt    string  `json:"created_at"`
	UpdatedAt    string  `json:"updated_at"`
}

// SyncStatus is the real-time sync state for a single storefront, returned by
// GET /api/sync/:storefront/status.
type SyncStatus struct {
	Storefront        string  `json:"storefront"`
	IsSyncing         bool    `json:"is_syncing"`
	LastSyncedAt      *string `json:"last_synced_at"`
	ActiveJobID       *string `json:"active_job_id"`
	ExternalGameCount int     `json:"external_game_count"`
}

// SyncTriggerResult is the response from POST /api/sync/:storefront.
type SyncTriggerResult struct {
	Message    string `json:"message"`
	JobID      string `json:"job_id"`
	Storefront string `json:"storefront"`
	Status     string `json:"status"`
}

// ListSyncConfigs returns the sync configuration for all storefronts.
// The response envelope is {"configs":[...],"total":N}; only the slice is returned.
func (c *Client) ListSyncConfigs(key string) ([]SyncConfig, error) {
	var env struct {
		Configs []SyncConfig `json:"configs"`
		Total   int          `json:"total"`
	}
	if err := c.doBearer(http.MethodGet, "/api/sync/config", key, nil, &env); err != nil {
		return nil, err
	}
	return env.Configs, nil
}

// GetSyncConfig returns the sync configuration for a single storefront.
func (c *Client) GetSyncConfig(key, storefront string) (*SyncConfig, error) {
	var out SyncConfig
	if err := c.doBearer(http.MethodGet, "/api/sync/config/"+url.PathEscape(storefront), key, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// UpdateSyncConfig sets the sync frequency for a storefront and returns the
// updated configuration.
func (c *Client) UpdateSyncConfig(key, storefront, frequency string) (*SyncConfig, error) {
	var out SyncConfig
	body := map[string]string{"frequency": frequency}
	if err := c.doBearer(http.MethodPut, "/api/sync/config/"+url.PathEscape(storefront), key, body, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// GetSyncStatus returns the real-time sync state for a storefront.
func (c *Client) GetSyncStatus(key, storefront string) (*SyncStatus, error) {
	var out SyncStatus
	if err := c.doBearer(http.MethodGet, "/api/sync/"+url.PathEscape(storefront)+"/status", key, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// TriggerSync starts a sync job for the given storefront.
func (c *Client) TriggerSync(key, storefront string) (*SyncTriggerResult, error) {
	var out SyncTriggerResult
	if err := c.doBearer(http.MethodPost, "/api/sync/"+url.PathEscape(storefront), key, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ConnectStorefront configures the connection credentials for a storefront.
// The response shape varies per storefront, so it is returned as a raw map.
func (c *Client) ConnectStorefront(key, storefront string, body map[string]string) (map[string]any, error) {
	var out map[string]any
	if err := c.doBearer(http.MethodPut, "/api/sync/"+url.PathEscape(storefront)+"/connection", key, body, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// DisconnectStorefront removes the connection credentials for a storefront.
func (c *Client) DisconnectStorefront(key, storefront string) error {
	return c.doBearer(http.MethodDelete, "/api/sync/"+url.PathEscape(storefront)+"/connection", key, nil, nil)
}

// ResetSyncData deletes all synced data for a storefront.
func (c *Client) ResetSyncData(key, storefront string) error {
	return c.doBearer(http.MethodDelete, "/api/sync/"+url.PathEscape(storefront)+"/data", key, nil, nil)
}

// FilterOptions mirrors the GET /api/user-games/filter-options response: the
// distinct facet values present in the caller's library.
type FilterOptions struct {
	Genres             []string `json:"genres"`
	GameModes          []string `json:"game_modes"`
	Themes             []string `json:"themes"`
	PlayerPerspectives []string `json:"player_perspectives"`
}

// GetFilterOptions fetches the distinct genre/game-mode/theme/perspective values
// present in the caller's library, for filter discovery.
func (c *Client) GetFilterOptions(key string) (*FilterOptions, error) {
	var out FilterOptions
	if err := c.doBearer(http.MethodGet, "/api/user-games/filter-options", key, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// Storefront is one entry from GET /api/platforms/storefronts/simple-list.
type Storefront struct {
	Name        string `json:"name"`
	DisplayName string `json:"display_name"`
}

// ListStorefronts returns the storefronts seeded on the server (name + display
// name), the valid values for the --storefront filter.
func (c *Client) ListStorefronts(key string) ([]Storefront, error) {
	var out []Storefront
	if err := c.doBearer(http.MethodGet, "/api/platforms/storefronts/simple-list", key, nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// SimplePlatform is one entry from GET /api/platforms/simple-list.
type SimplePlatform struct {
	Name        string `json:"name"`
	DisplayName string `json:"display_name"`
}

// ListPlatforms returns the platforms seeded on the server (name + display
// name), the valid values for a platform slug (e.g. on game add/edit/acquire).
func (c *Client) ListPlatforms(key string) ([]SimplePlatform, error) {
	var out []SimplePlatform
	if err := c.doBearer(http.MethodGet, "/api/platforms/simple-list", key, nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// ExternalGame is one external game entry as returned by
// GET /api/sync/:storefront/external-games.
type ExternalGame struct {
	ID                         string   `json:"id"`
	Storefront                 string   `json:"storefront"`
	ExternalID                 string   `json:"external_id"`
	Title                      string   `json:"title"`
	ResolvedIgdbID             *int     `json:"resolved_igdb_id"`
	IgdbTitle                  *string  `json:"igdb_title"`
	IsSkipped                  bool     `json:"is_skipped"`
	IsAvailable                bool     `json:"is_available"`
	IsSubscription             bool     `json:"is_subscription"`
	HasUserGame                bool     `json:"has_user_game"`
	UserGameID                 *string  `json:"user_game_id"`
	UserGameOtherPlatformCount int      `json:"user_game_other_platform_count"`
	SyncStatus                 string   `json:"sync_status"`
	FailedJobItemID            *string  `json:"failed_job_item_id"`
	Platforms                  []string `json:"platforms"`
	StoreURL                   *string  `json:"store_url"`
}

// SetupBackupManifest is the manifest sub-object of a setup-zone backup entry.
type SetupBackupManifest struct {
	CreatedAt        string `json:"created_at"`
	AppVersion       string `json:"app_version"`
	MigrationVersion string `json:"migration_version"`
	BackupType       string `json:"backup_type"`
	Stats            struct {
		Users int `json:"users"`
		Games int `json:"games"`
		Tags  int `json:"tags"`
	} `json:"stats"`
}

// SetupBackupEntry is one candidate archive from GET /api/auth/setup/backups.
type SetupBackupEntry struct {
	Filename   string               `json:"filename"`
	SizeBytes  int64                `json:"size_bytes"`
	ModTime    string               `json:"mtime"`
	Restorable bool                 `json:"restorable"`
	Reason     string               `json:"reason,omitempty"`
	Manifest   *SetupBackupManifest `json:"manifest,omitempty"`
}

// SetupListBackups lists candidate on-disk backup archives during initial
// setup via GET /api/auth/setup/backups. The endpoint is unauthenticated
// (pre-bootstrap), so no API key is sent.
func (c *Client) SetupListBackups() ([]SetupBackupEntry, error) {
	var env struct {
		Backups []SetupBackupEntry `json:"backups"`
	}
	if err := c.doBearer(http.MethodGet, "/api/auth/setup/backups", "", nil, &env); err != nil {
		return nil, err
	}
	return env.Backups, nil
}

// SetupRestoreFromDisk restores a fresh instance from a named on-disk backup
// via POST /api/auth/setup/restore/disk. Unauthenticated (pre-bootstrap).
func (c *Client) SetupRestoreFromDisk(filename string) error {
	return c.doBearer(http.MethodPost, "/api/auth/setup/restore/disk", "", map[string]string{"filename": filename}, nil)
}
