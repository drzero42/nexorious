# Prune Dead Sync Account-ID Fields Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Remove the dead account-ID fields (`steam_id`, `account_id`, `user_id`) from the four sync status/connection **GET** response shapes, end-to-end (Go handlers → wire → TS types), per issue #800 and spec `docs/superpowers/specs/2026-06-04-prune-sync-account-id-fields-design.md`.

**Architecture:** Pure field removal — no new behaviour, so no new tests (per project testing policy). Two tasks: backend prune + stale Go test assertions, then frontend prune + stale mock cleanup. POST/connect/configure shapes and stored encrypted credentials are explicitly untouched (the PSN worker reads `account_id` from stored credentials, not from these GET responses).

**Tech Stack:** Go (Echo v5), TypeScript (React/TanStack Query), vitest, stdlib `testing`.

**Branch:** `prune-sync-account-id-fields` (already created; spec committed).

---

### Task 1: Prune Go GET handlers and stale test assertions

**Files:**
- Modify: `internal/api/sync.go` (lines ~191–204, ~633–664, ~709–743, ~806–858, ~1480–1517)
- Modify: `internal/api/sync_test.go` (lines ~2161–2195, ~2366–2403)

- [ ] **Step 1: Drop `AccountID` from `psnStatusResponse`**

In `internal/api/sync.go`, change:

```go
type psnStatusResponse struct {
	IsConfigured     bool   `json:"is_configured"`
	CredentialsError bool   `json:"credentials_error,omitempty"`
	OnlineID         string `json:"online_id,omitempty"`
	AccountID        string `json:"account_id,omitempty"`
	Region           string `json:"region,omitempty"`
}
```

to:

```go
type psnStatusResponse struct {
	IsConfigured     bool   `json:"is_configured"`
	CredentialsError bool   `json:"credentials_error,omitempty"`
	OnlineID         string `json:"online_id,omitempty"`
	Region           string `json:"region,omitempty"`
}
```

- [ ] **Step 2: Drop `SteamID` from `steamConnectionResponse`**

In `internal/api/sync.go`, change:

```go
type steamConnectionResponse struct {
	Connected        bool   `json:"connected"`
	CredentialsError bool   `json:"credentials_error,omitempty"`
	SteamID          string `json:"steam_id,omitempty"`
	Username         string `json:"username,omitempty"`
}
```

to:

```go
type steamConnectionResponse struct {
	Connected        bool   `json:"connected"`
	CredentialsError bool   `json:"credentials_error,omitempty"`
	Username         string `json:"username,omitempty"`
}
```

- [ ] **Step 3: Update `HandleGetSteamConnection`**

In `internal/api/sync.go` (~line 649), the handler decodes stored credentials and builds the response. Change:

```go
	var creds struct {
		SteamID     string `json:"steam_id"`
		DisplayName string `json:"display_name"`
	}
	if err := json.Unmarshal(status.Plaintext, &creds); err != nil {
		slog.Error("steam: stored credentials are corrupted", "err", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "stored credentials are corrupted")
	}

	return c.JSON(http.StatusOK, steamConnectionResponse{
		Connected:        true,
		SteamID:          creds.SteamID,
		Username:         creds.DisplayName,
		CredentialsError: status.CredentialsError,
	})
```

to:

```go
	var creds struct {
		DisplayName string `json:"display_name"`
	}
	if err := json.Unmarshal(status.Plaintext, &creds); err != nil {
		slog.Error("steam: stored credentials are corrupted", "err", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "stored credentials are corrupted")
	}

	return c.JSON(http.StatusOK, steamConnectionResponse{
		Connected:        true,
		Username:         creds.DisplayName,
		CredentialsError: status.CredentialsError,
	})
```

- [ ] **Step 4: Update `HandleGetPSNStatus`**

In `internal/api/sync.go` (~line 725). Change:

