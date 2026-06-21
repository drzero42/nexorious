package models

import (
	"encoding/json"
	"time"

	"github.com/uptrace/bun"
)

type Game struct {
	bun.BaseModel `bun:"table:games"`

	ID                         int32      `bun:"id,pk"                          json:"id"`
	Title                      string     `bun:"title,notnull"                  json:"title"`
	Description                *string    `bun:"description"                    json:"description"`
	Genre                      *string    `bun:"genre"                          json:"genre"`
	Developer                  *string    `bun:"developer"                      json:"developer"`
	Publisher                  *string    `bun:"publisher"                      json:"publisher"`
	ReleaseDate                *time.Time `bun:"release_date"                   json:"release_date"`
	CoverArtUrl                *string    `bun:"cover_art_url"                  json:"cover_art_url"`
	RatingAverage              *float64   `bun:"rating_average"                 json:"rating_average"`
	RatingCount                *int32     `bun:"rating_count"                   json:"rating_count"`
	HowlongtobeatMain          *float64   `bun:"howlongtobeat_main"             json:"howlongtobeat_main"`
	HowlongtobeatExtra         *float64   `bun:"howlongtobeat_extra"            json:"howlongtobeat_extra"`
	HowlongtobeatCompletionist *float64   `bun:"howlongtobeat_completionist"    json:"howlongtobeat_completionist"`
	IgdbSlug                   *string    `bun:"igdb_slug"                      json:"igdb_slug"`
	IgdbPlatformIds            *string    `bun:"igdb_platform_ids"              json:"igdb_platform_ids"`
	IgdbPlatformNames          *string    `bun:"igdb_platform_names"            json:"igdb_platform_names"`
	GameModes                  *string    `bun:"game_modes"                     json:"game_modes"`
	Themes                     *string    `bun:"themes"                         json:"themes"`
	PlayerPerspectives         *string    `bun:"player_perspectives"            json:"player_perspectives"`
	GameMetadata               *string    `bun:"game_metadata"                  json:"game_metadata"`
	LastUpdated                time.Time  `bun:"last_updated,notnull"           json:"last_updated"`
	CreatedAt                  time.Time  `bun:"created_at,notnull"             json:"created_at"`
}

type User struct {
	bun.BaseModel `bun:"table:users"`

	ID           string    `bun:"id,pk"                 json:"id"`
	Username     string    `bun:"username,notnull"      json:"username"`
	PasswordHash string    `bun:"password_hash,notnull" json:"password_hash"`
	IsActive     bool      `bun:"is_active,notnull"     json:"is_active"`
	IsAdmin      bool      `bun:"is_admin,notnull"      json:"is_admin"`
	CreatedAt    time.Time `bun:"created_at,notnull"    json:"created_at"`
	UpdatedAt    time.Time `bun:"updated_at,notnull"    json:"updated_at"`
}

type UserSession struct {
	bun.BaseModel `bun:"table:user_sessions"`

	ID            string     `bun:"id,pk"                      json:"id"`
	UserID        string     `bun:"user_id,notnull"            json:"user_id"`
	SessionIDHash string     `bun:"session_id_hash,notnull"    json:"-"`
	UserAgent     *string    `bun:"user_agent"                 json:"user_agent"`
	IpAddress     *string    `bun:"ip_address"                 json:"ip_address"`
	CreatedAt     time.Time  `bun:"created_at,notnull"         json:"created_at"`
	ExpiresAt     time.Time  `bun:"expires_at,notnull"         json:"expires_at"`
	LastUsedAt    *time.Time `bun:"last_used_at"               json:"last_used_at"`
}

