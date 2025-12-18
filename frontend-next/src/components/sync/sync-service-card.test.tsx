import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { render, screen } from '@/test/test-utils';
import userEvent from '@testing-library/user-event';
import { SyncServiceCard } from './sync-service-card';
import { SyncPlatform, SyncFrequency } from '@/types';
import type { SyncConfig, SyncStatus } from '@/types';

// Mock next/link
vi.mock('next/link', () => ({
  default: ({ children, href }: { children: React.ReactNode; href: string }) => (
    <a href={href}>{children}</a>
  ),
}));

const createMockConfig = (overrides: Partial<SyncConfig> = {}): SyncConfig => ({
  id: 'config-1',
  userId: 'user-1',
  platform: SyncPlatform.STEAM,
  frequency: SyncFrequency.DAILY,
  autoAdd: true,
  enabled: true,
  lastSyncedAt: '2024-01-01T12:00:00Z',
  createdAt: '2024-01-01T00:00:00Z',
  updatedAt: '2024-01-01T00:00:00Z',
  ...overrides,
});

const createMockStatus = (overrides: Partial<SyncStatus> = {}): SyncStatus => ({
  platform: SyncPlatform.STEAM,
  isSyncing: false,
  lastSyncedAt: '2024-01-01T12:00:00Z',
  activeJobId: null,
  ...overrides,
});

