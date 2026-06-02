import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { GOGConnectionCard } from './gog-connection-card';
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

function stubGOGConnection(data: Partial<ReturnType<typeof hooks.useGOGConnection>['data']>) {
  vi.spyOn(hooks, 'useGOGConnection').mockReturnValue({
    data: { connected: false, ...data },
  } as ReturnType<typeof hooks.useGOGConnection>);
}

function stubConnectGOG(
  mutateAsync = vi.fn().mockResolvedValue({ username: 'goguser', userId: 'u1' }),
) {
  vi.spyOn(hooks, 'useConnectGOG').mockReturnValue({
    mutateAsync,
    isPending: false,
  } as Partial<ReturnType<typeof hooks.useConnectGOG>> as ReturnType<typeof hooks.useConnectGOG>);
  return mutateAsync;
}

function stubDisconnectGOG(mutateAsync = vi.fn().mockResolvedValue(undefined)) {
  vi.spyOn(hooks, 'useDisconnectGOG').mockReturnValue({
    mutateAsync,
    isPending: false,
  } as Partial<ReturnType<typeof hooks.useDisconnectGOG>> as ReturnType<
    typeof hooks.useDisconnectGOG
  >);
  return mutateAsync;
}

describe('GOGConnectionCard', () => {
  const mockOnConnectionChange = vi.fn();

  beforeEach(() => {
    vi.clearAllMocks();
    stubGOGConnection({ connected: false });
    stubConnectGOG();
    stubDisconnectGOG();
  });

  it('renders not-configured state with auth code form', () => {
    render(<GOGConnectionCard isConfigured={false} onConnectionChange={mockOnConnectionChange} />, {
      wrapper: createWrapper(),
    });

    expect(screen.getByText('GOG Connection')).toBeInTheDocument();
    expect(screen.getByText('Not Configured')).toBeInTheDocument();
    expect(screen.getByLabelText('GOG URL or Authorization Code')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Connect GOG' })).toBeInTheDocument();
  });

  it('tells the user they can paste the full URL or just the code', () => {
    render(<GOGConnectionCard isConfigured={false} onConnectionChange={mockOnConnectionChange} />, {
      wrapper: createWrapper(),
    });

    expect(
      screen.getByPlaceholderText('Paste the full GOG URL or just the code'),
    ).toBeInTheDocument();
  });

  it('renders connected state with username', () => {
    stubGOGConnection({ connected: true, username: 'goguser', userId: 'u1' });

    render(<GOGConnectionCard isConfigured={true} onConnectionChange={mockOnConnectionChange} />, {
      wrapper: createWrapper(),
    });

    expect(screen.getByText('Connected')).toBeInTheDocument();
    expect(screen.getByText(/Connected as goguser/)).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Disconnect' })).toBeInTheDocument();
  });

  it('submits auth code and calls onConnectionChange on success', async () => {
    const mutateAsync = stubConnectGOG();

    render(<GOGConnectionCard isConfigured={false} onConnectionChange={mockOnConnectionChange} />, {
      wrapper: createWrapper(),
    });

    await userEvent.type(screen.getByLabelText('GOG URL or Authorization Code'), 'my-gog-code');
    await userEvent.click(screen.getByRole('button', { name: 'Connect GOG' }));

    await waitFor(() => {
      expect(mutateAsync).toHaveBeenCalledWith('my-gog-code');
      expect(mockOnConnectionChange).toHaveBeenCalled();
    });
  });

  it('shows error when auth code is empty', async () => {
    render(<GOGConnectionCard isConfigured={false} onConnectionChange={mockOnConnectionChange} />, {
      wrapper: createWrapper(),
    });

    await userEvent.click(screen.getByRole('button', { name: 'Connect GOG' }));

    expect(await screen.findByText('Enter the GOG URL or authorization code')).toBeInTheDocument();
  });

  it('calls disconnect and onConnectionChange on disconnect', async () => {
    stubGOGConnection({ connected: true, username: 'goguser' });
    const mutateAsync = stubDisconnectGOG();

    render(<GOGConnectionCard isConfigured={true} onConnectionChange={mockOnConnectionChange} />, {
      wrapper: createWrapper(),
    });

    await userEvent.click(screen.getByRole('button', { name: 'Disconnect' }));
    await userEvent.click(screen.getByRole('button', { name: 'Disconnect', hidden: false }));

    await waitFor(() => {
      expect(mutateAsync).toHaveBeenCalled();
    });
  });
});
