import { describe, it, expect, beforeEach, afterEach } from 'vitest';
import { cleanup, screen, waitFor } from '@testing-library/svelte';
import SetupPage from './+page.svelte';
import {
  renderComponent,
  createUserEvent,
  fillFormField,
  testAccessibility
} from '../../test-utils/test-helpers';
import {
  mockAuthStore,
  resetAuthMocks
} from '../../test-utils/auth-mocks';
import { mockGoto } from '../../test-utils/navigation-mocks';

describe('Setup Page', () => {
  const userEvent = createUserEvent();

  beforeEach(() => {
    resetAuthMocks();
    mockGoto.mockClear();
    // Mock checkSetupStatus to return needs_setup: true by default
    mockAuthStore.checkSetupStatus.mockResolvedValue({ needs_setup: true });
  });

  afterEach(() => {
    cleanup();
  });

  describe('Basic Rendering', () => {
    it('should render the setup form when setup is needed', async () => {
      renderComponent(SetupPage);

      await waitFor(() => {
        expect(screen.getByText('Welcome to Nexorious')).toBeInTheDocument();
        expect(screen.getByText("Let's set up your admin account to get started")).toBeInTheDocument();
        expect(screen.getByLabelText('Admin Username')).toBeInTheDocument();
        expect(screen.getByLabelText('Password')).toBeInTheDocument();
        expect(screen.getByLabelText('Confirm Password')).toBeInTheDocument();
        expect(screen.getByRole('button', { name: 'Create Admin Account' })).toBeInTheDocument();
      });
    });

    it('should set the correct page title', () => {
      renderComponent(SetupPage);

      const titleElements = document.querySelectorAll('title');
      const hasSetupTitle = Array.from(titleElements).some(
        title => title.textContent === 'Initial Setup - Nexorious'
      );
      expect(hasSetupTitle).toBe(true);
    });

    it('should redirect to login if setup is not needed', async () => {
      mockAuthStore.checkSetupStatus.mockResolvedValue({ needs_setup: false });
      renderComponent(SetupPage);

      await waitFor(() => {
        expect(mockGoto).toHaveBeenCalledWith('/login');
      });
    });

    it('should have proper form structure', async () => {
      const { container } = renderComponent(SetupPage);

      await waitFor(() => {
        const form = container.querySelector('form');
        expect(form).toBeInTheDocument();
        
        const usernameInput = screen.getByLabelText('Admin Username');
        const passwordInput = screen.getByLabelText('Password');
        const confirmPasswordInput = screen.getByLabelText('Confirm Password');
        
        expect(usernameInput).toHaveAttribute('type', 'text');
        expect(usernameInput).toHaveAttribute('required');
        expect(usernameInput).toHaveAttribute('minlength', '3');
        expect(passwordInput).toHaveAttribute('type', 'password');
        expect(passwordInput).toHaveAttribute('required');
        expect(passwordInput).toHaveAttribute('minlength', '8');
        expect(confirmPasswordInput).toHaveAttribute('type', 'password');
        expect(confirmPasswordInput).toHaveAttribute('required');
      });
    });
  });

  describe('Form Validation', () => {
    it('should show error for empty fields', async () => {
      renderComponent(SetupPage);

      await waitFor(() => {
        const submitButton = screen.getByRole('button', { name: 'Create Admin Account' });
        userEvent.click(submitButton);
      });

      await waitFor(() => {
        expect(screen.getByText('Please fill in all fields')).toBeInTheDocument();
      });
    });

    it('should show error for username too short', async () => {
      renderComponent(SetupPage);

      await waitFor(async () => {
        const usernameInput = screen.getByLabelText('Admin Username') as HTMLInputElement;
        const passwordInput = screen.getByLabelText('Password') as HTMLInputElement;
        const confirmPasswordInput = screen.getByLabelText('Confirm Password') as HTMLInputElement;

        await fillFormField(usernameInput, 'ab', userEvent);
        await fillFormField(passwordInput, 'password123', userEvent);
        await fillFormField(confirmPasswordInput, 'password123', userEvent);

        const submitButton = screen.getByRole('button', { name: 'Create Admin Account' });
        await userEvent.click(submitButton);
      });

      await waitFor(() => {
        expect(screen.getByText('Username must be at least 3 characters long')).toBeInTheDocument();
      });
    });

    it('should show error for password too short', async () => {
      renderComponent(SetupPage);

      await waitFor(async () => {
        const usernameInput = screen.getByLabelText('Admin Username') as HTMLInputElement;
        const passwordInput = screen.getByLabelText('Password') as HTMLInputElement;
        const confirmPasswordInput = screen.getByLabelText('Confirm Password') as HTMLInputElement;

        await fillFormField(usernameInput, 'admin', userEvent);
        await fillFormField(passwordInput, 'short', userEvent);
        await fillFormField(confirmPasswordInput, 'short', userEvent);

        const submitButton = screen.getByRole('button', { name: 'Create Admin Account' });
        await userEvent.click(submitButton);
      });

      await waitFor(() => {
        expect(screen.getByText('Password must be at least 8 characters long')).toBeInTheDocument();
      });
    });

    it('should show error for password mismatch', async () => {
      renderComponent(SetupPage);

      await waitFor(async () => {
        const usernameInput = screen.getByLabelText('Admin Username') as HTMLInputElement;
        const passwordInput = screen.getByLabelText('Password') as HTMLInputElement;
        const confirmPasswordInput = screen.getByLabelText('Confirm Password') as HTMLInputElement;

        await fillFormField(usernameInput, 'admin', userEvent);
        await fillFormField(passwordInput, 'password123', userEvent);
        await fillFormField(confirmPasswordInput, 'password456', userEvent);

        const submitButton = screen.getByRole('button', { name: 'Create Admin Account' });
        await userEvent.click(submitButton);
      });

      await waitFor(() => {
        expect(screen.getByText('Passwords do not match')).toBeInTheDocument();
      });
    });
  });

  describe('Form Submission', () => {
    it('should call createInitialAdmin with correct credentials', async () => {
      mockAuthStore.createInitialAdmin.mockResolvedValue(undefined);
      renderComponent(SetupPage);

      await waitFor(async () => {
        const usernameInput = screen.getByLabelText('Admin Username') as HTMLInputElement;
        const passwordInput = screen.getByLabelText('Password') as HTMLInputElement;
        const confirmPasswordInput = screen.getByLabelText('Confirm Password') as HTMLInputElement;

        await fillFormField(usernameInput, 'admin', userEvent);
        await fillFormField(passwordInput, 'password123', userEvent);
        await fillFormField(confirmPasswordInput, 'password123', userEvent);

        const submitButton = screen.getByRole('button', { name: 'Create Admin Account' });
        await userEvent.click(submitButton);
      });

      await waitFor(() => {
        expect(mockAuthStore.createInitialAdmin).toHaveBeenCalledWith('admin', 'password123');
      });
    });

    it('should redirect to login page on successful setup', async () => {
      mockAuthStore.createInitialAdmin.mockResolvedValue(undefined);
      renderComponent(SetupPage);

      await waitFor(async () => {
        const usernameInput = screen.getByLabelText('Admin Username') as HTMLInputElement;
        const passwordInput = screen.getByLabelText('Password') as HTMLInputElement;
        const confirmPasswordInput = screen.getByLabelText('Confirm Password') as HTMLInputElement;

        await fillFormField(usernameInput, 'admin', userEvent);
        await fillFormField(passwordInput, 'password123', userEvent);
        await fillFormField(confirmPasswordInput, 'password123', userEvent);

        const submitButton = screen.getByRole('button', { name: 'Create Admin Account' });
        await userEvent.click(submitButton);
      });

      await waitFor(() => {
        expect(mockGoto).toHaveBeenCalledWith('/login');
      });
    });

    it('should show loading state during setup', async () => {
      // Mock createInitialAdmin to never resolve to test loading state
      mockAuthStore.createInitialAdmin.mockImplementation(() => new Promise(() => {}));
      renderComponent(SetupPage);

      await waitFor(async () => {
        const usernameInput = screen.getByLabelText('Admin Username') as HTMLInputElement;
        const passwordInput = screen.getByLabelText('Password') as HTMLInputElement;
        const confirmPasswordInput = screen.getByLabelText('Confirm Password') as HTMLInputElement;

        await fillFormField(usernameInput, 'admin', userEvent);
        await fillFormField(passwordInput, 'password123', userEvent);
        await fillFormField(confirmPasswordInput, 'password123', userEvent);

        const submitButton = screen.getByRole('button', { name: 'Create Admin Account' });
        await userEvent.click(submitButton);
      });

      await waitFor(() => {
        expect(screen.getByText('Creating Admin Account...')).toBeInTheDocument();
        const submitButton = screen.getByRole('button', { name: /Creating Admin Account/i });
        expect(submitButton).toBeDisabled();
      });
    });

    it('should show error on setup failure', async () => {
      mockAuthStore.createInitialAdmin.mockRejectedValue(new Error('Setup failed'));
      renderComponent(SetupPage);

      await waitFor(async () => {
        const usernameInput = screen.getByLabelText('Admin Username') as HTMLInputElement;
        const passwordInput = screen.getByLabelText('Password') as HTMLInputElement;
        const confirmPasswordInput = screen.getByLabelText('Confirm Password') as HTMLInputElement;

        await fillFormField(usernameInput, 'admin', userEvent);
        await fillFormField(passwordInput, 'password123', userEvent);
        await fillFormField(confirmPasswordInput, 'password123', userEvent);

        const submitButton = screen.getByRole('button', { name: 'Create Admin Account' });
        await userEvent.click(submitButton);
      });

      await waitFor(() => {
        expect(screen.getByText('Setup failed')).toBeInTheDocument();
      });
    });

    it('should handle generic setup errors', async () => {
      mockAuthStore.createInitialAdmin.mockRejectedValue('Something went wrong');
      renderComponent(SetupPage);

      await waitFor(async () => {
        const usernameInput = screen.getByLabelText('Admin Username') as HTMLInputElement;
        const passwordInput = screen.getByLabelText('Password') as HTMLInputElement;
        const confirmPasswordInput = screen.getByLabelText('Confirm Password') as HTMLInputElement;

        await fillFormField(usernameInput, 'admin', userEvent);
        await fillFormField(passwordInput, 'password123', userEvent);
        await fillFormField(confirmPasswordInput, 'password123', userEvent);

        const submitButton = screen.getByRole('button', { name: 'Create Admin Account' });
        await userEvent.click(submitButton);
      });

      await waitFor(() => {
        expect(screen.getByText('Setup failed')).toBeInTheDocument();
      });
    });
  });

  describe('Keyboard Navigation', () => {
    it('should submit form when Enter is pressed in username field', async () => {
      mockAuthStore.createInitialAdmin.mockResolvedValue(undefined);
      renderComponent(SetupPage);

      await waitFor(async () => {
        const usernameInput = screen.getByLabelText('Admin Username') as HTMLInputElement;
        const passwordInput = screen.getByLabelText('Password') as HTMLInputElement;
        const confirmPasswordInput = screen.getByLabelText('Confirm Password') as HTMLInputElement;

        await fillFormField(usernameInput, 'admin', userEvent);
        await fillFormField(passwordInput, 'password123', userEvent);
        await fillFormField(confirmPasswordInput, 'password123', userEvent);

        await userEvent.keyDown(usernameInput, 'Enter');
      });

      await waitFor(() => {
        expect(mockAuthStore.createInitialAdmin).toHaveBeenCalledWith('admin', 'password123');
      });
    });

    it('should submit form when Enter is pressed in password field', async () => {
      mockAuthStore.createInitialAdmin.mockResolvedValue(undefined);
      renderComponent(SetupPage);

      await waitFor(async () => {
        const usernameInput = screen.getByLabelText('Admin Username') as HTMLInputElement;
        const passwordInput = screen.getByLabelText('Password') as HTMLInputElement;
        const confirmPasswordInput = screen.getByLabelText('Confirm Password') as HTMLInputElement;

        await fillFormField(usernameInput, 'admin', userEvent);
        await fillFormField(passwordInput, 'password123', userEvent);
        await fillFormField(confirmPasswordInput, 'password123', userEvent);

        await userEvent.keyDown(passwordInput, 'Enter');
      });

      await waitFor(() => {
        expect(mockAuthStore.createInitialAdmin).toHaveBeenCalledWith('admin', 'password123');
      });
    });

    it('should submit form when Enter is pressed in confirm password field', async () => {
      mockAuthStore.createInitialAdmin.mockResolvedValue(undefined);
      renderComponent(SetupPage);

      await waitFor(async () => {
        const usernameInput = screen.getByLabelText('Admin Username') as HTMLInputElement;
        const passwordInput = screen.getByLabelText('Password') as HTMLInputElement;
        const confirmPasswordInput = screen.getByLabelText('Confirm Password') as HTMLInputElement;

        await fillFormField(usernameInput, 'admin', userEvent);
        await fillFormField(passwordInput, 'password123', userEvent);
        await fillFormField(confirmPasswordInput, 'password123', userEvent);

        await userEvent.keyDown(confirmPasswordInput, 'Enter');
      });

      await waitFor(() => {
        expect(mockAuthStore.createInitialAdmin).toHaveBeenCalledWith('admin', 'password123');
      });
    });
  });

  describe('Focus and Tab Order', () => {
    it('should have focus capability on username field', async () => {
      renderComponent(SetupPage);

      // Wait for the setup status check to complete and form to be displayed
      await waitFor(() => {
        expect(screen.getByLabelText('Admin Username')).toBeInTheDocument();
      });

      // Test that the username field can receive focus
      const usernameInput = screen.getByLabelText('Admin Username') as HTMLInputElement;
      usernameInput.focus();
      expect(document.activeElement).toBe(usernameInput);
    });

    it('should support proper tab navigation', async () => {
      renderComponent(SetupPage);
      
      // Wait for the setup status check to complete and form to be displayed
      await waitFor(() => {
        expect(screen.getByLabelText('Admin Username')).toBeInTheDocument();
      });

      // Get form elements
      const usernameInput = screen.getByLabelText('Admin Username') as HTMLInputElement;
      const passwordInput = screen.getByLabelText('Password') as HTMLInputElement;
      const confirmPasswordInput = screen.getByLabelText('Confirm Password') as HTMLInputElement;
      const submitButton = screen.getByRole('button', { name: 'Create Admin Account' });

      // Start with username focused
      usernameInput.focus();
      expect(document.activeElement).toBe(usernameInput);

      // Tab to password field
      await userEvent.keyDown(usernameInput, 'Tab');
      expect(document.activeElement).toBe(passwordInput);

      // Tab to confirm password field
      await userEvent.keyDown(passwordInput, 'Tab');
      expect(document.activeElement).toBe(confirmPasswordInput);

      // Tab to submit button
      await userEvent.keyDown(confirmPasswordInput, 'Tab');
      expect(document.activeElement).toBe(submitButton);
    });
  });

  describe('Accessibility', () => {
    it('should have proper accessibility attributes', async () => {
      const { container } = renderComponent(SetupPage);
      
      await waitFor(() => {
        testAccessibility(container);
      });
    });

    it('should have proper label associations', async () => {
      renderComponent(SetupPage);

      await waitFor(() => {
        const usernameInput = screen.getByLabelText('Admin Username');
        const passwordInput = screen.getByLabelText('Password');
        const confirmPasswordInput = screen.getByLabelText('Confirm Password');

        expect(usernameInput).toHaveAttribute('id', 'username');
        expect(passwordInput).toHaveAttribute('id', 'password');
        expect(confirmPasswordInput).toHaveAttribute('id', 'confirmPassword');
      });
    });

    it('should have proper placeholder text', async () => {
      renderComponent(SetupPage);

      await waitFor(() => {
        const usernameInput = screen.getByLabelText('Admin Username');
        const passwordInput = screen.getByLabelText('Password');
        const confirmPasswordInput = screen.getByLabelText('Confirm Password');

        expect(usernameInput).toHaveAttribute('placeholder', 'Choose a username');
        expect(passwordInput).toHaveAttribute('placeholder', 'Enter a secure password');
        expect(confirmPasswordInput).toHaveAttribute('placeholder', 'Confirm your password');
      });
    });
  });

  describe('Loading State Display', () => {
    it('should show loading indicator while checking setup status', () => {
      // Mock a slow setup status check
      mockAuthStore.checkSetupStatus.mockImplementation(() => new Promise(() => {}));
      renderComponent(SetupPage);

      expect(screen.getByText('Checking setup status...')).toBeInTheDocument();
      expect(screen.queryByText('Welcome to Nexorious')).not.toBeInTheDocument();
    });

    it('should show loading indicator by default before setup check completes', () => {
      // Don't mock checkSetupStatus, let it be undefined initially
      mockAuthStore.checkSetupStatus.mockImplementation(() => new Promise(() => {}));
      renderComponent(SetupPage);

      expect(screen.getByText('Checking setup status...')).toBeInTheDocument();
    });
  });
});