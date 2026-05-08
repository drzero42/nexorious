# Auth Profile Endpoints — Design Spec

## Scope

Four JWT-protected endpoints that complete the Phase 2 auth surface: profile update, password change, username availability check, and username change. All are added to `internal/api/auth.go` following the existing handler pattern (raw Bun SQL, no ORM models for auth).

## Existing Infrastructure

- `AuthHandler` struct in `internal/api/auth.go` — holds `*bun.DB` and `*config.Config`
- `auth.UserIDFromContext(c)` — extracts user ID from JWT middleware context
- `auth.HashToken(token)` — SHA-256 hex digest of a token string
- `meResponse` struct — shared response shape for user profile
- `issueTokensAndSession` — token issuance (not needed here, but shows session pattern)
- `bcrypt` for password hashing (cost constant `bcryptCost` in `setup.go`)
- Route registration via `authGroup` in `registerRoutes` (`internal/api/router.go`)

## Endpoint Specifications

### `PUT /api/auth/me` — Update Preferences

**Request body:**
```json
{ "preferences": { "theme": "dark", "language": "en" } }
```

**Handler:** `HandleUpdateMe`

**Behaviour:**
1. Bind JSON to `updateMeRequest` struct
2. Validate `preferences` is present and is a valid JSON object (unmarshal into `map[string]any` — rejects null, arrays, bare scalars)
3. Extract `userID` from context
4. Execute: `UPDATE users SET preferences = ?, updated_at = NOW() WHERE id = ?`
5. Re-query user row and return `meResponse`

**Errors:**
| Condition | Status | Detail |
|-----------|--------|--------|
| Invalid JSON body | 400 | `"invalid request body"` |
| `preferences` not a JSON object | 400 | `"preferences must be a JSON object"` |
| User not found (stale token) | 401 | `"unauthorized"` |

**Response:** `200 OK` with `meResponse`

---

### `PUT /api/auth/change-password` — Change Password

**Request body:**
```json
{ "current_password": "oldpass", "new_password": "newpass123" }
```

**Handler:** `HandleChangePassword`

**Behaviour:**
1. Bind JSON to `changePasswordRequest` struct
2. Validate:
   - `current_password` is non-empty
   - `new_password` is 8–128 characters
   - `new_password` differs from `current_password`
3. Query `password_hash` for the current user
4. Verify `current_password` against stored hash with `bcrypt.CompareHashAndPassword`
5. Hash new password with `bcrypt.GenerateFromPassword`
6. Execute: `UPDATE users SET password_hash = ?, updated_at = NOW() WHERE id = ?`
7. Invalidate **other** sessions: extract raw bearer token from `Authorization` header, hash with `auth.HashToken`, then `DELETE FROM user_sessions WHERE user_id = ? AND token_hash != ?`
8. Return success message

**Errors:**
| Condition | Status | Detail |
|-----------|--------|--------|
| Invalid JSON body | 400 | `"invalid request body"` |
| Missing `current_password` | 400 | `"current password is required"` |
| `new_password` too short/long | 400 | `"new password must be between 8 and 128 characters"` |
| Same password | 400 | `"new password must be different from current password"` |
| Wrong current password | 400 | `"current password is incorrect"` |
| User not found | 401 | `"unauthorized"` |

**Response:** `200 OK`
```json
{ "message": "Password changed successfully." }
```

---

### `GET /api/auth/username/check/:username` — Check Username Availability

**Handler:** `HandleCheckUsername`

**Behaviour:**
1. Extract `username` from path param
2. Validate length: 3–100 characters
3. Query: `SELECT 1 FROM users WHERE LOWER(username) = LOWER(?) LIMIT 1`
4. Return availability result

**Errors:**
| Condition | Status | Detail |
|-----------|--------|--------|
| Username too short/long | 400 | `"username must be between 3 and 100 characters"` |

**Response:** `200 OK`
```json
{ "available": true, "username": "newname" }
```

---

### `PUT /api/auth/username` — Change Username

**Request body:**
```json
{ "new_username": "newname" }
```

