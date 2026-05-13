> **Parent spec:** [Nexorious Go Port — Design Spec](2026-05-03-go-port-design.md)  
> Read the parent spec first for project goals, technology stack, architecture decisions, database schema, and the full phase roadmap.

---

### Phase 6 — Embedded PostgreSQL (Zero-Dependency Mode)
*Goal: single binary that works out of the box with no external dependencies, for evaluation and personal use.*

Use [`fergusstrange/embedded-postgres`](https://github.com/fergusstrange/embedded-postgres) to bundle a real PostgreSQL instance that the Go binary can start and manage itself. This is strictly opt-in — production deployments continue to use an external PostgreSQL configured via `DATABASE_URL`.

**Approach:**
- Add `POSTGRES_MODE=embedded|external` config flag (default `external`)
- When `embedded`: binary starts its own PostgreSQL on a local port, sets `DATABASE_URL` internally, manages data directory via `EMBEDDED_POSTGRES_DATA_DIR` (default `./data`)
- When `external`: behaviour is identical to the current design — `DATABASE_URL` is required
- Migration UI and all other behaviour is identical in both modes
- Embedded mode is not recommended for multi-user or production use; a startup warning makes this clear

**Why last:** embedded-postgres adds meaningful binary size and download complexity (it fetches a Postgres binary at first run). It should only be added once the port is stable, well-tested, and the external-Postgres path is the proven baseline. Doing it earlier risks conflating embedded-mode bugs with port bugs.

**Checkpoint:** user can download a single binary, run it, and have a working nexorious instance with no other setup.
