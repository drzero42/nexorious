# Steam Sync Configuration Design

## Problem Statement

Steam sync currently shows as "Connected" despite not being configured. The `enabled` flag defaults to true, and there's no UI for users to enter their Steam credentials (Steam ID and Web API Key). Users need clear instructions for obtaining these credentials and making their Steam profile public.

## Solution Overview

Add a Steam configuration UI to the `/sync/steam` page with credential verification, clear help instructions, and proper status badges reflecting the actual connection state.

## Data Model & API Changes

### 1. Default `enabled` to `false`

In `UserSyncConfig` model and wherever default configs are created, ensure `enabled=False` by default.

### 2. New API endpoint for credential verification

```
POST /sync/steam/verify
Body: { "steam_id": "...", "web_api_key": "..." }
Response (success): { "valid": true, "steam_username": "..." }
Response (failure): { "valid": false, "error": "invalid_api_key" | "invalid_steam_id" | "private_profile" | "rate_limited" | "network_error" }
```

This endpoint:
- Validates the API key format (32 alphanumeric characters)
- Validates the Steam ID format (17 digits starting with 7656119)
- Makes a test call to Steam API to verify credentials work
- Returns the Steam username on success (nice feedback for user)
- Returns specific error messages on failure

### 3. Extend `GET /sync/config` response

Add `is_configured: boolean` to `SyncConfigResponse`. Backend checks if `user.preferences.steam.is_verified` is true.

```python
class SyncConfigResponse(BaseModel):
    # existing fields...
    enabled: bool
    frequency: SyncFrequency
    auto_add: bool
    last_synced_at: Optional[datetime]
    # new field
    is_configured: bool  # true if credentials are verified
```

### 4. New endpoint for disconnect

```
DELETE /sync/steam/connection
```

- Clears Steam credentials from user preferences
- Sets `enabled: false` on the sync config

## Frontend State & Badge Logic

### Badge States

Three distinct states replace the current "Connected"/"Disconnected":

| State | Condition | Badge Style |
|-------|-----------|-------------|
| Not Configured | `!isConfigured` | Grey/neutral (`bg-muted text-muted-foreground`) |
| Enabled | `isConfigured && enabled` | Green (`bg-green-100 text-green-800`) |
| Disabled | `isConfigured && !enabled` | Yellow (`bg-yellow-100 text-yellow-800`) |

### Badge Logic

```typescript
function getBadgeState(config: SyncConfig): 'not-configured' | 'enabled' | 'disabled' {
  if (!config.isConfigured) return 'not-configured'
  return config.enabled ? 'enabled' : 'disabled'
}
```

### Control States

When `isConfigured` is false:
- Sync toggle is disabled (can't enable sync without credentials)
- Frequency dropdown is disabled
- Auto-add toggle is disabled
- "Sync Now" button is disabled or hidden

## Steam Configuration UI

### Layout on `/sync/steam` Page

At the top of the page, before the existing sync controls, add a Steam Connection section.

**When NOT configured:**
```
┌─────────────────────────────────────────────────────────┐
│ Steam Connection                        [Not Configured]│
├─────────────────────────────────────────────────────────┤
│ Connect your Steam account to sync your game library.   │
│                                                         │
│ Steam ID                                                │
│ ┌─────────────────────────────────────────────────────┐ │
│ │ 76561198012345678                                   │ │
│ └─────────────────────────────────────────────────────┘ │
│ ▶ How do I find my Steam ID?                            │
│                                                         │
│ Steam Web API Key                                       │
│ ┌─────────────────────────────────────────────────────┐ │
│ │ ••••••••••••••••••••••••••••••••                    │ │
│ └─────────────────────────────────────────────────────┘ │
│ ▶ How do I get an API key?                              │
│                                                         │
│                                    [Verify & Connect]   │
└─────────────────────────────────────────────────────────┘
```

**When configured:**
```
┌─────────────────────────────────────────────────────────┐
│ Steam Connection                             [Enabled]  │
├─────────────────────────────────────────────────────────┤
│ ✓ Connected as PlayerName (76561198012345678)           │
│                                          [Disconnect]   │
└─────────────────────────────────────────────────────────┘
```

The existing sync controls (Enable toggle, Frequency, Auto-add) appear below this section, but are disabled until configured.

## Help Instructions Content

### "How do I find my Steam ID?" (Accordion)

**Your Steam ID is a 17-digit number that uniquely identifies your account.**

1. Open Steam and go to your **Profile** (click your username in the top right)
2. Look at the URL in your browser or Steam client:
   - If it shows `steamcommunity.com/id/customname/`, you have a custom URL
   - If it shows `steamcommunity.com/profiles/76561198012345678/`, the number is your Steam ID
3. **If you have a custom URL:** Go to [steamid.io](https://steamid.io), paste your profile URL, and copy the **steamID64** value

**Important:** Your Steam profile must be set to **Public** for sync to work.

To make your profile public:
1. Go to **Steam → Settings → Privacy Settings**
2. Set "My profile" to **Public**
3. Set "Game details" to **Public**

### "How do I get an API key?" (Accordion)

**A Steam Web API key allows Nexorious to read your game library.**

1. Go to [Steam Web API Key Registration](https://steamcommunity.com/dev/apikey)
2. Sign in with your Steam account if prompted
3. Enter a domain name (you can use `localhost` or any domain)
4. Click **Register** and copy the 32-character key

**Note:** Keep your API key private. It's stored securely and only used to sync your library.

## Verification Flow & Error Handling

### Verification Process

When user clicks "Verify & Connect":

1. **Client-side validation first:**
   - Steam ID: Must be 17 digits, starts with `7656119`
   - API Key: Must be 32 alphanumeric characters
   - Show inline validation errors immediately

2. **API call to `POST /sync/steam/verify`:**
   - Show loading spinner on button ("Verifying...")
   - Disable form inputs during verification

3. **On success:**
   - Save credentials via `PUT /auth/me` with `preferences.steam`
   - Show success toast: "Steam connected successfully"
   - UI transitions to "Connected as PlayerName" state
   - Sync controls become enabled

4. **On failure, show specific errors:**
   - `invalid_api_key`: "Invalid API key. Please check and try again."
   - `invalid_steam_id`: "Steam ID not found. Please verify the number."
   - `private_profile`: "Your Steam profile or game details are set to private. Please make them public and try again."
   - `rate_limited`: "Steam API rate limit reached. Please try again in a few minutes."
   - `network_error`: "Could not connect to Steam. Please try again."

### Disconnect Flow

When user clicks "Disconnect":
- Show confirmation dialog: "Disconnect Steam? Your sync settings will be preserved but syncing will stop."
- On confirm: Clear `preferences.steam` via API, set `enabled: false`
- UI returns to "Not Configured" state

## Implementation Summary

### Backend Changes

1. **New endpoint `POST /sync/steam/verify`** - Validates and tests Steam credentials
2. **Extend `GET /sync/config` response** - Add `is_configured: boolean` field
3. **Update default config creation** - Ensure `enabled` defaults to `false`
4. **New endpoint `DELETE /sync/steam/connection`** - For disconnect functionality

### Frontend Changes

1. **Update `SyncConfig` type** - Add `isConfigured: boolean`
2. **Update badge logic in `sync-service-card.tsx`** - Three states: "Not Configured", "Enabled", "Disabled"
3. **New `SteamConnectionCard` component** - Configuration form with help accordions
4. **Update `/sync/steam` page** - Add `SteamConnectionCard` at top, disable controls when not configured
5. **Update `/sync` page cards** - Show correct badge state based on `isConfigured` and `enabled`
