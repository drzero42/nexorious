# Slumber Collection: Session-Based Auth Update

**Issue:** #655
**Status:** Approved

## Context

PR #656 replaced JWT auth with server-side session cookies (SPA) and Bearer API keys (CLI/programmatic). The `slumber.yaml` collection still references JWT-era fields that no longer exist.

## Changes

### `.authenticated` anchor

Replace `$.access_token` extraction from login with a chain from `create_api_key`:

```yaml
.authenticated:
  authentication:
    type: bearer
    token: "{{response('bootstrap.create_api_key', trigger='no_history') | jsonpath('$.key')}}"
```

Bootstrap workflow: run `bootstrap.login` once (sets session cookie), then any authenticated request auto-triggers `create_api_key` via `no_history`. Slumber's cookie jar carries the session into the `create_api_key` call.

### `bootstrap` additions

Add `create_api_key` after `login`:

```yaml
create_api_key:
  name: Create API Key
  method: POST
  url: "{{base_url}}/api/auth/api-keys"
  body:
    type: json
    data:
      name: slumber
```

### `auth` section cleanup

- **Remove** `refresh` request folder (endpoint deleted)
- **Remove** `refresh_token` from `logout` body (logout reads session cookie server-side)

### New auth management requests

PR #656 added session and API key management endpoints. Add matching slumber recipes:

**`auth.sessions` folder:**
- `list_sessions` — GET /api/auth/sessions
- `revoke_session` — DELETE /api/auth/sessions/:id
- `revoke_all_other_sessions` — DELETE /api/auth/sessions

**`auth.api_keys` folder:**
- `list_api_keys` — GET /api/auth/api-keys
- `create_api_key` — POST /api/auth/api-keys (manual use)
- `revoke_api_key` — DELETE /api/auth/api-keys/:id

All new requests use `$ref: "#/.authenticated"`.

### Close #625

Issue #625 ("Replace JWT auth with server-side sessions and API keys") was implemented by PR #656 and can be closed.

## Out of Scope

- Adding session/API key UI to the frontend (separate issue if needed)
- CLAUDE.md updates (workflow description still accurate)
