import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { toast } from 'sonner';
import { AmazonGamesConnectionCard } from './amazon-games-connection-card';
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

function stubConnection(data: Partial<ReturnType<typeof hooks.useAmazonGamesConnection>['data']>) {
  vi.spyOn(hooks, 'useAmazonGamesConnection').mockReturnValue({
    data: { connected: false, disabled: false, ...data },
  } as ReturnType<typeof hooks.useAmazonGamesConnection>);
}

function stubStart(
  mutateAsync = vi
    .fn()
    .mockResolvedValue({ attemptId: 'att-1', loginUrl: 'https://amazon.com/ap/signin?x=1' }),
) {
  vi.spyOn(hooks, 'useStartAmazonGamesLogin').mockReturnValue({
    mutateAsync,
    isPending: false,
  } as Partial<ReturnType<typeof hooks.useStartAmazonGamesLogin>> as ReturnType<
    typeof hooks.useStartAmazonGamesLogin
  >);
  return mutateAsync;
}

function stubConnect(mutateAsync = vi.fn().mockResolvedValue({ username: 'Ama' })) {
  vi.spyOn(hooks, 'useConnectAmazonGames').mockReturnValue({
    mutateAsync,
    isPending: false,
  } as Partial<ReturnType<typeof hooks.useConnectAmazonGames>> as ReturnType<
    typeof hooks.useConnectAmazonGames
  >);
  return mutateAsync;
}

function stubDisconnect(mutateAsync = vi.fn().mockResolvedValue(undefined)) {
  vi.spyOn(hooks, 'useDisconnectAmazonGames').mockReturnValue({
    mutateAsync,
    isPending: false,
  } as Partial<ReturnType<typeof hooks.useDisconnectAmazonGames>> as ReturnType<
    typeof hooks.useDisconnectAmazonGames
  >);
  return mutateAsync;
}

const landing = 'https://www.amazon.com/?openid.oa2.authorization_code=AN1';

