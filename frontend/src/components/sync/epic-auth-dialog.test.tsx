import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { EpicAuthDialog } from './epic-auth-dialog';
import * as syncApi from '@/api/sync';

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

describe('EpicAuthDialog', () => {
  const mockOnOpenChange = vi.fn();
  const mockOnSuccess = vi.fn();

  beforeEach(() => {
    vi.clearAllMocks();
  });

  it('should render step 1 with start button', () => {
    render(
      <EpicAuthDialog open={true} onOpenChange={mockOnOpenChange} onSuccess={mockOnSuccess} />,
      { wrapper: createWrapper() }
    );

    expect(screen.getByText('Connect Epic Games Store')).toBeInTheDocument();
    expect(screen.getByText('Start Authentication')).toBeInTheDocument();
  });

  it('should call startEpicAuth on button click', async () => {
    const user = userEvent.setup();
    const mockStartEpicAuth = vi.spyOn(syncApi, 'startEpicAuth');
    mockStartEpicAuth.mockResolvedValue({
      authUrl: 'https://epicgames.com/activate',
      instructions: 'Please login',
    });

    render(
      <EpicAuthDialog open={true} onOpenChange={mockOnOpenChange} onSuccess={mockOnSuccess} />,
      { wrapper: createWrapper() }
    );

    const startButton = screen.getByText('Start Authentication');
    await user.click(startButton);

    await waitFor(() => {
      expect(mockStartEpicAuth).toHaveBeenCalled();
    });

    mockStartEpicAuth.mockRestore();
  });

  it('should transition to step 2 after start', async () => {
    const user = userEvent.setup();
    const mockStartEpicAuth = vi.spyOn(syncApi, 'startEpicAuth');
    mockStartEpicAuth.mockResolvedValue({
      authUrl: 'https://epicgames.com/activate',
      instructions: 'Please login',
    });

    render(
      <EpicAuthDialog open={true} onOpenChange={mockOnOpenChange} onSuccess={mockOnSuccess} />,
      { wrapper: createWrapper() }
    );

    const startButton = screen.getByText('Start Authentication');
    await user.click(startButton);

    await waitFor(() => {
      expect(screen.getByText('Enter Authorization Code')).toBeInTheDocument();
      expect(screen.getByPlaceholderText('Paste the code from Epic Games')).toBeInTheDocument();
    });

    mockStartEpicAuth.mockRestore();
  });

  it('should open Epic URL in new tab', async () => {
    const user = userEvent.setup();
    const mockStartEpicAuth = vi.spyOn(syncApi, 'startEpicAuth');
    mockStartEpicAuth.mockResolvedValue({
      authUrl: 'https://epicgames.com/activate',
      instructions: 'Please login',
    });

    const mockWindowOpen = vi.spyOn(window, 'open').mockImplementation(() => null);

    render(
      <EpicAuthDialog open={true} onOpenChange={mockOnOpenChange} onSuccess={mockOnSuccess} />,
      { wrapper: createWrapper() }
    );

    // Start auth to get to step 2
    await user.click(screen.getByText('Start Authentication'));

    await waitFor(() => {
      expect(screen.getByText('Open Epic Login')).toBeInTheDocument();
    });

    await user.click(screen.getByText('Open Epic Login'));

    expect(mockWindowOpen).toHaveBeenCalledWith('https://epicgames.com/activate', '_blank');

    mockStartEpicAuth.mockRestore();
    mockWindowOpen.mockRestore();
  });

  it('should submit auth code', async () => {
    const user = userEvent.setup();
    const mockStartEpicAuth = vi.spyOn(syncApi, 'startEpicAuth');
    const mockCompleteEpicAuth = vi.spyOn(syncApi, 'completeEpicAuth');

    mockStartEpicAuth.mockResolvedValue({
      authUrl: 'https://epicgames.com/activate',
      instructions: 'Please login',
    });

    mockCompleteEpicAuth.mockResolvedValue({
      valid: true,
      displayName: 'EpicUser',
      error: null,
    });

    render(
      <EpicAuthDialog open={true} onOpenChange={mockOnOpenChange} onSuccess={mockOnSuccess} />,
      { wrapper: createWrapper() }
    );

    // Start auth
    await user.click(screen.getByText('Start Authentication'));

    await waitFor(() => {
      expect(screen.getByPlaceholderText('Paste the code from Epic Games')).toBeInTheDocument();
    });

    // Enter code
    const codeInput = screen.getByPlaceholderText('Paste the code from Epic Games');
    await user.type(codeInput, 'TESTCODE123');

    // Submit
    await user.click(screen.getByText('Connect'));

    await waitFor(() => {
      expect(mockCompleteEpicAuth).toHaveBeenCalledWith('TESTCODE123');
      expect(mockOnSuccess).toHaveBeenCalled();
    });

    mockStartEpicAuth.mockRestore();
    mockCompleteEpicAuth.mockRestore();
  });

  it('should show error for invalid code', async () => {
    const user = userEvent.setup();
    const mockStartEpicAuth = vi.spyOn(syncApi, 'startEpicAuth');
    const mockCompleteEpicAuth = vi.spyOn(syncApi, 'completeEpicAuth');
    const { toast } = await import('sonner');

    mockStartEpicAuth.mockResolvedValue({
      authUrl: 'https://epicgames.com/activate',
      instructions: 'Please login',
    });

    mockCompleteEpicAuth.mockResolvedValue({
      valid: false,
      displayName: null,
      error: 'invalid_code',
    });

    render(
      <EpicAuthDialog open={true} onOpenChange={mockOnOpenChange} onSuccess={mockOnSuccess} />,
      { wrapper: createWrapper() }
    );

    // Start auth
    await user.click(screen.getByText('Start Authentication'));
    await waitFor(() => screen.getByPlaceholderText('Paste the code from Epic Games'));

    // Enter invalid code
    await user.type(screen.getByPlaceholderText('Paste the code from Epic Games'), 'BADCODE');
    await user.click(screen.getByText('Connect'));

    await waitFor(() => {
      expect(toast.error).toHaveBeenCalled();
    });

    mockStartEpicAuth.mockRestore();
    mockCompleteEpicAuth.mockRestore();
  });

  it('should reset state on cancel', async () => {
    const user = userEvent.setup();
    const mockStartEpicAuth = vi.spyOn(syncApi, 'startEpicAuth');

    mockStartEpicAuth.mockResolvedValue({
      authUrl: 'https://epicgames.com/activate',
      instructions: 'Please login',
    });

    render(
      <EpicAuthDialog open={true} onOpenChange={mockOnOpenChange} onSuccess={mockOnSuccess} />,
      { wrapper: createWrapper() }
    );

    // Go to step 2
    await user.click(screen.getByText('Start Authentication'));
    await waitFor(() => screen.getByText('Cancel'));

    // Click cancel
    await user.click(screen.getByText('Cancel'));

    expect(mockOnOpenChange).toHaveBeenCalledWith(false);

    mockStartEpicAuth.mockRestore();
  });
});
