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
	// DB_ENCRYPTION_KEY is required.
	saved := os.Getenv("DB_ENCRYPTION_KEY")
	os.Unsetenv("DB_ENCRYPTION_KEY") //nolint:errcheck
	t.Cleanup(func() {
		if saved != "" {
			os.Setenv("DB_ENCRYPTION_KEY", saved) //nolint:errcheck
		} else {
			os.Unsetenv("DB_ENCRYPTION_KEY") //nolint:errcheck
		}
	})

	_, err := config.Load()
	if err == nil {
		t.Fatal("expected error when DB_ENCRYPTION_KEY is missing, got nil")
	}
}

func TestLoad_SucceedsWithoutIGDBVars(t *testing.T) {
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
	if cfg.SessionExpireDays != 30 {
		t.Errorf("SessionExpireDays = %d; want 30", cfg.SessionExpireDays)
	}
	if cfg.RateLimiterBackend != "local" {
		t.Errorf("RateLimiterBackend = %q; want local", cfg.RateLimiterBackend)
	}
}

func TestLoad_ObservabilityDefaults(t *testing.T) {
	t.Setenv("DB_ENCRYPTION_KEY", "test-db-encryption-key-32-bytes!!")
	t.Setenv("IGDB_CLIENT_ID", "testclientid")
	t.Setenv("IGDB_CLIENT_SECRET", "testclientsecret")
	// Isolate from ambient OTel/pprof env; empty values fall back to envDefault.
	t.Setenv("OTEL_SERVICE_NAME", "")
	t.Setenv("OTEL_METRICS_ENABLED", "")
	t.Setenv("PPROF_ENABLED", "")
	t.Setenv("PPROF_ADDR", "")

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	if cfg.OTELServiceName != "nexorious" {
		t.Errorf("OTELServiceName = %q; want %q", cfg.OTELServiceName, "nexorious")
	}
	if !cfg.OTELMetricsEnabled {
		t.Errorf("OTELMetricsEnabled = false; want true (default on)")
	}
	if cfg.PprofEnabled {
		t.Errorf("PprofEnabled = true; want false (default off)")
	}
	if cfg.PprofAddr != "127.0.0.1:6060" {
		t.Errorf("PprofAddr = %q; want %q", cfg.PprofAddr, "127.0.0.1:6060")
	}
}

func TestLoad_ObservabilityOverrides(t *testing.T) {
	t.Setenv("DB_ENCRYPTION_KEY", "test-db-encryption-key-32-bytes!!")
	t.Setenv("IGDB_CLIENT_ID", "testclientid")
	t.Setenv("IGDB_CLIENT_SECRET", "testclientsecret")
	t.Setenv("OTEL_SERVICE_NAME", "nexorious-staging")
	t.Setenv("OTEL_METRICS_ENABLED", "false")
	t.Setenv("PPROF_ENABLED", "true")
	t.Setenv("PPROF_ADDR", "127.0.0.1:7070")

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	if cfg.OTELServiceName != "nexorious-staging" {
		t.Errorf("OTELServiceName = %q; want %q", cfg.OTELServiceName, "nexorious-staging")
	}
	if cfg.OTELMetricsEnabled {
		t.Errorf("OTELMetricsEnabled = true; want false")
	}
	if !cfg.PprofEnabled {
		t.Errorf("PprofEnabled = false; want true")
	}
	if cfg.PprofAddr != "127.0.0.1:7070" {
		t.Errorf("PprofAddr = %q; want %q", cfg.PprofAddr, "127.0.0.1:7070")
	}
}

func TestLoad_TracingEndpointDefaultEmpty(t *testing.T) {
	t.Setenv("DB_ENCRYPTION_KEY", "test-db-encryption-key-32-bytes!!")
	t.Setenv("IGDB_CLIENT_ID", "testclientid")
	t.Setenv("IGDB_CLIENT_SECRET", "testclientsecret")
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "") // isolate from ambient OTel env

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if cfg.OTELExporterOTLPEndpoint != "" {
		t.Errorf("OTELExporterOTLPEndpoint = %q; want empty (tracing off by default)", cfg.OTELExporterOTLPEndpoint)
	}
}

func TestLoad_TracingEndpointOverride(t *testing.T) {
	t.Setenv("DB_ENCRYPTION_KEY", "test-db-encryption-key-32-bytes!!")
	t.Setenv("IGDB_CLIENT_ID", "testclientid")
	t.Setenv("IGDB_CLIENT_SECRET", "testclientsecret")
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "http://localhost:4318")

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if cfg.OTELExporterOTLPEndpoint != "http://localhost:4318" {
		t.Errorf("OTELExporterOTLPEndpoint = %q; want %q", cfg.OTELExporterOTLPEndpoint, "http://localhost:4318")
	}
}
