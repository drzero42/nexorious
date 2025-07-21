import { describe, it, expect, beforeEach, afterEach } from 'vitest';
import { cleanup, screen, waitFor } from '@testing-library/svelte';
import LoginPage from './+page.svelte';
import {
  renderComponent,
  createUserEvent,
  fillFormField,
  testAccessibility
} from '../../test-utils/test-helpers';
import {
  mockAuthStore,
  setAuthenticatedState,
  setUnauthenticatedState,
  resetAuthMocks
} from '../../test-utils/auth-mocks';
import { mockGoto, resetNavigationMocks } from '../../test-utils/navigation-mocks';

describe('Login Page', () => {
  const userEvent = createUserEvent();

  beforeEach(() => {
    resetAuthMocks();
    resetNavigationMocks();
    setUnauthenticatedState();
  });

  afterEach(() => {
    cleanup();
  });

  describe('Basic Rendering', () => {
    it('should render the login form', () => {
      renderComponent(LoginPage);

      expect(screen.getByText('Welcome Back')).toBeInTheDocument();
      expect(screen.getByText('Sign in to access your game collection')).toBeInTheDocument();
      expect(screen.getByLabelText('Username')).toBeInTheDocument();
      expect(screen.getByLabelText('Password')).toBeInTheDocument();
      expect(screen.getByRole('button', { name: 'Sign In' })).toBeInTheDocument();
    });

    it('should set the correct page title', () => {
      renderComponent(LoginPage);

      const titleElements = document.querySelectorAll('title');
      const hasLoginTitle = Array.from(titleElements).some(
        title => title.textContent === 'Login - Nexorious'
      );
      expect(hasLoginTitle).toBe(true);
    });

    it('should have a link to register page', () => {
      renderComponent(LoginPage);

      expect(screen.getByText('Sign up')).toBeInTheDocument();
      const registerLink = screen.getByText('Sign up').closest('a');
      expect(registerLink?.getAttribute('href')).toBe('/register');
    });

    it('should have proper form structure', () => {
      const { container } = renderComponent(LoginPage);

      const form = container.querySelector('form');
      expect(form).toBeInTheDocument();
      
      const usernameInput = screen.getByLabelText('Username');
      const passwordInput = screen.getByLabelText('Password');
      
      expect(usernameInput).toHaveAttribute('type', 'text');
      expect(usernameInput).toHaveAttribute('required');
      expect(passwordInput).toHaveAttribute('type', 'password');
      expect(passwordInput).toHaveAttribute('required');
    });
  });

  describe('Form Validation', () => {
    it('should show error for empty fields', async () => {
      renderComponent(LoginPage);

      const submitButton = screen.getByRole('button', { name: 'Sign In' });
      await userEvent.click(submitButton);

      await waitFor(() => {
        expect(screen.getByText('Please fill in all fields')).toBeInTheDocument();
      });
    });

    it('should show error for missing username', async () => {
      renderComponent(LoginPage);

      const passwordInput = screen.getByLabelText('Password') as HTMLInputElement;
      await fillFormField(passwordInput, 'password123', userEvent);

      const submitButton = screen.getByRole('button', { name: 'Sign In' });
      await userEvent.click(submitButton);

      await waitFor(() => {
        expect(screen.getByText('Please fill in all fields')).toBeInTheDocument();
      });
    });

    it('should show error for missing password', async () => {
      renderComponent(LoginPage);

      const usernameInput = screen.getByLabelText('Username') as HTMLInputElement;
      await fillFormField(usernameInput, 'testuser', userEvent);

      const submitButton = screen.getByRole('button', { name: 'Sign In' });
      await userEvent.click(submitButton);

      await waitFor(() => {
        expect(screen.getByText('Please fill in all fields')).toBeInTheDocument();
      });
    });

    it('should clear error when user starts typing', async () => {
      renderComponent(LoginPage);

      // Trigger an error first
      const submitButton = screen.getByRole('button', { name: 'Sign In' });
      await userEvent.click(submitButton);

      await waitFor(() => {
        expect(screen.getByText('Please fill in all fields')).toBeInTheDocument();
      });

      // Start typing in username field
      const usernameInput = screen.getByLabelText('Username') as HTMLInputElement;
      await fillFormField(usernameInput, 'testuser', userEvent);

      // Error should be cleared when form is submitted again with valid data
      const passwordInput = screen.getByLabelText('Password') as HTMLInputElement;
      await fillFormField(passwordInput, 'password123', userEvent);

      await userEvent.click(submitButton);

      expect(screen.queryByText('Please fill in all fields')).not.toBeInTheDocument();
    });
  });

  describe('Form Submission', () => {
    it('should call login function with correct credentials', async () => {
      mockAuthStore.login.mockResolvedValue(undefined);
      renderComponent(LoginPage);

      const usernameInput = screen.getByLabelText('Username') as HTMLInputElement;
      const passwordInput = screen.getByLabelText('Password') as HTMLInputElement;

      await fillFormField(usernameInput, 'testuser', userEvent);
      await fillFormField(passwordInput, 'password123', userEvent);

      const submitButton = screen.getByRole('button', { name: 'Sign In' });
      await userEvent.click(submitButton);

      await waitFor(() => {
        expect(mockAuthStore.login).toHaveBeenCalledWith('testuser', 'password123');
      });
    });

    it('should redirect to games page on successful login', async () => {
      mockAuthStore.login.mockResolvedValue(undefined);
      renderComponent(LoginPage);

      const usernameInput = screen.getByLabelText('Username') as HTMLInputElement;
      const passwordInput = screen.getByLabelText('Password') as HTMLInputElement;

      await fillFormField(usernameInput, 'testuser', userEvent);
      await fillFormField(passwordInput, 'password123', userEvent);

      const submitButton = screen.getByRole('button', { name: 'Sign In' });
      await userEvent.click(submitButton);

      await waitFor(() => {
        expect(mockGoto).toHaveBeenCalledWith('/games');
      });
    });

    it('should show loading state during login', async () => {
      // Mock login to never resolve to test loading state
      mockAuthStore.login.mockImplementation(() => new Promise(() => {}));
      renderComponent(LoginPage);

      const usernameInput = screen.getByLabelText('Username') as HTMLInputElement;
      const passwordInput = screen.getByLabelText('Password') as HTMLInputElement;

      await fillFormField(usernameInput, 'testuser', userEvent);
      await fillFormField(passwordInput, 'password123', userEvent);

      const submitButton = screen.getByRole('button', { name: 'Sign In' });
      await userEvent.click(submitButton);

      await waitFor(() => {
        expect(screen.getByText('Signing in...')).toBeInTheDocument();
        expect(submitButton).toBeDisabled();
      });
    });

    it('should show error on login failure', async () => {
      mockAuthStore.login.mockRejectedValue(new Error('Invalid credentials'));
      renderComponent(LoginPage);

      const usernameInput = screen.getByLabelText('Username') as HTMLInputElement;
      const passwordInput = screen.getByLabelText('Password') as HTMLInputElement;

      await fillFormField(usernameInput, 'testuser', userEvent);
      await fillFormField(passwordInput, 'wrongpassword', userEvent);

      const submitButton = screen.getByRole('button', { name: 'Sign In' });
      await userEvent.click(submitButton);

      await waitFor(() => {
        expect(screen.getByText('Invalid credentials')).toBeInTheDocument();
      });
    });

    it('should handle generic login errors', async () => {
      mockAuthStore.login.mockRejectedValue('Something went wrong');
      renderComponent(LoginPage);

      const usernameInput = screen.getByLabelText('Username') as HTMLInputElement;
      const passwordInput = screen.getByLabelText('Password') as HTMLInputElement;

      await fillFormField(usernameInput, 'testuser', userEvent);
      await fillFormField(passwordInput, 'password123', userEvent);

      const submitButton = screen.getByRole('button', { name: 'Sign In' });
      await userEvent.click(submitButton);

      await waitFor(() => {
        expect(screen.getByText('Login failed')).toBeInTheDocument();
      });
    });
  });

  describe('Keyboard Navigation', () => {
    it('should submit form when Enter is pressed in username field', async () => {
      mockAuthStore.login.mockResolvedValue(undefined);
      renderComponent(LoginPage);

      const usernameInput = screen.getByLabelText('Username') as HTMLInputElement;
      const passwordInput = screen.getByLabelText('Password') as HTMLInputElement;

      await fillFormField(usernameInput, 'testuser', userEvent);
      await fillFormField(passwordInput, 'password123', userEvent);

      await userEvent.keyDown(usernameInput, 'Enter');

      await waitFor(() => {
        expect(mockAuthStore.login).toHaveBeenCalledWith('testuser', 'password123');
      });
    });

    it('should submit form when Enter is pressed in password field', async () => {
      mockAuthStore.login.mockResolvedValue(undefined);
      renderComponent(LoginPage);

      const usernameInput = screen.getByLabelText('Username') as HTMLInputElement;
      const passwordInput = screen.getByLabelText('Password') as HTMLInputElement;

      await fillFormField(usernameInput, 'testuser', userEvent);
      await fillFormField(passwordInput, 'password123', userEvent);

      await userEvent.keyDown(passwordInput, 'Enter');

      await waitFor(() => {
        expect(mockAuthStore.login).toHaveBeenCalledWith('testuser', 'password123');
      });
    });
  });

  describe('Authentication State Handling', () => {
    it('should redirect to games page if already authenticated', () => {
      setAuthenticatedState();
      renderComponent(LoginPage);

      expect(mockGoto).toHaveBeenCalledWith('/games');
    });

    it('should not redirect if not authenticated', () => {
      setUnauthenticatedState();
      renderComponent(LoginPage);

      expect(mockGoto).not.toHaveBeenCalled();
    });
  });

  describe('Error Display', () => {
    it('should display error message', async () => {
      renderComponent(LoginPage);

      const submitButton = screen.getByRole('button', { name: 'Sign In' });
      await userEvent.click(submitButton);

      await waitFor(() => {
        const errorMessage = screen.getByText('Please fill in all fields');
        expect(errorMessage).toBeInTheDocument();
      });
    });

    it('should not show error message when no error', () => {
      renderComponent(LoginPage);

      expect(screen.queryByText('Please fill in all fields')).not.toBeInTheDocument();
    });
  });

  describe('Form Structure', () => {
    it('should have login form container', () => {
      const { container } = renderComponent(LoginPage);

      const form = container.querySelector('form');
      expect(form).toBeInTheDocument();
    });

    it('should have proper input elements', () => {
      renderComponent(LoginPage);

      const usernameInput = screen.getByLabelText('Username');
      const passwordInput = screen.getByLabelText('Password');

      expect(usernameInput).toBeInTheDocument();
      expect(passwordInput).toBeInTheDocument();
    });

    it('should have functional submit button', () => {
      renderComponent(LoginPage);

      const submitButton = screen.getByRole('button', { name: 'Sign In' });
      expect(submitButton).toBeInTheDocument();
    });
  });

  describe('Accessibility', () => {
    it('should have proper accessibility attributes', () => {
      const { container } = renderComponent(LoginPage);
      testAccessibility(container);
    });

    it('should have proper label associations', () => {
      renderComponent(LoginPage);

      const usernameInput = screen.getByLabelText('Username');
      const passwordInput = screen.getByLabelText('Password');

      expect(usernameInput).toHaveAttribute('id', 'username');
      expect(passwordInput).toHaveAttribute('id', 'password');
    });

    it('should have proper placeholder text', () => {
      renderComponent(LoginPage);

      const usernameInput = screen.getByLabelText('Username');
      const passwordInput = screen.getByLabelText('Password');

      expect(usernameInput).toHaveAttribute('placeholder', 'Enter your username');
      expect(passwordInput).toHaveAttribute('placeholder', 'Enter your password');
    });

    it('should support keyboard navigation', async () => {
      renderComponent(LoginPage);
      
      const usernameInput = screen.getByLabelText('Username');
      const passwordInput = screen.getByLabelText('Password');
      const submitButton = screen.getByRole('button', { name: 'Sign In' });
      const forgotPasswordLink = screen.getByText('Forgot your password?');
      const registerLink = screen.getByText('Sign up');

      // Test tab navigation
      usernameInput.focus();
      expect(document.activeElement).toBe(usernameInput);

      await userEvent.keyDown(usernameInput, 'Tab');
      expect(document.activeElement).toBe(passwordInput);

      await userEvent.keyDown(passwordInput, 'Tab');
      expect(document.activeElement).toBe(submitButton);

      await userEvent.keyDown(submitButton, 'Tab');
      expect(document.activeElement).toBe(forgotPasswordLink);

      await userEvent.keyDown(forgotPasswordLink, 'Tab');
      expect(document.activeElement).toBe(registerLink);
    });
  });
});