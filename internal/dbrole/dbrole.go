// Package dbrole keeps Nexorious's database role unprivileged.
//
// A restored backup is SQL that runs as the application's role. If that role
// is a PostgreSQL superuser (or holds pg_execute_server_program /
// pg_write_server_files), the SQL can run programs or write files on the
// database host. The bundled PostgreSQL in the Docker Compose file and the
// Helm chart creates POSTGRES_USER as a superuser, so EnsureAppRole sets up a
// separate, plain role that owns the database for the application to use
// instead (#1357). CheckConnectedRole reports when the application is
// connected as a privileged role anyway.
//
// EnsureAppRole runs as `nexorious ensure-db-role` (the Compose db-role
// service and the Helm db-role initContainer), connected as the bundled
// PostgreSQL's bootstrap superuser. It makes nexorious_app a non-superuser
// owner of the database and every object in public, idempotently and one
// object at a time: REASSIGN OWNED is refused for the bootstrap superuser,
// which also cannot be demoted. CheckConnectedRole backs the startup warning
// and the admin-only GET /api/admin/health-warnings banner.
package dbrole

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/uptrace/bun"
)

// ownedSchema is the only schema Nexorious creates objects in.
const ownedSchema = "public"

// advisoryLockKey serialises concurrent EnsureAppRole runs (e.g. several
// replicas' init containers starting at once) so CREATE ROLE can't race.
const advisoryLockKey = 1357_0001

// Outcome describes what EnsureAppRole did.
type Outcome int

const (
	// OutcomeEnsured: the role exists, is unprivileged, and owns the
	// database and every object in its schema.
	OutcomeEnsured Outcome = iota
	// OutcomeSkippedSameRole: the application role is the admin role itself,
	// so there is nothing to hand over. The application stays privileged.
	OutcomeSkippedSameRole
	// OutcomeSkippedPrivileged: the application role already exists and is a
	// superuser. It is left untouched; an operator made it that way.
	OutcomeSkippedPrivileged
)

// Result is returned by EnsureAppRole.
type Result struct {
	Outcome Outcome
	// Created is true when the role did not exist before this run.
	Created bool
	// Transferred counts the objects whose ownership moved to the role.
	Transferred int
}

// EnsureAppRole makes role a plain (non-superuser) login role that owns
// database and every object in the public schema. It must run as a
// superuser connected to database, and is idempotent, so it can run on every
// start. When password is non-empty it is (re)set on the role, which keeps
// the role in step with a rotated POSTGRES_PASSWORD.
//
// Ownership is moved object by object: REASSIGN OWNED is refused for the
// bootstrap superuser, which is what the official image's POSTGRES_USER is.
func EnsureAppRole(ctx context.Context, admin *bun.DB, role, password, database string) (Result, error) {
	if role == "" || database == "" {
		return Result{}, errors.New("dbrole: role and database are required")
	}

	var current string
	if err := admin.NewRaw("SELECT current_user").Scan(ctx, &current); err != nil {
		return Result{}, fmt.Errorf("dbrole: read current user: %w", err)
	}
	if current == role {
		return Result{Outcome: OutcomeSkippedSameRole}, nil
	}

	var res Result
	err := admin.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		if _, err := tx.ExecContext(ctx, "SELECT pg_advisory_xact_lock(?)", advisoryLockKey); err != nil {
			return fmt.Errorf("take advisory lock: %w", err)
		}

		var super bool
		err := tx.NewRaw("SELECT rolsuper FROM pg_roles WHERE rolname = ?", role).Scan(ctx, &super)
		switch {
		case errors.Is(err, sql.ErrNoRows):
			if _, err := tx.ExecContext(ctx,
				"CREATE ROLE ? LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOREPLICATION NOBYPASSRLS",
				bun.Ident(role)); err != nil {
				return fmt.Errorf("create role: %w", err)
			}
			res.Created = true
		case err != nil:
			return fmt.Errorf("look up role: %w", err)
		case super:
			res.Outcome = OutcomeSkippedPrivileged
			return nil
		}

		if password != "" {
			if _, err := tx.ExecContext(ctx, "ALTER ROLE ? PASSWORD ?", bun.Ident(role), password); err != nil {
				return fmt.Errorf("set role password: %w", err)
			}
		}
		if _, err := tx.ExecContext(ctx, "ALTER DATABASE ? OWNER TO ?", bun.Ident(database), bun.Ident(role)); err != nil {
			return fmt.Errorf("change database owner: %w", err)
		}
		n, err := transferOwnership(ctx, tx, role)
		if err != nil {
			return err
		}
		res.Transferred = n
		return nil
	})
	if err != nil {
		return Result{}, fmt.Errorf("dbrole: %w", err)
	}
	return res, nil
}