describe('SyncServiceCard', () => {
  const mockOnUpdate = vi.fn();
  const mockOnTriggerSync = vi.fn();

  beforeEach(() => {
    vi.clearAllMocks();
  });

  describe('rendering', () => {
    it('renders platform name', () => {
      const config = createMockConfig();
      render(
        <SyncServiceCard
          config={config}
          onUpdate={mockOnUpdate}
          onTriggerSync={mockOnTriggerSync}
        />
      );

      expect(screen.getByText('Steam')).toBeInTheDocument();
    });

    it('renders GOG platform name', () => {
      const config = createMockConfig({ platform: SyncPlatform.GOG });
      render(
        <SyncServiceCard
          config={config}
          onUpdate={mockOnUpdate}
          onTriggerSync={mockOnTriggerSync}
        />
      );

      expect(screen.getByText('GOG')).toBeInTheDocument();
    });

    it('renders Epic Games platform name', () => {
      const config = createMockConfig({ platform: SyncPlatform.EPIC });
      render(
        <SyncServiceCard
          config={config}
          onUpdate={mockOnUpdate}
          onTriggerSync={mockOnTriggerSync}
        />
      );

      expect(screen.getByText('Epic Games')).toBeInTheDocument();
    });
  });

  describe('connection badge', () => {
    it('shows Connected badge when enabled', () => {
      const config = createMockConfig({ enabled: true });
      render(
        <SyncServiceCard
          config={config}
          onUpdate={mockOnUpdate}
          onTriggerSync={mockOnTriggerSync}
        />
      );

      expect(screen.getByText('Connected')).toBeInTheDocument();
    });

    it('shows Disconnected badge when not enabled', () => {
      const config = createMockConfig({ enabled: false });
      render(
        <SyncServiceCard
          config={config}
          onUpdate={mockOnUpdate}
          onTriggerSync={mockOnTriggerSync}
        />
      );

      expect(screen.getByText('Disconnected')).toBeInTheDocument();
    });
  });

  describe('enable toggle', () => {
    it('calls onUpdate when enable toggle is changed to true', async () => {
      const user = userEvent.setup({ delay: null });
      const config = createMockConfig({ enabled: false });
      mockOnUpdate.mockResolvedValue(undefined);

      render(
        <SyncServiceCard
          config={config}
          onUpdate={mockOnUpdate}
          onTriggerSync={mockOnTriggerSync}
        />
      );

      const switches = screen.getAllByRole('switch');
      // First switch is enable toggle
      await user.click(switches[0]);

      expect(mockOnUpdate).toHaveBeenCalledWith({ enabled: true });
    });

    it('calls onUpdate when enable toggle is changed to false', async () => {
      const user = userEvent.setup({ delay: null });
      const config = createMockConfig({ enabled: true });
      mockOnUpdate.mockResolvedValue(undefined);

      render(
        <SyncServiceCard
          config={config}
          onUpdate={mockOnUpdate}
          onTriggerSync={mockOnTriggerSync}
        />
      );

      const switches = screen.getAllByRole('switch');
      // First switch is enable toggle
      await user.click(switches[0]);

      expect(mockOnUpdate).toHaveBeenCalledWith({ enabled: false });
    });

    it('disables toggle when isUpdating is true', () => {
      const config = createMockConfig();
      render(
        <SyncServiceCard
          config={config}
          onUpdate={mockOnUpdate}
          onTriggerSync={mockOnTriggerSync}
          isUpdating={true}
        />
      );

      const switches = screen.getAllByRole('switch');
      // First switch is enable toggle
      expect(switches[0]).toBeDisabled();
    });
  });

  describe('frequency select', () => {
    it('calls onUpdate when frequency is changed', async () => {
      const user = userEvent.setup({ delay: null });
      const config = createMockConfig({ frequency: SyncFrequency.DAILY });
      mockOnUpdate.mockResolvedValue(undefined);

      render(
        <SyncServiceCard
          config={config}
          onUpdate={mockOnUpdate}
          onTriggerSync={mockOnTriggerSync}
        />
      );

      const trigger = screen.getByRole('combobox');
      await user.click(trigger);

      const hourlyOption = screen.getByRole('option', { name: 'Every hour' });
      await user.click(hourlyOption);

      expect(mockOnUpdate).toHaveBeenCalledWith({ frequency: SyncFrequency.HOURLY });
    });

    it('disables frequency select when not enabled', () => {
      const config = createMockConfig({ enabled: false });
      render(
        <SyncServiceCard
          config={config}
          onUpdate={mockOnUpdate}
          onTriggerSync={mockOnTriggerSync}
        />
      );

      const trigger = screen.getByRole('combobox');
      expect(trigger).toBeDisabled();
    });

    it('disables frequency select when isUpdating is true', () => {
      const config = createMockConfig({ enabled: true });
      render(
        <SyncServiceCard
          config={config}
          onUpdate={mockOnUpdate}
          onTriggerSync={mockOnTriggerSync}
          isUpdating={true}
        />
      );

      const trigger = screen.getByRole('combobox');
      expect(trigger).toBeDisabled();
    });
  });

  describe('auto-add toggle', () => {
    it('calls onUpdate when auto-add toggle is changed to true', async () => {
      const user = userEvent.setup({ delay: null });
      const config = createMockConfig({ autoAdd: false });
      mockOnUpdate.mockResolvedValue(undefined);

      render(
        <SyncServiceCard
          config={config}
          onUpdate={mockOnUpdate}
          onTriggerSync={mockOnTriggerSync}
        />
      );

      const switches = screen.getAllByRole('switch');
      // Second switch is auto-add toggle
      await user.click(switches[1]);

      expect(mockOnUpdate).toHaveBeenCalledWith({ autoAdd: true });
    });

    it('calls onUpdate when auto-add toggle is changed to false', async () => {
      const user = userEvent.setup({ delay: null });
      const config = createMockConfig({ autoAdd: true });
      mockOnUpdate.mockResolvedValue(undefined);

      render(
        <SyncServiceCard
          config={config}
          onUpdate={mockOnUpdate}
          onTriggerSync={mockOnTriggerSync}
        />
      );

      const switches = screen.getAllByRole('switch');
      // Second switch is auto-add toggle
      await user.click(switches[1]);

      expect(mockOnUpdate).toHaveBeenCalledWith({ autoAdd: false });
    });

    it('disables auto-add toggle when not enabled', () => {
      const config = createMockConfig({ enabled: false });
      render(
        <SyncServiceCard
          config={config}
          onUpdate={mockOnUpdate}
          onTriggerSync={mockOnTriggerSync}
        />
      );

      const switches = screen.getAllByRole('switch');
      // Second switch is auto-add toggle
      expect(switches[1]).toBeDisabled();
    });

    it('disables auto-add toggle when isUpdating is true', () => {
      const config = createMockConfig({ enabled: true });
      render(
        <SyncServiceCard
          config={config}
          onUpdate={mockOnUpdate}
          onTriggerSync={mockOnTriggerSync}
          isUpdating={true}
        />
      );

      const switches = screen.getAllByRole('switch');
      // Second switch is auto-add toggle
      expect(switches[1]).toBeDisabled();
    });
  });

  describe('sync button', () => {
    it('calls onTriggerSync when sync button is clicked', async () => {
      const user = userEvent.setup({ delay: null });
      const config = createMockConfig({ enabled: true });
      mockOnTriggerSync.mockResolvedValue(undefined);

      render(
        <SyncServiceCard
          config={config}
          onUpdate={mockOnUpdate}
          onTriggerSync={mockOnTriggerSync}
        />
      );

      const syncButton = screen.getByRole('button', { name: /sync now/i });
      await user.click(syncButton);

      expect(mockOnTriggerSync).toHaveBeenCalled();
    });

    it('disables sync button when not enabled', () => {
      const config = createMockConfig({ enabled: false });
      render(
        <SyncServiceCard
          config={config}
          onUpdate={mockOnUpdate}
          onTriggerSync={mockOnTriggerSync}
        />
      );

      const syncButton = screen.getByRole('button', { name: /sync now/i });
      expect(syncButton).toBeDisabled();
    });

    it('disables sync button when isSyncing is true', () => {
      const config = createMockConfig({ enabled: true });
      render(
        <SyncServiceCard
          config={config}
          onUpdate={mockOnUpdate}
          onTriggerSync={mockOnTriggerSync}
          isSyncing={true}
        />
      );

      const syncButton = screen.getByRole('button', { name: /syncing/i });
      expect(syncButton).toBeDisabled();
    });

    it('disables sync button when status.isSyncing is true', () => {
      const config = createMockConfig({ enabled: true });
      const status = createMockStatus({ isSyncing: true });
      render(
        <SyncServiceCard
          config={config}
          status={status}
          onUpdate={mockOnUpdate}
          onTriggerSync={mockOnTriggerSync}
        />
      );

      const syncButton = screen.getByRole('button', { name: /syncing/i });
      expect(syncButton).toBeDisabled();
    });
  });

  describe('syncing state', () => {
    it('shows syncing state when isSyncing is true', () => {
      const config = createMockConfig();
      render(
        <SyncServiceCard
          config={config}
          onUpdate={mockOnUpdate}
          onTriggerSync={mockOnTriggerSync}
          isSyncing={true}
        />
      );

      expect(screen.getByText('Syncing...')).toBeInTheDocument();
    });

    it('shows syncing state when status.isSyncing is true', () => {
      const config = createMockConfig();
      const status = createMockStatus({ isSyncing: true });
      render(
        <SyncServiceCard
          config={config}
          status={status}
          onUpdate={mockOnUpdate}
          onTriggerSync={mockOnTriggerSync}
        />
      );

      expect(screen.getByText('Syncing...')).toBeInTheDocument();
    });

    it('shows Sync Now text when not syncing', () => {
      const config = createMockConfig();
      render(
        <SyncServiceCard
          config={config}
          onUpdate={mockOnUpdate}
          onTriggerSync={mockOnTriggerSync}
          isSyncing={false}
        />
      );

      expect(screen.getByText('Sync Now')).toBeInTheDocument();
    });
  });

  describe('view history link', () => {
    it('has view history link', () => {
      const config = createMockConfig({ platform: SyncPlatform.STEAM });
      render(
        <SyncServiceCard
          config={config}
          onUpdate={mockOnUpdate}
          onTriggerSync={mockOnTriggerSync}
        />
      );

      const link = screen.getByRole('link', { name: /view history/i });
      expect(link).toBeInTheDocument();
      expect(link).toHaveAttribute('href', '/jobs?source=steam&job_type=sync');
    });

    it('creates correct history link for GOG platform', () => {
      const config = createMockConfig({ platform: SyncPlatform.GOG });
      render(
        <SyncServiceCard
          config={config}
          onUpdate={mockOnUpdate}
          onTriggerSync={mockOnTriggerSync}
        />
      );

      const link = screen.getByRole('link', { name: /view history/i });
      expect(link).toHaveAttribute('href', '/jobs?source=gog&job_type=sync');
    });

    it('creates correct history link for Epic platform', () => {
      const config = createMockConfig({ platform: SyncPlatform.EPIC });
      render(
        <SyncServiceCard
          config={config}
          onUpdate={mockOnUpdate}
          onTriggerSync={mockOnTriggerSync}
        />
      );

      const link = screen.getByRole('link', { name: /view history/i });
      expect(link).toHaveAttribute('href', '/jobs?source=epic&job_type=sync');
    });
  });

  describe('last sync time formatting', () => {
    beforeEach(() => {
      vi.useFakeTimers();
      // Set a fixed date for consistent time-based tests
      vi.setSystemTime(new Date('2024-01-01T12:30:00Z'));
    });

    afterEach(() => {
      vi.useRealTimers();
    });

    it('formats last sync time as "Just now" for less than 1 minute', () => {
      const config = createMockConfig({
        lastSyncedAt: '2024-01-01T12:29:30Z', // 30 seconds ago
      });
      render(
        <SyncServiceCard
          config={config}
          onUpdate={mockOnUpdate}
          onTriggerSync={mockOnTriggerSync}
        />
      );

      expect(screen.getByText(/Just now/)).toBeInTheDocument();
    });

    it('formats last sync time as minutes ago', () => {
      const config = createMockConfig({
        lastSyncedAt: '2024-01-01T12:05:00Z', // 25 minutes ago
      });
      render(
        <SyncServiceCard
          config={config}
          onUpdate={mockOnUpdate}
          onTriggerSync={mockOnTriggerSync}
        />
      );

      expect(screen.getByText(/25m ago/)).toBeInTheDocument();
    });

    it('formats last sync time as hours ago', () => {
      const config = createMockConfig({
        lastSyncedAt: '2024-01-01T09:30:00Z', // 3 hours ago
      });
      render(
        <SyncServiceCard
          config={config}
          onUpdate={mockOnUpdate}
          onTriggerSync={mockOnTriggerSync}
        />
      );

      expect(screen.getByText(/3h ago/)).toBeInTheDocument();
    });

    it('formats last sync time as days ago', () => {
      const config = createMockConfig({
        lastSyncedAt: '2023-12-30T12:30:00Z', // 2 days ago
      });
      render(
        <SyncServiceCard
          config={config}
          onUpdate={mockOnUpdate}
          onTriggerSync={mockOnTriggerSync}
        />
      );

      expect(screen.getByText(/2d ago/)).toBeInTheDocument();
    });

    it('formats last sync time as date for more than 7 days', () => {
      const config = createMockConfig({
        lastSyncedAt: '2023-12-20T12:30:00Z', // 12 days ago
      });
      render(
        <SyncServiceCard
          config={config}
          onUpdate={mockOnUpdate}
          onTriggerSync={mockOnTriggerSync}
        />
      );

      // Should show a formatted date (locale-dependent format)
      // We test that it's not showing the relative format like "Xd ago"
      const lastSyncText = screen.getByText(/Last synced:/);
      expect(lastSyncText).toBeInTheDocument();
      expect(lastSyncText.textContent).not.toContain('d ago');
      expect(lastSyncText.textContent).not.toContain('h ago');
      expect(lastSyncText.textContent).not.toContain('m ago');
    });

    it('shows "Never" when lastSyncedAt is null', () => {
      const config = createMockConfig({
        lastSyncedAt: null,
      });
      render(
        <SyncServiceCard
          config={config}
          onUpdate={mockOnUpdate}
          onTriggerSync={mockOnTriggerSync}
        />
      );

      expect(screen.getByText(/Never/)).toBeInTheDocument();
    });
  });

  describe('edge cases', () => {
    it('handles missing status gracefully', () => {
      const config = createMockConfig();
      expect(() =>
        render(
          <SyncServiceCard
            config={config}
            onUpdate={mockOnUpdate}
            onTriggerSync={mockOnTriggerSync}
          />
        )
      ).not.toThrow();
    });

    it('handles status with null activeJobId', () => {
      const config = createMockConfig();
      const status = createMockStatus({ activeJobId: null });
      expect(() =>
        render(
          <SyncServiceCard
            config={config}
            status={status}
            onUpdate={mockOnUpdate}
            onTriggerSync={mockOnTriggerSync}
          />
        )
      ).not.toThrow();
    });

    it('handles status with activeJobId', () => {
      const config = createMockConfig();
      const status = createMockStatus({ activeJobId: 'job-123' });
      expect(() =>
        render(
          <SyncServiceCard
            config={config}
            status={status}
            onUpdate={mockOnUpdate}
            onTriggerSync={mockOnTriggerSync}
          />
        )
      ).not.toThrow();
    });
  });
});
