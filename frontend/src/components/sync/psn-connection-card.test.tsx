import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, waitFor } from '@/test/test-utils';
import userEvent from '@testing-library/user-event';
import { PSNConnectionCard } from './psn-connection-card';

// Mock hooks
const mockConfigureMutateAsync = vi.fn();
const mockDisconnectMutateAsync = vi.fn();

vi.mock('@/hooks', () => ({
  useConfigurePSN: vi.fn(() => ({
    mutateAsync: mockConfigureMutateAsync,
    isPending: false,
  })),
  useDisconnectPSN: vi.fn(() => ({
    mutateAsync: mockDisconnectMutateAsync,
    isPending: false,
  })),
}));

vi.mock('sonner', () => ({
  toast: {
    success: vi.fn(),
    error: vi.fn(),
  },
}));

describe('PSNConnectionCard', () => {
  const mockOnConnectionChange = vi.fn();

  beforeEach(() => {
    vi.clearAllMocks();
  });

  describe('badge states', () => {
    it('renders "Not Configured" badge when not configured', () => {
      render(
        <PSNConnectionCard
          isConfigured={false}
          tokenExpired={false}
          onConnectionChange={mockOnConnectionChange}
        />
      );

      expect(screen.getByText('Not Configured')).toBeInTheDocument();
    });

    it('renders "Connected" badge when configured', () => {
      render(
        <PSNConnectionCard
          isConfigured={true}
          tokenExpired={false}
          accountId="test-account-id"
          onlineId="TestPSNUser"
          onConnectionChange={mockOnConnectionChange}
        />
      );

      expect(screen.getByText('Connected')).toBeInTheDocument();
    });

    it('renders "Token Expired" badge when token expired', () => {
      render(
        <PSNConnectionCard
          isConfigured={true}
          tokenExpired={true}
          accountId="test-account-id"
          onlineId="TestPSNUser"
          onConnectionChange={mockOnConnectionChange}
        />
      );

      expect(screen.getByText('Token Expired')).toBeInTheDocument();
    });
  });

  describe('not configured state', () => {
    it('shows connection form with NPSSO token input', () => {
      render(
        <PSNConnectionCard
          isConfigured={false}
          tokenExpired={false}
          onConnectionChange={mockOnConnectionChange}
        />
      );

      expect(screen.getByLabelText('NPSSO Token')).toBeInTheDocument();
    });

    it('shows "Configure PSN" button', () => {
      render(
        <PSNConnectionCard
          isConfigured={false}
          tokenExpired={false}
          onConnectionChange={mockOnConnectionChange}
        />
      );

      expect(screen.getByRole('button', { name: 'Configure PSN' })).toBeInTheDocument();
    });

    it('shows help accordion', () => {
      render(
        <PSNConnectionCard
          isConfigured={false}
          tokenExpired={false}
          onConnectionChange={mockOnConnectionChange}
        />
      );

      expect(screen.getByText('How do I get my NPSSO token?')).toBeInTheDocument();
    });

    it('shows description for not configured state', () => {
      render(
        <PSNConnectionCard
          isConfigured={false}
          tokenExpired={false}
          onConnectionChange={mockOnConnectionChange}
        />
      );

      expect(
        screen.getByText('Connect your PlayStation Network account to sync your game library')
      ).toBeInTheDocument();
    });
  });

  describe('configured state', () => {
    it('shows "Connected as {onlineId}" message', () => {
      render(
        <PSNConnectionCard
          isConfigured={true}
          tokenExpired={false}
          accountId="test-account-id"
          onlineId="TestPSNUser"
          onConnectionChange={mockOnConnectionChange}
        />
      );

      expect(screen.getByText('Connected as TestPSNUser')).toBeInTheDocument();
    });

    it('shows account ID when configured', () => {
      render(
        <PSNConnectionCard
          isConfigured={true}
          tokenExpired={false}
          accountId="test-account-id"
          onlineId="TestPSNUser"
          onConnectionChange={mockOnConnectionChange}
        />
      );

      expect(screen.getByText('test-account-id')).toBeInTheDocument();
    });

    it('shows disconnect button', () => {
      render(
        <PSNConnectionCard
          isConfigured={true}
          tokenExpired={false}
          accountId="test-account-id"
          onlineId="TestPSNUser"
          onConnectionChange={mockOnConnectionChange}
        />
      );

      expect(screen.getByRole('button', { name: 'Disconnect' })).toBeInTheDocument();
    });

    it('shows description for configured state', () => {
      render(
        <PSNConnectionCard
          isConfigured={true}
          tokenExpired={false}
          accountId="test-account-id"
          onlineId="TestPSNUser"
          onConnectionChange={mockOnConnectionChange}
        />
      );

      expect(screen.getByText('Your PlayStation Network account is connected')).toBeInTheDocument();
    });

    it('does not show connection form when configured', () => {
      render(
        <PSNConnectionCard
          isConfigured={true}
          tokenExpired={false}
          accountId="test-account-id"
          onlineId="TestPSNUser"
          onConnectionChange={mockOnConnectionChange}
        />
      );

      expect(screen.queryByLabelText('NPSSO Token')).not.toBeInTheDocument();
    });
  });

  describe('token expired state', () => {
    it('shows warning when token expired', () => {
      render(
        <PSNConnectionCard
          isConfigured={true}
          tokenExpired={true}
          accountId="test-account-id"
          onlineId="TestPSNUser"
          onConnectionChange={mockOnConnectionChange}
        />
      );

      expect(screen.getByText(/Your NPSSO token has expired/)).toBeInTheDocument();
    });

    it('shows reconfigure form when token expired', () => {
      render(
        <PSNConnectionCard
          isConfigured={true}
          tokenExpired={true}
          accountId="test-account-id"
          onlineId="TestPSNUser"
          onConnectionChange={mockOnConnectionChange}
        />
      );

      expect(screen.getByLabelText('NPSSO Token')).toBeInTheDocument();
      expect(screen.getByRole('button', { name: 'Reconfigure' })).toBeInTheDocument();
    });
  });

  describe('form validation', () => {
    it('validates NPSSO token format - shows error for too short token', async () => {
      const user = userEvent.setup({ delay: null });

      render(
        <PSNConnectionCard
          isConfigured={false}
          tokenExpired={false}
          onConnectionChange={mockOnConnectionChange}
        />
      );

      const tokenInput = screen.getByLabelText('NPSSO Token');
      await user.type(tokenInput, '12345');

      const submitButton = screen.getByRole('button', { name: 'Configure PSN' });
      await user.click(submitButton);

      await waitFor(() => {
        expect(screen.getByText('NPSSO token must be exactly 64 characters')).toBeInTheDocument();
      });
    });

    it('validates NPSSO token format - shows error for too long token', async () => {
      const user = userEvent.setup({ delay: null });

      render(
        <PSNConnectionCard
          isConfigured={false}
          tokenExpired={false}
          onConnectionChange={mockOnConnectionChange}
        />
      );

      const tokenInput = screen.getByLabelText('NPSSO Token');
      await user.type(tokenInput, 'a'.repeat(70));

      const submitButton = screen.getByRole('button', { name: 'Configure PSN' });
      await user.click(submitButton);

      await waitFor(() => {
        expect(screen.getByText('NPSSO token must be exactly 64 characters')).toBeInTheDocument();
      });
    });

    it('validates NPSSO token format - shows error for invalid characters', async () => {
      const user = userEvent.setup({ delay: null });

      render(
        <PSNConnectionCard
          isConfigured={false}
          tokenExpired={false}
          onConnectionChange={mockOnConnectionChange}
        />
      );

      const tokenInput = screen.getByLabelText('NPSSO Token');
      // Exactly 64 characters but contains invalid characters (hyphens)
      await user.type(tokenInput, 'abcd1234-fgh5678-jkl9012-nop3456-rst7890-vwx1234-zab5678-1234567');

      const submitButton = screen.getByRole('button', { name: 'Configure PSN' });
      await user.click(submitButton);

      await waitFor(() => {
        expect(screen.getByText('NPSSO token must contain only alphanumeric characters')).toBeInTheDocument();
      });
    });

    it('accepts valid NPSSO token format (64 alphanumeric characters)', async () => {
      const user = userEvent.setup({ delay: null });
      mockConfigureMutateAsync.mockResolvedValue({
        valid: true,
        accountId: 'test-account-id',
        onlineId: 'TestPSNUser',
        error: null,
      });

      render(
        <PSNConnectionCard
          isConfigured={false}
          tokenExpired={false}
          onConnectionChange={mockOnConnectionChange}
        />
      );

      const tokenInput = screen.getByLabelText('NPSSO Token');
      const validToken = 'a'.repeat(64);
      await user.type(tokenInput, validToken);

      const submitButton = screen.getByRole('button', { name: 'Configure PSN' });
      await user.click(submitButton);

      await waitFor(() => {
        expect(mockConfigureMutateAsync).toHaveBeenCalledWith(validToken);
      });
    });
  });

  describe('form submission', () => {
    it('calls configurePSN on successful form submission', async () => {
      const user = userEvent.setup({ delay: null });
      mockConfigureMutateAsync.mockResolvedValue({
        valid: true,
        accountId: 'test-account-id',
        onlineId: 'TestPSNUser',
        error: null,
      });

      render(
        <PSNConnectionCard
          isConfigured={false}
          tokenExpired={false}
          onConnectionChange={mockOnConnectionChange}
        />
      );

      const tokenInput = screen.getByLabelText('NPSSO Token');
      const validToken = 'a'.repeat(64);
      await user.type(tokenInput, validToken);

      const submitButton = screen.getByRole('button', { name: 'Configure PSN' });
      await user.click(submitButton);

      await waitFor(() => {
        expect(mockConfigureMutateAsync).toHaveBeenCalledWith(validToken);
        expect(mockOnConnectionChange).toHaveBeenCalled();
      });
    });

    it('shows error for invalid token from API', async () => {
      const user = userEvent.setup({ delay: null });
      mockConfigureMutateAsync.mockResolvedValue({
        valid: false,
        accountId: null,
        onlineId: null,
        error: 'Invalid NPSSO token',
      });

      render(
        <PSNConnectionCard
          isConfigured={false}
          tokenExpired={false}
          onConnectionChange={mockOnConnectionChange}
        />
      );

      const tokenInput = screen.getByLabelText('NPSSO Token');
      await user.type(tokenInput, 'a'.repeat(64));

      const submitButton = screen.getByRole('button', { name: 'Configure PSN' });
      await user.click(submitButton);

      await waitFor(() => {
        expect(mockOnConnectionChange).not.toHaveBeenCalled();
      });
    });
  });

  describe('disconnect functionality', () => {
    it('shows disconnect confirmation dialog when button clicked', async () => {
      const user = userEvent.setup({ delay: null });

      render(
        <PSNConnectionCard
          isConfigured={true}
          tokenExpired={false}
          accountId="test-account-id"
          onlineId="TestPSNUser"
          onConnectionChange={mockOnConnectionChange}
        />
      );

      const disconnectButton = screen.getByRole('button', { name: 'Disconnect' });
      await user.click(disconnectButton);

      expect(screen.getByText('Disconnect PlayStation Network?')).toBeInTheDocument();
      expect(
        screen.getByText(
          'Your sync settings will be preserved but syncing will stop until you reconnect.'
        )
      ).toBeInTheDocument();
    });

    it('calls disconnect mutation when confirmed', async () => {
      const user = userEvent.setup({ delay: null });
      mockDisconnectMutateAsync.mockResolvedValue(undefined);

      render(
        <PSNConnectionCard
          isConfigured={true}
          tokenExpired={false}
          accountId="test-account-id"
          onlineId="TestPSNUser"
          onConnectionChange={mockOnConnectionChange}
        />
      );

      const disconnectButton = screen.getByRole('button', { name: 'Disconnect' });
      await user.click(disconnectButton);

      // Click confirm in the dialog
      const dialogConfirmButton = screen
        .getAllByRole('button', { name: 'Disconnect' })
        .find((btn) => btn.closest('[role="alertdialog"]'));

      if (dialogConfirmButton) {
        await user.click(dialogConfirmButton);
      }

      await waitFor(() => {
        expect(mockDisconnectMutateAsync).toHaveBeenCalled();
        expect(mockOnConnectionChange).toHaveBeenCalled();
      });
    });

    it('does not disconnect when cancelled', async () => {
      const user = userEvent.setup({ delay: null });

      render(
        <PSNConnectionCard
          isConfigured={true}
          tokenExpired={false}
          accountId="test-account-id"
          onlineId="TestPSNUser"
          onConnectionChange={mockOnConnectionChange}
        />
      );

      const disconnectButton = screen.getByRole('button', { name: 'Disconnect' });
      await user.click(disconnectButton);

      // Click cancel in the dialog
      const cancelButton = screen.getByRole('button', { name: 'Cancel' });
      await user.click(cancelButton);

      expect(mockDisconnectMutateAsync).not.toHaveBeenCalled();
      expect(mockOnConnectionChange).not.toHaveBeenCalled();
    });
  });

  describe('help accordion', () => {
    it('expands NPSSO token help accordion when clicked', async () => {
      const user = userEvent.setup({ delay: null });

      render(
        <PSNConnectionCard
          isConfigured={false}
          tokenExpired={false}
          onConnectionChange={mockOnConnectionChange}
        />
      );

      const tokenHelp = screen.getByText('How do I get my NPSSO token?');
      await user.click(tokenHelp);

      await waitFor(() => {
        expect(
          screen.getByText(/Your NPSSO token is a session cookie/)
        ).toBeInTheDocument();
      });
    });

    it('shows security warning in help text', async () => {
      const user = userEvent.setup({ delay: null });

      render(
        <PSNConnectionCard
          isConfigured={false}
          tokenExpired={false}
          onConnectionChange={mockOnConnectionChange}
        />
      );

      const tokenHelp = screen.getByText('How do I get my NPSSO token?');
      await user.click(tokenHelp);

      await waitFor(() => {
        expect(
          screen.getByText(/expires approximately every 2 months/)
        ).toBeInTheDocument();
      });
    });

    it('shows PS3 limitation in help text', async () => {
      const user = userEvent.setup({ delay: null });

      render(
        <PSNConnectionCard
          isConfigured={false}
          tokenExpired={false}
          onConnectionChange={mockOnConnectionChange}
        />
      );

      const tokenHelp = screen.getByText('How do I get my NPSSO token?');
      await user.click(tokenHelp);

      await waitFor(() => {
        expect(
          screen.getByText(/PS3 games cannot be synced/)
        ).toBeInTheDocument();
      });
    });
  });
});