**Handler:** `HandleChangeUsername`

**Behaviour:**
1. Bind JSON to `changeUsernameRequest` struct
2. Validate `new_username` length: 3–100 characters
3. Query current username for the user; if same as `new_username`, return 400
4. Check availability: `SELECT 1 FROM users WHERE LOWER(username) = LOWER(?) LIMIT 1`
5. Execute: `UPDATE users SET username = ?, updated_at = NOW() WHERE id = ?` — catch unique constraint violation (concurrent race) and return 400 `"username already taken"`
6. Re-query user row and return `meResponse`

**Errors:**
| Condition | Status | Detail |
|-----------|--------|--------|
| Invalid JSON body | 400 | `"invalid request body"` |
| Username too short/long | 400 | `"username must be between 3 and 100 characters"` |
| Same as current | 400 | `"new username must be different from current username"` |
| Already taken | 400 | `"username already taken"` |
| User not found | 401 | `"unauthorized"` |

**Response:** `200 OK` with `meResponse`

## Route Registration

All four endpoints are added to the existing `authGroup` in `registerRoutes`:

```go
authGroup.PUT("/me", ah.HandleUpdateMe)
authGroup.PUT("/change-password", ah.HandleChangePassword)
authGroup.GET("/username/check/:username", ah.HandleCheckUsername)
authGroup.PUT("/username", ah.HandleChangeUsername)
```

## Request/Response Types

New types added to `internal/api/auth.go`:

```go
type updateMeRequest struct {
    Preferences json.RawMessage `json:"preferences"`
}

type changePasswordRequest struct {
    CurrentPassword string `json:"current_password"`
    NewPassword     string `json:"new_password"`
}

type changeUsernameRequest struct {
    NewUsername string `json:"new_username"`
}

type usernameAvailabilityResponse struct {
    Available bool   `json:"available"`
    Username  string `json:"username"`
}

type messageResponse struct {
    Message string `json:"message"`
}
```

**Username uniqueness:** Usernames are case-sensitive (stored and displayed as entered) but unique case-insensitively. The `users` table needs a `UNIQUE(LOWER(username))` constraint. The initial migration has a plain `UNIQUE(username)` — add a migration to drop that and create `CREATE UNIQUE INDEX users_username_lower_idx ON users (LOWER(username))` instead. All availability checks use `LOWER()` comparison, and the handler catches the unique constraint violation on UPDATE as a fallback for TOCTOU races.

## Testing

Tests added to `internal/api/auth_test.go` using the existing testcontainers setup.

### `PUT /api/auth/me`
- Updates preferences and returns updated profile
- Rejects non-object preferences (null, array, string)
- Rejects missing preferences field

### `PUT /api/auth/change-password`
- Changes password and can log in with new password
- Wrong current password returns 400
- Same password returns 400
- Password too short returns 400
- Invalidates other sessions but preserves current session

### `GET /api/auth/username/check/:username`
- Returns `available: true` for unused username
- Returns `available: false` for taken username
- Rejects too-short and too-long usernames

### `PUT /api/auth/username`
- Changes username and returns updated profile
- Same username returns 400
- Taken username returns 400
- Too-short username returns 400

## Slumber Collection

Add to `slumber.yaml` under an `auth/` folder section:

- `PUT /api/auth/me` — update preferences
- `PUT /api/auth/change-password` — change password
- `GET /api/auth/username/check/:username` — check availability
- `PUT /api/auth/username` — change username

All with bearer auth using the existing login response chain.

## Checklist

- [ ] Add request/response structs to `internal/api/auth.go`
- [ ] Implement `HandleUpdateMe`
- [ ] Implement `HandleChangePassword` (with other-session invalidation)
- [ ] Implement `HandleCheckUsername`
- [ ] Implement `HandleChangeUsername`
- [ ] Register routes in `registerRoutes`
- [ ] Add tests for all endpoints
- [ ] Add Slumber collection entries
- [ ] Run `go test ./...` — all pass
- [ ] Run `golangci-lint run` — clean
