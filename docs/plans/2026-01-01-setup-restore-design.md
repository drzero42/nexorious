# Design: Restore from Backup During Initial Setup

## Overview

Add the ability to restore from a backup file during initial setup, as an alternative to creating a new admin user. This enables disaster recovery, server migration, and testing scenarios without requiring a throwaway admin account.

## UI Changes

### Setup Page (`/setup`)

The existing admin creation form remains the primary UI. Below the form, add a secondary link:

```
"Restore from backup"
```

Clicking this link reveals an inline file upload area below the form:

- Dropzone for `.tar.gz` backup files
- File size and name displayed after selection
- "Cancel" link to collapse the upload area
- "Restore" button to initiate restore

The admin creation form remains visible but disabled while the restore area is expanded (or vice versa - one active at a time).

## Backend Changes

### New Endpoint: `POST /api/auth/setup/restore`

- **No authentication required** (like the existing setup endpoints)
- **Guard**: Only works when `needs_setup=true` (no users exist)
- **Accepts**: Multipart file upload (`.tar.gz` backup)
- **Process**:
  1. Validate backup structure and manifest (reuse existing `BackupService` validation)
  2. Restore database from backup (reuse existing restore logic)
  3. Restore files (cover art, logos)
  4. Skip pre-restore backup creation (database is empty anyway)
  5. Run Alembic migrations if needed
- **Response**: Success message, redirect client to `/login`

The endpoint reuses the existing `BackupService.restore_from_upload()` logic but:
- Skips the admin authentication check
- Skips pre-restore backup (nothing to preserve)
- Doesn't attempt session preservation

## Frontend Changes

### Setup Page (`/setup/page.tsx`)

New state:
- `showRestore: boolean` - toggles restore area visibility
- `restoreFile: File | null` - selected backup file
- `isRestoring: boolean` - loading state during restore

New UI elements:
- Link below the form: "Restore from backup"
- Collapsible restore section with:
  - File input/dropzone accepting `.tar.gz`
  - Selected file display (name, size)
  - "Cancel" to collapse and clear file
  - "Restore" button (disabled until file selected)
  - Progress/loading state during restore

Behavior:
- When restore area is shown, disable the admin creation form (and vice versa)
- On successful restore: toast success message, redirect to `/login`
- On failure: toast error message, keep restore area open

### API Client (`/api/auth.ts`)

New function:
```typescript
setupRestore(file: File): Promise<{ message: string }>
```

## Error Handling

### Backend errors:
- Invalid file format → 400 "Invalid backup file format"
- Corrupt/incomplete backup → 400 "Backup file is corrupt or incomplete"
- Setup not needed (users exist) → 400 "Setup has already been completed"
- Database restore failure → 500 with details

### Frontend handling:
- Display error messages via toast
- Keep restore area open on error so user can try again
- No special handling needed - standard error pattern

## Files to Modify

| File | Changes |
|------|---------|
| `backend/app/api/auth.py` | Add `POST /api/auth/setup/restore` endpoint |
| `backend/app/services/backup_service.py` | Add `restore_for_setup()` method (or param to skip pre-backup) |
| `frontend/src/app/(auth)/setup/page.tsx` | Add restore link, collapsible upload area, restore logic |
| `frontend/src/api/auth.ts` | Add `setupRestore()` function |
