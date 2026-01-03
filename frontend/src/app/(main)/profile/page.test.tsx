import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { render, screen, waitFor, act } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import ProfilePage from './page';

// Mock useAuth hook
const mockLogout = vi.fn();
const mockUseAuth = vi.fn();

vi.mock('@/providers', () => ({
  useAuth: () => mockUseAuth(),
}));

// Mock next/navigation
const mockPush = vi.fn();
const mockRouter = {
  push: mockPush,
  replace: vi.fn(),
  prefetch: vi.fn(),
};

vi.mock('next/navigation', () => ({
  useRouter: () => mockRouter,
}));

// Mock sonner toast - use hoisted mock functions
vi.mock('sonner', () => ({
  toast: {
    success: vi.fn(),
    error: vi.fn(),
  },
}));

// Import the mocked toast after vi.mock is set up
import { toast } from 'sonner';

// Mock auth API
const mockChangeUsername = vi.fn();
const mockChangePassword = vi.fn();
const mockCheckUsernameAvailability = vi.fn();

vi.mock('@/api/auth', () => ({
  changeUsername: (...args: unknown[]) => mockChangeUsername(...args),
  changePassword: (...args: unknown[]) => mockChangePassword(...args),
  checkUsernameAvailability: (...args: unknown[]) => mockCheckUsernameAvailability(...args),
}));

const mockUser = {
  id: '1',
  username: 'testuser',
  isAdmin: false,
  preferences: {},
};

