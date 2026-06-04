import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { EpicConnectionCard } from './epic-connection-card';
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

function stubEpicConnection(data: Partial<ReturnType<typeof hooks.useEpicConnection>['data']>) {
  vi.spyOn(hooks, 'useEpicConnection').mockReturnValue({
    data: { connected: false, disabled: false, ...data },
  } as ReturnType<typeof hooks.useEpicConnection>);
}

function stubConnectEpic(
  mutateAsync = vi.fn().mockResolvedValue({ displayName: 'X', accountId: 'y' }),
) {
  vi.spyOn(hooks, 'useConnectEpic').mockReturnValue({
    mutateAsync,
    isPending: false,
  } as Partial<ReturnType<typeof hooks.useConnectEpic>> as ReturnType<typeof hooks.useConnectEpic>);
  return mutateAsync;
}

function stubDisconnectEpic(mutateAsync = vi.fn().mockResolvedValue(undefined)) {
  vi.spyOn(hooks, 'useDisconnectEpic').mockReturnValue({
    mutateAsync,
    isPending: false,
  } as Partial<ReturnType<typeof hooks.useDisconnectEpic>> as ReturnType<
    typeof hooks.useDisconnectEpic
  >);
  return mutateAsync;
}

describe('EpicConnectionCard', () => {
  const mockOnConnectionChange = vi.fn();

  beforeEach(() => {
    vi.clearAllMocks();
    stubEpicConnection({ connected: false, disabled: false });
    stubConnectEpic();
    stubDisconnectEpic();
  });

  it('renders not-configured state with inline auth-code form', () => {
    render(
      <EpicConnectionCard isConfigured={false} onConnectionChange={mockOnConnectionChange} />,
      { wrapper: createWrapper() },
    );

    expect(screen.getByText('Epic Games Store Connection')).toBeInTheDocument();
    expect(screen.getByLabelText('Authorization Code')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Connect Epic Games Store' })).toBeInTheDocument();
  });

  it('renders disabled state keyed off the reason code, without naming the backend env var', () => {
    stubEpicConnection({ connected: false, disabled: true, reason: 'legendary_not_configured' });

    render(
      <EpicConnectionCard isConfigured={false} onConnectionChange={mockOnConnectionChange} />,
      { wrapper: createWrapper() },
    );

    expect(screen.getByText(/sync is disabled on this server/i)).toBeInTheDocument();
    expect(screen.getByText(/contact your administrator/i)).toBeInTheDocument();
    // The SPA must not bake in the backend's internal env var name (see #789).
    expect(screen.queryByText(/LEGENDARY_WORK_DIR/)).not.toBeInTheDocument();
    expect(screen.queryByLabelText('Authorization Code')).not.toBeInTheDocument();
  });

  it('renders a generic disabled message when the reason code is absent or unknown', () => {
    stubEpicConnection({ connected: false, disabled: true });

    render(
      <EpicConnectionCard isConfigured={false} onConnectionChange={mockOnConnectionChange} />,
      { wrapper: createWrapper() },
    );

    expect(screen.getByText(/sync is disabled on this server/i)).toBeInTheDocument();
    expect(screen.queryByText(/LEGENDARY_WORK_DIR/)).not.toBeInTheDocument();
  });

  it('renders connected state with display name from connection hook', () => {
    stubEpicConnection({
      connected: true,
      disabled: false,
      displayName: 'EpicUser123',
      accountId: 'epic-account-id',
    });

    render(<EpicConnectionCard isConfigured={true} onConnectionChange={mockOnConnectionChange} />, {
      wrapper: createWrapper(),
    });

    expect(screen.getByText(/Connected as EpicUser123/)).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Disconnect' })).toBeInTheDocument();
  });

  it('calls connectEpic with the entered auth code', async () => {
    const user = userEvent.setup();
    const connect = stubConnectEpic();

    render(
      <EpicConnectionCard isConfigured={false} onConnectionChange={mockOnConnectionChange} />,
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
    const connect = stubConnectEpic();

    render(
      <EpicConnectionCard isConfigured={false} onConnectionChange={mockOnConnectionChange} />,
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
    stubEpicConnection({ connected: true, disabled: false, displayName: 'X', accountId: 'y' });

    render(<EpicConnectionCard isConfigured={true} onConnectionChange={mockOnConnectionChange} />, {
      wrapper: createWrapper(),
    });

    await user.click(screen.getByRole('button', { name: 'Disconnect' }));

    await waitFor(() => {
      expect(screen.getByText('Disconnect Epic Games Store?')).toBeInTheDocument();
    });
  });

  it('calls disconnectEpic when the confirmation is accepted', async () => {
    const user = userEvent.setup();
    const disconnect = stubDisconnectEpic();
    stubEpicConnection({ connected: true, disabled: false, displayName: 'X', accountId: 'y' });

    render(<EpicConnectionCard isConfigured={true} onConnectionChange={mockOnConnectionChange} />, {
      wrapper: createWrapper(),
    });

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
      <EpicConnectionCard isConfigured={false} onConnectionChange={mockOnConnectionChange} />,
      { wrapper: createWrapper() },
    );
    expect(
      screen.getByText(/Epic Games Store does not provide playtime data/i),
    ).toBeInTheDocument();

    stubEpicConnection({ connected: true, disabled: false, displayName: 'X', accountId: 'y' });
    rerender(
      <EpicConnectionCard isConfigured={true} onConnectionChange={mockOnConnectionChange} />,
    );
    expect(
      screen.getByText(/Epic Games Store does not provide playtime data/i),
    ).toBeInTheDocument();
  });
});
