package config_test

import (
	"os"
	"testing"

	"github.com/drzero42/nexorious-go/internal/config"
)

// Alias so tests call Load() without the package prefix.
var Load = config.Load

func TestLoad_DatabaseURLFromIndividualVars(t *testing.T) {
	// Clear DATABASE_URL so the fallback path is exercised.
	os.Unsetenv("DATABASE_URL")
	os.Setenv("DB_HOST", "db.example.com")
	os.Setenv("DB_PORT", "5433")
	os.Setenv("DB_USER", "myuser")
	os.Setenv("DB_PASSWORD", "p@ss word!")
	os.Setenv("DB_NAME", "mydb")
	// Required fields.
	os.Setenv("SECRET_KEY", "testsecretkey")
	os.Setenv("IGDB_CLIENT_ID", "testclientid")
	os.Setenv("IGDB_CLIENT_SECRET", "testclientsecret")
	t.Cleanup(func() {
		for _, k := range []string{
			"DB_HOST", "DB_PORT", "DB_USER", "DB_PASSWORD", "DB_NAME",
			"SECRET_KEY", "IGDB_CLIENT_ID", "IGDB_CLIENT_SECRET",
		} {
			os.Unsetenv(k)
		}
	})

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	// Password and user must be percent-encoded; special chars in password.
	want := "postgresql://myuser:p%40ss+word%21@db.example.com:5433/mydb"
	if cfg.DatabaseURL != want {
		t.Errorf("DatabaseURL = %q; want %q", cfg.DatabaseURL, want)
	}
}

func TestLoad_DatabaseURLExplicit(t *testing.T) {
	os.Setenv("DATABASE_URL", "postgresql://override:pass@host/db")
	os.Setenv("SECRET_KEY", "testsecretkey")
	os.Setenv("IGDB_CLIENT_ID", "testclientid")
	os.Setenv("IGDB_CLIENT_SECRET", "testclientsecret")
	t.Cleanup(func() {
		for _, k := range []string{
			"DATABASE_URL", "SECRET_KEY", "IGDB_CLIENT_ID", "IGDB_CLIENT_SECRET",
		} {
			os.Unsetenv(k)
		}
	})

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if cfg.DatabaseURL != "postgresql://override:pass@host/db" {
		t.Errorf("DatabaseURL = %q; want explicit value", cfg.DatabaseURL)
	}
}

func TestLoad_RequiredFieldsMissing(t *testing.T) {
	os.Unsetenv("SECRET_KEY")
	os.Unsetenv("IGDB_CLIENT_ID")
	os.Unsetenv("IGDB_CLIENT_SECRET")

	_, err := Load()
	if err == nil {
		t.Fatal("expected error when required fields are missing, got nil")
	}
}

func TestLoad_Defaults(t *testing.T) {
	os.Setenv("SECRET_KEY", "testsecretkey")
	os.Setenv("IGDB_CLIENT_ID", "testclientid")
	os.Setenv("IGDB_CLIENT_SECRET", "testclientsecret")
	t.Cleanup(func() {
		for _, k := range []string{
			"SECRET_KEY", "IGDB_CLIENT_ID", "IGDB_CLIENT_SECRET",
		} {
			os.Unsetenv(k)
		}
	})

	cfg, err := Load()
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
