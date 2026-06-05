import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { render, screen } from '@/test/test-utils';
import userEvent from '@testing-library/user-event';
import { SyncServiceCard } from './sync-service-card';
import { SyncStorefront, SyncFrequency } from '@/types';
import type { SyncConfig, SyncStatus } from '@/types';

const createMockConfig = (overrides: Partial<SyncConfig> = {}): SyncConfig => ({
  id: 'config-1',
  userId: 'user-1',
  storefront: SyncStorefront.STEAM,
  frequency: SyncFrequency.DAILY,
  lastSyncedAt: '2024-01-01T12:00:00Z',
  createdAt: '2024-01-01T00:00:00Z',
  updatedAt: '2024-01-01T00:00:00Z',
  isConfigured: true,
  ...overrides,
});

const createMockStatus = (overrides: Partial<SyncStatus> = {}): SyncStatus => ({
  storefront: SyncStorefront.STEAM,
  isSyncing: false,
  lastSyncedAt: '2024-01-01T12:00:00Z',
  activeJobId: null,
  externalGameCount: 0,
  ...overrides,
});

describe('SyncServiceCard', () => {
  const mockOnTriggerSync = vi.fn();

  beforeEach(() => {
    vi.clearAllMocks();
  });

  describe('rendering', () => {
    it('renders platform name as a link to detail page', () => {
      const config = createMockConfig();
      render(<SyncServiceCard config={config} onTriggerSync={mockOnTriggerSync} />);

      const link = screen.getByRole('link', { name: 'Steam' });
      expect(link).toBeInTheDocument();
      expect(link).toHaveAttribute('href', '/sync/steam');
    });

    it('renders GOG platform name', () => {
      const config = createMockConfig({ storefront: SyncStorefront.GOG });
      render(<SyncServiceCard config={config} onTriggerSync={mockOnTriggerSync} />);

      expect(screen.getByText('GOG')).toBeInTheDocument();
    });

    it('renders Epic Games platform name', () => {
      const config = createMockConfig({ storefront: SyncStorefront.EPIC });
      render(<SyncServiceCard config={config} onTriggerSync={mockOnTriggerSync} />);

      expect(screen.getByText('Epic Games')).toBeInTheDocument();
    });

    it('creates correct detail link for GOG platform', () => {
      const config = createMockConfig({ storefront: SyncStorefront.GOG });
      render(<SyncServiceCard config={config} onTriggerSync={mockOnTriggerSync} />);

      const link = screen.getByRole('link', { name: 'GOG' });
      expect(link).toHaveAttribute('href', '/sync/gog');
    });

    it('creates correct detail link for Epic platform', () => {
      const config = createMockConfig({ storefront: SyncStorefront.EPIC });
      render(<SyncServiceCard config={config} onTriggerSync={mockOnTriggerSync} />);

      const link = screen.getByRole('link', { name: 'Epic Games' });
      expect(link).toHaveAttribute('href', '/sync/epic');
    });

    it('does not render frequency select', () => {
      const config = createMockConfig();
      render(<SyncServiceCard config={config} onTriggerSync={mockOnTriggerSync} />);

      expect(screen.queryByRole('combobox')).not.toBeInTheDocument();
    });

    it('does not render auto-add toggle', () => {
      const config = createMockConfig();
      render(<SyncServiceCard config={config} onTriggerSync={mockOnTriggerSync} />);

      expect(screen.queryByRole('switch')).not.toBeInTheDocument();
    });

    it('does not render a View details footer link', () => {
      const config = createMockConfig();
      render(<SyncServiceCard config={config} onTriggerSync={mockOnTriggerSync} />);

      expect(screen.queryByRole('link', { name: /view details/i })).not.toBeInTheDocument();
    });
  });

  describe('connection badge', () => {
    it('shows Connected badge when configured', () => {
      const config = createMockConfig({ isConfigured: true });
      render(<SyncServiceCard config={config} onTriggerSync={mockOnTriggerSync} />);

      expect(screen.getByText('Connected')).toBeInTheDocument();
    });

    it('shows Not Configured badge when not configured', () => {
      const config = createMockConfig({ isConfigured: false });
      render(<SyncServiceCard config={config} onTriggerSync={mockOnTriggerSync} />);

      expect(screen.getByText('Not Configured')).toBeInTheDocument();
    });

    it('shows Credentials Error badge when credentialsError is true', () => {
      const config = createMockConfig({ isConfigured: true });
      render(
        <SyncServiceCard
          config={config}
          credentialsError={true}
          onTriggerSync={mockOnTriggerSync}
        />,
      );

      expect(screen.getByText('Credentials Error')).toBeInTheDocument();
    });
  });

  describe('sync button', () => {
    it('calls onTriggerSync when sync button is clicked', async () => {
      const user = userEvent.setup({ delay: null });
      const config = createMockConfig();
      mockOnTriggerSync.mockResolvedValue(undefined);

      render(<SyncServiceCard config={config} onTriggerSync={mockOnTriggerSync} />);

      const syncButton = screen.getByRole('button', { name: /sync now/i });
      await user.click(syncButton);

      expect(mockOnTriggerSync).toHaveBeenCalled();
    });

    it('disables sync button when isSyncing is true', () => {
      const config = createMockConfig();
      render(
        <SyncServiceCard config={config} onTriggerSync={mockOnTriggerSync} isSyncing={true} />,
      );

      const syncButton = screen.getByRole('button', { name: /syncing/i });
      expect(syncButton).toBeDisabled();
    });

    it('disables sync button when status.isSyncing is true', () => {
      const config = createMockConfig();
      const status = createMockStatus({ isSyncing: true });
      render(<SyncServiceCard config={config} status={status} onTriggerSync={mockOnTriggerSync} />);

      const syncButton = screen.getByRole('button', { name: /syncing/i });
      expect(syncButton).toBeDisabled();
    });

    it('disables sync button when not configured', () => {
      const config = createMockConfig({ isConfigured: false });
      render(<SyncServiceCard config={config} onTriggerSync={mockOnTriggerSync} />);

      const syncButton = screen.getByRole('button', { name: /sync now/i });
      expect(syncButton).toBeDisabled();
    });
  });

  describe('syncing state', () => {
    it('shows syncing state when isSyncing is true', () => {
      const config = createMockConfig();
      render(
        <SyncServiceCard config={config} onTriggerSync={mockOnTriggerSync} isSyncing={true} />,
      );

      expect(screen.getByText('Syncing...')).toBeInTheDocument();
    });

    it('shows syncing state when status.isSyncing is true', () => {
      const config = createMockConfig();
      const status = createMockStatus({ isSyncing: true });
      render(<SyncServiceCard config={config} status={status} onTriggerSync={mockOnTriggerSync} />);

      expect(screen.getByText('Syncing...')).toBeInTheDocument();
    });

    it('shows Sync Now text when not syncing', () => {
      const config = createMockConfig();
      render(
        <SyncServiceCard config={config} onTriggerSync={mockOnTriggerSync} isSyncing={false} />,
      );

      expect(screen.getByText('Sync Now')).toBeInTheDocument();
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
      render(<SyncServiceCard config={config} onTriggerSync={mockOnTriggerSync} />);

      expect(screen.getByText(/Just now/)).toBeInTheDocument();
    });

    it('formats last sync time as minutes ago', () => {
      const config = createMockConfig({
        lastSyncedAt: '2024-01-01T12:05:00Z', // 25 minutes ago
      });
      render(<SyncServiceCard config={config} onTriggerSync={mockOnTriggerSync} />);

      expect(screen.getByText(/25m ago/)).toBeInTheDocument();
    });

    it('formats last sync time as hours ago', () => {
      const config = createMockConfig({
        lastSyncedAt: '2024-01-01T09:30:00Z', // 3 hours ago
      });
      render(<SyncServiceCard config={config} onTriggerSync={mockOnTriggerSync} />);

      expect(screen.getByText(/3h ago/)).toBeInTheDocument();
    });

    it('formats last sync time as days ago', () => {
      const config = createMockConfig({
        lastSyncedAt: '2023-12-30T12:30:00Z', // 2 days ago
      });
      render(<SyncServiceCard config={config} onTriggerSync={mockOnTriggerSync} />);

      expect(screen.getByText(/2d ago/)).toBeInTheDocument();
    });

    it('formats last sync time as date for more than 7 days', () => {
      const config = createMockConfig({
        lastSyncedAt: '2023-12-20T12:30:00Z', // 12 days ago
      });
      render(<SyncServiceCard config={config} onTriggerSync={mockOnTriggerSync} />);

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
      render(<SyncServiceCard config={config} onTriggerSync={mockOnTriggerSync} />);

      expect(screen.getByText(/Never/)).toBeInTheDocument();
    });
  });

  describe('pending review badge', () => {
    it('shows pending review badge when count > 0', () => {
      const config = createMockConfig();
      render(
        <SyncServiceCard
          config={config}
          pendingReviewCount={5}
          onTriggerSync={mockOnTriggerSync}
        />,
      );

      expect(screen.getByText('5')).toBeInTheDocument();
    });

    it('does not show pending review badge when count is 0', () => {
      const config = createMockConfig();
      render(
        <SyncServiceCard
          config={config}
          pendingReviewCount={0}
          onTriggerSync={mockOnTriggerSync}
        />,
      );

      expect(screen.queryByText(/to review/)).not.toBeInTheDocument();
    });

    it('does not show pending review badge when count is undefined', () => {
      const config = createMockConfig();
      render(<SyncServiceCard config={config} onTriggerSync={mockOnTriggerSync} />);

      expect(screen.queryByText(/to review/)).not.toBeInTheDocument();
    });
  });

  describe('external game count display', () => {
    it('shows game count when externalGameCount > 0', () => {
      const config = createMockConfig();
      render(
        <SyncServiceCard
          config={config}
          externalGameCount={42}
          onTriggerSync={mockOnTriggerSync}
        />,
      );

      expect(screen.getByText('42 games')).toBeInTheDocument();
    });

    it('hides game count when externalGameCount is 0', () => {
      const config = createMockConfig();
      render(
        <SyncServiceCard config={config} externalGameCount={0} onTriggerSync={mockOnTriggerSync} />,
      );

      expect(screen.queryByText(/games/)).not.toBeInTheDocument();
    });

    it('hides game count when externalGameCount is undefined', () => {
      const config = createMockConfig();
      render(<SyncServiceCard config={config} onTriggerSync={mockOnTriggerSync} />);

      expect(screen.queryByText(/games/)).not.toBeInTheDocument();
    });
  });
});
