package backup_test

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/pgdialect"
	"github.com/uptrace/bun/driver/pgdriver"

	"github.com/drzero42/nexorious/internal/backup"
)

const restoreTestSchema = "psqlrestore_t"

func requirePsqlTools(t *testing.T) backup.DBConnParams {
	t.Helper()
	backup.CheckTools()
	if !backup.PgDumpAvailable() || !backup.PsqlAvailable() {
		t.Skip("pg_dump or psql not available")
	}
	conn, err := backup.ParseDatabaseURL(testDSN)
	if err != nil {
		t.Fatalf("ParseDatabaseURL: %v", err)
	}
	dropRestoreTestSchema(t)
	t.Cleanup(func() { dropRestoreTestSchema(t) })
	return conn
}

func dropRestoreTestSchema(t *testing.T) {
	t.Helper()
	if _, err := testDB.ExecContext(context.Background(), "DROP SCHEMA IF EXISTS "+restoreTestSchema+" CASCADE"); err != nil {
		t.Fatalf("drop test schema: %v", err)
	}
}

func restoreSQL(t *testing.T, conn backup.DBConnParams, dump string) error {
	t.Helper()
	path := filepath.Join(t.TempDir(), "database.sql")
	if err := os.WriteFile(path, []byte(dump), 0o600); err != nil {
		t.Fatal(err)
	}
	return backup.RunPsqlRestore(conn, path)
}

func schemaExists(t *testing.T) bool {
	t.Helper()
	var n int
	if err := testDB.QueryRowContext(context.Background(),
		"SELECT count(*) FROM pg_namespace WHERE nspname = ?", restoreTestSchema).Scan(&n); err != nil {
		t.Fatalf("query schema: %v", err)
	}
	return n > 0
}

func TestRunPsqlRestore_RefusesBackslashCommands(t *testing.T) {
	conn := requirePsqlTools(t)
	const restricted = "backslash commands are restricted"

	cases := []struct {
		name    string
		cmd     func(marker string) string
		wantErr string
	}{
		{"shell escape", func(m string) string { return `\! touch ` + m }, restricted},
		{"output redirect", func(m string) string { return `\o ` + m + "\nSELECT 1;" }, restricted},
		{"client-side copy", func(m string) string { return `\copy (SELECT 1) TO '` + m + `'` }, restricted},
		{"gexec", func(string) string { return `SELECT 'SELECT 1' \gexec` }, restricted},
		{"nested restrict", func(string) string { return `\restrict abc` }, restricted},
		{"unrestrict with a guessed key", func(string) string { return `\unrestrict abc` }, "wrong key"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			marker := filepath.Join(t.TempDir(), "pwned")
			dump := "CREATE SCHEMA " + restoreTestSchema + ";\n" +
				"CREATE TABLE " + restoreTestSchema + ".a (x int);\n" +
				tc.cmd(marker) + "\n" +
				"CREATE TABLE " + restoreTestSchema + ".b (x int);\n"

			err := restoreSQL(t, conn, dump)
			if err == nil {
				t.Fatal("restore succeeded; want it refused")
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("error does not contain %q: %v", tc.wantErr, err)
			}
			if _, statErr := os.Stat(marker); !errors.Is(statErr, fs.ErrNotExist) {
				t.Errorf("backslash command had an effect: marker stat = %v", statErr)
			}
			if schemaExists(t) {
				t.Error("statements before the refused command were committed; want the whole load rolled back")
			}
		})
	}
}

func TestRunPsqlRestore_FailedStatementRollsBackLoad(t *testing.T) {
	conn := requirePsqlTools(t)

	dump := "CREATE SCHEMA " + restoreTestSchema + ";\n" +
		"CREATE TABLE " + restoreTestSchema + ".a (x int);\n" +
		"SELECT 1/0;\n" +
		"CREATE TABLE " + restoreTestSchema + ".b (x int);\n"

	if err := restoreSQL(t, conn, dump); err == nil {
		t.Fatal("restore succeeded; want failure on the invalid statement")
	}
	if schemaExists(t) {
		t.Error("statements before the failure were committed; want the whole load rolled back")
	}
}

// copyTestRows are values whose COPY text encoding starts with a backslash
// escape (\r, \N) — the filter must not mistake them for meta-commands.
var copyTestRows = []sql.NullString{
	{String: "\rleading carriage return", Valid: true},
	{Valid: false},
	{String: `back\slash`, Valid: true},
	{String: "plain", Valid: true},
}

