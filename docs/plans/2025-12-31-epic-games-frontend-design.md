# Epic Games Store Frontend Implementation Design

**Date**: 2025-12-31
**Status**: Design Phase
**Related Backend**: Epic Games Store Sync Integration (Implemented)

## Executive Summary

Add Epic Games Store frontend integration to enable users to connect their Epic account and sync their game library. This implementation follows the existing Steam pattern with a two-step OAuth dialog for Epic's device code authentication flow, proactive auth expiration handling, and complete feature parity with Steam sync capabilities.

## Goals

1. Enable users to authenticate with Epic Games Store via OAuth device code flow
2. Provide clear, guided two-step authentication dialog
3. Match Steam's sync functionality (frequency, auto-add, manual trigger)
4. Handle auth expiration proactively with user notifications
5. Maintain >70% frontend test coverage
6. Follow existing design patterns and component structure

## Non-Goals

- Modifying Steam sync functionality
- Implementing playtime tracking (Epic doesn't provide this data)
- GOG or other platform integrations (separate tasks)
- Real-time sync progress beyond existing polling pattern
- Custom Epic-specific settings beyond standard sync config

## Architecture Overview

### Core Approach

Follow the established Steam pattern with parallel component structure, reusing existing hooks patterns and UI components where possible. Epic's OAuth device code flow requires a two-step dialog instead of inline form fields.

### Key Components

1. **EpicAuthDialog** - Two-step modal for OAuth device code flow
2. **EpicConnectionCard** - Main connection UI showing connected/disconnected state
3. **Epic Settings Page** - Dedicated page for Epic sync frequency and auto-add settings
4. **API Layer Extensions** - Epic auth functions with snake_case transformation
5. **Hook Extensions** - TanStack Query hooks for Epic auth and sync

### Component Structure

```
frontend/src/
├── api/
│   └── sync.ts (extend with Epic auth functions)
├── components/
│   └── sync/
│       ├── epic-connection-card.tsx (NEW)
│       ├── epic-connection-card.test.tsx (NEW)
│       ├── epic-auth-dialog.tsx (NEW)
│       └── epic-auth-dialog.test.tsx (NEW)
├── hooks/
│   └── use-sync.ts (extend with Epic hooks)
├── types/
│   └── sync.ts (extend with Epic types)
└── app/
    └── settings/
        └── sync/
            └── epic/
                └── page.tsx (NEW - Epic settings page)
```

## Data Types & API Integration

### TypeScript Types

Extend existing `types/sync.ts`:

```typescript
// Epic Auth Types
export interface EpicAuthStartResponse {
  authUrl: string;
  instructions: string;
}

export interface EpicAuthCompleteRequest {
  code: string;
}

export interface EpicAuthCompleteResponse {
  valid: boolean;
  displayName: string | null;
  error: string | null;
}

export interface EpicAuthCheckResponse {
  isAuthenticated: boolean;
  displayName: string | null;
}

export interface EpicConnectionInfo {
  configured: boolean;
  displayName: string | null;
  accountId: string | null;
}

// Error messages for Epic auth
export const EPIC_AUTH_ERROR_MESSAGES: Record<string, string> = {
  invalid_code: 'Invalid authorization code. Please try again.',
  network_error: 'Could not connect to Epic Games. Please try again.',
  expired_code: 'Authorization code expired. Please request a new one.',
};

// Update supported platforms
export const SUPPORTED_SYNC_PLATFORMS: SyncPlatform[] = [
  SyncPlatform.STEAM,
  SyncPlatform.EPIC, // ADD THIS
];

// Update SyncStatus for auth expiration
export interface SyncStatus {
  platform: SyncPlatform;
  isSyncing: boolean;
  lastSyncedAt: string | null;
  activeJobId: string | null;
  requiresReauth?: boolean;  // NEW
  authExpired?: boolean;      // NEW
}
```

### API Functions

Extend `api/sync.ts` with Epic-specific functions:

```typescript
// Epic Auth API Response Types (snake_case from backend)
interface EpicAuthStartApiResponse {
  auth_url: string;
  instructions: string;
}

interface EpicAuthCompleteApiResponse {
  valid: boolean;
  display_name: string | null;
  error: string | null;
}

interface EpicAuthCheckApiResponse {
  is_authenticated: boolean;
  display_name: string | null;
}

// Epic Auth API Functions
export async function startEpicAuth(): Promise<EpicAuthStartResponse> {
  const response = await api.post<EpicAuthStartApiResponse>('/sync/epic/auth/start');
  return {
    authUrl: response.auth_url,
    instructions: response.instructions,
  };
}

export async function completeEpicAuth(code: string): Promise<EpicAuthCompleteResponse> {
  const response = await api.post<EpicAuthCompleteApiResponse>('/sync/epic/auth/complete', {
    code,
  });
  return {
    valid: response.valid,
    displayName: response.display_name,
    error: response.error,
  };
}

export async function checkEpicAuth(): Promise<EpicAuthCheckResponse> {
  const response = await api.get<EpicAuthCheckApiResponse>('/sync/epic/auth/check');
  return {
    isAuthenticated: response.is_authenticated,
    displayName: response.display_name,
  };
}

export async function disconnectEpic(): Promise<void> {
  await api.delete('/sync/epic/connection');
}
```

## React Hooks & State Management

### Custom Hooks

Extend `hooks/use-sync.ts` with Epic-specific hooks using TanStack Query:

```typescript
// Epic Auth Hooks

export function useStartEpicAuth() {
  return useMutation({
    mutationFn: startEpicAuth,
    onError: (error) => {
      console.error('Failed to start Epic auth:', error);
    },
  });
}

export function useCompleteEpicAuth() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: (code: string) => completeEpicAuth(code),
    onSuccess: () => {
      // Invalidate sync configs to refresh connection status
      queryClient.invalidateQueries({ queryKey: ['syncConfigs'] });
      queryClient.invalidateQueries({ queryKey: ['syncConfig', SyncPlatform.EPIC] });
    },
  });
}

export function useCheckEpicAuth() {
  return useQuery({
    queryKey: ['epicAuth'],
    queryFn: checkEpicAuth,
    staleTime: 5 * 60 * 1000, // 5 minutes
    refetchOnWindowFocus: true,
  });
}

export function useDisconnectEpic() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: disconnectEpic,
    onSuccess: () => {
      // Invalidate all Epic-related queries
      queryClient.invalidateQueries({ queryKey: ['syncConfigs'] });
      queryClient.invalidateQueries({ queryKey: ['syncConfig', SyncPlatform.EPIC] });
      queryClient.invalidateQueries({ queryKey: ['epicAuth'] });
    },
    onError: (error) => {
      console.error('Failed to disconnect Epic:', error);
    },
  });
}
```

### Hook Integration

These hooks integrate with TanStack Query's caching and invalidation:

- `useStartEpicAuth`: Initiates OAuth flow, returns auth URL
- `useCompleteEpicAuth`: Submits auth code, invalidates configs on success
- `useCheckEpicAuth`: Checks current auth status (cached for 5 min)
- `useDisconnectEpic`: Removes Epic connection, invalidates all Epic queries

## Component Design

### EpicAuthDialog Component

**Purpose**: Two-step modal for Epic OAuth device code flow

**Location**: `components/sync/epic-auth-dialog.tsx`

**Props**:
```typescript
interface EpicAuthDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onSuccess: () => void;
}
```

**State Management**:
- `step`: 'start' | 'code' - tracks current step in flow
- `authUrl`: string - Epic OAuth URL from backend
- `urlCopied`: boolean - for copy button feedback

**Step 1: Start Authentication**
- Title: "Connect Epic Games Store"
- Description explaining the OAuth flow
- "Start Authentication" button
- Calls `startEpicAuth()` on click
- Shows loading spinner while requesting URL
- Transitions to step 2 on success

**Step 2: Enter Code**
- Title: "Enter Authorization Code"
- Box with "Open Epic Login" button (opens authUrl in new tab)
- Copy URL button with visual feedback
- Input field for authorization code
- Minimal validation (just required field check)
- "Cancel" and "Connect" buttons
- Calls `completeEpicAuth(code)` on submit
- Shows loading spinner during verification

**Success Flow**:
- On successful auth completion:
  - Auto-close dialog
  - Show toast: "Epic Games connected as [displayName]"
  - Call `onSuccess()` callback
  - Reset dialog state

**Error Handling**:
- Map backend error codes to user-friendly messages
- Show toast for network/API errors
- Keep dialog open on error for retry

**Form Validation**:
```typescript
const authCodeSchema = z.object({
  code: z.string().min(1, 'Authorization code is required'),
});
```

### EpicConnectionCard Component

**Purpose**: Main connection UI showing Epic account status

**Location**: `components/sync/epic-connection-card.tsx`

**Props**:
```typescript
interface EpicConnectionCardProps {
  isConfigured: boolean;
  displayName?: string;
  accountId?: string;
  onConnectionChange: () => void;
}
```

**Card Structure**:
- Card with header: "Epic Games Connection"
- Status badge (Connected/Not Configured/Auth Expired)
- Description changes based on state

**Not Connected State**:
- "Connect your Epic account to sync your game library" description
- "Connect Epic Games" button
- Info callout: "Note: Playtime data is not available for Epic games"
- Clicking connect opens EpicAuthDialog

**Connected State**:
- "Your Epic account is connected" description
- Display connected account name and ID
- "Disconnect" button with AlertDialog confirmation
- Same confirmation pattern as Steam
- Info callout about playtime limitation

**Auth Expired State**:
- Badge: "Auth Expired" (yellow/warning colors)
- "Your Epic authentication has expired" description
- "Re-authenticate" button (opens auth dialog)
- Same flow as initial connection

**Badge States**:
```typescript
const getBadgeState = () => {
  if (syncStatus?.authExpired || syncStatus?.requiresReauth) {
    return {
      label: 'Auth Expired',
      className: 'bg-yellow-100 text-yellow-800 dark:bg-yellow-900/30 dark:text-yellow-400'
    };
  }
  if (!isConfigured) {
    return {
      label: 'Not Configured',
      className: 'bg-muted text-muted-foreground'
    };
  }
  return {
    label: 'Connected',
    className: 'bg-green-100 text-green-800 dark:bg-green-900/30 dark:text-green-400'
  };
};
```

**Disconnect Confirmation**:
```typescript
<AlertDialog>
  <AlertDialogTrigger asChild>
    <Button variant="outline">Disconnect</Button>
  </AlertDialogTrigger>
  <AlertDialogContent>
    <AlertDialogHeader>
      <AlertDialogTitle>Disconnect Epic Games?</AlertDialogTitle>
      <AlertDialogDescription>
        Your sync settings will be preserved but syncing will stop until you reconnect.
      </AlertDialogDescription>
    </AlertDialogHeader>
    <AlertDialogFooter>
      <AlertDialogCancel>Cancel</AlertDialogCancel>
      <AlertDialogAction onClick={handleDisconnect}>
        Disconnect
      </AlertDialogAction>
    </AlertDialogFooter>
  </AlertDialogContent>
</AlertDialog>
```

## Page Integration

### Main Sync Settings Page

**Location**: `app/settings/sync/page.tsx` (existing)

**Updates**:
```typescript
import { SteamConnectionCard } from '@/components/sync/steam-connection-card';
import { EpicConnectionCard } from '@/components/sync/epic-connection-card';
import { useSyncConfigs } from '@/hooks';

// In the page component:
const { data: configs, refetch } = useSyncConfigs();

const steamConfig = configs?.configs.find(c => c.platform === SyncPlatform.STEAM);
const epicConfig = configs?.configs.find(c => c.platform === SyncPlatform.EPIC);

// Get Epic connection info from user preferences
const epicPrefs = user?.preferences?.epic;

// Render both cards:
<div className="space-y-6">
  <SteamConnectionCard
    isConfigured={steamConfig?.isConfigured ?? false}
    steamId={steamPrefs?.steam_id}
    steamUsername={steamPrefs?.username}
    onConnectionChange={refetch}
  />

  <EpicConnectionCard
    isConfigured={epicConfig?.isConfigured ?? false}
    displayName={epicPrefs?.display_name}
    accountId={epicPrefs?.account_id}
    onConnectionChange={refetch}
  />
</div>
```

### Epic Settings Detail Page

**Location**: `app/settings/sync/epic/page.tsx` (NEW)

**Structure**: Mirrors Steam detail page

**Sections**:
1. Page header with back navigation to main sync page
2. Connection status section (shows Epic account or "Not connected")
3. Sync frequency dropdown (Manual/Hourly/Daily/Weekly)
4. Auto-add toggle switch
5. Manual "Sync Now" button with status polling
6. Last synced timestamp display

**Implementation**:
```typescript
'use client';

import { SyncServiceCard } from '@/components/sync/sync-service-card';
import { SyncPlatform } from '@/types';

export default function EpicSyncSettingsPage() {
  return (
    <div className="container max-w-4xl py-8">
      <SyncServiceCard platform={SyncPlatform.EPIC} />
    </div>
  );
}
```

Reuses existing `SyncServiceCard` component (same as Steam), just configured for Epic platform. This component already handles:
- Sync frequency selection
- Auto-add toggle
- Manual sync trigger
- Sync status polling
- Last synced display

## Auth Expiration Handling

### Detection Strategy

The backend includes `requires_reauth` and `auth_expired` flags in the sync status response when legendary reports authentication errors.

**Polling Integration**:
```typescript
// In Epic settings page or connection card
const { data: syncStatus, refetch } = useSyncStatus(SyncPlatform.EPIC);

// Monitor for auth expiration
useEffect(() => {
  if (syncStatus?.requiresReauth || syncStatus?.authExpired) {
    toast.error('Epic authentication expired. Please reconnect.', {
      action: {
        label: 'Reconnect',
        onClick: () => setAuthDialogOpen(true),
      },
    });
  }
}, [syncStatus?.requiresReauth, syncStatus?.authExpired]);
```

### User Experience Flow

1. User triggers Epic sync (manual or automatic)
2. Backend attempts sync, legendary reports auth error
3. Backend marks job as failed with `requires_reauth: true`
4. Frontend polls sync status, detects auth expiration
5. Connection card updates badge to "Auth Expired" (yellow)
6. Toast notification appears with "Reconnect" action button
7. User clicks "Reconnect" → opens EpicAuthDialog
8. User completes OAuth flow again (same as initial connection)
9. Connection restored, sync can be triggered again

### Proactive Notification

**Toast with Action**:
```typescript
toast.error('Epic authentication expired. Please reconnect.', {
  action: {
    label: 'Reconnect',
    onClick: () => setAuthDialogOpen(true),
  },
  duration: 10000, // Longer duration for important action
});
```

**Badge Update**:
Connection card automatically shows warning state based on sync status polling, no additional API calls needed.

## Testing Strategy

### Test Coverage Goals

- **API layer**: 100% coverage (all functions tested)
- **Hooks**: >80% coverage (all mutations and queries)
- **Components**: >70% coverage (key user paths and error states)
- **Overall frontend**: Maintain existing >70% threshold

### API Layer Tests

**File**: `api/sync.test.ts` (extend existing)

**Tests**:
```typescript
describe('Epic Auth API', () => {
  it('should start Epic auth and return URL', async () => {
    // Mock POST /sync/epic/auth/start
    // Verify authUrl and instructions returned
  });

  it('should complete Epic auth with valid code', async () => {
    // Mock POST /sync/epic/auth/complete
    // Verify successful response with displayName
  });

  it('should handle invalid auth code', async () => {
    // Mock error response
    // Verify error field populated
  });

  it('should check Epic auth status', async () => {
    // Mock GET /sync/epic/auth/check
    // Verify isAuthenticated and displayName
  });

  it('should disconnect Epic', async () => {
    // Mock DELETE /sync/epic/connection
    // Verify no errors
  });

  it('should transform snake_case to camelCase', async () => {
    // Verify all API responses properly transformed
  });
});
```

### Hook Tests

**File**: `hooks/use-sync.test.ts` (extend existing)

**Tests**:
```typescript
describe('Epic Auth Hooks', () => {
  it('should call startEpicAuth mutation', async () => {
    // Test useStartEpicAuth hook
    // Verify API function called
  });

  it('should invalidate queries on successful auth', async () => {
    // Test useCompleteEpicAuth hook
    // Verify queryClient.invalidateQueries called
  });

  it('should cache Epic auth status', async () => {
    // Test useCheckEpicAuth hook
    // Verify staleTime and refetch behavior
  });

  it('should invalidate all Epic queries on disconnect', async () => {
    // Test useDisconnectEpic hook
    // Verify all Epic queries invalidated
  });
});
```

### Component Tests

**EpicAuthDialog Tests** (`epic-auth-dialog.test.tsx`):
```typescript
describe('EpicAuthDialog', () => {
  it('should render step 1 with start button', () => {
    // Verify initial dialog state
  });

  it('should call startEpicAuth on button click', async () => {
    // Mock startEpicAuth
    // Click start button
    // Verify mutation called
  });

  it('should transition to step 2 after start', async () => {
    // Start auth flow
    // Verify step 2 renders with code input
  });

  it('should open Epic URL in new tab', async () => {
    // Mock window.open
    // Click "Open Epic Login"
    // Verify window.open called with authUrl
  });

  it('should copy URL to clipboard', async () => {
    // Mock navigator.clipboard
    // Click copy button
    // Verify clipboard.writeText called
  });

  it('should submit auth code', async () => {
    // Fill code input
    // Submit form
    // Verify completeEpicAuth called
  });

  it('should show error for invalid code', async () => {
    // Mock failed auth response
    // Verify toast.error called
  });

  it('should close and reset on success', async () => {
    // Mock successful auth
    // Verify dialog closes
    // Verify onSuccess callback called
  });

  it('should reset state on cancel', async () => {
    // Navigate to step 2
    // Click cancel
    // Verify state reset and dialog closed
  });
});
```

**EpicConnectionCard Tests** (`epic-connection-card.test.tsx`):
```typescript
describe('EpicConnectionCard', () => {
  it('should render not configured state', () => {
    // Pass isConfigured: false
    // Verify "Connect Epic Games" button
  });

  it('should open auth dialog on connect click', async () => {
    // Click connect button
    // Verify dialog opens
  });

  it('should render connected state', () => {
    // Pass isConfigured: true with displayName
    // Verify display name and disconnect button
  });

  it('should show disconnect confirmation', async () => {
    // Click disconnect
    // Verify AlertDialog appears
  });

  it('should call disconnectEpic on confirm', async () => {
    // Open disconnect dialog
    // Click confirm
    // Verify disconnectEpic called
  });

  it('should render auth expired state', () => {
    // Mock syncStatus with authExpired: true
    // Verify "Auth Expired" badge
    // Verify "Re-authenticate" button
  });

  it('should show playtime limitation note', () => {
    // Verify info callout about playtime
  });
});
```

### Integration Tests

**Full Auth Flow**:
```typescript
it('should complete full Epic auth flow', async () => {
  // 1. Render connection card (not configured)
  // 2. Click "Connect Epic Games"
  // 3. Verify dialog opens
  // 4. Click "Start Authentication"
  // 5. Verify step 2 with code input
  // 6. Enter authorization code
  // 7. Click "Connect"
  // 8. Verify success toast
  // 9. Verify dialog closes
  // 10. Verify connection card shows connected state
});
```

**Auth Expiration Flow**:
```typescript
it('should handle auth expiration during sync', async () => {
  // 1. Mock syncStatus with authExpired: true
  // 2. Render connection card
  // 3. Verify "Auth Expired" badge
  // 4. Verify toast notification
  // 5. Click "Reconnect" in toast
  // 6. Verify auth dialog opens
  // 7. Complete re-authentication
  // 8. Verify connection restored
});
```

## Implementation Plan

### Phase 1: Foundation (Types & API)

**Files**:
- `types/sync.ts`
- `api/sync.ts`
- `api/sync.test.ts`

**Tasks**:
1. Add Epic type definitions to `types/sync.ts`
2. Update `SUPPORTED_SYNC_PLATFORMS` array
3. Add Epic error messages constant
4. Implement Epic API functions in `api/sync.ts`
5. Write API tests in `api/sync.test.ts`
6. Run tests: `npm run test api/sync.test.ts`
7. Verify type checking: `npm run check`

**Acceptance**:
- All Epic types defined
- API functions transform snake_case correctly
- All API tests pass

### Phase 2: Hooks Layer

**Files**:
- `hooks/use-sync.ts`
- `hooks/use-sync.test.ts`

**Tasks**:
1. Add Epic auth hooks to `use-sync.ts`
2. Implement query/mutation with proper cache invalidation
3. Write hook tests in `use-sync.test.ts`
4. Run tests: `npm run test hooks/use-sync.test.ts`
5. Verify TanStack Query integration

**Acceptance**:
- All four Epic hooks implemented
- Cache invalidation works correctly
- All hook tests pass

### Phase 3: Auth Dialog Component

**Files**:
- `components/sync/epic-auth-dialog.tsx`
- `components/sync/epic-auth-dialog.test.tsx`

**Tasks**:
1. Create `EpicAuthDialog` component
2. Implement two-step flow (start → code)
3. Add form validation with Zod
4. Implement URL copy functionality
5. Add error handling and loading states
6. Write component tests
7. Run tests: `npm run test epic-auth-dialog.test.tsx`

**Acceptance**:
- Dialog renders both steps correctly
- Form validation works
- Success/error flows tested
- Component tests pass

### Phase 4: Connection Card Component

**Files**:
- `components/sync/epic-connection-card.tsx`
- `components/sync/epic-connection-card.test.tsx`

**Tasks**:
1. Create `EpicConnectionCard` component
2. Implement three states: not configured, connected, auth expired
3. Integrate `EpicAuthDialog`
4. Add disconnect confirmation with `AlertDialog`
5. Add playtime limitation note
6. Implement badge state logic
7. Write component tests
8. Run tests: `npm run test epic-connection-card.test.tsx`

**Acceptance**:
- All three states render correctly
- Auth dialog integration works
- Disconnect confirmation works
- Component tests pass

### Phase 5: Page Integration

**Files**:
- Main sync settings page (existing)
- `app/settings/sync/epic/page.tsx` (NEW)

**Tasks**:
1. Add `EpicConnectionCard` to main sync page
2. Update sync page to fetch Epic preferences
3. Create Epic detail page using `SyncServiceCard`
4. Test navigation between pages
5. Verify data flow and refetching

**Acceptance**:
- Epic card appears on main sync page
- Epic detail page accessible
- Navigation works correctly
- Data updates propagate

### Phase 6: Auth Expiration Integration

**Files**:
- `components/sync/epic-connection-card.tsx`
- Epic detail page

**Tasks**:
1. Add sync status polling to connection card
2. Implement auth expiration detection
3. Add toast notification with reconnect action
4. Update badge state for expired auth
5. Test full expiration → re-auth flow

**Acceptance**:
- Auth expiration detected from sync status
- Toast appears with reconnect action
- Badge shows warning state
- Re-authentication works

### Phase 7: Testing & Polish

**Tasks**:
1. Run full test suite: `npm run test`
2. Verify coverage: `npm run test:coverage`
3. Run type checking: `npm run check`
4. Manual testing:
   - Full auth flow (start → code → connected)
   - Disconnect flow with confirmation
   - Sync trigger and status polling
   - Auth expiration and re-auth
   - Navigation between pages
5. Fix any failing tests or type errors
6. Polish UI/UX based on manual testing

**Acceptance**:
- All tests pass
- Coverage >70%
- No TypeScript errors
- Manual testing checklist complete

## Success Criteria

### Functional Requirements

✅ **Authentication Flow**
- Users can click "Connect Epic Games" and start OAuth flow
- Two-step dialog guides users through Epic login
- Authorization code submission works correctly
- Success toast shows Epic display name
- Connection card updates to show connected state

✅ **Sync Functionality**
- Users can trigger manual Epic sync from detail page
- Sync status polling shows real-time progress (matching Steam behavior)
- Last synced timestamp displays correctly
- Sync frequency and auto-add settings persist

✅ **Auth Expiration Handling**
- Expired auth detected via sync status polling
- Connection card shows "Auth Expired" badge
- Toast notification with "Reconnect" action appears
- Re-authentication flow works identically to initial auth

✅ **UI/UX Quality**
- Epic card appears alongside Steam on main sync page
- Epic detail page accessible and fully functional
- Playtime limitation note visible to users
- Disconnect confirmation dialog prevents accidents
- All components follow existing design system

✅ **Testing & Quality**
- All new tests pass (`npm run test`)
- Type checking passes (`npm run check`)
- Frontend coverage remains >70%
- Manual testing confirms full flow works end-to-end

### Non-Functional Requirements

✅ **Code Quality**
- Follows existing Steam patterns
- Components are reusable and well-typed
- Proper error handling throughout
- Clear component and function naming

✅ **Performance**
- No unnecessary re-renders
- TanStack Query caching optimized
- Sync status polling efficient

✅ **Maintainability**
- Clear component structure
- Well-documented props and types
- Test coverage for key flows
- Easy to extend for future platforms (GOG, etc.)

## Files Checklist

### New Files (7)

- [ ] `components/sync/epic-auth-dialog.tsx`
- [ ] `components/sync/epic-auth-dialog.test.tsx`
- [ ] `components/sync/epic-connection-card.tsx`
- [ ] `components/sync/epic-connection-card.test.tsx`
- [ ] `app/settings/sync/epic/page.tsx`

### Modified Files (4)

- [ ] `types/sync.ts` (add Epic types)
- [ ] `api/sync.ts` (add Epic API functions)
- [ ] `hooks/use-sync.ts` (add Epic hooks)
- [ ] Main sync settings page (add Epic card)

### Test Files (2)

- [ ] `api/sync.test.ts` (extend with Epic tests)
- [ ] `hooks/use-sync.test.ts` (extend with Epic tests)

## Future Enhancements

### Post-Implementation Considerations

1. **User Documentation**
   - Update help docs with Epic setup instructions
   - Add screenshots of auth flow
   - Document playtime limitation

2. **Monitoring**
   - Track Epic sync success/failure rates
   - Monitor auth expiration frequency
   - Measure user adoption

3. **Platform Expansion**
   - GOG integration following same patterns
   - PlayStation, Xbox integrations
   - Unified platform management UI

4. **Performance Optimization**
   - Adjust polling frequency based on usage
   - Implement WebSocket for real-time sync updates
   - Optimize multi-platform concurrent syncs

5. **Enhanced Error Handling**
   - More granular error codes from backend
   - Better retry mechanisms
   - Offline support detection

## References

- [Epic Games Store Sync Backend Design](./2025-12-31-epic-games-store-sync-design.md)
- [Epic Games Store Sync Implementation](./2025-12-31-epic-games-store-sync-implementation.md)
- Existing Steam implementation: `components/sync/steam-connection-card.tsx`
- Sync service card: `components/sync/sync-service-card.tsx`
- TanStack Query docs: https://tanstack.com/query/latest