```go
	var creds struct {
		OnlineID   string `json:"online_id"`
		AccountID  string `json:"account_id"`
		Region     string `json:"region"`
		IsVerified bool   `json:"is_verified"`
	}
	if err := json.Unmarshal(status.Plaintext, &creds); err != nil {
		slog.Error("psn: stored credentials are corrupted", "err", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "stored credentials are corrupted")
	}

	return c.JSON(http.StatusOK, psnStatusResponse{
		IsConfigured:     true,
		CredentialsError: status.CredentialsError,
		OnlineID:         creds.OnlineID,
		AccountID:        creds.AccountID,
		Region:           creds.Region,
	})
```

to:

```go
	var creds struct {
		OnlineID   string `json:"online_id"`
		Region     string `json:"region"`
		IsVerified bool   `json:"is_verified"`
	}
	if err := json.Unmarshal(status.Plaintext, &creds); err != nil {
		slog.Error("psn: stored credentials are corrupted", "err", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "stored credentials are corrupted")
	}

	return c.JSON(http.StatusOK, psnStatusResponse{
		IsConfigured:     true,
		CredentialsError: status.CredentialsError,
		OnlineID:         creds.OnlineID,
		Region:           creds.Region,
	})
```

- [ ] **Step 5: Update `HandleGetEpicConnection`**

In `internal/api/sync.go` (~lines 832–857). Change:

```go
	// Epic persists the raw legendary snapshot: a map[relPath]content where the
	// account details live inside user.json (fields displayName/account_id), not
	// as top-level keys. Decode the snapshot, then parse user.json.
	var snapshot map[string]string
	if err := json.Unmarshal(status.Plaintext, &snapshot); err != nil {
		slog.Error("epic: stored credentials are corrupted", "err", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "stored credentials are corrupted")
	}
	var creds struct {
		DisplayName string `json:"displayName"`
		AccountID   string `json:"account_id"`
	}
	if userJSON, ok := snapshot["user.json"]; ok {
		if err := json.Unmarshal([]byte(userJSON), &creds); err != nil {
			slog.Error("epic: stored user.json is corrupted", "err", err)
			return echo.NewHTTPError(http.StatusInternalServerError, "stored credentials are corrupted")
		}
	}

	return c.JSON(http.StatusOK, map[string]any{
		"connected":         true,
		"disabled":          false,
		"credentials_error": status.CredentialsError,
		"display_name":      creds.DisplayName,
		"account_id":        creds.AccountID,
	})
```

to:

```go
	// Epic persists the raw legendary snapshot: a map[relPath]content where the
	// account details live inside user.json (field displayName), not as
	// top-level keys. Decode the snapshot, then parse user.json.
	var snapshot map[string]string
	if err := json.Unmarshal(status.Plaintext, &snapshot); err != nil {
		slog.Error("epic: stored credentials are corrupted", "err", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "stored credentials are corrupted")
	}
	var creds struct {
		DisplayName string `json:"displayName"`
	}
	if userJSON, ok := snapshot["user.json"]; ok {
		if err := json.Unmarshal([]byte(userJSON), &creds); err != nil {
			slog.Error("epic: stored user.json is corrupted", "err", err)
			return echo.NewHTTPError(http.StatusInternalServerError, "stored credentials are corrupted")
		}
	}

	return c.JSON(http.StatusOK, map[string]any{
		"connected":         true,
		"disabled":          false,
		"credentials_error": status.CredentialsError,
		"display_name":      creds.DisplayName,
	})
```

- [ ] **Step 6: Update `HandleGetGOGConnection`**

In `internal/api/sync.go` (~lines 1501–1516). Change:

```go
	var creds struct {
		Username string `json:"username"`
		UserID   string `json:"user_id"`
	}
	if err := json.Unmarshal(status.Plaintext, &creds); err != nil {
		slog.Error("gog: stored credentials are corrupted", "err", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "stored credentials are corrupted")
	}

	return c.JSON(http.StatusOK, map[string]any{
		"connected":         true,
		"credentials_error": status.CredentialsError,
		"username":          creds.Username,
		"user_id":           creds.UserID,
		"auth_url":          authURL,
	})
```

to:

```go
	var creds struct {
		Username string `json:"username"`
	}
	if err := json.Unmarshal(status.Plaintext, &creds); err != nil {
		slog.Error("gog: stored credentials are corrupted", "err", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "stored credentials are corrupted")
	}

	return c.JSON(http.StatusOK, map[string]any{
		"connected":         true,
		"credentials_error": status.CredentialsError,
		"username":          creds.Username,
		"auth_url":          authURL,
	})
```

