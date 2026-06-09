import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, waitFor } from '@/test/test-utils';
import userEvent from '@testing-library/user-event';
import { SteamConnectionCard } from './steam-connection-card';

// Mock hooks
const mockVerifyMutateAsync = vi.fn();
const mockDisconnectMutateAsync = vi.fn();

vi.mock('@/hooks', () => ({
  useVerifySteamCredentials: vi.fn(() => ({
    mutateAsync: mockVerifyMutateAsync,
    isPending: false,
  })),
  useDisconnectSteam: vi.fn(() => ({
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

describe('SteamConnectionCard', () => {
  const mockOnConnectionChange = vi.fn();

  beforeEach(() => {
    vi.clearAllMocks();
  });

  describe('not configured state', () => {
    it('shows connection form with Steam ID and API Key inputs', () => {
      render(
        <SteamConnectionCard isConfigured={false} onConnectionChange={mockOnConnectionChange} />,
      );

      expect(screen.getByLabelText('Steam ID')).toBeInTheDocument();
      expect(screen.getByLabelText('Steam Web API Key')).toBeInTheDocument();
    });

    it('shows "Verify & Connect" button', () => {
      render(
        <SteamConnectionCard isConfigured={false} onConnectionChange={mockOnConnectionChange} />,
      );

      expect(screen.getByRole('button', { name: 'Verify & Connect' })).toBeInTheDocument();
    });
  });

  describe('configured state', () => {
    it('shows "Connected as {username}" message', () => {
      render(
        <SteamConnectionCard
          isConfigured={true}
          steamUsername="TestUser"
          onConnectionChange={mockOnConnectionChange}
        />,
      );

      expect(screen.getByText('Connected as TestUser')).toBeInTheDocument();
    });

    it('shows disconnect button', () => {
      render(
        <SteamConnectionCard
          isConfigured={true}
          steamUsername="TestUser"
          onConnectionChange={mockOnConnectionChange}
        />,
      );

      expect(screen.getByRole('button', { name: 'Disconnect' })).toBeInTheDocument();
    });

    it('does not show connection form when configured', () => {
      render(
        <SteamConnectionCard
          isConfigured={true}
          steamUsername="TestUser"
          onConnectionChange={mockOnConnectionChange}
        />,
      );

      expect(screen.queryByLabelText('Steam ID')).not.toBeInTheDocument();
      expect(screen.queryByLabelText('Steam Web API Key')).not.toBeInTheDocument();
    });
  });

  describe('form validation', () => {
    it('validates Steam ID format - shows error for too short ID', async () => {
      const user = userEvent.setup({ delay: null });

      render(
        <SteamConnectionCard isConfigured={false} onConnectionChange={mockOnConnectionChange} />,
      );

      const steamIdInput = screen.getByLabelText('Steam ID');
      await user.type(steamIdInput, '12345');

      const submitButton = screen.getByRole('button', { name: 'Verify & Connect' });
      await user.click(submitButton);

      await waitFor(() => {
        expect(screen.getByText('Steam ID must be 17 digits')).toBeInTheDocument();
      });
    });

    it('validates Steam ID format - shows error for invalid format', async () => {
      const user = userEvent.setup({ delay: null });

      render(
        <SteamConnectionCard isConfigured={false} onConnectionChange={mockOnConnectionChange} />,
      );

      const steamIdInput = screen.getByLabelText('Steam ID');
      // Valid length but doesn't start with 7656119
      await user.type(steamIdInput, '12345678901234567');

      const apiKeyInput = screen.getByLabelText('Steam Web API Key');
      await user.type(apiKeyInput, 'ABCD1234ABCD1234ABCD1234ABCD1234');

      const submitButton = screen.getByRole('button', { name: 'Verify & Connect' });
      await user.click(submitButton);

      await waitFor(() => {
        expect(screen.getByText('Invalid Steam ID format')).toBeInTheDocument();
      });
    });

    it('validates API key format - shows error for too short key', async () => {
      const user = userEvent.setup({ delay: null });

      render(
        <SteamConnectionCard isConfigured={false} onConnectionChange={mockOnConnectionChange} />,
      );

      const steamIdInput = screen.getByLabelText('Steam ID');
      await user.type(steamIdInput, '76561198012345678');

      const apiKeyInput = screen.getByLabelText('Steam Web API Key');
      await user.type(apiKeyInput, 'ABCD1234');

      const submitButton = screen.getByRole('button', { name: 'Verify & Connect' });
      await user.click(submitButton);

      await waitFor(() => {
        expect(screen.getByText('API key must be 32 characters')).toBeInTheDocument();
      });
    });

    it('validates API key format - shows error for invalid hex characters', async () => {
      const user = userEvent.setup({ delay: null });

      render(
        <SteamConnectionCard isConfigured={false} onConnectionChange={mockOnConnectionChange} />,
      );

      const steamIdInput = screen.getByLabelText('Steam ID');
      await user.type(steamIdInput, '76561198012345678');

      const apiKeyInput = screen.getByLabelText('Steam Web API Key');
      // Contains invalid characters (G, H, etc. are not hex)
      await user.type(apiKeyInput, 'GHIJ1234GHIJ1234GHIJ1234GHIJ1234');

      const submitButton = screen.getByRole('button', { name: 'Verify & Connect' });
      await user.click(submitButton);

      await waitFor(() => {
        expect(screen.getByText('Invalid API key format')).toBeInTheDocument();
      });
    });

    // The uppercase-hex happy path is covered by 'verifies credentials and signals
    // connection change on success'; this retains the distinct lowercase-hex input.
    it('accepts a valid lowercase-hex API key format', async () => {
      const user = userEvent.setup({ delay: null });
      mockVerifyMutateAsync.mockResolvedValue({
        valid: true,
        steamUsername: 'TestUser',
      });

      render(
        <SteamConnectionCard isConfigured={false} onConnectionChange={mockOnConnectionChange} />,
      );

      const steamIdInput = screen.getByLabelText('Steam ID');
      await user.type(steamIdInput, '76561198012345678');

      const apiKeyInput = screen.getByLabelText('Steam Web API Key');
      await user.type(apiKeyInput, 'abcdef1234567890abcdef1234567890');

      const submitButton = screen.getByRole('button', { name: 'Verify & Connect' });
      await user.click(submitButton);

      await waitFor(() => {
        expect(mockVerifyMutateAsync).toHaveBeenCalledWith({
          steamId: '76561198012345678',
          webApiKey: 'abcdef1234567890abcdef1234567890',
        });
      });
    });
  });

  describe('form submission', () => {
    it('verifies credentials and signals connection change on success', async () => {
      const user = userEvent.setup({ delay: null });
      mockVerifyMutateAsync.mockResolvedValue({
        valid: true,
        steamUsername: 'TestUser',
      });

      render(
        <SteamConnectionCard isConfigured={false} onConnectionChange={mockOnConnectionChange} />,
      );

      const steamIdInput = screen.getByLabelText('Steam ID');
      await user.type(steamIdInput, '76561198012345678');

      const apiKeyInput = screen.getByLabelText('Steam Web API Key');
      await user.type(apiKeyInput, 'ABCD1234ABCD1234ABCD1234ABCD1234');

      const submitButton = screen.getByRole('button', { name: 'Verify & Connect' });
      await user.click(submitButton);

      await waitFor(() => {
        expect(mockVerifyMutateAsync).toHaveBeenCalledWith({
          steamId: '76561198012345678',
          webApiKey: 'ABCD1234ABCD1234ABCD1234ABCD1234',
        });
        expect(mockOnConnectionChange).toHaveBeenCalled();
      });
    });

    it.each(['invalid_steam_id', 'invalid_api_key'])(
      'does not signal connection change when verification fails with %s',
      async (error) => {
        const user = userEvent.setup({ delay: null });
        mockVerifyMutateAsync.mockResolvedValue({ valid: false, error });

        render(
          <SteamConnectionCard isConfigured={false} onConnectionChange={mockOnConnectionChange} />,
        );

        const steamIdInput = screen.getByLabelText('Steam ID');
        await user.type(steamIdInput, '76561198012345678');

        const apiKeyInput = screen.getByLabelText('Steam Web API Key');
        await user.type(apiKeyInput, 'ABCD1234ABCD1234ABCD1234ABCD1234');

        const submitButton = screen.getByRole('button', { name: 'Verify & Connect' });
        await user.click(submitButton);

        await waitFor(() => {
          expect(mockOnConnectionChange).not.toHaveBeenCalled();
        });
      },
    );
  });

  describe('disconnect functionality', () => {
    it('shows disconnect confirmation dialog when button clicked', async () => {
      const user = userEvent.setup({ delay: null });

      render(
        <SteamConnectionCard
          isConfigured={true}
          steamUsername="TestUser"
          onConnectionChange={mockOnConnectionChange}
        />,
      );

      const disconnectButton = screen.getByRole('button', { name: 'Disconnect' });
      await user.click(disconnectButton);

      expect(screen.getByText('Disconnect Steam?')).toBeInTheDocument();
      expect(
        screen.getByText(
          'Your sync settings will be preserved but syncing will stop until you reconnect.',
        ),
      ).toBeInTheDocument();
    });

    it('calls disconnect mutation when confirmed', async () => {
      const user = userEvent.setup({ delay: null });
      mockDisconnectMutateAsync.mockResolvedValue(undefined);

      render(
        <SteamConnectionCard
          isConfigured={true}
          steamUsername="TestUser"
          onConnectionChange={mockOnConnectionChange}
        />,
      );

      const disconnectButton = screen.getByRole('button', { name: 'Disconnect' });
      await user.click(disconnectButton);

      // Click confirm in the dialog - find the one inside alertdialog
      // There should be two buttons with Disconnect - one in the trigger, one in the dialog
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
        <SteamConnectionCard
          isConfigured={true}
          steamUsername="TestUser"
          onConnectionChange={mockOnConnectionChange}
        />,
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

  describe('help accordions', () => {
    it('expands Steam ID help accordion when clicked', async () => {
      const user = userEvent.setup({ delay: null });

      render(
        <SteamConnectionCard isConfigured={false} onConnectionChange={mockOnConnectionChange} />,
      );

      const steamIdHelp = screen.getByText('How do I find my Steam ID?');
      await user.click(steamIdHelp);

      await waitFor(() => {
        expect(screen.getByText(/Your Steam ID is a 17-digit number/)).toBeInTheDocument();
      });
    });

    it('expands API key help accordion when clicked', async () => {
      const user = userEvent.setup({ delay: null });

      render(
        <SteamConnectionCard isConfigured={false} onConnectionChange={mockOnConnectionChange} />,
      );

      const apiKeyHelp = screen.getByText('How do I get an API key?');
      await user.click(apiKeyHelp);

      await waitFor(() => {
        expect(
          screen.getByText(/A Steam Web API key allows Nexorious to read your game library/),
        ).toBeInTheDocument();
      });
    });
  });
});