type APIKey struct {
	bun.BaseModel `bun:"table:api_keys"`

	ID         string     `bun:"id,pk"              json:"id"`
	UserID     string     `bun:"user_id,notnull"    json:"user_id"`
	Name       string     `bun:"name,notnull"       json:"name"`
	KeyHash    string     `bun:"key_hash,notnull"   json:"-"`
	Scopes     string     `bun:"scopes,notnull"     json:"scopes"`
	LastUsedAt *time.Time `bun:"last_used_at"       json:"last_used_at"`
	CreatedAt  time.Time  `bun:"created_at,notnull" json:"created_at"`
	ExpiresAt  *time.Time `bun:"expires_at"         json:"expires_at"`
	RevokedAt  *time.Time `bun:"revoked_at"         json:"revoked_at"`
}

type UserGame struct {
	bun.BaseModel `bun:"table:user_games"`

	ID             string    `bun:"id,pk"              json:"id"`
	UserID         string    `bun:"user_id,notnull"    json:"user_id"`
	GameID         int32     `bun:"game_id,notnull"    json:"game_id"`
	PlayStatus     *string   `bun:"play_status"        json:"play_status"`
	PersonalRating *int32    `bun:"personal_rating"    json:"personal_rating"`
	IsLoved        bool      `bun:"is_loved,notnull"      json:"is_loved"`
	IsWishlisted   bool      `bun:"is_wishlisted,notnull" json:"is_wishlisted"`
	PersonalNotes  *string   `bun:"personal_notes"        json:"personal_notes"`
	CreatedAt      time.Time `bun:"created_at,notnull" json:"created_at"`
	UpdatedAt      time.Time `bun:"updated_at,notnull" json:"updated_at"`

	Game      *Game              `bun:"rel:belongs-to,join:game_id=id"    json:"game,omitempty"`
	Platforms []UserGamePlatform `bun:"rel:has-many,join:id=user_game_id" json:"platforms"`
	Tags      []UserGameTag      `bun:"rel:has-many,join:id=user_game_id" json:"tags"`
}

type UserGamePlatform struct {
	bun.BaseModel `bun:"table:user_game_platforms"`

	ID              string     `bun:"id,pk"                        json:"id"`
	UserGameID      string     `bun:"user_game_id,notnull"         json:"user_game_id"`
	Platform        *string    `bun:"platform"                     json:"platform"`
	Storefront      *string    `bun:"storefront"                   json:"storefront"`
	IsAvailable     bool       `bun:"is_available,notnull"         json:"is_available"`
	HoursPlayed     *float64   `bun:"hours_played"                 json:"hours_played"`
	OwnershipStatus *string    `bun:"ownership_status"             json:"ownership_status"`
	AcquiredDate    *time.Time `bun:"acquired_date"                json:"acquired_date"`
	ExternalGameID  *string    `bun:"external_game_id"             json:"external_game_id"`
	SyncFromSource  bool       `bun:"sync_from_source,notnull"     json:"sync_from_source"`
	CreatedAt       time.Time  `bun:"created_at,notnull"           json:"created_at"`
	UpdatedAt       time.Time  `bun:"updated_at,notnull"           json:"updated_at"`

	PlatformRecord   *Platform     `bun:"rel:belongs-to,join:platform=name"        json:"-"`
	StorefrontRecord *Storefront   `bun:"rel:belongs-to,join:storefront=name"      json:"-"`
	ExternalGame     *ExternalGame `bun:"rel:belongs-to,join:external_game_id=id"  json:"-"`
}

type Platform struct {
	bun.BaseModel `bun:"table:platforms"`

	Name              string       `bun:"name,pk"               json:"name"`
	DisplayName       string       `bun:"display_name,notnull"  json:"display_name"`
	Icon              *string      `bun:"icon"                  json:"icon"`
	IgdbPlatformID    *int32       `bun:"igdb_platform_id"      json:"igdb_platform_id"`
	DefaultStorefront *string      `bun:"default_storefront"    json:"default_storefront"`
	Storefronts       []Storefront `bun:"m2m:platform_storefronts,join:Platform=Storefront" json:"storefronts,omitempty"`
}

