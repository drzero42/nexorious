// Package clicfg reads and writes the Nexorious CLI's local config file.
//
// The file lives at $XDG_CONFIG_HOME/nexorious/config.yaml (falling back to
// ~/.config/nexorious/config.yaml) and stores a live API key, so it is written
// with owner-only permissions.
package clicfg

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

const defaultProfile = "default"

// Profile holds the credentials for one server.
type Profile struct {
	URL      string `yaml:"url"`
	Username string `yaml:"username"`
	KeyName  string `yaml:"key_name"`
	KeyID    string `yaml:"key_id"`
	Key      string `yaml:"key"`
}

// Config is the whole config file: a set of named profiles and a pointer to the
// active one. Only a single profile is created today, but the schema leaves room
// for multiple without a breaking format change.
type Config struct {
	Current  string             `yaml:"current"`
	Profiles map[string]Profile `yaml:"profiles"`
}

// Path returns the config file path, honoring XDG_CONFIG_HOME.
func Path() (string, error) {
	base := os.Getenv("XDG_CONFIG_HOME")
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("resolve home dir: %w", err)
		}
		base = filepath.Join(home, ".config")
	}
	return filepath.Join(base, "nexorious", "config.yaml"), nil
}

// Load reads the config file. A missing file yields an empty Config rather than
// an error so first-time `login` works.
func Load() (*Config, error) {
	p, err := Path()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(p)
	if err != nil {
		if os.IsNotExist(err) {
			return &Config{Profiles: map[string]Profile{}}, nil
		}
		return nil, fmt.Errorf("read config: %w", err)
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	if cfg.Profiles == nil {
		cfg.Profiles = map[string]Profile{}
	}
	return &cfg, nil
}

// Save writes the config atomically (temp file + rename) with 0600 perms in a
// 0700 directory.
func Save(cfg *Config) error {
	p, err := Path()
	if err != nil {
		return err
	}
	dir := filepath.Dir(p)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}

	tmp, err := os.CreateTemp(dir, "config-*.yaml.tmp")
	if err != nil {
		return fmt.Errorf("create temp config: %w", err)
	}
	tmpName := tmp.Name()
	//nolint:errcheck // best-effort cleanup; the rename below is what matters
	defer os.Remove(tmpName)

	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("chmod temp config: %w", err)
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write temp config: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temp config: %w", err)
	}
	if err := os.Rename(tmpName, p); err != nil {
		return fmt.Errorf("rename config: %w", err)
	}
	return nil
}

// CurrentProfile returns the active profile and whether it exists.
func (c *Config) CurrentProfile() (Profile, bool) {
	name := c.Current
	if name == "" {
		name = defaultProfile
	}
	p, ok := c.Profiles[name]
	return p, ok
}

// CurrentName returns the active profile name, defaulting to "default".
func (c *Config) CurrentName() string {
	if c.Current == "" {
		return defaultProfile
	}
	return c.Current
}

// SetProfile stores a profile and marks it current.
func (c *Config) SetProfile(name string, p Profile) {
	if name == "" {
		name = defaultProfile
	}
	if c.Profiles == nil {
		c.Profiles = map[string]Profile{}
	}
	c.Profiles[name] = p
	c.Current = name
}
