# Slumber API Client — Design

**Date:** 2026-05-07  
**Status:** Approved

## Overview

Add a [Slumber](https://github.com/LucasPickering/slumber) TUI API client collection to the project. The collection lives alongside the Go source code, is committed to the repo, and covers all current API endpoints. It supports a full bootstrap workflow — from an empty database through migrations and admin setup — so a developer can go from `make && ./nexorious` to a fully exercised API without touching a browser.

---

## Goals

- Zero-friction API testing from the terminal (no browser, no Postman)
- Bootstrap flow: trigger migrations → create admin → log in automatically
- JWT auth handled transparently via Slumber's `response()` chaining (no manual token copy-paste)
- Collection stays in sync with the codebase via CLAUDE.md rules

---

## File Structure

```
slumber/
  slumber.yaml     ← entire collection: profiles and all requests
```

A single file is idiomatic Slumber and easier to maintain than split files. All future domains are appended to this file.

---

## Profile

One profile: `local`.

```yaml
profiles:
  local:
    name: Local
    data:
      base_url: http://localhost:8000
      username: admin
      password: abcd1234
```

All requests reference `{{base_url}}`, `{{username}}`, and `{{password}}`. Adding a `staging` profile in future requires only a new stanza with different values — no request changes needed.

---

## Authentication

Slumber v4 has no `chains:` key. Request chaining is done inline via the `response()` template function. All JWT-protected requests use Slumber's first-class `authentication` field:

```yaml
authentication:
  type: bearer
  token: "{{ response('auth/login', trigger='no_history') | jsonpath('$.access_token') }}"
```

- `response('auth/login', trigger='no_history')` — fires `auth/login` automatically the first time a protected request is triggered if no prior response exists in history; uses the cached response thereafter.
- `| jsonpath('$.access_token')` — extracts the token from the login response body.
- The `refresh_token` is not chained — it is visible in the `auth/login` response body in the TUI when needed.

---

## Request Organisation

Six folders. `bootstrap` sorts first; the remainder are alphabetical by domain.

### `bootstrap/` — Run top-to-bottom on a fresh empty DB

| # | Name | Method | Path |
|---|------|--------|------|
| 1 | `run-migrations` | `POST` | `/api/migrate/run` |
| 2 | `migration-status` | `GET` | `/api/migrate/status` |
| 3 | `create-admin` | `POST` | `/api/auth/setup/admin` |

`migration-status` may need to be run more than once until the response shows `ready`. `create-admin` uses `{{username}}` and `{{password}}` from the profile — no typing required.

### `auth/` — Session lifecycle

| # | Name | Method | Path | Auth |
|---|------|--------|------|------|
| 1 | `login` | `POST` | `/api/auth/login` | — *(chain source)* |
| 2 | `refresh` | `POST` | `/api/auth/refresh` | — |
| 3 | `logout` | `POST` | `/api/auth/logout` | JWT |
| 4 | `me` | `GET` | `/api/auth/me` | JWT |

`login` is the upstream source — all protected requests in any folder use the `authentication: type: bearer` block shown above, referencing `response('auth/login', trigger='no_history')`.

### `health/`

| # | Name | Method | Path |
|---|------|--------|------|
| 1 | `health-check` | `GET` | `/health` |

### `migrate/` — Post-bootstrap migration management

| # | Name | Method | Path |
|---|------|--------|------|
| 1 | `status` | `GET` | `/api/migrate/status` |
| 2 | `run` | `POST` | `/api/migrate/run` |
| 3 | `progress` | `GET` | `/api/migrate/progress` |

### `setup/` — Standalone setup reference

| # | Name | Method | Path |
|---|------|--------|------|
| 1 | `create-admin` | `POST` | `/api/auth/setup/admin` |

---

## Typical Developer Workflow

1. Start the server: `make && ./nexorious`
2. Open Slumber: `slumber -f slumber/slumber.yaml`
3. Select `local` profile
4. Run `bootstrap/run-migrations`
5. Run `bootstrap/migration-status` until status is `ready`
6. Run `bootstrap/create-admin`
7. Any subsequent request that needs JWT will auto-trigger `auth/login` on first use

---

## CLAUDE.md Updates

### Quick Reference table — new row

| Task | Command |
|------|---------|
| Run API client | `slumber -f slumber/slumber.yaml` |

### New section: Slumber Collection Maintenance

> When adding a new API route, always add a corresponding request to `slumber/slumber.yaml`:
>
> - Add it to the matching domain folder (e.g. a new `GET /api/games` goes in a `games/` folder)
> - If the route requires JWT, add the `authentication: type: bearer` block with `response('auth/login', trigger='no_history') | jsonpath('$.access_token')`
> - If it's a new domain with no existing folder, create the folder in alphabetical order after `bootstrap/`
> - Use profile variables (`{{base_url}}`) for all URLs — never hardcode `localhost:8000`
> - Run `slumber -f slumber/slumber.yaml` to verify the collection loads without errors after any change

---

## Out of Scope

- Polling loop for migration completion (Slumber has no native polling; run `migration-status` manually)
- Automated assertions / test scripts (can be added later per-request as the API matures)
- Additional profiles (`staging`, `production`) — add when those environments exist