func assertCopyTestRows(t *testing.T) {
	t.Helper()
	rows, err := testDB.QueryContext(context.Background(),
		"SELECT s FROM "+restoreTestSchema+".v ORDER BY id")
	if err != nil {
		t.Fatalf("query restored rows: %v", err)
	}
	defer func() { _ = rows.Close() }()
	var got []sql.NullString
	for rows.Next() {
		var s sql.NullString
		if err := rows.Scan(&s); err != nil {
			t.Fatal(err)
		}
		got = append(got, s)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if len(got) != len(copyTestRows) {
		t.Fatalf("restored %d rows, want %d", len(got), len(copyTestRows))
	}
	for i := range got {
		if got[i] != copyTestRows[i] {
			t.Errorf("row %d = %q (valid=%v), want %q (valid=%v)",
				i, got[i].String, got[i].Valid, copyTestRows[i].String, copyTestRows[i].Valid)
		}
	}
}

func TestRunPsqlRestore_CurrentPgDumpRoundTrip(t *testing.T) {
	conn := requirePsqlTools(t)
	ctx := context.Background()

	if _, err := testDB.ExecContext(ctx, "CREATE SCHEMA "+restoreTestSchema+
		"; CREATE TABLE "+restoreTestSchema+".v (s text, id int PRIMARY KEY)"); err != nil {
		t.Fatal(err)
	}
	for i, r := range copyTestRows {
		if _, err := testDB.ExecContext(ctx, "INSERT INTO "+restoreTestSchema+".v (id, s) VALUES (?, ?)", i, r); err != nil {
			t.Fatal(err)
		}
	}

	dumpPath := filepath.Join(t.TempDir(), "database.sql")
	cmd := exec.Command("pg_dump", "--format=plain", "--no-owner", "--no-acl",
		"--schema="+restoreTestSchema,
		"--host="+conn.Host, "--port="+conn.Port,
		"--username="+conn.User, "--dbname="+conn.DBName,
		"--file="+dumpPath)
	cmd.Env = append(cmd.Environ(), "PGPASSWORD="+conn.Password)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("pg_dump: %v\n%s", err, out)
	}
	dump, err := os.ReadFile(dumpPath)
	if err != nil {
		t.Fatal(err)
	}
	// The point of this test is pg_dump's own \restrict pair; fail loudly if
	// the local pg_dump no longer writes it rather than passing vacuously.
	if !strings.Contains(string(dump), "\n\\restrict ") || !strings.Contains(string(dump), "\n\\unrestrict ") {
		t.Fatalf("pg_dump output has no \\restrict/\\unrestrict pair; test needs pg_dump 18 / 17.6+")
	}
	// The value with a leading CR must appear as a COPY line starting with \r.
	if !strings.Contains(string(dump), "\n\\rleading carriage return\t0\n") {
		t.Fatalf("expected a COPY data line starting with \\r in the dump")
	}

	dropRestoreTestSchema(t)
	if err := backup.RunPsqlRestore(conn, dumpPath); err != nil {
		t.Fatalf("RunPsqlRestore: %v", err)
	}
	assertCopyTestRows(t)
}

func TestRunPsqlRestore_DumpWithoutRestrictLines(t *testing.T) {
	conn := requirePsqlTools(t)

	// Shape of a plain dump from pg_dump before 17.6 / 16.10: no \restrict pair.
	dump := "--\n-- PostgreSQL database dump\n--\n\n" +
		"SET statement_timeout = 0;\n" +
		"CREATE SCHEMA " + restoreTestSchema + ";\n" +
		"CREATE TABLE " + restoreTestSchema + ".v (s text, id int PRIMARY KEY);\n" +
		"COPY " + restoreTestSchema + ".v (s, id) FROM stdin;\n" +
		"\\rleading carriage return\t0\n" +
		"\\N\t1\n" +
		"back\\\\slash\t2\n" +
		"plain\t3\n" +
		"\\.\n\n" +
		"--\n-- PostgreSQL database dump complete\n--\n\n"

	if err := restoreSQL(t, conn, dump); err != nil {
		t.Fatalf("RunPsqlRestore: %v", err)
	}
	assertCopyTestRows(t)
}

