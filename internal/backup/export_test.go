package backup

import "testing"

// SetPgDumpAvailableForTest overrides whether pg_dump counts as installed.
func SetPgDumpAvailableForTest(t testing.TB, v bool) {
	t.Helper()
	old := pgDumpAvailable
	pgDumpAvailable = v
	t.Cleanup(func() { pgDumpAvailable = old })
}

// SetRunPgDumpForTest replaces the pg_dump run a backup makes.
func SetRunPgDumpForTest(t testing.TB, f func(DBConnParams, string) error) {
	t.Helper()
	old := runPgDump
	runPgDump = f
	t.Cleanup(func() { runPgDump = old })
}

// DropPublicSQL is the statement a restore drops the public schema with.
const DropPublicSQL = dropPublicSQL
