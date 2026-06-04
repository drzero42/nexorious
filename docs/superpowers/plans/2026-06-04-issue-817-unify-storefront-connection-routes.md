# Unify Storefront Connection Routes Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Normalize the four existing storefront connection APIs (Steam, PSN, Epic, GOG) onto a single RESTful `connection` resource with three verbs — `GET|PUT|DELETE /sync/{storefront}/connection` — matching the shape Humble Bundle already ships.

**Architecture:** Pure refactor, no DB or behavior change. The establish action (today named three ways: `POST /steam/verify`, `POST /psn/configure`, `POST /epic/connect`, `POST /gog/connect`) becomes `PUT /{sf}/connection`. Handler bodies are preserved verbatim — only route registration, handler/type names, HTTP method, and call paths change. Wire-format JSON is unchanged (only Go type *names* change, not their `json:` tags), so the establish/status response payloads are byte-identical.

**Tech Stack:** Go + Echo v5 (`internal/api/sync.go`), Bun (unchanged), React + TS (`ui/frontend/src/api/sync.ts`), stdlib `testing` + testcontainers (Go), Vitest (frontend).

---

## Scope & Boundaries

**In scope (live surfaces):**
- Backend route table, handler names, response-type names, and tests — `internal/api/sync.go`, `internal/api/sync_test.go`.
- Frontend API call paths + HTTP methods, and the tests that assert them — `ui/frontend/src/api/sync.ts`, `ui/frontend/src/api/sync.test.ts`, `ui/frontend/src/api/sync-psn.test.ts`.

**Deliberately NOT in scope:**
- **Humble Bundle** — already on `GET|PUT|DELETE /sync/humble-bundle/connection`. Untouched. (Note: its slug is `humble-bundle`, not `humble` as the issue text says.)
- **Slumber** — removed from the repo (see `docs/superpowers/plans/2026-06-02-remove-slumber.md`). No live Slumber collection exists; the issue's "Slumber references" bullet is moot. Verified: `find` for live `*slumber*` config files returns nothing — only historical plan/spec docs and a stale git branch ref remain. Nothing to update.
- **Historical docs** in `docs/superpowers/plans/` and `docs/superpowers/specs/` — these are dated records of past work; we do not rewrite history.
- **Frontend JS function names** (`verifySteamCredentials`, `configurePSN`, `connectEpic`, `connectGOG`, `getPSNStatus`) and the hooks/components/tests that call them. These are internal identifiers, not part of the REST contract the issue normalizes. Renaming them would ripple through `hooks/use-sync.ts`, `hooks/index.ts`, multiple components, and ~6 test files for cosmetic gain. Only the paths + methods inside these functions change.
- **Response semantics** — preserved exactly. In particular, Steam's establish endpoint returns **HTTP 200 with `{valid: false, error: ...}`** on bad credentials (it does NOT 401). PSN/Epic/GOG keep their existing verify-on-save behavior too. PUT replacing POST does not change any status codes or bodies.

## Backend rename map (mirror Humble's `Connect` / `GetConnection` / `Disconnect` naming)

| Storefront | Old route + handler | New route + handler |
|---|---|---|
| Steam | `POST /steam/verify` → `HandleSteamVerify` | `PUT /steam/connection` → `HandleSteamConnect` |
| PSN | `POST /psn/configure` → `HandlePSNConfigure` | `PUT /psn/connection` → `HandlePSNConnect` |
| PSN (status) | `GET /psn/connection` → `HandleGetPSNStatus` | `GET /psn/connection` → `HandleGetPSNConnection` |
| Epic | `POST /epic/connect` → `HandleEpicConnect` | `PUT /epic/connection` → `HandleEpicConnect` (name kept) |
| GOG | `POST /gog/connect` → `HandleGOGConnect` | `PUT /gog/connection` → `HandleGOGConnect` (name kept) |

GET/DELETE for steam, epic, gog already live at `/{sf}/connection` with already-consistent handler names (`HandleGet{SF}Connection`, `Handle{SF}Disconnect`) — only PSN's GET handler name is off and gets renamed.

Response-type renames (Go names only; `json:` tags unchanged):
- `steamVerifyResponse` → `steamConnectResponse`
- `psnConfigureResponse` → `psnConnectResponse`
- `psnStatusResponse` → `psnConnectionResponse`

---

### Task 1: Backend — flip backend tests to PUT + new paths (RED)

**Files:**
- Test: `internal/api/sync_test.go` (lines with `/api/sync/steam/verify`, `/api/sync/psn/configure`, `/api/sync/epic/connect`, `/api/sync/gog/connect`)

