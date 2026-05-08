package models

import (
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
	EstimatedPlaytimeHours     *int32     `bun:"estimated_playtime_hours"       json:"estimated_playtime_hours"`
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
	Preferences  string    `bun:"preferences,notnull"   json:"preferences"`
	CreatedAt    time.Time `bun:"created_at,notnull"    json:"created_at"`
	UpdatedAt    time.Time `bun:"updated_at,notnull"    json:"updated_at"`
}

type UserSession struct {
	bun.BaseModel `bun:"table:user_sessions"`

	ID               string    `bun:"id,pk"                      json:"id"`
	UserID           string    `bun:"user_id,notnull"            json:"user_id"`
	TokenHash        string    `bun:"token_hash,notnull"         json:"token_hash"`
	RefreshTokenHash string    `bun:"refresh_token_hash,notnull" json:"refresh_token_hash"`
	UserAgent        *string   `bun:"user_agent"                 json:"user_agent"`
	IpAddress        *string   `bun:"ip_address"                 json:"ip_address"`
	CreatedAt        time.Time `bun:"created_at,notnull"         json:"created_at"`
	ExpiresAt        time.Time `bun:"expires_at,notnull"         json:"expires_at"`
}

type UserGame struct {
	bun.BaseModel `bun:"table:user_games"`

	ID             string    `bun:"id,pk"              json:"id"`
	UserID         string    `bun:"user_id,notnull"    json:"user_id"`
	GameID         int32     `bun:"game_id,notnull"    json:"game_id"`
	PlayStatus     *string   `bun:"play_status"        json:"play_status"`
	PersonalRating *int32    `bun:"personal_rating"    json:"personal_rating"`
	IsLoved        bool      `bun:"is_loved,notnull"   json:"is_loved"`
	HoursPlayed    *float64  `bun:"hours_played"       json:"hours_played"`
	PersonalNotes  *string   `bun:"personal_notes"     json:"personal_notes"`
	CreatedAt      time.Time `bun:"created_at,notnull" json:"created_at"`
	UpdatedAt      time.Time `bun:"updated_at,notnull" json:"updated_at"`
}

type UserGamePlatform struct {
	bun.BaseModel `bun:"table:user_game_platforms"`

	ID                     string     `bun:"id,pk"                        json:"id"`
	UserGameID             string     `bun:"user_game_id,notnull"         json:"user_game_id"`
	Platform               string     `bun:"platform,notnull"             json:"platform"`
	Storefront             string     `bun:"storefront,notnull"           json:"storefront"`
	StoreGameID            *string    `bun:"store_game_id"                json:"store_game_id"`
	StoreUrl               *string    `bun:"store_url"                    json:"store_url"`
	IsAvailable            bool       `bun:"is_available,notnull"         json:"is_available"`
	HoursPlayed            *float64   `bun:"hours_played"                 json:"hours_played"`
	OwnershipStatus        *string    `bun:"ownership_status"             json:"ownership_status"`
	AcquiredDate           *time.Time `bun:"acquired_date"                json:"acquired_date"`
	OriginalPlatformName   *string    `bun:"original_platform_name"       json:"original_platform_name"`
	OriginalStorefrontName *string    `bun:"original_storefront_name"     json:"original_storefront_name"`
	ExternalGameID         *string    `bun:"external_game_id"             json:"external_game_id"`
	SyncFromSource         bool       `bun:"sync_from_source,notnull"     json:"sync_from_source"`
	CreatedAt              time.Time  `bun:"created_at,notnull"           json:"created_at"`
	UpdatedAt              time.Time  `bun:"updated_at,notnull"           json:"updated_at"`
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
}