describe('AmazonGamesConnectionCard', () => {
  const onConnectionChange = vi.fn();

  beforeEach(() => {
    vi.clearAllMocks();
    stubConnection({});
    stubStart();
    stubConnect();
    stubDisconnect();
  });

  it('starts a sign-in, links to Amazon, and finishes with the pasted address', async () => {
    const user = userEvent.setup();
    const start = stubStart();
    const connect = stubConnect();
    render(
      <AmazonGamesConnectionCard isConfigured={false} onConnectionChange={onConnectionChange} />,
      {
        wrapper: createWrapper(),
      },
    );

    expect(screen.queryByLabelText('Address after signing in')).not.toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: 'Sign in with Amazon' }));
    expect(start).toHaveBeenCalledTimes(1);

    const link = await screen.findByRole('link', { name: /Open the Amazon sign-in page/ });
    expect(link).toHaveAttribute('href', 'https://amazon.com/ap/signin?x=1');
    expect(link).toHaveAttribute('target', '_blank');

    await user.type(screen.getByLabelText('Address after signing in'), landing);
    await user.click(screen.getByRole('button', { name: 'Connect Amazon Games' }));

    await waitFor(() => {
      expect(connect).toHaveBeenCalledWith({ attemptId: 'att-1', redirectUrl: landing });
    });
    expect(onConnectionChange).toHaveBeenCalled();
    expect(toast.success).toHaveBeenCalledWith('Amazon Games connected as Ama');
  });

  it('requires an address before finishing', async () => {
    const user = userEvent.setup();
    const connect = stubConnect();
    render(
      <AmazonGamesConnectionCard isConfigured={false} onConnectionChange={onConnectionChange} />,
      {
        wrapper: createWrapper(),
      },
    );
    await user.click(screen.getByRole('button', { name: 'Sign in with Amazon' }));
    await user.click(await screen.findByRole('button', { name: 'Connect Amazon Games' }));

    expect(await screen.findByText('Paste the address Amazon took you to')).toBeInTheDocument();
    expect(screen.getByLabelText('Address after signing in')).toHaveAttribute(
      'aria-invalid',
      'true',
    );
    expect(screen.getByLabelText('Address after signing in')).toHaveAccessibleDescription(
      'Paste the address Amazon took you to',
    );
    expect(connect).not.toHaveBeenCalled();
  });

  it('shows the server error under the field when finishing fails', async () => {
    const user = userEvent.setup();
    stubConnect(vi.fn().mockRejectedValue(new Error('this Amazon sign-in has expired')));
    render(
      <AmazonGamesConnectionCard isConfigured={false} onConnectionChange={onConnectionChange} />,
      {
        wrapper: createWrapper(),
      },
    );
    await user.click(screen.getByRole('button', { name: 'Sign in with Amazon' }));
    await user.type(await screen.findByLabelText('Address after signing in'), landing);
    await user.click(screen.getByRole('button', { name: 'Connect Amazon Games' }));

    expect(await screen.findByText('this Amazon sign-in has expired')).toBeInTheDocument();
    expect(onConnectionChange).not.toHaveBeenCalled();
  });

  it('starts a new sign-in from the paste step, replacing the old link', async () => {
    const user = userEvent.setup();
    const start = stubStart(
      vi
        .fn()
        .mockResolvedValueOnce({ attemptId: 'att-1', loginUrl: 'https://amazon.com/ap/signin?x=1' })
        .mockResolvedValueOnce({
          attemptId: 'att-2',
          loginUrl: 'https://amazon.com/ap/signin?x=2',
        }),
    );
    const connect = stubConnect();
    render(
      <AmazonGamesConnectionCard isConfigured={false} onConnectionChange={onConnectionChange} />,
      { wrapper: createWrapper() },
    );
    await user.click(screen.getByRole('button', { name: 'Sign in with Amazon' }));
    await user.click(await screen.findByRole('button', { name: 'Start a new sign-in' }));

    await waitFor(() => {
      expect(screen.getByRole('link', { name: /Open the Amazon sign-in page/ })).toHaveAttribute(
        'href',
        'https://amazon.com/ap/signin?x=2',
      );
    });
    expect(start).toHaveBeenCalledTimes(2);
    await user.type(screen.getByLabelText('Address after signing in'), landing);
    await user.click(screen.getByRole('button', { name: 'Connect Amazon Games' }));
    await waitFor(() => {
      expect(connect).toHaveBeenCalledWith({ attemptId: 'att-2', redirectUrl: landing });
    });
  });

  it('shows the account and a disconnect button when connected', () => {
    stubConnection({ connected: true, username: 'Ama' });
    render(
      <AmazonGamesConnectionCard isConfigured={true} onConnectionChange={onConnectionChange} />,
      {
        wrapper: createWrapper(),
      },
    );
    expect(screen.getByText('Connected as Ama')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /Disconnect/ })).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Sign in with Amazon' })).not.toBeInTheDocument();
  });

  it('asks to sign in again when the stored sign-in was rejected', () => {
    stubConnection({ connected: true, credentialsError: true, username: 'Ama' });
    render(
      <AmazonGamesConnectionCard isConfigured={true} onConnectionChange={onConnectionChange} />,
      {
        wrapper: createWrapper(),
      },
    );
    expect(screen.getByText('Amazon sign-in is no longer valid')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Sign in to Amazon again' })).toBeInTheDocument();
  });

  it('explains when the server has no nile', () => {
    stubConnection({ disabled: true, reason: 'nile_not_installed' });
    render(
      <AmazonGamesConnectionCard isConfigured={false} onConnectionChange={onConnectionChange} />,
      {
        wrapper: createWrapper(),
      },
    );
    expect(screen.getByText(/Amazon Games sync is disabled on this server/)).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Sign in with Amazon' })).not.toBeInTheDocument();
  });
});
