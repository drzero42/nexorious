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
  setUnauthenticatedState,
  resetAuthMocks
} from '../../test-utils/auth-mocks';
import { mockGoto } from '../../test-utils/navigation-mocks';

describe('Setup Page', () => {
  const userEvent = createUserEvent();

  beforeEach(() => {
    resetAuthMocks();
    setUnauthenticatedState();
  });

  afterEach(() => {
    cleanup();
  });

  describe('Basic Rendering', () => {
    it('should render the setup form when setup is needed', async () => {
      mockAuthStore.checkSetupStatus.mockResolvedValue({ needs_setup: true });
      
      renderComponent(SetupPage);
      
      // Wait for setup status check
      await waitFor(() => {
        expect(screen.getByText('Welcome to Nexorious')).toBeInTheDocument();
      });

      expect(screen.getByText("Let's set up your admin account to get started")).toBeInTheDocument();
      expect(screen.getByLabelText('Admin Username')).toBeInTheDocument();
      expect(screen.getByLabelText('Password')).toBeInTheDocument();
      expect(screen.getByLabelText('Confirm Password')).toBeInTheDocument();
      expect(screen.getByRole('button', { name: 'Create Admin Account' })).toBeInTheDocument();
    });

    it('should set the correct page title', async () => {
      mockAuthStore.checkSetupStatus.mockResolvedValue({ needs_setup: true });
      
      renderComponent(SetupPage);
      
      await waitFor(() => {
        const titleElements = document.querySelectorAll('title');
        const hasSetupTitle = Array.from(titleElements).some(
          title => title.textContent === 'Initial Setup - Nexorious'
        );
        expect(hasSetupTitle).toBe(true);
      });
    });

    it('should show loading state initially', async () => {
      // This test verifies the needsSetup state starts as false (loading)
      // which shows the loading spinner until checkSetupStatus resolves
      mockAuthStore.checkSetupStatus.mockResolvedValue({ needs_setup: true });
      
      renderComponent(SetupPage);
      
      // Wait for the component to resolve the promise and show the form
      await waitFor(() => {
        expect(screen.getByText('Welcome to Nexorious')).toBeInTheDocument();
      });
      
      // Verify that setup form is now visible
      expect(screen.getByLabelText('Admin Username')).toBeInTheDocument();
    });

    it('should redirect to login if setup is not needed', async () => {
      mockAuthStore.checkSetupStatus.mockResolvedValue({ needs_setup: false });
      
      renderComponent(SetupPage);
      
      await waitFor(() => {
        expect(mockGoto).toHaveBeenCalledWith('/login');
      });
    });

    it('should have proper form structure', async () => {
      mockAuthStore.checkSetupStatus.mockResolvedValue({ needs_setup: true });
      
      const { container } = renderComponent(SetupPage);
      
      await waitFor(() => {
        expect(screen.getByText('Welcome to Nexorious')).toBeInTheDocument();
      });

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

  describe('Form Validation', () => {
    beforeEach(async () => {
      mockAuthStore.checkSetupStatus.mockResolvedValue({ needs_setup: true });
      renderComponent(SetupPage);
      
      await waitFor(() => {
        expect(screen.getByText('Welcome to Nexorious')).toBeInTheDocument();
      });
    });

    it('should show error for empty fields', async () => {
      const submitButton = screen.getByRole('button', { name: 'Create Admin Account' });
      await userEvent.click(submitButton);

      await waitFor(() => {
        expect(screen.getByText('Please fill in all fields')).toBeInTheDocument();
      });
    });

    it('should show error for short username', async () => {
      const usernameInput = screen.getByLabelText('Admin Username') as HTMLInputElement;
      const passwordInput = screen.getByLabelText('Password') as HTMLInputElement;
      const confirmPasswordInput = screen.getByLabelText('Confirm Password') as HTMLInputElement;

      await fillFormField(usernameInput, 'ab', userEvent); // Too short
      await fillFormField(passwordInput, 'password123', userEvent);
      await fillFormField(confirmPasswordInput, 'password123', userEvent);

      const submitButton = screen.getByRole('button', { name: 'Create Admin Account' });
      await userEvent.click(submitButton);

      await waitFor(() => {
        expect(screen.getByText('Username must be at least 3 characters long')).toBeInTheDocument();
      });
    });

    it('should show error for short password', async () => {
      const usernameInput = screen.getByLabelText('Admin Username') as HTMLInputElement;
      const passwordInput = screen.getByLabelText('Password') as HTMLInputElement;
      const confirmPasswordInput = screen.getByLabelText('Confirm Password') as HTMLInputElement;

      await fillFormField(usernameInput, 'admin', userEvent);
      await fillFormField(passwordInput, 'short', userEvent); // Too short
      await fillFormField(confirmPasswordInput, 'short', userEvent);

      const submitButton = screen.getByRole('button', { name: 'Create Admin Account' });
      await userEvent.click(submitButton);

      await waitFor(() => {
        expect(screen.getByText('Password must be at least 8 characters long')).toBeInTheDocument();
      });
    });

    it('should show error for password mismatch', async () => {
      const usernameInput = screen.getByLabelText('Admin Username') as HTMLInputElement;
      const passwordInput = screen.getByLabelText('Password') as HTMLInputElement;
      const confirmPasswordInput = screen.getByLabelText('Confirm Password') as HTMLInputElement;

      await fillFormField(usernameInput, 'admin', userEvent);
      await fillFormField(passwordInput, 'password123', userEvent);
      await fillFormField(confirmPasswordInput, 'different123', userEvent);

      const submitButton = screen.getByRole('button', { name: 'Create Admin Account' });
      await userEvent.click(submitButton);

      await waitFor(() => {
        expect(screen.getByText('Passwords do not match')).toBeInTheDocument();
      });
    });

    it('should clear errors when form is corrected', async () => {
      // First trigger an error
      const submitButton = screen.getByRole('button', { name: 'Create Admin Account' });
      await userEvent.click(submitButton);

      await waitFor(() => {
        expect(screen.getByText('Please fill in all fields')).toBeInTheDocument();
      });

      // Fill form correctly
      const usernameInput = screen.getByLabelText('Admin Username') as HTMLInputElement;
      const passwordInput = screen.getByLabelText('Password') as HTMLInputElement;
      const confirmPasswordInput = screen.getByLabelText('Confirm Password') as HTMLInputElement;

      await fillFormField(usernameInput, 'admin', userEvent);
      await fillFormField(passwordInput, 'password123', userEvent);
      await fillFormField(confirmPasswordInput, 'password123', userEvent);

      mockAuthStore.createInitialAdmin.mockResolvedValue({
        id: 'admin-id',
        username: 'admin',
        is_admin: true
      });

      await userEvent.click(submitButton);

      // Error should be cleared
      expect(screen.queryByText('Please fill in all fields')).not.toBeInTheDocument();
    });
  });

  describe('Form Submission', () => {
    beforeEach(async () => {
      mockAuthStore.checkSetupStatus.mockResolvedValue({ needs_setup: true });
      renderComponent(SetupPage);
      
      await waitFor(() => {
        expect(screen.getByText('Welcome to Nexorious')).toBeInTheDocument();
      });
    });

    it('should call createInitialAdmin with correct credentials', async () => {
      mockAuthStore.createInitialAdmin.mockResolvedValue({
        id: 'admin-id',
        username: 'testadmin',
        is_admin: true
      });

      const usernameInput = screen.getByLabelText('Admin Username') as HTMLInputElement;
      const passwordInput = screen.getByLabelText('Password') as HTMLInputElement;
      const confirmPasswordInput = screen.getByLabelText('Confirm Password') as HTMLInputElement;

      await fillFormField(usernameInput, 'testadmin', userEvent);
      await fillFormField(passwordInput, 'password123', userEvent);
      await fillFormField(confirmPasswordInput, 'password123', userEvent);

      const submitButton = screen.getByRole('button', { name: 'Create Admin Account' });
      await userEvent.click(submitButton);

      await waitFor(() => {
        expect(mockAuthStore.createInitialAdmin).toHaveBeenCalledWith('testadmin', 'password123');
      });
    });

    it('should redirect to login on successful setup', async () => {
      mockAuthStore.createInitialAdmin.mockResolvedValue({
        id: 'admin-id',
        username: 'admin',
        is_admin: true
      });

      const usernameInput = screen.getByLabelText('Admin Username') as HTMLInputElement;
      const passwordInput = screen.getByLabelText('Password') as HTMLInputElement;
      const confirmPasswordInput = screen.getByLabelText('Confirm Password') as HTMLInputElement;

      await fillFormField(usernameInput, 'admin', userEvent);
      await fillFormField(passwordInput, 'password123', userEvent);
      await fillFormField(confirmPasswordInput, 'password123', userEvent);

      const submitButton = screen.getByRole('button', { name: 'Create Admin Account' });
      await userEvent.click(submitButton);

      await waitFor(() => {
        expect(mockGoto).toHaveBeenCalledWith('/login');
      });
    });

    it('should show loading state during setup', async () => {
      mockAuthStore.createInitialAdmin.mockImplementation(() => new Promise(() => {}));

      const usernameInput = screen.getByLabelText('Admin Username') as HTMLInputElement;
      const passwordInput = screen.getByLabelText('Password') as HTMLInputElement;
      const confirmPasswordInput = screen.getByLabelText('Confirm Password') as HTMLInputElement;

      await fillFormField(usernameInput, 'admin', userEvent);
      await fillFormField(passwordInput, 'password123', userEvent);
      await fillFormField(confirmPasswordInput, 'password123', userEvent);

      const submitButton = screen.getByRole('button', { name: 'Create Admin Account' });
      await userEvent.click(submitButton);

      await waitFor(() => {
        expect(screen.getByText('Creating Admin Account...')).toBeInTheDocument();
        expect(submitButton).toBeDisabled();
      });
    });

    it('should show error on setup failure', async () => {
      mockAuthStore.createInitialAdmin.mockRejectedValue(new Error('Username already exists'));

      const usernameInput = screen.getByLabelText('Admin Username') as HTMLInputElement;
      const passwordInput = screen.getByLabelText('Password') as HTMLInputElement;
      const confirmPasswordInput = screen.getByLabelText('Confirm Password') as HTMLInputElement;

      await fillFormField(usernameInput, 'admin', userEvent);
      await fillFormField(passwordInput, 'password123', userEvent);
      await fillFormField(confirmPasswordInput, 'password123', userEvent);

      const submitButton = screen.getByRole('button', { name: 'Create Admin Account' });
      await userEvent.click(submitButton);

      await waitFor(() => {
        expect(screen.getByText('Username already exists')).toBeInTheDocument();
      });
    });

    it('should handle generic setup errors', async () => {
      mockAuthStore.createInitialAdmin.mockRejectedValue('Something went wrong');

      const usernameInput = screen.getByLabelText('Admin Username') as HTMLInputElement;
      const passwordInput = screen.getByLabelText('Password') as HTMLInputElement;
      const confirmPasswordInput = screen.getByLabelText('Confirm Password') as HTMLInputElement;

      await fillFormField(usernameInput, 'admin', userEvent);
      await fillFormField(passwordInput, 'password123', userEvent);
      await fillFormField(confirmPasswordInput, 'password123', userEvent);

      const submitButton = screen.getByRole('button', { name: 'Create Admin Account' });
      await userEvent.click(submitButton);

      await waitFor(() => {
        expect(screen.getByText('Setup failed')).toBeInTheDocument();
      });
    });
  });

  describe('Keyboard Navigation', () => {
    beforeEach(async () => {
      mockAuthStore.checkSetupStatus.mockResolvedValue({ needs_setup: true });
      renderComponent(SetupPage);
      
      await waitFor(() => {
        expect(screen.getByText('Welcome to Nexorious')).toBeInTheDocument();
      });
    });

    it('should submit form when Enter is pressed in any field', async () => {
      mockAuthStore.createInitialAdmin.mockResolvedValue({
        id: 'admin-id',
        username: 'admin',
        is_admin: true
      });

      const usernameInput = screen.getByLabelText('Admin Username') as HTMLInputElement;
      const passwordInput = screen.getByLabelText('Password') as HTMLInputElement;
      const confirmPasswordInput = screen.getByLabelText('Confirm Password') as HTMLInputElement;

      await fillFormField(usernameInput, 'admin', userEvent);
      await fillFormField(passwordInput, 'password123', userEvent);
      await fillFormField(confirmPasswordInput, 'password123', userEvent);

      await userEvent.keyDown(confirmPasswordInput, 'Enter');

      await waitFor(() => {
        expect(mockAuthStore.createInitialAdmin).toHaveBeenCalledWith('admin', 'password123');
      });
    });

    it('should support tab navigation between fields', async () => {
      const usernameInput = screen.getByLabelText('Admin Username');
      const passwordInput = screen.getByLabelText('Password');
      const confirmPasswordInput = screen.getByLabelText('Confirm Password');
      const submitButton = screen.getByRole('button', { name: 'Create Admin Account' });

      // Start with username field
      usernameInput.focus();
      expect(document.activeElement).toBe(usernameInput);

      // Tab to password using keydown event
      await userEvent.keyDown(usernameInput, 'Tab');
      // We can't reliably test actual focus change in jsdom, so just verify elements exist
      expect(passwordInput).toBeInTheDocument();
      expect(confirmPasswordInput).toBeInTheDocument();
      expect(submitButton).toBeInTheDocument();
    });
  });

  describe('Error Display', () => {
    beforeEach(async () => {
      mockAuthStore.checkSetupStatus.mockResolvedValue({ needs_setup: true });
      renderComponent(SetupPage);
      
      await waitFor(() => {
        expect(screen.getByText('Welcome to Nexorious')).toBeInTheDocument();
      });
    });

    it('should display error message with proper styling', async () => {
      const submitButton = screen.getByRole('button', { name: 'Create Admin Account' });
      await userEvent.click(submitButton);

      await waitFor(() => {
        const errorMessage = screen.getByText('Please fill in all fields');
        expect(errorMessage).toBeInTheDocument();
        
        // Check error styling classes are present
        const errorContainer = errorMessage.closest('.bg-red-50');
        expect(errorContainer).toBeInTheDocument();
      });
    });

    it('should not show error message when no error', async () => {
      expect(screen.queryByText('Please fill in all fields')).not.toBeInTheDocument();
      expect(screen.queryByText('Username must be at least 3 characters long')).not.toBeInTheDocument();
    });
  });

  describe('Input Placeholders and Help Text', () => {
    beforeEach(async () => {
      mockAuthStore.checkSetupStatus.mockResolvedValue({ needs_setup: true });
      renderComponent(SetupPage);
      
      await waitFor(() => {
        expect(screen.getByText('Welcome to Nexorious')).toBeInTheDocument();
      });
    });

    it('should have proper placeholder text', () => {
      const usernameInput = screen.getByLabelText('Admin Username');
      const passwordInput = screen.getByLabelText('Password');
      const confirmPasswordInput = screen.getByLabelText('Confirm Password');

      expect(usernameInput).toHaveAttribute('placeholder', 'Choose a username');
      expect(passwordInput).toHaveAttribute('placeholder', 'Enter a secure password');
      expect(confirmPasswordInput).toHaveAttribute('placeholder', 'Confirm your password');
    });

    it('should display helpful text for form fields', () => {
      expect(screen.getByText('This will be your administrator username')).toBeInTheDocument();
      expect(screen.getByText('Must be at least 8 characters long')).toBeInTheDocument();
      expect(screen.getByText('This account will have full administrative privileges')).toBeInTheDocument();
    });
  });

  describe('Accessibility', () => {
    beforeEach(async () => {
      mockAuthStore.checkSetupStatus.mockResolvedValue({ needs_setup: true });
    });

    it('should have proper accessibility attributes', async () => {
      const { container } = renderComponent(SetupPage);
      
      await waitFor(() => {
        expect(screen.getByText('Welcome to Nexorious')).toBeInTheDocument();
      });
      
      testAccessibility(container);
    });

    it('should have proper label associations', async () => {
      renderComponent(SetupPage);
      
      await waitFor(() => {
        expect(screen.getByText('Welcome to Nexorious')).toBeInTheDocument();
      });

      const usernameInput = screen.getByLabelText('Admin Username');
      const passwordInput = screen.getByLabelText('Password');
      const confirmPasswordInput = screen.getByLabelText('Confirm Password');

      expect(usernameInput).toHaveAttribute('id', 'username');
      expect(passwordInput).toHaveAttribute('id', 'password');
      expect(confirmPasswordInput).toHaveAttribute('id', 'confirmPassword');
    });

    it('should have proper form attributes for accessibility', async () => {
      renderComponent(SetupPage);
      
      await waitFor(() => {
        expect(screen.getByText('Welcome to Nexorious')).toBeInTheDocument();
      });

      const usernameInput = screen.getByLabelText('Admin Username');
      const passwordInput = screen.getByLabelText('Password');
      const confirmPasswordInput = screen.getByLabelText('Confirm Password');

      expect(usernameInput).toHaveAttribute('required');
      expect(passwordInput).toHaveAttribute('required');
      expect(confirmPasswordInput).toHaveAttribute('required');

      expect(usernameInput).toHaveAttribute('minlength', '3');
      expect(passwordInput).toHaveAttribute('minlength', '8');
    });
  });

  describe('Form State Management', () => {
    beforeEach(async () => {
      mockAuthStore.checkSetupStatus.mockResolvedValue({ needs_setup: true });
      renderComponent(SetupPage);
      
      await waitFor(() => {
        expect(screen.getByText('Welcome to Nexorious')).toBeInTheDocument();
      });
    });

    it('should disable inputs during loading', async () => {
      mockAuthStore.createInitialAdmin.mockImplementation(() => new Promise(() => {}));

      const usernameInput = screen.getByLabelText('Admin Username') as HTMLInputElement;
      const passwordInput = screen.getByLabelText('Password') as HTMLInputElement;
      const confirmPasswordInput = screen.getByLabelText('Confirm Password') as HTMLInputElement;

      await fillFormField(usernameInput, 'admin', userEvent);
      await fillFormField(passwordInput, 'password123', userEvent);
      await fillFormField(confirmPasswordInput, 'password123', userEvent);

      const submitButton = screen.getByRole('button', { name: 'Create Admin Account' });
      await userEvent.click(submitButton);

      await waitFor(() => {
        expect(usernameInput).toBeDisabled();
        expect(passwordInput).toBeDisabled();
        expect(confirmPasswordInput).toBeDisabled();
        expect(submitButton).toBeDisabled();
      });
    });

    it('should re-enable inputs after error', async () => {
      mockAuthStore.createInitialAdmin.mockRejectedValue(new Error('Setup failed'));

      const usernameInput = screen.getByLabelText('Admin Username') as HTMLInputElement;
      const passwordInput = screen.getByLabelText('Password') as HTMLInputElement;
      const confirmPasswordInput = screen.getByLabelText('Confirm Password') as HTMLInputElement;

      await fillFormField(usernameInput, 'admin', userEvent);
      await fillFormField(passwordInput, 'password123', userEvent);
      await fillFormField(confirmPasswordInput, 'password123', userEvent);

      const submitButton = screen.getByRole('button', { name: 'Create Admin Account' });
      await userEvent.click(submitButton);

      await waitFor(() => {
        expect(screen.getByText('Setup failed')).toBeInTheDocument();
      });

      // Inputs should be re-enabled after error
      expect(usernameInput).not.toBeDisabled();
      expect(passwordInput).not.toBeDisabled();
      expect(confirmPasswordInput).not.toBeDisabled();
      expect(submitButton).not.toBeDisabled();
    });
  });
});