- [ ] **Step 7: Drop stale assertion in `TestGetSteamConnection_Connected`**

In `internal/api/sync_test.go` (~lines 2394–2396), delete this block (the surrounding `connected`, `username`, and `credentials_error` assertions stay; the stored-credential fixture `rawCreds` keeps its `steam_id` — it tests the storage format, which is unchanged):

```go
	if body["steam_id"] != "76561198012345678" {
		t.Errorf("steam_id: got %v", body["steam_id"])
	}
```

- [ ] **Step 8: Update Epic GET connection test**

In `internal/api/sync_test.go` (~line 2161), `TestHandleGetEpicConnection_ConnectedReturnsAccountInfo` asserts `account_id`. Rename the test to match its remaining behaviour and drop the `account_id` half of the assertion. Change:

```go
func TestHandleGetEpicConnection_ConnectedReturnsAccountInfo(t *testing.T) {
```

to:

```go
func TestHandleGetEpicConnection_ConnectedReturnsDisplayName(t *testing.T) {
```

and change (~lines 2192–2194):

```go
	if resp["display_name"] != "PlayerOne" || resp["account_id"] != "acct-xyz" {
		t.Errorf("expected display_name/account_id from creds, got: %v", resp)
	}
```

to:

```go
	if resp["display_name"] != "PlayerOne" {
		t.Errorf("expected display_name from creds, got: %v", resp)
	}
```

Do NOT touch `TestHandleEpicConnect_HappyPathPersistsConfig` (~line 2037) — its `resp["account_id"]` assertion is against the **POST** `/sync/epic/connect` response, which is out of scope. The seeded fixture `rawCreds` in the GET test also keeps its `account_id` (storage format unchanged).

- [ ] **Step 9: Run the affected Go tests**

```bash
go test ./internal/api/... -run 'SteamConnection|PSNStatus|EpicConnection|GOGConnection' -v
```

Expected: all PASS (note: package test setup starts a PostgreSQL testcontainer — first run takes ~1 min).

- [ ] **Step 10: Commit**

```bash
git add internal/api/sync.go internal/api/sync_test.go
git commit -m "fix(sync): drop dead account-ID fields from connection/status GET responses"
```

---

### Task 2: Prune frontend wire types, domain types, and stale mocks

**Files:**
- Modify: `ui/frontend/src/api/sync.ts` (wire interfaces + 4 mapping functions)
- Modify: `ui/frontend/src/types/sync.ts` (4 domain interfaces)
- Modify: `ui/frontend/src/api/sync.test.ts`
- Modify: `ui/frontend/src/hooks/use-sync.test.ts`
- Modify: `ui/frontend/src/hooks/use-sync-psn.test.ts`
- Modify: `ui/frontend/src/components/sync/epic-connection-card.test.tsx`
- Modify: `ui/frontend/src/components/sync/gog-connection-card.test.tsx`

**Scope guard — these stay untouched (POST/connect/configure shapes):** `EpicConnectResponse`/`EpicConnectApiResponse`, `GOGConnectResponse`/`GOGConnectApiResponse`, `PSNConfigureResponse`/`PSNConfigureApiResponse`, Steam verify types, and every test mock for `connectEpic`, `connectGOG`, `configurePSN`, or `useConnectEpic`/`useConnectGOG`/`useConfigurePSN` `mutateAsync`. In particular `psn-connection-card.test.tsx` needs **no changes** (all its `accountId` mocks are `configurePSN` results) and `steam-connection-card.test.tsx` needs **no changes** (its `steamId` usages are the verify-form schema).

- [ ] **Step 1: Prune wire interfaces in `ui/frontend/src/api/sync.ts`**

Remove `steam_id?: string;` from `SteamConnectionApiResponse` (~line 174):

```ts
interface SteamConnectionApiResponse {
  connected: boolean;
  credentials_error?: boolean;
  username?: string;
}
```

Remove `account_id?: string;` from `EpicConnectionApiResponse` (~line 196):