// ownershipStatementsSQL lists one ALTER … OWNER TO statement per object in
// the schema that the role does not already own. Indexes, and sequences owned
// by a table column (serial/identity), follow their table and are skipped;
// so are array types and table row types, which follow their element/table.
const ownershipStatementsSQL = `
SELECT format('ALTER SCHEMA %I OWNER TO %I', n.nspname, ?0)
  FROM pg_namespace n
 WHERE n.nspname = ?1 AND n.nspowner <> (SELECT oid FROM pg_roles WHERE rolname = ?0)
UNION ALL
SELECT format('ALTER %s %s OWNER TO %I',
              CASE c.relkind
                WHEN 'v' THEN 'VIEW'
                WHEN 'm' THEN 'MATERIALIZED VIEW'
                WHEN 'S' THEN 'SEQUENCE'
                WHEN 'f' THEN 'FOREIGN TABLE'
                WHEN 'c' THEN 'TYPE'
                ELSE 'TABLE'
              END,
              c.oid::regclass, ?0)
  FROM pg_class c
  JOIN pg_namespace n ON n.oid = c.relnamespace
 WHERE n.nspname = ?1
   AND c.relkind IN ('r', 'p', 'v', 'm', 'S', 'f', 'c')
   AND c.relowner <> (SELECT oid FROM pg_roles WHERE rolname = ?0)
   AND NOT (c.relkind = 'S' AND EXISTS (
         SELECT 1 FROM pg_depend d
          WHERE d.classid = 'pg_class'::regclass AND d.objid = c.oid
            AND d.refclassid = 'pg_class'::regclass AND d.deptype IN ('a', 'i')))
UNION ALL
SELECT format('ALTER ROUTINE %s OWNER TO %I', p.oid::regprocedure, ?0)
  FROM pg_proc p
  JOIN pg_namespace n ON n.oid = p.pronamespace
 WHERE n.nspname = ?1
   AND p.proowner <> (SELECT oid FROM pg_roles WHERE rolname = ?0)
UNION ALL
SELECT format('ALTER %s %s OWNER TO %I',
              CASE t.typtype WHEN 'd' THEN 'DOMAIN' ELSE 'TYPE' END,
              t.oid::regtype, ?0)
  FROM pg_type t
  JOIN pg_namespace n ON n.oid = t.typnamespace
 WHERE n.nspname = ?1
   AND t.typtype IN ('e', 'd', 'r', 'm')
   AND t.typowner <> (SELECT oid FROM pg_roles WHERE rolname = ?0)
`

// transferOwnership gives role every object in ownedSchema it doesn't own
// and returns how many objects moved.
func transferOwnership(ctx context.Context, tx bun.Tx, role string) (int, error) {
	var stmts []string
	if err := tx.NewRaw(ownershipStatementsSQL, role, ownedSchema).Scan(ctx, &stmts); err != nil {
		return 0, fmt.Errorf("list objects to transfer: %w", err)
	}
	for _, stmt := range stmts {
		if _, err := tx.ExecContext(ctx, stmt); err != nil {
			return 0, fmt.Errorf("transfer ownership (%s): %w", stmt, err)
		}
	}
	return len(stmts), nil
}

// RoleCheck describes the role the application is connected as.
type RoleCheck struct {
	Role string
	// Superuser is true when the role is a PostgreSQL superuser.
	Superuser bool
	// ServerAccess is true when the role can run programs or write files on
	// the database host without being a superuser (member of
	// pg_execute_server_program or pg_write_server_files).
	ServerAccess bool
}

// Privileged reports whether SQL run as this role (e.g. a restored backup)
// could reach the database host.
func (c RoleCheck) Privileged() bool { return c.Superuser || c.ServerAccess }

// CheckConnectedRole inspects the role db is connected as.
func CheckConnectedRole(ctx context.Context, db bun.IDB) (RoleCheck, error) {
	var c RoleCheck
	err := db.NewRaw(`
SELECT r.rolname AS role,
       r.rolsuper AS superuser,
       pg_has_role(r.oid, 'pg_execute_server_program', 'MEMBER')
         OR pg_has_role(r.oid, 'pg_write_server_files', 'MEMBER') AS server_access
  FROM pg_roles r
 WHERE r.rolname = current_user`).Scan(ctx, &c.Role, &c.Superuser, &c.ServerAccess)
	if err != nil {
		return RoleCheck{}, fmt.Errorf("dbrole: check connected role: %w", err)
	}
	return c, nil
}
