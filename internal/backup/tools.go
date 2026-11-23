package backup

import (
	"context"
	"fmt"
	"net/url"
	"os/exec"
	"time"
)

var (
	pgDumpAvailable bool
	psqlAvailable   bool
)

func CheckTools() {
	_, err := exec.LookPath("pg_dump")
	pgDumpAvailable = err == nil
	_, err = exec.LookPath("psql")
	psqlAvailable = err == nil
}

func PgDumpAvailable() bool { return pgDumpAvailable }
func PsqlAvailable() bool   { return psqlAvailable }

type DBConnParams struct {
	Host     string
	Port     string
	User     string
	Password string
	DBName   string
}

func ParseDatabaseURL(databaseURL string) (DBConnParams, error) {
	u, err := url.Parse(databaseURL)
	if err != nil {
		return DBConnParams{}, fmt.Errorf("parse database URL: %w", err)
	}
	port := u.Port()
	if port == "" {
		port = "5432"
	}
	password, _ := u.User.Password()
	dbName := ""
	if len(u.Path) > 1 {
		dbName = u.Path[1:]
	}
	return DBConnParams{
		Host:     u.Hostname(),
		Port:     port,
		User:     u.User.Username(),
		Password: password,
		DBName:   dbName,
	}, nil
}

// pgDumpExclusions keep in-flight work and restore bookkeeping out of a backup
// (#1341). River's queue and leader rows describe work of the moment, not data:
// restored, they would run again against a database that has moved on. Their
// tables stay in the dump (empty), and so does river_job's id sequence, so job
// ids keep increasing after a restore. nexorious_restore holds the marker of a
// restore in progress, which belongs to the instance, not to its data.
// Patterns that match nothing are not an error.
var pgDumpExclusions = []string{
	"--exclude-table-data=public.river_job",
	"--exclude-table-data=public.river_leader",
	"--exclude-schema=nexorious_restore",
}

// toolTimeoutBase is the least time pg_dump or a psql load gets;
// toolTimeoutPerMiB is added for every MiB of data, so a large database gets
// proportionally longer. The deadline only guards against a hung tool.
const (
	toolTimeoutBase   = 5 * time.Minute
	toolTimeoutPerMiB = 2 * time.Second
)

// toolTimeout scales a pg_dump or psql deadline with the size of the data.
func toolTimeout(sizeBytes int64) time.Duration {
	return toolTimeoutBase + time.Duration(sizeBytes/(1<<20))*toolTimeoutPerMiB
}

// RunPgDump dumps the database into outputPath as plain SQL, giving up after
// timeout.
func RunPgDump(conn DBConnParams, outputPath string, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	args := append([]string{
		"--format=plain", "--no-owner", "--no-acl",
		"--host=" + conn.Host, "--port=" + conn.Port,
		"--username=" + conn.User, "--dbname=" + conn.DBName,
		"--file=" + outputPath,
	}, pgDumpExclusions...)
	cmd := exec.CommandContext(ctx, "pg_dump", args...) //nolint:gosec // fixed binary; only validated DB-connection flag values are interpolated, not the command
	cmd.Env = append(cmd.Environ(), "PGPASSWORD="+conn.Password)
	output, err := cmd.CombinedOutput()
	if err != nil {
		if ctx.Err() != nil {
			return fmt.Errorf("pg_dump timed out after %s: %w\noutput: %s", timeout, err, output)
		}
		return fmt.Errorf("pg_dump failed: %w\noutput: %s", err, output)
	}
	return nil
}

var RunPsqlCommand = func(conn DBConnParams, command string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "psql", //nolint:gosec // fixed binary; only validated DB-connection flag values are interpolated, not the command
		"--host="+conn.Host, "--port="+conn.Port,
		"--username="+conn.User, "--dbname="+conn.DBName,
		"--command="+command,
	)
	cmd.Env = append(cmd.Environ(), "PGPASSWORD="+conn.Password)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("psql command failed: %w\noutput: %s", err, output)
	}
	return nil
}