```ts
interface EpicConnectionApiResponse {
  connected: boolean;
  disabled: boolean;
  credentials_error?: boolean;
  display_name?: string;
  reason?: string;
}
```

Remove `user_id?: string;` from `GOGConnectionApiResponse` (~line 288):

```ts
interface GOGConnectionApiResponse {
  connected: boolean;
  credentials_error?: boolean;
  username?: string;
  auth_url?: string;
}
```

Remove `account_id: string | null;` from `PSNStatusApiResponse` (~line 341):

```ts
interface PSNStatusApiResponse {
  is_configured: boolean;
  credentials_error?: boolean;
  online_id: string | null;
  region: string | null;
}
```

- [ ] **Step 2: Prune mapping lines in the four GET functions in `ui/frontend/src/api/sync.ts`**

`getEpicConnection` (~line 259): delete `accountId: response.account_id,`:

```ts
export async function getEpicConnection(): Promise<EpicConnectionResponse> {
  const response = await api.get<EpicConnectionApiResponse>('/sync/epic/connection');
  return {
    connected: response.connected,
    disabled: response.disabled,
    credentialsError: response.credentials_error ?? false,
    displayName: response.display_name,
    reason: response.reason,
  };
}
```

`getGOGConnection` (~line 312): delete `userId: response.user_id,`:

```ts
export async function getGOGConnection(): Promise<GOGConnectionResponse> {
  const response = await api.get<GOGConnectionApiResponse>('/sync/gog/connection');
  return {
    connected: response.connected,
    credentialsError: response.credentials_error ?? false,
    username: response.username,
    authUrl: response.auth_url,
  };
}
```

`getPSNStatus` (~line 373): delete `accountId: response.account_id,`:

```ts
export async function getPSNStatus(): Promise<PSNStatusResponse> {
  const response = await api.get<PSNStatusApiResponse>('/sync/psn/connection');

  return {
    configured: response.is_configured,
    onlineId: response.online_id,
    credentialsError: response.credentials_error ?? false,
  };
}
```

`getSteamConnection` (~line 387): delete `steamId: response.steam_id ?? '',`:

```ts
export async function getSteamConnection(): Promise<SteamConnectionData> {
  const response = await api.get<SteamConnectionApiResponse>('/sync/steam/connection');
  return {
    connected: response.connected,
    credentialsError: response.credentials_error ?? false,
    username: response.username ?? '',
  };
}
```

- [ ] **Step 3: Prune domain types in `ui/frontend/src/types/sync.ts`**

Remove `accountId?: string;` from `EpicConnectionResponse` (~line 147):

```ts
export interface EpicConnectionResponse {
  connected: boolean;
  disabled: boolean;
  credentialsError?: boolean;
  displayName?: string;
  /** Machine-readable cause when disabled=true, e.g. "legendary_not_configured". */
  reason?: string;
}
```

Remove `userId?: string;` from `GOGConnectionResponse` (~line 162):

```ts
export interface GOGConnectionResponse {
  connected: boolean;
  credentialsError?: boolean;
  username?: string;
  authUrl?: string;
}
```

Remove `accountId: string | null;` from `PSNStatusResponse` (~line 176):

```ts
export interface PSNStatusResponse {
  configured: boolean;
  onlineId: string | null;
  credentialsError: boolean;
}
```

Remove `steamId: string;` from `SteamConnectionData` (~line 185):

```ts
export interface SteamConnectionData {
  connected: boolean;
  credentialsError: boolean;
  username: string;
}
```

Do NOT touch `EpicConnectResponse` (~line 137), `GOGConnectResponse` (~line 153), or `PSNConfigureResponse` (~line 167) — POST shapes, out of scope.

- [ ] **Step 4: Update `ui/frontend/src/api/sync.test.ts`**

In `'should get Epic connection status when connected'` (~lines 159–180): remove `account_id: 'acct-abc',` from `mockResponse` and `accountId: 'acct-abc',` from the `expect(result).toEqual({...})` object.

In the disabled-Epic test below it (~line 197): remove `accountId: undefined,` from the `expect(result).toEqual({...})` object.

