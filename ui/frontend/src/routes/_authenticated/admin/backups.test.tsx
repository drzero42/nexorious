import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { toast } from 'sonner';
import { ApiErrorException } from '@/api/client';
import * as backupApi from '@/api/backup';
import * as serverRestart from '@/lib/server-restart';
import { takeRestoreOutcome } from '@/lib/server-restart';
import { BackupPage } from './backups';

vi.mock('@/providers', () => ({
  useAuth: () => ({ user: { id: 'u1', isAdmin: true } }),
}));

vi.mock('@/hooks', () => ({
  useDateFormat: () => ({ formatDateTime: (s: string) => s }),
}));

vi.mock('sonner', () => ({
  toast: { success: vi.fn(), error: vi.fn() },
}));

vi.mock('@/api/backup', () => ({
  getBackupConfig: vi.fn(),
  listBackups: vi.fn(),
  restoreBackup: vi.fn(),
  uploadAndRestoreBackup: vi.fn(),
}));

vi.mock('@/lib/server-restart', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/lib/server-restart')>();
  return { ...actual, readHealth: vi.fn(), waitForNewInstance: vi.fn() };
});

const assign = vi.fn();

beforeEach(() => {
  vi.clearAllMocks();
  sessionStorage.clear();
  vi.stubGlobal('location', { ...window.location, assign, reload: vi.fn() });
  vi.mocked(backupApi.getBackupConfig).mockResolvedValue({
    schedule: 'manual',
    scheduleTime: '02:00',
    scheduleDay: null,
    retentionMode: 'count',
    retentionValue: 5,
    updatedAt: '2026-11-01T00:00:00Z',
  });
  vi.mocked(backupApi.listBackups).mockResolvedValue([
    {
      id: 'bk-1',
      createdAt: '2026-11-01T00:00:00Z',
      backupType: 'manual',
      sizeBytes: 1024,
      stats: { users: 1, games: 2, tags: 3 },
    },
  ]);
  vi.mocked(serverRestart.readHealth).mockResolvedValue({
    status: 'ok',
    instance_id: 'before',
    restore_in_progress: false,
  });
  vi.mocked(serverRestart.waitForNewInstance).mockResolvedValue({
    status: 'ok',
    instance_id: 'new',
  });
});

afterEach(() => {
  vi.unstubAllGlobals();
});

async function restoreBk1() {
  const user = userEvent.setup();
  render(<BackupPage />);
  await user.click(await screen.findByRole('button', { name: 'Restore backup' }));
  await user.click(screen.getByLabelText('I understand this action cannot be undone'));
  await user.click(screen.getByRole('button', { name: 'Restore' }));
}

describe('BackupPage restore', () => {
  it('waits for the new server after a success, then loads /login with the outcome kept', async () => {
    vi.mocked(backupApi.restoreBackup).mockResolvedValue({
      success: true,
      message: 'Restore completed from bk-1.',
      restarting: true,
      instance_id: 'old',
    });
    await restoreBk1();

    await vi.waitFor(() => expect(assign).toHaveBeenCalledWith('/login'));
    expect(serverRestart.waitForNewInstance).toHaveBeenCalledWith('old');
    expect(takeRestoreOutcome()).toEqual({
      kind: 'success',
      message: 'Restore completed from bk-1.',
    });
  });

  it('waits after a failure that restarts, and keeps the error', async () => {
    vi.mocked(backupApi.restoreBackup).mockRejectedValue(
      new ApiErrorException('restore failed: bad dump', 500, {
        restarting: true,
        instance_id: 'old',
        error: 'restore failed: bad dump',
      }),
    );
    await restoreBk1();

    await vi.waitFor(() => expect(assign).toHaveBeenCalledWith('/login'));
    expect(takeRestoreOutcome()).toEqual({ kind: 'error', message: 'restore failed: bad dump' });
  });

  it('waits past the id read before the request when no answer arrives', async () => {
    vi.mocked(backupApi.restoreBackup).mockRejectedValue(new TypeError('Failed to fetch'));
    await restoreBk1();

    await vi.waitFor(() => expect(assign).toHaveBeenCalledWith('/login'));
    expect(serverRestart.waitForNewInstance).toHaveBeenCalledWith('before');
    expect(takeRestoreOutcome()?.message).toMatch(/outcome is unknown/);
  });

  it('shows a refusal at once without waiting', async () => {
    vi.mocked(backupApi.restoreBackup).mockRejectedValue(
      new ApiErrorException('A backup or restore operation is already in progress', 409, {
        restarting: false,
        instance_id: 'old',
      }),
    );
    await restoreBk1();

    await vi.waitFor(() =>
      expect(toast.error).toHaveBeenCalledWith(
        'A backup or restore operation is already in progress',
      ),
    );
    expect(serverRestart.waitForNewInstance).not.toHaveBeenCalled();
    expect(assign).not.toHaveBeenCalled();
  });

  it('offers a reload when the server does not come back', async () => {
    vi.mocked(backupApi.restoreBackup).mockResolvedValue({
      success: true,
      message: 'Restore completed from bk-1.',
      restarting: true,
      instance_id: 'old',
    });
    vi.mocked(serverRestart.waitForNewInstance).mockRejectedValue(
      new serverRestart.RestartTimeoutError(),
    );
    await restoreBk1();

    expect(await screen.findByRole('button', { name: 'Reload' })).toBeInTheDocument();
    expect(screen.getByText(/did not come back/)).toBeInTheDocument();
    expect(assign).not.toHaveBeenCalled();
  });
});