type Storefront struct {
	bun.BaseModel `bun:"table:storefronts"`

	Name        string  `bun:"name,pk"              json:"name"`
	DisplayName string  `bun:"display_name,notnull" json:"display_name"`
	Icon        *string `bun:"icon"                 json:"icon"`
	BaseUrl     *string `bun:"base_url"             json:"base_url"`
}

type PlatformStorefront struct {
	bun.BaseModel `bun:"table:platform_storefronts"`

	PlatformName   string      `bun:"platform,pk"`
	StorefrontName string      `bun:"storefront,pk"`
	Platform       *Platform   `bun:"rel:belongs-to,join:platform=name"`
	Storefront     *Storefront `bun:"rel:belongs-to,join:storefront=name"`
}

type Tag struct {
	bun.BaseModel `bun:"table:tags"`

	ID        string    `bun:"id,pk"              json:"id"`
	UserID    string    `bun:"user_id,notnull"    json:"user_id"`
	Name      string    `bun:"name,notnull"       json:"name"`
	Color     *string   `bun:"color"              json:"color"`
	CreatedAt time.Time `bun:"created_at,notnull" json:"created_at"`
	UpdatedAt time.Time `bun:"updated_at,notnull" json:"updated_at"`
}

type UserGameTag struct {
	bun.BaseModel `bun:"table:user_game_tags"`

	ID         string    `bun:"id,pk"                 json:"id"`
	UserGameID string    `bun:"user_game_id,notnull"  json:"user_game_id"`
	TagID      string    `bun:"tag_id,notnull"        json:"tag_id"`
	CreatedAt  time.Time `bun:"created_at,notnull"    json:"created_at"`

	Tag *Tag `bun:"rel:belongs-to,join:tag_id=id" json:"tag,omitempty"`
}

// ExternalGame mirrors the external_games table — one row per (user_id, storefront, external_id).
type ExternalGame struct {
	bun.BaseModel `bun:"table:external_games"`

	ID              string          `bun:"id,pk"                   json:"id"`
	UserID          string          `bun:"user_id,notnull"          json:"user_id"`
	Storefront      string          `bun:"storefront,notnull"       json:"storefront"`
	ExternalID      string          `bun:"external_id,notnull"      json:"external_id"`
	Title           string          `bun:"title,notnull"            json:"title"`
	ResolvedIGDBID  *int32          `bun:"resolved_igdb_id"         json:"resolved_igdb_id"`
	IsSkipped       bool            `bun:"is_skipped,notnull"       json:"is_skipped"`
	IsAvailable     bool            `bun:"is_available,notnull"     json:"is_available"`
	IsSubscription  bool            `bun:"is_subscription,notnull"  json:"is_subscription"`
	OwnershipStatus *string         `bun:"ownership_status"         json:"ownership_status"`
	ParentID        *string         `bun:"parent_id"                json:"parent_id,omitempty"`
	StoreLink       *string         `bun:"store_link"               json:"store_link,omitempty"`
	SourceMetadata  json.RawMessage `bun:"source_metadata"          json:"source_metadata,omitempty"`
	CreatedAt       time.Time       `bun:"created_at,notnull"       json:"created_at"`
	UpdatedAt       time.Time       `bun:"updated_at,notnull"       json:"updated_at"`

	Platforms []ExternalGamePlatform `bun:"rel:has-many,join:id=external_game_id" json:"-"`
}

// ExternalGamePlatform mirrors the external_game_platforms table.
// platform holds a canonical slug matching platforms.name.
type ExternalGamePlatform struct {
	bun.BaseModel `bun:"table:external_game_platforms"`

	ID             string    `bun:"id,pk"                    json:"id"`
	ExternalGameID string    `bun:"external_game_id,notnull" json:"external_game_id"`
	Platform       string    `bun:"platform,notnull"         json:"platform"`
	HoursPlayed    float64   `bun:"hours_played,notnull"     json:"hours_played"`
	CreatedAt      time.Time `bun:"created_at,notnull"       json:"created_at"`
}