The Steam and PSN establish tests use `postJSONAuth` / `postAuth`; Epic and GOG use `postJSONAuth`. All must move to `putJSONAuth` / `putAuth` and the new `/connection` path. **Verified:** `putJSONAuth(t, handler, path, body, token)` exists (`auth_test.go:663`), but **`putAuth` does NOT exist** — only `postAuth` (`games_test.go:161`). Since the Steam (line ~2513) and PSN (line ~2543) establish tests call `postAuth` with a raw body reader, you must add a `putAuth` helper first.

- [ ] **Step 0: Add the `putAuth` helper**

In `internal/api/games_test.go`, immediately after `func postAuth(...)` (~line 161), copy it verbatim and change the function name to `putAuth` and the request method from `http.MethodPost` to `http.MethodPut`. (Keep the same signature: `(t *testing.T, handler interface{...}, path, token string, body io.Reader)`.)

- [ ] **Step 1: Update the Steam establish test calls**

In `internal/api/sync_test.go`, replace each occurrence:
- `postJSONAuth(t, e, "/api/sync/steam/verify", ...)` → `putJSONAuth(t, e, "/api/sync/steam/connection", ...)` (lines ~228, ~252, ~276)
- `postAuth(t, e, "/api/sync/steam/verify", token, body)` → `putAuth(t, e, "/api/sync/steam/connection", token, body)` (line ~2513)

- [ ] **Step 2: Update the PSN establish test calls**

- `postJSONAuth(t, e, "/api/sync/psn/configure", ...)` → `putJSONAuth(t, e, "/api/sync/psn/connection", ...)` (lines ~330, ~347)
- `postAuth(t, e, "/api/sync/psn/configure", token, body)` → `putAuth(t, e, "/api/sync/psn/connection", token, body)` (line ~2543)
- Rename the comment marker `// ─── TestHandleGetPSNStatus with credentials ───` → `// ─── TestHandleGetPSNConnection with credentials ───` (line ~761)

- [ ] **Step 3: Update the Epic establish test calls**

Replace every `postJSONAuth(t, e, "/api/sync/epic/connect", ...)` → `putJSONAuth(t, e, "/api/sync/epic/connection", ...)` (lines ~1989, ~2004, ~2019, ~2031, ~2047).

- [ ] **Step 4: Update the GOG establish test calls**

Replace every `postJSONAuth(t, app, "/api/sync/gog/connect", ...)` → `putJSONAuth(t, app, "/api/sync/gog/connection", ...)` (lines ~2205, ~2217, ~2234, ~2256).

- [ ] **Step 5: Run the tests to verify they FAIL**

Run: `go test ./internal/api/... -run 'TestHandleSteamVerify|TestHandleSteam|TestHandlePSN|TestHandleEpic|TestHandleGOG' -v`
Expected: FAIL — the new `PUT /{sf}/connection` establish routes don't exist yet, so requests return 404/405 and assertions on `valid`/`success`/status fail.

Note: do not commit yet — Task 2 makes these pass and they commit together.

---

### Task 2: Backend — rename routes, handlers, and types (GREEN)

**Files:**
- Modify: `internal/api/sync.go`

- [ ] **Step 1: Rewrite the four establish route registrations**

In `RegisterRoutes` (around lines 237–248), change:

```go
	g.POST("/steam/verify", h.HandleSteamVerify)
	g.GET("/steam/connection", h.HandleGetSteamConnection)
	g.DELETE("/steam/connection", h.HandleSteamDisconnect)
	g.POST("/psn/configure", h.HandlePSNConfigure)
	g.GET("/psn/connection", h.HandleGetPSNStatus)
	g.DELETE("/psn/connection", h.HandlePSNDisconnect)
	g.POST("/epic/connect", h.HandleEpicConnect)
	g.DELETE("/epic/connection", h.HandleEpicDisconnect)
	g.GET("/epic/connection", h.HandleGetEpicConnection)
	g.POST("/gog/connect", h.HandleGOGConnect)
	g.GET("/gog/connection", h.HandleGetGOGConnection)
	g.DELETE("/gog/connection", h.HandleGOGDisconnect)
```

to:

```go
	g.PUT("/steam/connection", h.HandleSteamConnect)
	g.GET("/steam/connection", h.HandleGetSteamConnection)
	g.DELETE("/steam/connection", h.HandleSteamDisconnect)
	g.PUT("/psn/connection", h.HandlePSNConnect)
	g.GET("/psn/connection", h.HandleGetPSNConnection)
	g.DELETE("/psn/connection", h.HandlePSNDisconnect)
	g.PUT("/epic/connection", h.HandleEpicConnect)
	g.GET("/epic/connection", h.HandleGetEpicConnection)
	g.DELETE("/epic/connection", h.HandleEpicDisconnect)
	g.PUT("/gog/connection", h.HandleGOGConnect)
	g.GET("/gog/connection", h.HandleGetGOGConnection)
	g.DELETE("/gog/connection", h.HandleGOGDisconnect)
```

