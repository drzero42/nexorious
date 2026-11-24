# The app connects to its database as a role that is not a superuser

Restoring a backup runs the dump's SQL as the app's database role, and the restore is reachable without authentication in the setup zone before the first admin exists. The bundled Postgres in Compose and Helm made the app connect as the image's `POSTGRES_USER`, which is a superuser and can run programs on the database host (`COPY … TO PROGRAM`). A crafted backup could therefore run commands in the database container (#1357).

We decided that the app connects as `nexorious_app`, an ordinary role that owns the database and every object in `public`. `nexorious` remains the superuser, for administration only. A `nexorious ensure-db-role` step (`internal/dbrole`) runs as that superuser before every start (the Compose `db-role` service, the Helm `db-role` initContainer). It creates the role if missing, and moves ownership to it object by object. It is idempotent, so existing installs move over on their first start with no manual steps (#1361).

## Considered options

- **Demote the existing `nexorious` role.** Not possible: on existing installs it is the bootstrap superuser, which PostgreSQL 16+ refuses to demote.
- **`REASSIGN OWNED BY nexorious`.** Refused for the bootstrap superuser, hence the per-object transfer.
- **An init script in `/docker-entrypoint-initdb.d`.** It runs only on an empty data directory, so it cannot fix existing installs.

## Consequences

- Migrations must not need superuser rights (no `CREATE EXTENSION`, `ALTER SYSTEM` and the like). The restore path (`DROP SCHEMA public CASCADE`, recreate, migrate) works as a database owner.
- Deployments that bring their own database are not covered by the step. The server checks the connected role at startup and logs a warning, and admins see a banner (`GET /api/admin/health-warnings`) until it is fixed. With CloudNativePG, use the `-app` Secret, not `-superuser`.
- The superuser's password is not changed. It still shares `POSTGRES_PASSWORD` in the bundled setups; giving it its own is the operator's call.