// UserSyncConfig mirrors the user_sync_configs table.
type UserSyncConfig struct {
	bun.BaseModel `bun:"table:user_sync_configs"`

	ID                    string     `bun:"id,pk"                  json:"id"`
	UserID                string     `bun:"user_id,notnull"         json:"user_id"`
	Storefront            string     `bun:"storefront,notnull"      json:"storefront"`
	Frequency             string     `bun:"frequency,notnull"       json:"frequency"`
	StorefrontCredentials *string    `bun:"storefront_credentials"  json:"-"`
	LastSyncedAt          *time.Time `bun:"last_synced_at"          json:"last_synced_at"`
	CredentialsError      bool       `bun:"credentials_error"       json:"-"`
	CreatedAt             time.Time  `bun:"created_at,notnull"      json:"created_at"`
	UpdatedAt             time.Time  `bun:"updated_at,notnull"      json:"updated_at"`
}

// JobChange mirrors the changes table — one row per library outcome per job run
// (sync, import). The job's type is derived by joining jobs.job_type.
type JobChange struct {
	bun.BaseModel `bun:"table:changes"`

	ID             string    `bun:"id,pk"               json:"id"`
	JobID          string    `bun:"job_id,notnull"      json:"job_id"`
	UserID         string    `bun:"user_id,notnull"     json:"user_id"`
	ExternalGameID *string   `bun:"external_game_id"    json:"external_game_id"`
	UserGameID     *string   `bun:"user_game_id"        json:"user_game_id"`
	ChangeType     string    `bun:"change_type,notnull" json:"change_type"`
	Title          string    `bun:"title,notnull"       json:"title"`
	OldStatus      *string   `bun:"old_status"          json:"old_status"`
	NewStatus      *string   `bun:"new_status"          json:"new_status"`
	CreatedAt      time.Time `bun:"created_at,notnull"  json:"created_at"`
}

// UserSettings mirrors the user_settings table — one row per user, lazily
// upserted. Holds typed per-user app preferences (e.g. deal_region).
type UserSettings struct {
	bun.BaseModel `bun:"table:user_settings"`

	UserID                   string    `bun:"user_id,pk"                     json:"user_id"`
	DealRegion               string    `bun:"deal_region,notnull"            json:"deal_region"`
	LastSeenChangelogVersion *string   `bun:"last_seen_changelog_version"    json:"last_seen_changelog_version,omitempty"`
	CreatedAt                time.Time `bun:"created_at,notnull"             json:"created_at"`
	UpdatedAt                time.Time `bun:"updated_at,notnull"             json:"updated_at"`
}

// Pool is a Play Planning pool — a sibling of Tag with ordering, an optional
// saved filter, and queue membership via PoolGame (#955).
type Pool struct {
	bun.BaseModel `bun:"table:pools"`

	ID        string          `bun:"id,pk"               json:"id"`
	UserID    string          `bun:"user_id,notnull"     json:"user_id"`
	Name      string          `bun:"name,notnull"        json:"name"`
	Color     *string         `bun:"color"               json:"color"`
	Position  int             `bun:"position,notnull"    json:"position"`
	Filter    json.RawMessage `bun:"filter,type:jsonb"   json:"filter"`
	CreatedAt time.Time       `bun:"created_at,notnull"  json:"created_at"`
	UpdatedAt time.Time       `bun:"updated_at,notnull"  json:"updated_at"`
}

// PoolGame is a pool membership row. position IS NULL = Candidate;
// position NOT NULL = queued (Up Next). Unique per (pool_id, user_game_id).
type PoolGame struct {
	bun.BaseModel `bun:"table:pool_games"`

	ID         string    `bun:"id,pk"                 json:"id"`
	PoolID     string    `bun:"pool_id,notnull"       json:"pool_id"`
	UserGameID string    `bun:"user_game_id,notnull"  json:"user_game_id"`
	Position   *int      `bun:"position"              json:"position"`
	CreatedAt  time.Time `bun:"created_at,notnull"    json:"created_at"`
}
