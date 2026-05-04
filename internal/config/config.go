package config

import (
	"fmt"

	"github.com/caarlos0/env/v11"
)

// Config holds all application configuration parsed from environment variables.
type Config struct {
	Port     int    `env:"PORT"      envDefault:"8000"`
	LogLevel string `env:"LOG_LEVEL" envDefault:"info"`
	Debug    bool   `env:"DEBUG"     envDefault:"false"`
}

// Load parses Config from environment variables.
// Returns an error if a required variable is missing or unparseable.
func Load() (*Config, error) {
	cfg := &Config{}
	if err := env.Parse(cfg); err != nil {
		return nil, fmt.Errorf("config: %w", err)
	}
	return cfg, nil
}