// rewriteArchiveDump edits database.sql inside a backup archive and refreshes
// the manifest checksum so the archive still validates.
func rewriteArchiveDump(t *testing.T, archivePath string, edit func(string) string) {
	t.Helper()
	work := t.TempDir()
	if err := backup.ExtractTarGz(archivePath, work); err != nil {
		t.Fatalf("extract: %v", err)
	}
	entries, err := os.ReadDir(work)
	if err != nil || len(entries) != 1 {
		t.Fatalf("unexpected archive layout: %v %v", entries, err)
	}
	dirName := entries[0].Name()
	dir := filepath.Join(work, dirName)

	sqlPath := filepath.Join(dir, "database.sql")
	raw, err := os.ReadFile(sqlPath)
	if err != nil {
		t.Fatal(err)
	}
	edited := []byte(edit(string(raw)))
	if err := os.WriteFile(sqlPath, edited, 0o600); err != nil {
		t.Fatal(err)
	}

	manifestPath := filepath.Join(dir, "manifest.json")
	mraw, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(mraw, &m); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(edited)
	m["database_checksum"] = "sha256:" + hex.EncodeToString(sum[:])
	m["database_size_bytes"] = len(edited)
	mraw, err = json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(manifestPath, mraw, 0o600); err != nil {
		t.Fatal(err)
	}

	f, err := os.Create(archivePath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	gw := gzip.NewWriter(f)
	tw := tar.NewWriter(gw)
	add := func(path string, d fs.DirEntry) error {
		rel, err := filepath.Rel(work, path)
		if err != nil {
			return err
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		hdr, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return err
		}
		hdr.Name = rel
		if err := tw.WriteHeader(hdr); err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		src, err := os.Open(path)
		if err != nil {
			return err
		}
		defer func() { _ = src.Close() }()
		_, err = io.Copy(tw, src)
		return err
	}
	// Manifest first, matching createTarGz.
	mInfo, err := os.Stat(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := add(manifestPath, fs.FileInfoToDirEntry(mInfo)); err != nil {
		t.Fatal(err)
	}
	if err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || path == manifestPath {
			return err
		}
		return add(path, d)
	}); err != nil {
		t.Fatalf("re-tar: %v", err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gw.Close(); err != nil {
		t.Fatal(err)
	}
}

// A backup whose dump carries a shell escape must be refused, and the restore
// must roll back to the pre-restore backup so the live data survives.
func TestRestoreBackup_BackslashCommandRollsBackToPreRestore(t *testing.T) {
	requirePsqlTools(t)
	truncateAllTables(t)
	ctx := context.Background()
	dsn := testDSN
	backupDir := t.TempDir()
	storageDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(storageDir, "cover_art"), 0o755); err != nil {
		t.Fatal(err)
	}

	if _, err := testDB.ExecContext(ctx, "INSERT INTO users (id, username, password_hash) VALUES ('from-backup', 'from-backup', 'x')"); err != nil {
		t.Fatal(err)
	}
	svc := backup.NewService(testDB, dsn, backupDir, storageDir, "0.1.0")
	id, err := svc.CreateBackup("manual")
	if err != nil {
		t.Fatalf("CreateBackup: %v", err)
	}
	archivePath, err := svc.GetBackupPath(id)
	if err != nil {
		t.Fatal(err)
	}

	marker := filepath.Join(t.TempDir(), "pwned")
	rewriteArchiveDump(t, archivePath, func(dump string) string {
		// Inject in the middle of the dump, after its first statement.
		i := strings.Index(dump, ";\n")
		if i < 0 {
			t.Fatal("dump has no statements")
		}
		return dump[:i+2] + `\! touch ` + marker + "\n" + dump[i+2:]
	})
	// Backup ids have one-second resolution, so the pre-restore backup taken
	// below could reuse this id and overwrite the archive; move it aside.
	id = "nexorious-backup-20000101-000000"
	if err := os.Rename(archivePath, filepath.Join(backupDir, id+".tar.gz")); err != nil {
		t.Fatal(err)
	}

	if _, err := testDB.ExecContext(ctx, "DELETE FROM users; INSERT INTO users (id, username, password_hash) VALUES ('live-row', 'live-row', 'x')"); err != nil {
		t.Fatal(err)
	}

	var appState string
	opts := backup.RestoreOpts{
		SkipPreRestore: false,
		SetMaintenance: func(bool) {},
		ShutdownPool:   func() {},
		StopScheduler:  func() {},
		CloseDB:        func() error { return nil },
		ReconnectDB: func() (*bun.DB, error) {
			sqldb := sql.OpenDB(pgdriver.NewConnector(pgdriver.WithDSN(dsn)))
			testDB = bun.NewDB(sqldb, pgdialect.New())
			return testDB, nil
		},
		RebuildServices: func(*bun.DB) error { return nil },
		ReinitMigrator:  func(*bun.DB) error { return nil },
		SetAppState:     func(s string) { appState = s },
		MaxMigration:    "99999999999999",
	}

	err = svc.RestoreBackup(id, opts)
	if err == nil {
		t.Fatal("RestoreBackup succeeded; want the dump refused")
	}
	if !strings.Contains(err.Error(), "rolled back to pre-restore backup") {
		t.Errorf("want a rolled-back error, got: %v", err)
	}
	if appState == "db_unavailable" {
		t.Error("rollback reported db_unavailable")
	}
	if _, statErr := os.Stat(marker); !errors.Is(statErr, fs.ErrNotExist) {
		t.Errorf("shell escape in the dump ran: marker stat = %v", statErr)
	}

	var ids []string
	if err := testDB.NewSelect().Table("users").Column("id").Order("id").Scan(ctx, &ids); err != nil {
		t.Fatalf("query users after rollback: %v", err)
	}
	if len(ids) != 1 || ids[0] != "live-row" {
		t.Errorf("users after rollback = %v, want [live-row]", ids)
	}
}
