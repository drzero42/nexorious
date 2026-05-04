package config

import (
	"fmt"
	"net/url"

	"github.com/caarlos0/env/v11"
)

// Config holds all application configuration parsed from environment variables.
type Config struct {
	// -------------------------------------------------------------------------
	// Database
	// -------------------------------------------------------------------------

	// DatabaseURL takes priority when non-empty. When absent, the URL is
	// constructed from the individual DB_* vars below.
	DatabaseURL string `env:"DATABASE_URL"`

	// Individual DB connection vars — fallback when DATABASE_URL is not set.
	// Defaults match the dev URL: postgresql://nexorious:nexorious@localhost:5432/nexorious
	DbHost     string `env:"DB_HOST"     envDefault:"localhost"`
	DbPort     int    `env:"DB_PORT"     envDefault:"5432"`
	DbUser     string `env:"DB_USER"     envDefault:"nexorious"`
	DbPassword string `env:"DB_PASSWORD" envDefault:"nexorious"`
	DbName     string `env:"DB_NAME"     envDefault:"nexorious"`

	// -------------------------------------------------------------------------
	// Security
	// -------------------------------------------------------------------------

	// SecretKey is used for JWT signing and credential encryption.
	SecretKey string `env:"SECRET_KEY,required"`

	// JWT lifetimes. Go port uses 15 min access (Python defaulted to 30).
	AccessTokenExpireMinutes int `env:"ACCESS_TOKEN_EXPIRE_MINUTES" envDefault:"15"`
	RefreshTokenExpireDays   int `env:"REFRESH_TOKEN_EXPIRE_DAYS"   envDefault:"30"`

	// -------------------------------------------------------------------------
	// IGDB
	// -------------------------------------------------------------------------

	IGDBClientID          string  `env:"IGDB_CLIENT_ID,required"`
	IGDBClientSecret      string  `env:"IGDB_CLIENT_SECRET,required"`
	IGDBAccessToken       string  `env:"IGDB_ACCESS_TOKEN"`
	IGDBRequestsPerSecond float64 `env:"IGDB_REQUESTS_PER_SECOND" envDefault:"4.0"`
	IGDBBurstCapacity     int     `env:"IGDB_BURST_CAPACITY"      envDefault:"8"`
	IGDBMaxRetries        int     `env:"IGDB_MAX_RETRIES"         envDefault:"3"`
	IGDBBackoffFactor     float64 `env:"IGDB_BACKOFF_FACTOR"      envDefault:"1.0"`

	// -------------------------------------------------------------------------
	// Steam
	// -------------------------------------------------------------------------

	SteamRequestsPerSecond float64 `env:"STEAM_REQUESTS_PER_SECOND" envDefault:"4.0"`
	SteamBurstCapacity     int     `env:"STEAM_BURST_CAPACITY"      envDefault:"10"`

	// -------------------------------------------------------------------------
	// Storage
	// -------------------------------------------------------------------------

	StoragePath    string `env:"STORAGE_PATH"     envDefault:"./storage"`
	BackupPath     string `env:"BACKUP_PATH"      envDefault:"./storage/backups"`
	TempStorageDir string `env:"TEMP_STORAGE_DIR" envDefault:"/tmp/nexorious_uploads"`

	// -------------------------------------------------------------------------
	// Application
	// -------------------------------------------------------------------------

	Port     int    `env:"PORT"      envDefault:"8000"`
	LogLevel string `env:"LOG_LEVEL" envDefault:"info"`
	Debug    bool   `env:"DEBUG"     envDefault:"false"`

	// CORSOrigins is only needed in development; production is same-origin.
	CORSOrigins []string `env:"CORS_ORIGINS" envSeparator:","`

	// -------------------------------------------------------------------------
	// Workers
	// -------------------------------------------------------------------------

	WorkerCount int `env:"WORKER_COUNT" envDefault:"4"`

	// -------------------------------------------------------------------------
	// Scheduler
	// -------------------------------------------------------------------------

	// MetadataRefreshInterval is a Go duration string (e.g. "24h").
	// The backup schedule is stored in the backup_config table, not here.
	MetadataRefreshInterval string `env:"METADATA_REFRESH_INTERVAL" envDefault:"24h"`

	// -------------------------------------------------------------------------
	// Rate limiter
	// -------------------------------------------------------------------------

	// RateLimiterBackend selects the rate limiter implementation: "local" or "postgres".
	RateLimiterBackend string `env:"RATE_LIMITER_BACKEND" envDefault:"local"`
}

// Load parses Config from environment variables and assembles DatabaseURL
// when DATABASE_URL is not set explicitly.
func Load() (*Config, error) {
	cfg := &Config{}
	if err := env.Parse(cfg); err != nil {
		return nil, fmt.Errorf("config: %w", err)
	}
	cfg.resolveDatabaseURL()
	return cfg, nil
}

// resolveDatabaseURL builds DatabaseURL from individual DB_* vars when
// DATABASE_URL is not set. Special characters in user/password are
// percent-encoded, matching Python's urllib.parse.quote(value, safe='').
func (c *Config) resolveDatabaseURL() {
	if c.DatabaseURL != "" {
		return
	}
	user := url.QueryEscape(c.DbUser)
	pass := url.QueryEscape(c.DbPassword)
	c.DatabaseURL = fmt.Sprintf(
		"postgresql://%s:%s@%s:%d/%s",
		user, pass, c.DbHost, c.DbPort, c.DbName,
	)
}