These remain in the static-segment block (before the `/:storefront` parameterised routes), so Echo v5 ordering rules still hold. `PUT` does not collide with the existing `PUT /config/:storefront` (different first segment) and there is no `PUT /:storefront`.

- [ ] **Step 2: Rename the Steam establish handler**

`func (h *SyncHandler) HandleSteamVerify(c *echo.Context) error {` → `func (h *SyncHandler) HandleSteamConnect(c *echo.Context) error {` (line ~625).

- [ ] **Step 3: Rename the PSN establish + status handlers**

- `func (h *SyncHandler) HandlePSNConfigure(...)` → `func (h *SyncHandler) HandlePSNConnect(...)` (line ~699).
- `func (h *SyncHandler) HandleGetPSNStatus(...)` → `func (h *SyncHandler) HandleGetPSNConnection(...)` (line ~740).

- [ ] **Step 4: Rename the response types (all occurrences)**

Use whole-word replace-all across `sync.go`:
- `steamVerifyResponse` → `steamConnectResponse` (type def line ~182 + 7 construction sites in `HandleSteamConnect`).
- `psnConfigureResponse` → `psnConnectResponse` (type def line ~188 + construction site line ~732).
- `psnStatusResponse` → `psnConnectionResponse` (type def line ~195 + sites lines ~742, ~743, ~751).

Epic/GOG establish handlers return inline `map[string]string` (no named type) — nothing to rename there.

- [ ] **Step 5: Build and run the backend tests to verify they PASS**

Run: `go build ./...`
Expected: success (the `golangci-lint`/`gofmt` PostToolUse hook also runs on each edit).

Run: `go test ./internal/api/... -run 'TestHandleSteam|TestHandlePSN|TestHandleEpic|TestHandleGOG' -v`
Expected: PASS — establish routes now resolve via PUT; status/disconnect untouched.

- [ ] **Step 6: Commit backend changes**

```bash
git add internal/api/sync.go internal/api/sync_test.go
git commit -m "refactor(api): unify storefront establish routes onto PUT /{sf}/connection"
```

---

### Task 3: Frontend — flip API tests to PUT + new paths (RED)

**Files:**
- Test: `ui/frontend/src/api/sync.test.ts` (Epic establish assertion ~line 147)
- Test: `ui/frontend/src/api/sync-psn.test.ts` (PSN establish assertions, `describe('configurePSN')`)

The frontend tests assert `api.post('/sync/epic/connect', body)` etc. These must assert `api.put('/sync/epic/connection', body)`. Mocks of `vi.mocked(api.post)` for the establish calls become `vi.mocked(api.put)`.

- [ ] **Step 1: Update the Epic establish test**

In `ui/frontend/src/api/sync.test.ts`, in the `connectEpic` test (~line 145–158):
- `vi.mocked(api.post).mockResolvedValueOnce(mockResponse);` → `vi.mocked(api.put).mockResolvedValueOnce(mockResponse);`
- `expect(api.post).toHaveBeenCalledWith('/sync/epic/connect', {` → `expect(api.put).toHaveBeenCalledWith('/sync/epic/connection', {`

**Verified:** `sync.test.ts`'s mock factory already exposes `put: vi.fn()` (line 10), so no factory change is needed here.

- [ ] **Step 2: Update the PSN establish test**

In `ui/frontend/src/api/sync-psn.test.ts`, inside `describe('configurePSN')` (~lines 18–80):
- every `vi.mocked(api.post)` used for the configure call → `vi.mocked(api.put)`
- every `expect(api.post).toHaveBeenCalledWith('/sync/psn/configure', ...)` → `expect(api.put).toHaveBeenCalledWith('/sync/psn/connection', ...)`
- **Verified:** `sync-psn.test.ts`'s mock factory (lines 7–9) exposes only `get`/`post`/`delete` — you MUST add `put: vi.fn()` to it.

