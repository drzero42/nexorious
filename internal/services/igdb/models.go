package igdb

import "errors"

// Sentinel errors for IGDB service operations.
var (
	ErrIGDBNotConfigured = errors.New("IGDB credentials not configured")
	ErrGameNotFound      = errors.New("game not found in IGDB")
	ErrTwitchAuth        = errors.New("Twitch authentication failed")
)

// GameMetadata is the internal representation of an IGDB game result.
type GameMetadata struct {
	IgdbID                     int
	IgdbSlug                   string
	Title                      string
	Description                *string
	Genre                      *string
	Developer                  *string
	Publisher                  *string
	ReleaseDate                *string  // ISO date string "YYYY-MM-DD"
	CoverArtURL                *string
	RatingAverage              *float64
	RatingCount                *int32
	EstimatedPlaytimeHours     *int32
	HowlongtobeatMain         *float64
	HowlongtobeatExtra        *float64
	HowlongtobeatCompletionist *float64
	PlatformIDs                []int
	PlatformNames              []string
	GameModes                  *string
	Themes                     *string
	PlayerPerspectives         *string
}

// twitchTokenResponse matches the JSON response from Twitch OAuth.
type twitchTokenResponse struct {
	AccessToken string `json:"access_token"`
	ExpiresIn   int    `json:"expires_in"`
	TokenType   string `json:"token_type"`
}

// igdbGameResponse represents a single game from the IGDB API.
type igdbGameResponse struct {
	ID               int              `json:"id"`
	Name             string           `json:"name"`
	Slug             string           `json:"slug"`
	Summary          *string          `json:"summary"`
	FirstReleaseDate *int64           `json:"first_release_date"`
	Cover            *igdbCover       `json:"cover"`
	Genres           []igdbNamedItem  `json:"genres"`
	InvolvedCompanies []igdbCompany   `json:"involved_companies"`
	Platforms        []igdbPlatform   `json:"platforms"`
	TotalRating      *float64         `json:"total_rating"`
	TotalRatingCount *int32           `json:"total_rating_count"`
	GameModes        []igdbNamedItem  `json:"game_modes"`
	Themes           []igdbNamedItem  `json:"themes"`
	PlayerPerspectives []igdbNamedItem `json:"player_perspectives"`
}

type igdbCover struct {
	ImageID string `json:"image_id"`
}

type igdbNamedItem struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

type igdbCompany struct {
	Company   igdbNamedItem `json:"company"`
	Developer bool          `json:"developer"`
	Publisher bool          `json:"publisher"`
}

type igdbPlatform struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}