Do NOT touch `'should connect Epic with auth code and return account info'` (~line 140) — POST shape.

- [ ] **Step 5: Update `ui/frontend/src/hooks/use-sync.test.ts`**

In `'should fetch Epic connection status'` (~lines 348–370): remove `accountId: 'acct-abc',` from both the `mockGetEpicConnection.mockResolvedValue({...})` object (~line 353) and the `expect(result.current.data).toEqual({...})` object (~line 368).

Do NOT touch `'should call connectEpic with the auth code'` (~line 332) — POST shape.

- [ ] **Step 6: Update `ui/frontend/src/hooks/use-sync-psn.test.ts`**

In the `usePSNStatus` describe block, remove `accountId: 'test-account-id',` from the three `getPSNStatus` mock/expected objects (~lines 103, 118, 130).

Do NOT touch the `useConfigurePSN` describe block (`configurePSN` mocks at ~lines 17, 37, 74) — POST shape.

- [ ] **Step 7: Update `ui/frontend/src/components/sync/epic-connection-card.test.tsx`**

Remove `accountId: 'epic-account-id',` / `accountId: 'y'` from the four `stubEpicConnection({...})` calls (~lines 108, 156, 172, 202). Example — change:

```ts
    stubEpicConnection({ connected: true, disabled: false, displayName: 'X', accountId: 'y' });
```

to:

```ts
    stubEpicConnection({ connected: true, disabled: false, displayName: 'X' });
```

Do NOT touch the `mutateAsync` default mock at ~line 36 (`{ displayName: 'X', accountId: 'y' }`) — that is the `useConnectEpic` POST result.

- [ ] **Step 8: Update `ui/frontend/src/components/sync/gog-connection-card.test.tsx`**

At ~line 86, change:

```ts
    stubGOGConnection({ connected: true, username: 'goguser', userId: 'u1' });
```

to:

```ts
    stubGOGConnection({ connected: true, username: 'goguser' });
```

Do NOT touch the `mutateAsync` default mock at ~line 36 (`{ username: 'goguser', userId: 'u1' }`) — `useConnectGOG` POST result.

- [ ] **Step 9: Typecheck — the completeness gate**

From `ui/frontend/`:

```bash
npm run check
```

Expected: zero errors. TypeScript excess-property checks flag any mock still carrying a pruned field; if it reports one, remove that field there too (applying the same POST-shape scope guard).

- [ ] **Step 10: Run affected frontend tests**

From `ui/frontend/`:

```bash
npm run test sync.test.ts use-sync.test.ts use-sync-psn.test.ts epic-connection-card.test.tsx gog-connection-card.test.tsx psn-connection-card.test.tsx steam-connection-card.test.tsx
```

Expected: all PASS.

- [ ] **Step 11: Dead-code check**

From `ui/frontend/`:

```bash
npm run knip
```

Expected: zero findings.

- [ ] **Step 12: Commit**

```bash
git add ui/frontend/src/api/sync.ts ui/frontend/src/types/sync.ts ui/frontend/src/api/sync.test.ts ui/frontend/src/hooks/use-sync.test.ts ui/frontend/src/hooks/use-sync-psn.test.ts ui/frontend/src/components/sync/epic-connection-card.test.tsx ui/frontend/src/components/sync/gog-connection-card.test.tsx
git commit -m "fix(sync): drop dead account-ID fields from frontend sync GET types"
```

---

### Final verification

- [ ] Full Go suite runs at `git push` via the pre-push hook; frontend `npm run check && npm run knip && npm run test` likewise. No manual full-suite run needed beyond the targeted runs above.
- [ ] `grep -rn "account_id\|user_id\|steam_id" internal/api/sync.go` — remaining hits must all be in POST/connect/configure handlers, stored-credential decode for those handlers, log fields, or SQL `user_id` columns; none in the four GET response shapes.
- [ ] `grep -n "accountId\|userId\|steamId" ui/frontend/src/types/sync.ts` — remaining hits must be only in `SyncConfig.userId`, `EpicConnectResponse.accountId`, `GOGConnectResponse.userId`, `PSNConfigureResponse.accountId`.