(There is no dedicated frontend api-test file for Steam-verify or GOG-connect establish calls; their paths are covered by Task 4's `sync.ts` edits. If a search of `ui/frontend/src/api/*.test.ts` for `steam/verify` or `gog/connect` returns hits, update them the same way.)

- [ ] **Step 3: Run the frontend tests to verify they FAIL**

Run (from `ui/frontend/`): `npm run test -- sync.test.ts sync-psn.test.ts`
Expected: FAIL — assertions expect `api.put` + `/connection` paths, but `sync.ts` still calls `api.post` + old paths.

---

### Task 4: Frontend — update establish call paths + methods (GREEN)

**Files:**
- Modify: `ui/frontend/src/api/sync.ts`

- [ ] **Step 1: Steam establish — POST→PUT, new path**

In `verifySteamCredentials` (~line 211):
```ts
  const response = await api.post<SteamVerifyApiResponse>('/sync/steam/verify', {
```
→
```ts
  const response = await api.put<SteamVerifyApiResponse>('/sync/steam/connection', {
```

- [ ] **Step 2: Epic establish — POST→PUT, new path**

In `connectEpic` (~line 240):
```ts
  const response = await api.post<EpicConnectApiResponse>('/sync/epic/connect', {
```
→
```ts
  const response = await api.put<EpicConnectApiResponse>('/sync/epic/connection', {
```

- [ ] **Step 3: GOG establish — POST→PUT, new path**

In `connectGOG` (~line 294):
```ts
  const response = await api.post<GOGConnectApiResponse>('/sync/gog/connect', {
```
→
```ts
  const response = await api.put<GOGConnectApiResponse>('/sync/gog/connection', {
```

- [ ] **Step 4: PSN establish — POST→PUT, new path**

In `configurePSN` (~line 345):
```ts
  const response = await api.post<PSNConfigureApiResponse>('/sync/psn/configure', {
```
→
```ts
  const response = await api.put<PSNConfigureApiResponse>('/sync/psn/connection', {
```

GET (`getSteamConnection`, `getEpicConnection`, `getGOGConnection`, `getPSNStatus`) and DELETE (`disconnect*`) calls already target `/{sf}/connection` — leave them unchanged.

- [ ] **Step 5: Run frontend tests + checks to verify GREEN**

Run (from `ui/frontend/`): `npm run test -- sync.test.ts sync-psn.test.ts`
Expected: PASS.

Run (from `ui/frontend/`): `npm run check && npm run knip`
Expected: zero type/lint errors, zero knip findings. (`api.put` is already exported and used by Humble, so no new symbol is introduced.)

- [ ] **Step 6: Commit frontend changes**

```bash
git add ui/frontend/src/api/sync.ts ui/frontend/src/api/sync.test.ts ui/frontend/src/api/sync-psn.test.ts
git commit -m "refactor(api): point frontend storefront establish calls at PUT /{sf}/connection"
```

---

### Task 5: Full-suite verification + plan commit

- [ ] **Step 1: Run the full Go test suite for the api package**

Run: `go test -timeout 600s ./internal/api/...`
Expected: PASS (the pre-push git hook runs the full `go test ./...` anyway).

- [ ] **Step 2: Grep for any stragglers**

Run:
```bash
grep -rn "steam/verify\|psn/configure\|epic/connect\b\|gog/connect\b" internal/ ui/frontend/src --include="*.go" --include="*.ts" --include="*.tsx"
```
Expected: no hits outside `*_test`-historical or already-updated lines. (Historical references under `docs/` are intentionally left.)

- [ ] **Step 3: Commit the plan file (if not already committed at branch start)**

```bash
git add docs/superpowers/plans/2026-06-04-issue-817-unify-storefront-connection-routes.md
git commit -m "docs: plan for issue #817 storefront connection route unification"
```

- [ ] **Step 4: Push and open PR**

```bash
git push -u origin refactor/817-unify-storefront-connection-routes
gh pr create --title "refactor(api): unify storefront connection routes onto a single RESTful resource" --body "Closes #817 ..."
```

---

## Self-Review

- **Spec coverage:** Issue's three coordinated changes — backend route remap (Tasks 1–2), frontend paths+methods (Tasks 3–4), Slumber/docs (addressed: nothing live to change, documented in Scope). All four storefronts covered; Humble correctly excluded. ✅
- **Placeholder scan:** No TBD/TODO/"handle edge cases"; every code step shows the exact before/after. ✅
- **Type consistency:** Rename map is internally consistent — `HandleSteamConnect`, `HandlePSNConnect`, `HandleGetPSNConnection`, `steamConnectResponse`, `psnConnectResponse`, `psnConnectionResponse` are used identically wherever referenced. Epic/GOG handler names intentionally unchanged. ✅
- **Behavior preservation:** Handler bodies untouched; Steam's 200+`valid:false` contract and all JSON wire formats preserved. ✅
