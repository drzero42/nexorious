import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { EpicGamesStoreConnectionCard } from './epic-games-store-connection-card';
import * as hooks from '@/hooks';

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

function stubEpicGamesStoreConnection(
  data: Partial<ReturnType<typeof hooks.useEpicGamesStoreConnection>['data']>,
) {
  vi.spyOn(hooks, 'useEpicGamesStoreConnection').mockReturnValue({
    data: { connected: false, disabled: false, ...data },
  } as ReturnType<typeof hooks.useEpicGamesStoreConnection>);
}

function stubConnectEpicGamesStore(
  mutateAsync = vi.fn().mockResolvedValue({ displayName: 'X', accountId: 'y' }),
) {
  vi.spyOn(hooks, 'useConnectEpicGamesStore').mockReturnValue({
    mutateAsync,
    isPending: false,
  } as Partial<ReturnType<typeof hooks.useConnectEpicGamesStore>> as ReturnType<
    typeof hooks.useConnectEpicGamesStore
  >);
  return mutateAsync;
}

function stubDisconnectEpicGamesStore(mutateAsync = vi.fn().mockResolvedValue(undefined)) {
  vi.spyOn(hooks, 'useDisconnectEpicGamesStore').mockReturnValue({
    mutateAsync,
    isPending: false,
  } as Partial<ReturnType<typeof hooks.useDisconnectEpicGamesStore>> as ReturnType<
    typeof hooks.useDisconnectEpicGamesStore
  >);
  return mutateAsync;
}

describe('EpicGamesStoreConnectionCard', () => {
  const mockOnConnectionChange = vi.fn();

  beforeEach(() => {
    vi.clearAllMocks();
    stubEpicGamesStoreConnection({ connected: false, disabled: false });
    stubConnectEpicGamesStore();
    stubDisconnectEpicGamesStore();
  });

  it('renders not-configured state with inline auth-code form', () => {
    render(
      <EpicGamesStoreConnectionCard
        isConfigured={false}
        onConnectionChange={mockOnConnectionChange}
      />,
      { wrapper: createWrapper() },
    );

    expect(screen.getByText('Epic Games Store Connection')).toBeInTheDocument();
    expect(screen.getByLabelText('Authorization Code')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Connect Epic Games Store' })).toBeInTheDocument();
  });

  it.each([
    ['with a known reason code', { reason: 'legendary_not_configured' }],
    ['when the reason code is absent or unknown', {}],
  ])('renders disabled state %s without naming the backend env var', (_desc, extra) => {
    stubEpicGamesStoreConnection({ connected: false, disabled: true, ...extra });

    render(
      <EpicGamesStoreConnectionCard
        isConfigured={false}
        onConnectionChange={mockOnConnectionChange}
      />,
      { wrapper: createWrapper() },
    );

    expect(screen.getByText(/sync is disabled on this server/i)).toBeInTheDocument();
    expect(screen.getByText(/contact your administrator/i)).toBeInTheDocument();
    // The SPA must not bake in the backend's internal env var name (see #789).
    expect(screen.queryByText(/LEGENDARY_WORK_DIR/)).not.toBeInTheDocument();
    expect(screen.queryByLabelText('Authorization Code')).not.toBeInTheDocument();
  });

  it('renders connected state with display name from connection hook', () => {
    stubEpicGamesStoreConnection({
      connected: true,
      disabled: false,
      displayName: 'EpicUser123',
    });

    render(
      <EpicGamesStoreConnectionCard
        isConfigured={true}
        onConnectionChange={mockOnConnectionChange}
      />,
      {
        wrapper: createWrapper(),
      },
    );

    expect(screen.getByText(/Connected as EpicUser123/)).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Disconnect' })).toBeInTheDocument();
  });

  it('calls connectEpicGamesStore with the entered auth code', async () => {
    const user = userEvent.setup();
    const connect = stubConnectEpicGamesStore();

    render(
      <EpicGamesStoreConnectionCard
        isConfigured={false}
        onConnectionChange={mockOnConnectionChange}
      />,
      { wrapper: createWrapper() },
    );

    await user.type(screen.getByLabelText('Authorization Code'), '  abc123  ');
    await user.click(screen.getByRole('button', { name: 'Connect Epic Games Store' }));

    await waitFor(() => {
      expect(connect).toHaveBeenCalledWith('abc123');
      expect(mockOnConnectionChange).toHaveBeenCalled();
    });
  });

  it('shows a validation error when auth code is blank', async () => {
    const user = userEvent.setup();
    const connect = stubConnectEpicGamesStore();

    render(
      <EpicGamesStoreConnectionCard
        isConfigured={false}
        onConnectionChange={mockOnConnectionChange}
      />,
      { wrapper: createWrapper() },
    );

    await user.click(screen.getByRole('button', { name: 'Connect Epic Games Store' }));

    await waitFor(() => {
      expect(screen.getByText('Authorization code is required')).toBeInTheDocument();
    });
    expect(connect).not.toHaveBeenCalled();
  });

  it('opens a confirmation dialog before disconnecting', async () => {
    const user = userEvent.setup();
    stubEpicGamesStoreConnection({ connected: true, disabled: false, displayName: 'X' });

    render(
      <EpicGamesStoreConnectionCard
        isConfigured={true}
        onConnectionChange={mockOnConnectionChange}
      />,
      {
        wrapper: createWrapper(),
      },
    );

    await user.click(screen.getByRole('button', { name: 'Disconnect' }));

    await waitFor(() => {
      expect(screen.getByText('Disconnect Epic Games Store?')).toBeInTheDocument();
    });
  });

  it('calls disconnectEpicGamesStore when the confirmation is accepted', async () => {
    const user = userEvent.setup();
    const disconnect = stubDisconnectEpicGamesStore();
    stubEpicGamesStoreConnection({ connected: true, disabled: false, displayName: 'X' });

    render(
      <EpicGamesStoreConnectionCard
        isConfigured={true}
        onConnectionChange={mockOnConnectionChange}
      />,
      {
        wrapper: createWrapper(),
      },
    );

    await user.click(screen.getByRole('button', { name: 'Disconnect' }));
    await waitFor(() => screen.getByText('Disconnect Epic Games Store?'));

    const confirmButton = screen
      .getAllByRole('button', { name: /Disconnect$/i })
      .find((btn) => btn.closest('[role="alertdialog"]'));
    if (!confirmButton) throw new Error('confirm Disconnect button not found inside alert dialog');
    await user.click(confirmButton);

    await waitFor(() => {
      expect(disconnect).toHaveBeenCalled();
      expect(mockOnConnectionChange).toHaveBeenCalled();
    });
  });

  it('shows the playtime limitation note both before and after connecting', () => {
    const { rerender } = render(
      <EpicGamesStoreConnectionCard
        isConfigured={false}
        onConnectionChange={mockOnConnectionChange}
      />,
      { wrapper: createWrapper() },
    );
    expect(
      screen.getByText(/Epic Games Store does not provide playtime data/i),
    ).toBeInTheDocument();

    stubEpicGamesStoreConnection({ connected: true, disabled: false, displayName: 'X' });
    rerender(
      <EpicGamesStoreConnectionCard
        isConfigured={true}
        onConnectionChange={mockOnConnectionChange}
      />,
    );
    expect(
      screen.getByText(/Epic Games Store does not provide playtime data/i),
    ).toBeInTheDocument();
  });
});
