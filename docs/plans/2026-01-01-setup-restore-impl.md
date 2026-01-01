# Setup Restore Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Allow users to restore from a backup file during initial setup instead of creating a new admin account.

**Architecture:** Add a new unauthenticated endpoint `POST /api/auth/setup/restore` that reuses the existing `BackupService` restore logic with `skip_prerestore=True`. The frontend setup page gets a collapsible restore section that appears when clicking "Restore from backup".

**Tech Stack:** FastAPI, SQLModel, pytest (backend); React, TypeScript, Vitest (frontend)

---

## Task 1: Add Backend Schema for Setup Restore Response

**Files:**
- Modify: `backend/app/schemas/auth.py`

**Step 1: Add the response schema**

Add at the end of `backend/app/schemas/auth.py`:

```python
class SetupRestoreResponse(BaseModel):
    """Response schema for setup restore."""
    success: bool = Field(..., description="Whether restore was successful")
    message: str = Field(..., description="Status message")
```

**Step 2: Commit**

```bash
git add backend/app/schemas/auth.py
git commit -m "feat(auth): add SetupRestoreResponse schema"
```

---

## Task 2: Add Backend Endpoint for Setup Restore

**Files:**
- Modify: `backend/app/api/auth.py`

**Step 1: Add imports at the top of the file**

Add to the existing imports in `backend/app/api/auth.py`:

```python
from fastapi import UploadFile, File
import tempfile
import shutil
from pathlib import Path
```

Also add `SetupRestoreResponse` to the schemas import:

```python
from ..schemas.auth import (
    # ... existing imports ...
    SetupRestoreResponse,
)
```

**Step 2: Add the setup restore endpoint**

Add after the `create_initial_admin` endpoint (around line 114):

```python
@router.post("/setup/restore", response_model=SetupRestoreResponse)
async def restore_from_backup_setup(
    file: Annotated[UploadFile, File(description="Backup archive file (.tar.gz)")],
    session: Annotated[Session, Depends(get_session)]
):
    """Restore from backup during initial setup. Only works when no users exist."""
    from ..services.backup_service import backup_service

    # Check if any users already exist
    existing_user = session.exec(select(User)).first()
    if existing_user:
        raise HTTPException(
            status_code=status.HTTP_400_BAD_REQUEST,
            detail="Setup has already been completed. Users already exist."
        )

    # Validate file format
    if not file.filename or not file.filename.endswith(".tar.gz"):
        raise HTTPException(
            status_code=status.HTTP_400_BAD_REQUEST,
            detail="Invalid file format. Expected .tar.gz"
        )

    # Save uploaded file to temp location
    with tempfile.NamedTemporaryFile(delete=False, suffix=".tar.gz") as tmp:
        content = await file.read()
        tmp.write(content)
        tmp_path = Path(tmp.name)

    try:
        # Validate backup archive with checksum verification
        backup_service.validate_backup_archive(tmp_path, verify_checksums=True)

        # Close the session before restore - restore will terminate all DB connections
        session.close()

        # Move to backups dir with generated ID
        backup_id = backup_service.generate_backup_id()
        dest_path = backup_service.get_backup_path(backup_id)
        shutil.move(str(tmp_path), str(dest_path))

        # Restore without pre-restore backup (database is empty)
        # Use a placeholder admin_user_id since we don't have one yet
        backup_service.restore_backup(
            backup_id=backup_id,
            admin_user_id="setup-restore",
            admin_session_data=None,
            skip_prerestore=True,
        )

        return SetupRestoreResponse(
            success=True,
            message="Backup restored successfully. Please log in with your restored credentials."
        )

    except ValueError as e:
        # Clean up temp file if it still exists
        if tmp_path.exists():
            tmp_path.unlink()
        raise HTTPException(
            status_code=status.HTTP_400_BAD_REQUEST,
            detail=str(e)
        )
    except RuntimeError as e:
        raise HTTPException(
            status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
            detail=str(e)
        )
```

**Step 3: Run type check**

```bash
cd /home/abo/workspace/home/nexorious/backend && uv run pyrefly check
```

Expected: No errors

**Step 4: Commit**

```bash
git add backend/app/api/auth.py
git commit -m "feat(auth): add POST /api/auth/setup/restore endpoint"
```

