import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { EpicConnectionCard } from './epic-connection-card';
import * as syncApi from '@/api/sync';
import * as hooks from '@/hooks';

// Mock toast
vi.mock('sonner', () => ({
  toast: {
    success: vi.fn(),
    error: vi.fn(),
  },
}));

const createWrapper = () => {
  const queryClient = new QueryClient({
    defaultOptions: {
      queries: { retry: false },
      mutations: { retry: false },
    },
  });
  const Wrapper = ({ children }: { children: React.ReactNode }) => (
    <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
  );
  Wrapper.displayName = 'QueryClientWrapper';
  return Wrapper;
};

describe('EpicConnectionCard', () => {
  const mockOnConnectionChange = vi.fn();

  beforeEach(() => {
    vi.clearAllMocks();
    // Mock useSyncStatus to return no auth expiration by default
    vi.spyOn(hooks, 'useSyncStatus').mockReturnValue({
      data: {
        platform: 'epic',
        isSyncing: false,
        lastSyncedAt: null,
        activeJobId: null,
      },
    } as any);
    // Mock useDisconnectEpic to return a mutation object
    vi.spyOn(hooks, 'useDisconnectEpic').mockReturnValue({
      mutateAsync: vi.fn().mockResolvedValue(undefined),
      isPending: false,
    } as any);
  });

  it('should render not configured state', () => {
    render(
      <EpicConnectionCard
        isConfigured={false}
        onConnectionChange={mockOnConnectionChange}
      />,
      { wrapper: createWrapper() }
    );

    expect(screen.getByText('Epic Games Store')).toBeInTheDocument();
    expect(screen.getByText('Connect Epic Games Store')).toBeInTheDocument();
    expect(screen.getByText('Not Configured')).toBeInTheDocument();
  });

  it('should render connected state', () => {
    render(
      <EpicConnectionCard
        isConfigured={true}
        displayName="EpicUser123"
        accountId="epic-account-id"
        onConnectionChange={mockOnConnectionChange}
      />,
      { wrapper: createWrapper() }
    );

    expect(screen.getByText('EpicUser123')).toBeInTheDocument();
    expect(screen.getByText('epic-account-id')).toBeInTheDocument();
    expect(screen.getByText('Connected')).toBeInTheDocument();
    expect(screen.getByText('Disconnect')).toBeInTheDocument();
  });

  it('should show disconnect confirmation', async () => {
    const user = userEvent.setup();

    render(
      <EpicConnectionCard
        isConfigured={true}
        displayName="EpicUser123"
        onConnectionChange={mockOnConnectionChange}
      />,
      { wrapper: createWrapper() }
    );

    await user.click(screen.getByText('Disconnect'));

    await waitFor(() => {
      expect(screen.getByText('Disconnect Epic Games Store?')).toBeInTheDocument();
    });
  });

  it('should call disconnectEpic on confirm', async () => {
    const user = userEvent.setup();
    const mockMutateAsync = vi.fn().mockResolvedValue(undefined);

    vi.spyOn(hooks, 'useDisconnectEpic').mockReturnValue({
      mutateAsync: mockMutateAsync,
      isPending: false,
    } as any);

    render(
      <EpicConnectionCard
        isConfigured={true}
        displayName="EpicUser123"
        onConnectionChange={mockOnConnectionChange}
      />,
      { wrapper: createWrapper() }
    );

    await user.click(screen.getByText('Disconnect'));
    await waitFor(() => screen.getByRole('button', { name: /Disconnect$/i }));

    const confirmButton = screen.getAllByRole('button', { name: /Disconnect$/i }).find(
      btn => btn.closest('[role="alertdialog"]')
    );
    if (confirmButton) {
      await user.click(confirmButton);
    }

    await waitFor(() => {
      expect(mockMutateAsync).toHaveBeenCalled();
      expect(mockOnConnectionChange).toHaveBeenCalled();
    });
  });

  it('should render auth expired state', () => {
    vi.spyOn(hooks, 'useSyncStatus').mockReturnValue({
      data: {
        platform: 'epic',
        isSyncing: false,
        lastSyncedAt: null,
        activeJobId: null,
        authExpired: true,
      },
    } as any);

    render(
      <EpicConnectionCard
        isConfigured={true}
        displayName="EpicUser123"
        onConnectionChange={mockOnConnectionChange}
      />,
      { wrapper: createWrapper() }
    );

    expect(screen.getByText('Auth Expired')).toBeInTheDocument();
    expect(screen.getByText('Reconnect')).toBeInTheDocument();
  });

  it('should show playtime limitation note', () => {
    render(
      <EpicConnectionCard
        isConfigured={false}
        onConnectionChange={mockOnConnectionChange}
      />,
      { wrapper: createWrapper() }
    );

    expect(screen.getByText(/Epic Games Store does not provide playtime data/i)).toBeInTheDocument();
  });

  it('should open auth dialog on connect click', async () => {
    const user = userEvent.setup();

    render(
      <EpicConnectionCard
        isConfigured={false}
        onConnectionChange={mockOnConnectionChange}
      />,
      { wrapper: createWrapper() }
    );

    await user.click(screen.getByRole('button', { name: 'Connect Epic Games Store' }));

    // Dialog should open (check for "Start Authentication" button which only appears in dialog)
    await waitFor(() => {
      expect(screen.getByText('Start Authentication')).toBeInTheDocument();
    });
  });
});