describe('ProfilePage', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockUseAuth.mockReturnValue({
      user: mockUser,
      logout: mockLogout,
    });
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  describe('loading state', () => {
    it('shows skeleton when user is not loaded', () => {
      mockUseAuth.mockReturnValue({
        user: null,
        logout: mockLogout,
      });

      render(<ProfilePage />);

      // Check for skeleton elements (they use animate-pulse class from Skeleton component)
      const skeletons = document.querySelectorAll('[class*="animate-pulse"]');
      expect(skeletons.length).toBeGreaterThan(0);
    });
  });

  describe('page rendering', () => {
    it('renders the profile page with header', () => {
      render(<ProfilePage />);

      expect(screen.getByText('Profile Settings')).toBeInTheDocument();
      expect(
        screen.getByText('Manage your account information and security settings')
      ).toBeInTheDocument();
    });

    it('renders account information section', () => {
      render(<ProfilePage />);

      expect(screen.getByText('Account Information')).toBeInTheDocument();
      expect(screen.getByText('Update your username')).toBeInTheDocument();
    });

    it('renders password & security section', () => {
      render(<ProfilePage />);

      expect(screen.getByText('Password & Security')).toBeInTheDocument();
      expect(screen.getByText('Change your password')).toBeInTheDocument();
    });

    it('displays current username', () => {
      render(<ProfilePage />);

      expect(screen.getByText('testuser')).toBeInTheDocument();
    });

    it('initializes new username field with current username', () => {
      render(<ProfilePage />);

      const newUsernameInput = screen.getByLabelText('New Username');
      expect(newUsernameInput).toHaveValue('testuser');
    });

    it('renders requirements info box on desktop', () => {
      render(<ProfilePage />);

      expect(screen.getByText('Username Requirements')).toBeInTheDocument();
      expect(screen.getByText('Password Requirements')).toBeInTheDocument();
    });
  });

  describe('username management', () => {
    it('disables update username button when username unchanged', () => {
      render(<ProfilePage />);

      const updateButton = screen.getByRole('button', { name: 'Update Username' });
      expect(updateButton).toBeDisabled();
    });

    it('checks username availability after typing', async () => {
      const user = userEvent.setup();
      mockCheckUsernameAvailability.mockResolvedValue({ available: true, username: 'newuser' });

      render(<ProfilePage />);

      const usernameInput = screen.getByLabelText('New Username');
      await user.clear(usernameInput);
      await user.type(usernameInput, 'newuser');

      await waitFor(() => {
        expect(mockCheckUsernameAvailability).toHaveBeenCalledWith('newuser');
      });
    });

    it('shows availability status when username is available', async () => {
      const user = userEvent.setup();
      mockCheckUsernameAvailability.mockResolvedValue({ available: true, username: 'newuser' });

      render(<ProfilePage />);

      const usernameInput = screen.getByLabelText('New Username');
      await user.clear(usernameInput);
      await user.type(usernameInput, 'newuser');

      await waitFor(() => {
        expect(screen.getByText('Username is available')).toBeInTheDocument();
      });
    });

    it('shows error when username is taken', async () => {
      const user = userEvent.setup();
      mockCheckUsernameAvailability.mockResolvedValue({ available: false, username: 'takenuser' });

      render(<ProfilePage />);

      const usernameInput = screen.getByLabelText('New Username');
      await user.clear(usernameInput);
      await user.type(usernameInput, 'takenuser');

      await waitFor(() => {
        expect(screen.getByText('Username is already taken')).toBeInTheDocument();
      });
    });

    it('shows error when username is too short', async () => {
      const user = userEvent.setup();

      render(<ProfilePage />);

      const usernameInput = screen.getByLabelText('New Username');
      await user.clear(usernameInput);
      await user.type(usernameInput, 'ab');

      await waitFor(() => {
        expect(screen.getByText('Username must be at least 3 characters')).toBeInTheDocument();
      });
    });

    it('enables update button when username is available', async () => {
      const user = userEvent.setup();
      mockCheckUsernameAvailability.mockResolvedValue({ available: true, username: 'newuser' });

      render(<ProfilePage />);

      const usernameInput = screen.getByLabelText('New Username');
      await user.clear(usernameInput);
      await user.type(usernameInput, 'newuser');

      await waitFor(() => {
        const updateButton = screen.getByRole('button', { name: 'Update Username' });
        expect(updateButton).toBeEnabled();
      });
    });

    it('calls changeUsername API on submit', async () => {
      const user = userEvent.setup();
      mockCheckUsernameAvailability.mockResolvedValue({ available: true, username: 'newuser' });
      mockChangeUsername.mockResolvedValue({ ...mockUser, username: 'newuser' });

      render(<ProfilePage />);

      const usernameInput = screen.getByLabelText('New Username');
      await user.clear(usernameInput);
      await user.type(usernameInput, 'newuser');

      await waitFor(() => {
        expect(screen.getByRole('button', { name: 'Update Username' })).toBeEnabled();
      });

      await user.click(screen.getByRole('button', { name: 'Update Username' }));

      await waitFor(() => {
        expect(mockChangeUsername).toHaveBeenCalledWith('newuser');
      });
    });

    it('shows success toast and logs out after username change', async () => {
      vi.useFakeTimers({ shouldAdvanceTime: true });
      const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
      mockCheckUsernameAvailability.mockResolvedValue({ available: true, username: 'newuser' });
      mockChangeUsername.mockResolvedValue({ ...mockUser, username: 'newuser' });

      render(<ProfilePage />);

      const usernameInput = screen.getByLabelText('New Username');
      await user.clear(usernameInput);
      await user.type(usernameInput, 'newuser');

      await waitFor(() => {
        expect(screen.getByRole('button', { name: 'Update Username' })).toBeEnabled();
      });

      await user.click(screen.getByRole('button', { name: 'Update Username' }));

      await waitFor(() => {
        expect(toast.success).toHaveBeenCalledWith(
          'Username updated successfully! Please log in again.'
        );
      });

      await vi.advanceTimersByTimeAsync(1500);

      expect(mockLogout).toHaveBeenCalled();
      vi.useRealTimers();
    });

    it('shows error toast on username change failure', async () => {
      const user = userEvent.setup();
      mockCheckUsernameAvailability.mockResolvedValue({ available: true, username: 'newuser' });
      mockChangeUsername.mockRejectedValue(new Error('Server error'));

      render(<ProfilePage />);

      const usernameInput = screen.getByLabelText('New Username');
      await user.clear(usernameInput);
      await user.type(usernameInput, 'newuser');

      await waitFor(() => {
        expect(screen.getByRole('button', { name: 'Update Username' })).toBeEnabled();
      });

      await user.click(screen.getByRole('button', { name: 'Update Username' }));

      await waitFor(() => {
        expect(toast.error).toHaveBeenCalledWith('Server error');
      });
    });
  });

  describe('password management', () => {
    it('renders all password fields', () => {
      render(<ProfilePage />);

      expect(screen.getByLabelText('Current Password')).toBeInTheDocument();
      expect(screen.getByLabelText('New Password')).toBeInTheDocument();
      expect(screen.getByLabelText('Confirm New Password')).toBeInTheDocument();
    });

    it('disables change password button when fields are empty', () => {
      render(<ProfilePage />);

      const changeButton = screen.getByRole('button', { name: 'Change Password' });
      expect(changeButton).toBeDisabled();
    });

    it('toggles password visibility for current password', async () => {
      const user = userEvent.setup();
      render(<ProfilePage />);

      const currentPasswordInput = screen.getByLabelText('Current Password');
      expect(currentPasswordInput).toHaveAttribute('type', 'password');

      const toggleButton = screen.getByRole('button', {
        name: 'Show current password',
      });
      await user.click(toggleButton);

      expect(currentPasswordInput).toHaveAttribute('type', 'text');
    });

    it('toggles password visibility for new password', async () => {
      const user = userEvent.setup();
      render(<ProfilePage />);

      const newPasswordInput = screen.getByLabelText('New Password');
      expect(newPasswordInput).toHaveAttribute('type', 'password');

      const toggleButton = screen.getByRole('button', { name: 'Show new password' });
      await user.click(toggleButton);

      expect(newPasswordInput).toHaveAttribute('type', 'text');
    });

    it('toggles password visibility for confirm password', async () => {
      const user = userEvent.setup();
      render(<ProfilePage />);

      const confirmPasswordInput = screen.getByLabelText('Confirm New Password');
      expect(confirmPasswordInput).toHaveAttribute('type', 'password');

      const toggleButton = screen.getByRole('button', {
        name: 'Show confirm password',
      });
      await user.click(toggleButton);

      expect(confirmPasswordInput).toHaveAttribute('type', 'text');
    });

    it('shows password strength meter when typing new password', async () => {
      const user = userEvent.setup();
      render(<ProfilePage />);

      const newPasswordInput = screen.getByLabelText('New Password');
      await user.type(newPasswordInput, 'weak');

      expect(screen.getByText('Weak')).toBeInTheDocument();
    });

    it('shows medium password strength', async () => {
      const user = userEvent.setup();
      render(<ProfilePage />);

      const newPasswordInput = screen.getByLabelText('New Password');
      await user.type(newPasswordInput, 'Password1');

      expect(screen.getByText('Medium')).toBeInTheDocument();
    });

    it('shows strong password strength', async () => {
      const user = userEvent.setup();
      render(<ProfilePage />);

      const newPasswordInput = screen.getByLabelText('New Password');
      await user.type(newPasswordInput, 'StrongP@ss123');

      expect(screen.getByText('Strong')).toBeInTheDocument();
    });

    it('shows passwords do not match error', async () => {
      const user = userEvent.setup();
      render(<ProfilePage />);

      await user.type(screen.getByLabelText('New Password'), 'password123');
      await user.type(screen.getByLabelText('Confirm New Password'), 'different');

      expect(screen.getByText('Passwords do not match')).toBeInTheDocument();
    });

    it('enables change password button when form is valid', async () => {
      const user = userEvent.setup();
      render(<ProfilePage />);

      await user.type(screen.getByLabelText('Current Password'), 'oldpassword');
      await user.type(screen.getByLabelText('New Password'), 'newpassword123');
      await user.type(screen.getByLabelText('Confirm New Password'), 'newpassword123');

      const changeButton = screen.getByRole('button', { name: 'Change Password' });
      expect(changeButton).toBeEnabled();
    });

    it('keeps button disabled when passwords are identical', async () => {
      const user = userEvent.setup();
      render(<ProfilePage />);

      await user.type(screen.getByLabelText('Current Password'), 'samepassword');
      await user.type(screen.getByLabelText('New Password'), 'samepassword');
      await user.type(screen.getByLabelText('Confirm New Password'), 'samepassword');

      // Button should stay disabled because current === new password
      const changeButton = screen.getByRole('button', { name: 'Change Password' });
      expect(changeButton).toBeDisabled();
    });

    it('keeps button disabled when new password is too short', async () => {
      const user = userEvent.setup();
      render(<ProfilePage />);

      await user.type(screen.getByLabelText('Current Password'), 'oldpassword');
      await user.type(screen.getByLabelText('New Password'), 'short');
      await user.type(screen.getByLabelText('Confirm New Password'), 'short');

      // Button should stay disabled because password is too short
      const changeButton = screen.getByRole('button', { name: 'Change Password' });
      expect(changeButton).toBeDisabled();
    });

    it('calls changePassword API on valid submit', async () => {
      const user = userEvent.setup();
      mockChangePassword.mockResolvedValue(undefined);

      render(<ProfilePage />);

      await user.type(screen.getByLabelText('Current Password'), 'oldpassword');
      await user.type(screen.getByLabelText('New Password'), 'newpassword123');
      await user.type(screen.getByLabelText('Confirm New Password'), 'newpassword123');
      await user.click(screen.getByRole('button', { name: 'Change Password' }));

      await waitFor(() => {
        expect(mockChangePassword).toHaveBeenCalledWith('oldpassword', 'newpassword123');
      });
    });

    it('shows success toast and logs out after password change', async () => {
      vi.useFakeTimers({ shouldAdvanceTime: true });
      const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
      mockChangePassword.mockResolvedValue(undefined);

      render(<ProfilePage />);

      await user.type(screen.getByLabelText('Current Password'), 'oldpassword');
      await user.type(screen.getByLabelText('New Password'), 'newpassword123');
      await user.type(screen.getByLabelText('Confirm New Password'), 'newpassword123');
      await user.click(screen.getByRole('button', { name: 'Change Password' }));

      await waitFor(() => {
        expect(toast.success).toHaveBeenCalledWith(
          'Password changed successfully! Please log in again.'
        );
      });

      await act(async () => {
        await vi.advanceTimersByTimeAsync(1500);
      });

      expect(mockLogout).toHaveBeenCalled();
      vi.useRealTimers();
    });

    it('shows error on password change failure', async () => {
      const user = userEvent.setup();
      mockChangePassword.mockRejectedValue(new Error('Current password is incorrect'));

      render(<ProfilePage />);

      await user.type(screen.getByLabelText('Current Password'), 'wrongpassword');
      await user.type(screen.getByLabelText('New Password'), 'newpassword123');
      await user.type(screen.getByLabelText('Confirm New Password'), 'newpassword123');
      await user.click(screen.getByRole('button', { name: 'Change Password' }));

      await waitFor(() => {
        expect(screen.getByText('Current password is incorrect')).toBeInTheDocument();
      });
    });

    it('resets password form on cancel', async () => {
      const user = userEvent.setup();
      render(<ProfilePage />);

      await user.type(screen.getByLabelText('Current Password'), 'oldpassword');
      await user.type(screen.getByLabelText('New Password'), 'newpassword123');
      await user.type(screen.getByLabelText('Confirm New Password'), 'newpassword123');

      await user.click(screen.getByRole('button', { name: 'Cancel' }));

      expect(screen.getByLabelText('Current Password')).toHaveValue('');
      expect(screen.getByLabelText('New Password')).toHaveValue('');
      expect(screen.getByLabelText('Confirm New Password')).toHaveValue('');
    });
  });

  describe('accessibility', () => {
    it('has accessible labels for all form fields', () => {
      render(<ProfilePage />);

      expect(screen.getByLabelText('New Username')).toBeInTheDocument();
      expect(screen.getByLabelText('Current Password')).toBeInTheDocument();
      expect(screen.getByLabelText('New Password')).toBeInTheDocument();
      expect(screen.getByLabelText('Confirm New Password')).toBeInTheDocument();
    });

    it('has accessible aria labels for password toggle buttons', () => {
      render(<ProfilePage />);

      expect(screen.getByRole('button', { name: 'Show current password' })).toBeInTheDocument();
      expect(screen.getByRole('button', { name: 'Show new password' })).toBeInTheDocument();
      expect(screen.getByRole('button', { name: 'Show confirm password' })).toBeInTheDocument();
    });
  });
});