---

## Task 3: Write Backend Tests for Setup Restore Endpoint

**Files:**
- Create: `backend/app/tests/test_setup_restore.py`

**Step 1: Create test file**

Create `backend/app/tests/test_setup_restore.py`:

```python
"""
Tests for the setup restore endpoint.
"""

import io
import tarfile
import json
from datetime import datetime, timezone
from pathlib import Path

import pytest
from fastapi.testclient import TestClient
from sqlmodel import Session, select

from ..models.user import User


def create_minimal_backup_archive() -> bytes:
    """Create a minimal valid backup archive for testing."""
    buffer = io.BytesIO()

    with tarfile.open(fileobj=buffer, mode="w:gz") as tar:
        # Create manifest
        manifest = {
            "version": 1,
            "created_at": datetime.now(timezone.utc).isoformat(),
            "app_version": "1.0.0",
            "alembic_revision": "test",
            "backup_type": "manual",
            "database": {
                "file": "database.sql",
                "size_bytes": 100,
                "checksum": "sha256:test",
            },
            "files": {
                "cover_art": {
                    "count": 0,
                    "total_size_bytes": 0,
                    "checksum": "sha256:empty",
                },
                "logos": {
                    "count": 0,
                    "total_size_bytes": 0,
                    "checksum": "sha256:empty",
                },
            },
            "stats": {
                "users": 1,
                "games": 0,
                "tags": 0,
            },
        }

        # Add manifest to archive
        manifest_data = json.dumps(manifest).encode()
        manifest_info = tarfile.TarInfo(name="backup-test/manifest.json")
        manifest_info.size = len(manifest_data)
        tar.addfile(manifest_info, io.BytesIO(manifest_data))

        # Add empty database.sql
        db_data = b"-- Empty database\n"
        db_info = tarfile.TarInfo(name="backup-test/database.sql")
        db_info.size = len(db_data)
        tar.addfile(db_info, io.BytesIO(db_data))

    buffer.seek(0)
    return buffer.read()


class TestSetupRestoreEndpoint:
    """Test POST /api/auth/setup/restore endpoint."""

    def test_setup_restore_rejects_when_users_exist(self, client: TestClient, session: Session):
        """Test that restore fails when users already exist."""
        # Create a user first
        from ..core.security import get_password_hash
        user = User(
            username="existinguser",
            password_hash=get_password_hash("password123"),
        )
        session.add(user)
        session.commit()

        # Try to restore
        backup_data = create_minimal_backup_archive()
        response = client.post(
            "/api/auth/setup/restore",
            files={"file": ("backup.tar.gz", backup_data, "application/gzip")},
        )

        assert response.status_code == 400
        assert "already been completed" in response.json()["detail"]

    def test_setup_restore_rejects_invalid_file_format(self, client: TestClient):
        """Test that restore fails with non-.tar.gz file."""
        response = client.post(
            "/api/auth/setup/restore",
            files={"file": ("backup.txt", b"not a backup", "text/plain")},
        )

        assert response.status_code == 400
        assert "Invalid file format" in response.json()["detail"]

    def test_setup_restore_rejects_invalid_archive(self, client: TestClient):
        """Test that restore fails with invalid archive content."""
        response = client.post(
            "/api/auth/setup/restore",
            files={"file": ("backup.tar.gz", b"not a valid archive", "application/gzip")},
        )

        assert response.status_code == 400
        # Should get validation error from BackupService

    def test_setup_restore_endpoint_exists(self, client: TestClient):
        """Test that the endpoint exists and accepts file uploads."""
        # Send without a file to verify endpoint routing
        response = client.post("/api/auth/setup/restore")

        # Should get 422 (validation error for missing file), not 404
        assert response.status_code == 422
```

**Step 2: Run tests to verify they work**

```bash
cd /home/abo/workspace/home/nexorious/backend && uv run pytest app/tests/test_setup_restore.py -v
```

