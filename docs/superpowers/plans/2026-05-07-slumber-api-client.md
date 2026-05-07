# Slumber API Client Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a Slumber TUI API client collection to the project with a full bootstrap workflow and auto-login for JWT-protected routes.

**Architecture:** A single `slumber.yaml` at the project root defines one `local` profile and all request recipes organised into domain folders. JWT auth is handled inline via Slumber's `response()` template function — no manual token management. CLAUDE.md and DEV.md are updated to document the collection.

**Tech Stack:** [Slumber](https://github.com/LucasPickering/slumber) v4+ (YAML collection format), already available in devenv shell.

---

## File Map

| File | Action | Purpose |
|------|--------|---------|
| `slumber.yaml` | Create | Entire collection: profile, all request recipes |
| `CLAUDE.md` | Modify | Add `slumber` to Quick Reference table; add Slumber Collection Maintenance section |
| `DEV.md` | Modify | Add API Client (Slumber) section |

---

### Task 1: Create the branch

- [ ] **Step 1: Create and switch to feature branch**

```bash
git checkout -b feat/slumber-api-client
```

---

### Task 2: Create `slumber.yaml`

- [ ] **Step 1: Create `slumber.yaml` at the project root**

```yaml
name: Nexorious

profiles:
  local:
    name: Local
    data:
      base_url: http://localhost:8000
      username: admin
      password: abcd1234

requests:
  bootstrap:
    name: Bootstrap
    requests:
      run_migrations:
        name: Run Migrations
        method: POST
        url: "{{base_url}}/api/migrate/run"

      migration_status:
        name: Migration Status
        method: GET
        url: "{{base_url}}/api/migrate/status"

      create_admin:
        name: Create Admin
        method: POST
        url: "{{base_url}}/api/auth/setup/admin"
        body:
          type: json
          data:
            username: "{{username}}"
            password: "{{password}}"

  auth:
    name: Auth
    requests:
      login:
        name: Login
        method: POST
        url: "{{base_url}}/api/auth/login"
        body:
          type: json
          data:
            username: "{{username}}"
            password: "{{password}}"

      refresh:
        name: Refresh Token
        method: POST
        url: "{{base_url}}/api/auth/refresh"
        body:
          type: json
          data:
            refresh_token: "{{prompt(message='Paste refresh_token from auth/login response')}}"

      logout:
        name: Logout
        method: POST
        url: "{{base_url}}/api/auth/logout"
        authentication:
          type: bearer
          token: "{{response('auth/login', trigger='no_history') | jsonpath('$.access_token')}}"
        body:
          type: json
          data:
            refresh_token: ""

      me:
        name: Me
        method: GET
        url: "{{base_url}}/api/auth/me"
        authentication:
          type: bearer
          token: "{{response('auth/login', trigger='no_history') | jsonpath('$.access_token')}}"

  health:
    name: Health
    requests:
      health_check:
        name: Health Check
        method: GET
        url: "{{base_url}}/health"

  migrate:
    name: Migrate
    requests:
      status:
        name: Status
        method: GET
        url: "{{base_url}}/api/migrate/status"

      run:
        name: Run
        method: POST
        url: "{{base_url}}/api/migrate/run"

      progress:
        name: Progress
        method: GET
        url: "{{base_url}}/api/migrate/progress"

  setup:
    name: Setup
    requests:
      create_admin:
        name: Create Admin
        method: POST
        url: "{{base_url}}/api/auth/setup/admin"
        body:
          type: json
          data:
            username: "{{username}}"
            password: "{{password}}"
```

- [ ] **Step 2: Verify Slumber loads the collection without errors**

```bash
devenv shell -- slumber show collection
```

Expected: prints the collection path and name (`Nexorious`), no errors.

- [ ] **Step 3: Commit**

```bash
git add slumber.yaml
git commit -m "feat: add slumber API client collection"
```

---

### Task 3: Update CLAUDE.md

- [ ] **Step 1: Add `slumber` row to the Quick Reference table**

In `CLAUDE.md`, add a new row to the Common Commands table (after the `golangci-lint run` row):

```markdown
| Run API client           | `slumber`                                                |
```

The table should look like:
```markdown
| Task                     | Command                                                  |
|--------------------------|----------------------------------------------------------|
| Enter dev shell          | `devenv shell`                                           |
| Build backend            | `make build`                                             |
| Build frontend           | `make frontend`                                          |
| Build everything         | `make`                                                   |
| Run server               | `./nexorious`                                            |
| Run tests (Go)           | `go test ./...`                                          |
| Run single test          | `go test ./internal/api/... -run TestGamesList -v`       |
| Run tests with coverage  | `go test -cover ./...`                                   |
| Generate sqlc code       | `make sqlc`                                              |
| Type check (frontend)    | `npm run check`  (from `ui/`)                            |
| Run frontend tests       | `npm run test`   (from `ui/`)                            |
| Lint Go                  | `golangci-lint run`                                      |
| Run API client           | `slumber`                                                |
```

- [ ] **Step 2: Add Slumber Collection Maintenance section to Development Rules**

At the end of the `## Development Rules` section (before `## Known Gotchas`), add:

```markdown
### Slumber Collection Maintenance
When adding a new API route, always add a corresponding request to `slumber.yaml`:
- Add it to the matching domain folder (e.g. a new `GET /api/games` goes in a `games/` folder)
- If the route requires JWT, add the `authentication: type: bearer` block with `"{{response('auth/login', trigger='no_history') | jsonpath('$.access_token')}}"`
- If it's a new domain with no existing folder, create the folder in alphabetical order after `bootstrap/`
- Use profile variables (`{{base_url}}`) for all URLs — never hardcode `localhost:8000`
- Run `slumber show collection` to verify the collection loads without errors after any change
```

- [ ] **Step 3: Commit**

```bash
git add CLAUDE.md
git commit -m "docs: add slumber to CLAUDE.md quick reference and maintenance rules"
```

---

### Task 4: Update DEV.md

- [ ] **Step 1: Add API Client section to DEV.md**

After the `## Resetting the database` section (at the end of the file), add:

```markdown
## API Client (Slumber)

The project includes a [Slumber](https://github.com/LucasPickering/slumber) collection for testing the API from the terminal. Slumber is included in the devenv shell — no separate install needed.

**Starting Slumber:**

```bash
slumber
```

**First-time setup (fresh database):**

Run these requests in order from the `bootstrap/` folder:

1. `bootstrap/run-migrations` — applies all pending database migrations
2. `bootstrap/migration-status` — check until status shows `ready` (run a few times if needed)
3. `bootstrap/create-admin` — creates the admin user (`admin` / `abcd1234`)

After that, any request requiring authentication will automatically log in on first use — no manual token handling.

**Day-to-day use:**

Open `slumber`, select the `local` profile, and run any request. JWT-protected routes auto-login when needed using the cached credentials from the `local` profile.
```

- [ ] **Step 2: Commit**

```bash
git add DEV.md
git commit -m "docs: add Slumber API client section to DEV.md"
```

---

### Task 5: Open a PR

- [ ] **Step 1: Push branch and open PR**

```bash
git push -u origin feat/slumber-api-client
gh pr create --title "feat: add Slumber API client collection" --body "$(cat <<'EOF'
## Summary
- Adds `slumber.yaml` at the project root with a `local` profile and all current API endpoints organised into domain folders (`bootstrap`, `auth`, `health`, `migrate`, `setup`)
- JWT auth is handled automatically via `response('auth/login', trigger='no_history')` — no manual token copy-paste
- Bootstrap folder covers the full fresh-DB workflow: run migrations → check status → create admin
- Updates CLAUDE.md with quick reference entry and collection maintenance rules
- Updates DEV.md with usage instructions

## Test plan
- [ ] Run `devenv shell -- slumber show collection` — confirms collection loads without errors
- [ ] Start server (`make && ./nexorious`) and open `slumber`
- [ ] Run `bootstrap/run-migrations`, `bootstrap/migration-status`, `bootstrap/create-admin` in order
- [ ] Run `auth/me` — confirm it auto-triggers `auth/login` and returns user details
- [ ] Run `health/health-check` — confirm 200 OK

🤖 Generated with [Claude Code](https://claude.com/claude-code)
EOF
)"
```
