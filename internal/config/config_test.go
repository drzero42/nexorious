package config_test

import (
	"os"
	"testing"

	"github.com/drzero42/nexorious/internal/config"
)

func TestLoad_DatabaseURLFromIndividualVars(t *testing.T) {
	// Clear DATABASE_URL so the fallback path is exercised.
	t.Setenv("DATABASE_URL", "")
	t.Setenv("DB_HOST", "db.example.com")
	t.Setenv("DB_PORT", "5433")
	t.Setenv("DB_USER", "myuser")
	t.Setenv("DB_PASSWORD", "p@ss word!")
	t.Setenv("DB_NAME", "mydb")
	// Required fields.
	t.Setenv("SECRET_KEY", "testsecretkey")
	t.Setenv("DB_ENCRYPTION_KEY", "test-db-encryption-key-32-bytes!!")
	t.Setenv("IGDB_CLIENT_ID", "testclientid")
	t.Setenv("IGDB_CLIENT_SECRET", "testclientsecret")

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	// Password and user must be percent-encoded; special chars in password.
	want := "postgresql://myuser:p%40ss%20word%21@db.example.com:5433/mydb"
	if cfg.DatabaseURL != want {
		t.Errorf("DatabaseURL = %q; want %q", cfg.DatabaseURL, want)
	}
}

func TestLoad_DatabaseURLExplicit(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgresql://override:pass@host/db")
	t.Setenv("SECRET_KEY", "testsecretkey")
	t.Setenv("DB_ENCRYPTION_KEY", "test-db-encryption-key-32-bytes!!")
	t.Setenv("IGDB_CLIENT_ID", "testclientid")
	t.Setenv("IGDB_CLIENT_SECRET", "testclientsecret")

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	const wantURL = "postgresql://override:pass@host/db"
	if cfg.DatabaseURL != wantURL {
		t.Errorf("DatabaseURL = %q; want %q", cfg.DatabaseURL, wantURL)
	}
}

func TestLoad_RequiredFieldsMissing(t *testing.T) {
	// SECRET_KEY and DB_ENCRYPTION_KEY are both required.
	t.Setenv("DB_ENCRYPTION_KEY", "test-db-encryption-key-32-bytes!!")
	saved := os.Getenv("SECRET_KEY")
	os.Unsetenv("SECRET_KEY") //nolint:errcheck
	t.Cleanup(func() {
		if saved != "" {
			os.Setenv("SECRET_KEY", saved) //nolint:errcheck
		} else {
			os.Unsetenv("SECRET_KEY") //nolint:errcheck
		}
	})

	_, err := config.Load()
	if err == nil {
		t.Fatal("expected error when SECRET_KEY is missing, got nil")
	}
}

func TestLoad_SucceedsWithoutIGDBVars(t *testing.T) {
	t.Setenv("SECRET_KEY", "testsecretkey")
	t.Setenv("DB_ENCRYPTION_KEY", "test-db-encryption-key-32-bytes!!")
	// Explicitly unset IGDB vars to ensure they're not inherited.
	os.Unsetenv("IGDB_CLIENT_ID")     //nolint:errcheck
	os.Unsetenv("IGDB_CLIENT_SECRET") //nolint:errcheck

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load() should succeed without IGDB vars, got error: %v", err)
	}
	if cfg.IGDBClientID != "" {
		t.Errorf("IGDBClientID = %q; want empty string", cfg.IGDBClientID)
	}
	if cfg.IGDBClientSecret != "" {
		t.Errorf("IGDBClientSecret = %q; want empty string", cfg.IGDBClientSecret)
	}
}

func TestLoad_Defaults(t *testing.T) {
	t.Setenv("SECRET_KEY", "testsecretkey")
	t.Setenv("DB_ENCRYPTION_KEY", "test-db-encryption-key-32-bytes!!")
	t.Setenv("IGDB_CLIENT_ID", "testclientid")
	t.Setenv("IGDB_CLIENT_SECRET", "testclientsecret")

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if cfg.Port != 8000 {
		t.Errorf("Port = %d; want 8000", cfg.Port)
	}
	if cfg.WorkerCount != 4 {
		t.Errorf("WorkerCount = %d; want 4", cfg.WorkerCount)
	}
	if cfg.AccessTokenExpireMinutes != 15 {
		t.Errorf("AccessTokenExpireMinutes = %d; want 15", cfg.AccessTokenExpireMinutes)
	}
	if cfg.RateLimiterBackend != "local" {
		t.Errorf("RateLimiterBackend = %q; want local", cfg.RateLimiterBackend)
	}
}
