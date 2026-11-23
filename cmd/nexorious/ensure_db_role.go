package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/joho/godotenv"
	"github.com/spf13/cobra"
	"github.com/uptrace/bun"

	"github.com/drzero42/nexorious/internal/dbrole"
	"github.com/drzero42/nexorious/internal/logging"
)

// newEnsureDBRoleCmd returns the `ensure-db-role` subcommand (#1357).
//
// The bundled PostgreSQL in the Docker Compose file and the Helm chart creates
// POSTGRES_USER as a superuser. This command, run before `migrate` on every
// start, connects as that superuser and makes the DATABASE_URL role a plain
// role that owns the database, so a restored backup can't run programs on the
// database host.
func newEnsureDBRoleCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "ensure-db-role",
		Short: "Make the DATABASE_URL role a plain owner of the database, then exit",
		Long: "Connect as a PostgreSQL superuser (DB_ADMIN_USER / DB_ADMIN_PASSWORD, on the\n" +
			"host and database from DATABASE_URL) and make the DATABASE_URL role a\n" +
			"non-superuser login role that owns the database and every object in its\n" +
			"public schema. The role is created if missing and its password is set\n" +
			"from DATABASE_URL. Safe to run on every start; intended for the bundled\n" +
			"PostgreSQL in the Docker Compose file and the Helm chart.\n\n" +
			"It does not change the admin role. If the DATABASE_URL role is the admin\n" +
			"role itself, or is already a superuser, it logs a warning and leaves it.",
		Args: cobra.NoArgs,
		RunE: runEnsureDBRole,
	}
}

func runEnsureDBRole(cmd *cobra.Command, _ []string) error {
	if err := loadEnvFile(cmd); err != nil {
		return err
	}
	appURL := os.Getenv("DATABASE_URL")
	adminUser := os.Getenv("DB_ADMIN_USER")
	adminPassword := os.Getenv("DB_ADMIN_PASSWORD")
	if appURL == "" || adminUser == "" {
		return errors.New("ensure-db-role: DATABASE_URL and DB_ADMIN_USER are required")
	}

	target, adminURL, err := splitAppAndAdminURL(appURL, adminUser, adminPassword)
	if err != nil {
		return err
	}

	db := openBunDB(adminURL)
	defer func() { _ = db.Close() }()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	if err := waitForDB(ctx, db, 60*time.Second); err != nil {
		return err
	}

	res, err := dbrole.EnsureAppRole(ctx, db, target.role, target.password, target.database)
	if err != nil {
		return err
	}
	switch res.Outcome {
	case dbrole.OutcomeSkippedSameRole:
		slog.Warn("ensure-db-role: DATABASE_URL uses the admin role itself; it stays a superuser",
			"role", target.role)
	case dbrole.OutcomeSkippedPrivileged:
		slog.Warn("ensure-db-role: the DATABASE_URL role is a superuser; left unchanged",
			"role", target.role)
	default:
		slog.Info("ensure-db-role: database role ready",
			"role", target.role, "database", target.database,
			"created", res.Created, "objects_transferred", res.Transferred)
	}
	return nil
}

// warnIfPrivilegedDBRole logs a warning at startup when the server is
// connected as a role that could reach the database host (#1357). Admins also
// see it in the UI via GET /api/admin/health-warnings.
func warnIfPrivilegedDBRole(ctx context.Context, db *bun.DB) {
	check, err := dbrole.CheckConnectedRole(ctx, db)
	if err != nil {
		slog.WarnContext(ctx, "could not check the database role's privileges", logging.KeyErr, err, logging.Cat(logging.CategoryDB))
		return
	}
	if check.Privileged() {
		slog.WarnContext(ctx, "the database role is a superuser or can run programs on the database host; "+
			"a crafted backup restore could run commands there. Connect as a plain role that owns the database "+
			"(see the admin guide, \"Database role\")",
			"role", check.Role, "superuser", check.Superuser, logging.Cat(logging.CategoryConfig))
	}
}

type appTarget struct {
	role, password, database string
}

// splitAppAndAdminURL reads the application role, password and database from
// appURL, and returns a DSN for the same host and database as the admin user.
func splitAppAndAdminURL(appURL, adminUser, adminPassword string) (appTarget, string, error) {
	u, err := url.Parse(appURL)
	if err != nil {
		return appTarget{}, "", fmt.Errorf("ensure-db-role: parse DATABASE_URL: %w", err)
	}
	if u.User == nil || u.User.Username() == "" {
		return appTarget{}, "", errors.New("ensure-db-role: DATABASE_URL has no user")
	}
	database := strings.TrimPrefix(u.Path, "/")
	if database == "" {
		return appTarget{}, "", errors.New("ensure-db-role: DATABASE_URL has no database name")
	}
	pw, _ := u.User.Password()
	target := appTarget{role: u.User.Username(), password: pw, database: database}

	admin := *u
	if adminPassword != "" {
		admin.User = url.UserPassword(adminUser, adminPassword)
	} else {
		admin.User = url.User(adminUser)
	}
	return target, admin.String(), nil
}

// loadEnvFile loads --config (or ./.env when present) into the environment,
// like loadEnvAndConfig, without requiring the server's full config.
func loadEnvFile(cmd *cobra.Command) error {
	configFile, _ := cmd.Root().PersistentFlags().GetString("config") //nolint:errcheck // "config" persistent flag is always registered; cannot error
	if configFile != "" {
		if err := godotenv.Load(configFile); err != nil {
			return fmt.Errorf("load env file %q: %w", configFile, err)
		}
		return nil
	}
	if err := godotenv.Load(".env"); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("load .env: %w", err)
	}
	return nil
}
