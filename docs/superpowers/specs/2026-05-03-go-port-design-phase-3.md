### Phase 3 — Background Workers + Import/Export
*Goal: data migration path from Python version, plus export/backup.*

- Worker pool (database-backed task queue: `pending_tasks` table, `SELECT FOR UPDATE SKIP LOCKED`, goroutine workers with in-process notify channel for zero-latency wake-up)
- Job tracking (jobs + job_items tables, full `/api/jobs` and `/api/job-items` endpoints including review workflow; Bun model structs for these tables)
- gocron scheduler (cleanup jobs wired up)
- Import handler (`POST /api/import/nexorious` — nexorious JSON format, the upgrade path from Python)
- Export handler
- Backup create + scheduled backup + full restore with rollback — see `2026-05-10-backup-restore-design.md` for detailed design
- `POST /api/auth/setup/restore` — deferred from Phase 1; shares restore logic with the backup system implemented here
- Admin backup endpoints: list, create, delete, download, restore, upload-restore, config get/put
- Maintenance mode middleware for restore operations

**Checkpoint:** existing Python users can export their data and import it into the Go version.
