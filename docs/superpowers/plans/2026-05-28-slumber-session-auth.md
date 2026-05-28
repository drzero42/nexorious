# Slumber Collection: Session-Based Auth Update — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Update `slumber.yaml` to work with the new session cookie + API key auth system (PR #656), and close the now-complete tracking issue #625.

**Architecture:** Single-file edit to `slumber.yaml`. The `.authenticated` anchor chains through `bootstrap.create_api_key` (leaf key: `create_api_key`); all JWT-era fields are removed; new session/API key management recipes are added to the `auth` folder. Recipe IDs in Slumber are leaf keys — all new leaf keys must be unique across the file.

**Tech Stack:** Slumber 5.3.0, YAML, `gh` CLI

---

### Task 1: Create feature branch

**Files:**
- No file changes

- [ ] **Step 1: Create and switch to feature branch**

```bash
git checkout -b feat/slumber-session-auth
```

Expected: `Switched to a new branch 'feat/slumber-session-auth'`

---

### Task 2: Update `.authenticated` anchor + add `bootstrap.create_api_key`

**Files:**
- Modify: `slumber.yaml:5-8` (anchor)
- Modify: `slumber.yaml:42-54` (add recipe after `setup_restore_disk`)

- [ ] **Step 1: Update `.authenticated` anchor**

Replace lines 5–8:
```yaml
.authenticated:
  authentication:
    type: bearer
    token: "{{response('login', trigger='no_history') | jsonpath('$.access_token')}}"
```
With:
```yaml
.authenticated:
  authentication:
    type: bearer
    token: "{{response('create_api_key', trigger='no_history') | jsonpath('$.key')}}"
```

- [ ] **Step 2: Add `create_api_key` to `bootstrap/`**

After the `setup_restore_disk` recipe (currently ending at line 54), insert:

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

No `$ref: "#/.authenticated"` — this request relies on the session cookie that Slumber holds after running `bootstrap.login`. It is the one-time bootstrap step; subsequent authenticated requests auto-trigger it via `trigger='no_history'` in the anchor.

---

### Task 3: Clean up `auth` section

**Files:**
- Modify: `slumber.yaml:195-212`

- [ ] **Step 1: Delete the `refresh` recipe block (lines 195–202)**

Remove entirely:
```yaml
      refresh:
        name: Refresh Token
        method: POST
        url: "{{base_url}}/api/auth/refresh"
        body:
          type: json
          data:
            refresh_token: "{{response('login', trigger='no_history') | jsonpath('$.refresh_token')}}"
```

- [ ] **Step 2: Strip `body` from `logout`**

Replace:
```yaml
      logout:
        name: Logout
        method: POST
        url: "{{base_url}}/api/auth/logout"
        $ref: "#/.authenticated"
        body:
          type: json
          data:
            refresh_token: "{{response('login', trigger='no_history') | jsonpath('$.refresh_token')}}"
```
With:
```yaml
      logout:
        name: Logout
        method: POST
        url: "{{base_url}}/api/auth/logout"
        $ref: "#/.authenticated"
```

The server reads the session cookie; no request body is needed.

---

### Task 4: Add session and API key management recipes to `auth` folder

**Files:**
- Modify: `slumber.yaml` — after `change_username` (currently at line 249–257)

- [ ] **Step 1: Add session management recipes**

After the `change_username` recipe, append inside the `auth.requests` block:

```yaml
      list_sessions:
        name: List Sessions
        method: GET
        url: "{{base_url}}/api/auth/sessions"
        $ref: "#/.authenticated"

      revoke_session:
        name: Revoke Session
        method: DELETE
        url: "{{base_url}}/api/auth/sessions/{{prompt(message='Session ID')}}"
        $ref: "#/.authenticated"

      revoke_all_other_sessions:
        name: Revoke All Other Sessions
        method: DELETE
        url: "{{base_url}}/api/auth/sessions"
        $ref: "#/.authenticated"
```

- [ ] **Step 2: Add API key management recipes**

After the session recipes, still inside `auth.requests`:

```yaml
      list_api_keys:
        name: List API Keys
        method: GET
        url: "{{base_url}}/api/auth/api-keys"
        $ref: "#/.authenticated"

      revoke_api_key:
        name: Revoke API Key
        method: DELETE
        url: "{{base_url}}/api/auth/api-keys/{{response('list_api_keys', trigger='no_history') | jsonpath('$[*].id', mode='array') | select()}}"
        $ref: "#/.authenticated"
```

Note: No `create_api_key` here — the `bootstrap.create_api_key` recipe (leaf key `create_api_key`) already serves that purpose and must remain the unique entry for the `.authenticated` chain.

---

### Task 5: Verify, commit, and create PR

**Files:**
- `slumber.yaml`
- `docs/superpowers/specs/2026-05-28-slumber-session-auth-design.md`
- `docs/superpowers/plans/2026-05-28-slumber-session-auth.md`

- [ ] **Step 1: Verify collection loads**

```bash
slumber collection
```

Expected: YAML output with no error lines. If any parse error appears, fix the indentation in `slumber.yaml` before continuing.

- [ ] **Step 2: Commit**

```bash
git add slumber.yaml docs/superpowers/specs/2026-05-28-slumber-session-auth-design.md docs/superpowers/plans/2026-05-28-slumber-session-auth.md
git commit -m "chore: update slumber collection for session-based auth"
```

- [ ] **Step 3: Push and open PR**

```bash
git push -u origin feat/slumber-session-auth
gh pr create \
  --title "chore: update slumber collection for session-based auth" \
  --body "$(cat <<'EOF'
Closes #655

Updates `slumber.yaml` to match the session-based auth system introduced in #656:

- `.authenticated` anchor now chains through `bootstrap.create_api_key` (reads `$.key`) instead of extracting `$.access_token` from login
- New `bootstrap.create_api_key` recipe: one-time bootstrap step, POSTs to `/api/auth/api-keys`, relies on session cookie from `bootstrap.login`
- Removed `auth.refresh` recipe and endpoint (`POST /api/auth/refresh` was deleted)
- Removed `refresh_token` body from `auth.logout` (server reads session cookie)
- Added `auth.list_sessions`, `auth.revoke_session`, `auth.revoke_all_other_sessions`
- Added `auth.list_api_keys`, `auth.revoke_api_key`
EOF
)"
```

---

### Task 6: Close issue #625

- [ ] **Step 1: Close the tracking issue**

```bash
gh issue close 625 --comment "Implemented and merged in #656."
```
