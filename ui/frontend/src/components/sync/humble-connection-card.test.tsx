import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { HumbleConnectionCard } from './humble-connection-card';
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

function stubConnectHumble(mutateAsync = vi.fn().mockResolvedValue({ valid: true, error: null })) {
  vi.spyOn(hooks, 'useConnectHumble').mockReturnValue({
    mutateAsync,
    isPending: false,
  } as Partial<ReturnType<typeof hooks.useConnectHumble>> as ReturnType<
    typeof hooks.useConnectHumble
  >);
  return mutateAsync;
}

function stubDisconnectHumble(mutateAsync = vi.fn().mockResolvedValue(undefined)) {
  vi.spyOn(hooks, 'useDisconnectHumble').mockReturnValue({
    mutateAsync,
    isPending: false,
  } as Partial<ReturnType<typeof hooks.useDisconnectHumble>> as ReturnType<
    typeof hooks.useDisconnectHumble
  >);
  return mutateAsync;
}

describe('HumbleConnectionCard', () => {
  const mockOnConnectionChange = vi.fn();

  beforeEach(() => {
    vi.clearAllMocks();
    stubConnectHumble();
    stubDisconnectHumble();
  });

  it('renders not-configured state with session cookie form', () => {
    render(
      <HumbleConnectionCard
        isConfigured={false}
        credentialsError={false}
        onConnectionChange={mockOnConnectionChange}
      />,
      { wrapper: createWrapper() },
    );

    expect(screen.getByText('Humble Bundle Connection')).toBeInTheDocument();
    expect(screen.getByLabelText('Session cookie (_simpleauth_sess)')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Connect Humble Bundle' })).toBeInTheDocument();
  });

  it('renders humblebundle.com as an external link opening in a new tab', async () => {
    render(
      <HumbleConnectionCard
        isConfigured={false}
        credentialsError={false}
        onConnectionChange={mockOnConnectionChange}
      />,
      { wrapper: createWrapper() },
    );

    await userEvent.click(
      screen.getByRole('button', { name: /How do I get my _simpleauth_sess cookie\?/ }),
    );

    const link = await screen.findByRole('link', { name: /humblebundle\.com/ });
    expect(link).toHaveAttribute('href', 'https://www.humblebundle.com');
    expect(link).toHaveAttribute('target', '_blank');
    expect(link).toHaveAttribute('rel', 'noopener noreferrer');
  });

  it('uses a single-line text input so Enter submits the form', async () => {
    const mutateAsync = stubConnectHumble();

    render(
      <HumbleConnectionCard
        isConfigured={false}
        credentialsError={false}
        onConnectionChange={mockOnConnectionChange}
      />,
      { wrapper: createWrapper() },
    );

    const field = screen.getByLabelText('Session cookie (_simpleauth_sess)');
    expect(field).toHaveAttribute('type', 'text');

    // Typing the value then pressing Enter must submit (a <textarea> would insert a newline).
    await userEvent.type(field, 'sess-cookie-value{Enter}');

    await waitFor(() => {
      expect(mutateAsync).toHaveBeenCalledWith('sess-cookie-value');
      expect(mockOnConnectionChange).toHaveBeenCalled();
    });
  });

  it('renders connected state with disconnect option', () => {
    render(
      <HumbleConnectionCard
        isConfigured={true}
        credentialsError={false}
        onConnectionChange={mockOnConnectionChange}
      />,
      { wrapper: createWrapper() },
    );

    expect(screen.getByRole('button', { name: 'Disconnect' })).toBeInTheDocument();
  });
});
