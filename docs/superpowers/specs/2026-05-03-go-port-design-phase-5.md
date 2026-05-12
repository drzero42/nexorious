### Phase 5 — Polish + Production Readiness
*Goal: production-grade deployment.*

- Admin user management endpoints (JWT + admin role required): `POST /api/auth/admin/users` (create), `GET /api/auth/admin/users` (list all), `GET /api/auth/admin/users/:id`, `PUT /api/auth/admin/users/:id` (update role/enabled), `PUT /api/auth/admin/users/:id/password` (reset password), `GET /api/auth/admin/users/:id/deletion-impact`, `DELETE /api/auth/admin/users/:id` — see Admin User Management section; handlers go in `internal/api/auth.go` or a new `internal/api/admin_users.go`; route group reuses the existing `adminGroup` in `registerRoutes`
- PostgreSQL-backed rate limiter (multi-instance support)
- Migrate CLI surface to `cobra` subcommands (`serve`, `migrate`, `migrate status`, `version`); update Helm chart, systemd units, and any tooling that uses `--migrate-only`
- Full test coverage (testcontainers-go, >80%)
- Dockerfile (single-stage: React build → go build → minimal runtime image)
- Helm chart (adapted from existing nexorious chart)
- Documentation updates

**Checkpoint:** ready to replace the Python version in production.