Expected: Tests pass (some may fail due to restore complexity - that's OK for now, the endpoint structure tests should pass)

**Step 3: Commit**

```bash
git add backend/app/tests/test_setup_restore.py
git commit -m "test(auth): add tests for setup restore endpoint"
```

---

## Task 4: Add Frontend API Function for Setup Restore

**Files:**
- Modify: `frontend/src/api/auth.ts`

**Step 1: Add the setupRestore function**

Add at the end of `frontend/src/api/auth.ts`:

```typescript
interface SetupRestoreResponse {
  success: boolean;
  message: string;
}

export async function setupRestore(file: File): Promise<SetupRestoreResponse> {
  const formData = new FormData();
  formData.append('file', file);

  const response = await fetch(`${config.apiUrl}/auth/setup/restore`, {
    method: 'POST',
    body: formData,
  });

  if (!response.ok) {
    const errorData = await response.json().catch(() => ({}));
    const message = errorData.detail || `HTTP ${response.status}: ${response.statusText}`;
    throw new Error(message);
  }

  return response.json();
}
```

**Step 2: Add the config import at the top**

Add to existing imports:

```typescript
import { config } from '@/lib/env';
```

**Step 3: Run type check**

```bash
cd /home/abo/workspace/home/nexorious/frontend && npm run check
```

Expected: No errors

**Step 4: Commit**

```bash
git add frontend/src/api/auth.ts
git commit -m "feat(auth): add setupRestore API function"
```

---

## Task 5: Add Restore UI to Setup Page

**Files:**
- Modify: `frontend/src/app/(auth)/setup/page.tsx`

**Step 1: Add new imports**

Add to the imports at the top of `frontend/src/app/(auth)/setup/page.tsx`:

```typescript
import { Upload, X, Loader2 } from 'lucide-react';
```

**Step 2: Add new state variables**

Add after the existing state declarations (around line 27):

```typescript
const [showRestore, setShowRestore] = useState(false);
const [restoreFile, setRestoreFile] = useState<File | null>(null);
const [isRestoring, setIsRestoring] = useState(false);
const fileInputRef = useRef<HTMLInputElement>(null);
```

**Step 3: Add restore handler function**

Add after the `handleSubmit` function:

```typescript
const handleRestore = async () => {
  if (!restoreFile) return;

  setError(null);
  setIsRestoring(true);

  try {
    await authApi.setupRestore(restoreFile);
    router.push('/login');
  } catch (err) {
    setError(err instanceof Error ? err.message : 'Failed to restore from backup');
  } finally {
    setIsRestoring(false);
  }
};

const handleFileSelect = (e: React.ChangeEvent<HTMLInputElement>) => {
  const file = e.target.files?.[0];
  if (file) {
    if (!file.name.endsWith('.tar.gz')) {
      setError('Please select a .tar.gz backup file');
      return;
    }
    setRestoreFile(file);
    setError(null);
  }
};

const formatFileSize = (bytes: number): string => {
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
};

const cancelRestore = () => {
  setShowRestore(false);
  setRestoreFile(null);
  setError(null);
  if (fileInputRef.current) {
    fileInputRef.current.value = '';
  }
};
```

**Step 4: Update the form JSX**

Replace the entire return statement starting from `return (` with:

```tsx
return (
  <Card className="w-full max-w-sm">
    <CardHeader className="space-y-1 text-center">
      <CardTitle className="text-2xl font-bold">
        {showRestore ? 'Restore from Backup' : 'Create Admin Account'}
      </CardTitle>
      <CardDescription>
        {showRestore
          ? 'Upload a backup file to restore your data'
          : 'Set up your administrator account to get started'}
      </CardDescription>
    </CardHeader>
    <CardContent>
      {error && (
        <Alert variant="destructive" className="mb-4">
          <AlertDescription>{error}</AlertDescription>
        </Alert>
      )}

      {!showRestore ? (
        <>
          <form onSubmit={handleSubmit} className="space-y-4">
            <div className="space-y-2">
              <Label htmlFor="username">Username</Label>
              <Input
                ref={usernameInputRef}
                id="username"
                type="text"
                placeholder="Choose a username"
                value={username}
                onChange={(e) => setUsername(e.target.value)}
                required
                minLength={3}
                autoComplete="username"
                disabled={isSubmitting}
              />
              <p className="text-sm text-muted-foreground">
                Must be at least 3 characters
              </p>
            </div>

            <div className="space-y-2">
              <Label htmlFor="password">Password</Label>
              <Input
                id="password"
                type="password"
                placeholder="Enter a secure password"
                value={password}
                onChange={(e) => setPassword(e.target.value)}
                required
                minLength={8}
                autoComplete="new-password"
                disabled={isSubmitting}
              />
              <p className="text-sm text-muted-foreground">
                Must be at least 8 characters
              </p>
            </div>

            <div className="space-y-2">
              <Label htmlFor="confirmPassword">Confirm Password</Label>
              <Input
                id="confirmPassword"
                type="password"
                placeholder="Confirm your password"
                value={confirmPassword}
                onChange={(e) => setConfirmPassword(e.target.value)}
                required
                autoComplete="new-password"
                disabled={isSubmitting}
              />
            </div>

            <Button
              type="submit"
              className="w-full"
              disabled={isSubmitting || !username || !password || !confirmPassword}
            >
              {isSubmitting ? 'Creating Account...' : 'Create Admin Account'}
            </Button>

            <p className="text-center text-sm text-muted-foreground">
              This account will have full administrative privileges
            </p>
          </form>

          <div className="mt-4 text-center">
            <button
              type="button"
              onClick={() => setShowRestore(true)}
              className="text-sm text-muted-foreground hover:text-foreground hover:underline"
            >
              Restore from backup
            </button>
          </div>
        </>
      ) : (
        <div className="space-y-4">
          <input
            ref={fileInputRef}
            type="file"
            accept=".tar.gz"
            onChange={handleFileSelect}
            className="hidden"
          />

          {!restoreFile ? (
            <button
              type="button"
              onClick={() => fileInputRef.current?.click()}
              className="w-full rounded-lg border-2 border-dashed border-muted-foreground/25 p-8 text-center hover:border-muted-foreground/50 transition-colors"
              disabled={isRestoring}
            >
              <Upload className="mx-auto h-8 w-8 text-muted-foreground mb-2" />
              <p className="text-sm text-muted-foreground">
                Click to select a backup file
              </p>
              <p className="text-xs text-muted-foreground mt-1">
                .tar.gz files only
              </p>
            </button>
          ) : (
            <div className="rounded-lg border p-4">
              <div className="flex items-center justify-between">
                <div className="flex items-center gap-3 min-w-0">
                  <Upload className="h-5 w-5 text-muted-foreground flex-shrink-0" />
                  <div className="min-w-0">
                    <p className="text-sm font-medium truncate">{restoreFile.name}</p>
                    <p className="text-xs text-muted-foreground">
                      {formatFileSize(restoreFile.size)}
                    </p>
                  </div>
                </div>
                <button
                  type="button"
                  onClick={() => {
                    setRestoreFile(null);
                    if (fileInputRef.current) fileInputRef.current.value = '';
                  }}
                  className="text-muted-foreground hover:text-foreground"
                  disabled={isRestoring}
                >
                  <X className="h-4 w-4" />
                </button>
              </div>
            </div>
          )}

          <Button
            onClick={handleRestore}
            className="w-full"
            disabled={!restoreFile || isRestoring}
          >
            {isRestoring ? (
              <>
                <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                Restoring...
              </>
            ) : (
              'Restore'
            )}
          </Button>

          <button
            type="button"
            onClick={cancelRestore}
            className="w-full text-sm text-muted-foreground hover:text-foreground hover:underline"
            disabled={isRestoring}
          >
            Cancel
          </button>
        </div>
      )}
    </CardContent>
  </Card>
);
```

**Step 5: Run type check and lint**

```bash
cd /home/abo/workspace/home/nexorious/frontend && npm run check
```

Expected: No errors

**Step 6: Commit**

```bash
git add frontend/src/app/\(auth\)/setup/page.tsx
git commit -m "feat(setup): add restore from backup UI"
```

---

## Task 6: Write Frontend Tests for Setup Restore

**Files:**
- Create: `frontend/src/app/(auth)/setup/page.test.tsx`

**Step 1: Create test file**

Create `frontend/src/app/(auth)/setup/page.test.tsx`:

```typescript
import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import SetupPage from './page';

// Mock next/navigation
const mockPush = vi.fn();
const mockReplace = vi.fn();
vi.mock('next/navigation', () => ({
  useRouter: () => ({
    push: mockPush,
    replace: mockReplace,
  }),
}));

// Mock auth API
vi.mock('@/api/auth', () => ({
  checkSetupStatus: vi.fn(),
  createInitialAdmin: vi.fn(),
  setupRestore: vi.fn(),
}));

import * as authApi from '@/api/auth';

describe('SetupPage', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.mocked(authApi.checkSetupStatus).mockResolvedValue({ needs_setup: true });
  });

  it('renders admin creation form by default', async () => {
    render(<SetupPage />);

    await waitFor(() => {
      expect(screen.getByText('Create Admin Account')).toBeInTheDocument();
    });

    expect(screen.getByLabelText(/username/i)).toBeInTheDocument();
    expect(screen.getByLabelText(/^password$/i)).toBeInTheDocument();
    expect(screen.getByText('Restore from backup')).toBeInTheDocument();
  });

  it('shows restore UI when clicking restore link', async () => {
    render(<SetupPage />);

    await waitFor(() => {
      expect(screen.getByText('Restore from backup')).toBeInTheDocument();
    });

    fireEvent.click(screen.getByText('Restore from backup'));

    expect(screen.getByText('Restore from Backup')).toBeInTheDocument();
    expect(screen.getByText(/click to select a backup file/i)).toBeInTheDocument();
  });

  it('shows cancel button in restore mode', async () => {
    render(<SetupPage />);

    await waitFor(() => {
      expect(screen.getByText('Restore from backup')).toBeInTheDocument();
    });

    fireEvent.click(screen.getByText('Restore from backup'));

    expect(screen.getByText('Cancel')).toBeInTheDocument();
  });

  it('returns to admin creation when clicking cancel', async () => {
    render(<SetupPage />);

    await waitFor(() => {
      expect(screen.getByText('Restore from backup')).toBeInTheDocument();
    });

    fireEvent.click(screen.getByText('Restore from backup'));
    fireEvent.click(screen.getByText('Cancel'));

    expect(screen.getByText('Create Admin Account')).toBeInTheDocument();
  });

  it('redirects to login when setup not needed', async () => {
    vi.mocked(authApi.checkSetupStatus).mockResolvedValue({ needs_setup: false });

    render(<SetupPage />);

    await waitFor(() => {
      expect(mockReplace).toHaveBeenCalledWith('/login');
    });
  });
});
```

**Step 2: Run tests**

```bash
cd /home/abo/workspace/home/nexorious/frontend && npm run test -- src/app/\(auth\)/setup/page.test.tsx
```

Expected: Tests pass

**Step 3: Commit**

```bash
git add frontend/src/app/\(auth\)/setup/page.test.tsx
git commit -m "test(setup): add tests for restore from backup UI"
```

---

## Task 7: Run Full Test Suite and Fix Any Issues

**Step 1: Run backend tests**

```bash
cd /home/abo/workspace/home/nexorious/backend && uv run pytest --cov=app --cov-report=term-missing
```

Expected: All tests pass with >80% coverage

**Step 2: Run frontend tests**

```bash
cd /home/abo/workspace/home/nexorious/frontend && npm run test
```

Expected: All tests pass

**Step 3: Run type checks**

```bash
cd /home/abo/workspace/home/nexorious/backend && uv run pyrefly check
cd /home/abo/workspace/home/nexorious/frontend && npm run check
```

Expected: No errors

**Step 4: Final commit if any fixes were needed**

```bash
git add -A
git commit -m "fix: address test and type check issues"
```

---

## Task 8: Manual Testing

**Step 1: Start the development environment**

```bash
cd /home/abo/workspace/home/nexorious && podman-compose up --build
```

**Step 2: Test the restore flow**

1. Navigate to http://localhost:3000/setup
2. Verify "Create Admin Account" form is shown
3. Click "Restore from backup" link
4. Verify restore UI appears with file upload area
5. Click "Cancel" and verify return to admin creation form
6. Click "Restore from backup" again
7. Try selecting a non-.tar.gz file - should show error
8. (If you have a valid backup) Upload it and verify restore works

**Step 3: Stop the environment**

```bash
cd /home/abo/workspace/home/nexorious && podman-compose down
```
