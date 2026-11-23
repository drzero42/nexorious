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

func RunPgDump(conn DBConnParams, outputPath string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Second)
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
