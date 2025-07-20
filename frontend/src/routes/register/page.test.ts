import { describe, it, expect, beforeEach, afterEach } from 'vitest';
import { cleanup, screen, waitFor } from '@testing-library/svelte';
import RegisterPage from './+page.svelte';
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

describe('Register Page', () => {
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
    it('should render the registration form', () => {
      renderComponent(RegisterPage);

      expect(screen.getByText('Create Account')).toBeInTheDocument();
      expect(screen.getByText('Join Nexorious to start managing your game collection')).toBeInTheDocument();
      expect(screen.getByLabelText('Email Address')).toBeInTheDocument();
      expect(screen.getByLabelText('Username')).toBeInTheDocument();
      expect(screen.getByLabelText('Password')).toBeInTheDocument();
      expect(screen.getByLabelText('Confirm Password')).toBeInTheDocument();
      expect(screen.getByRole('button', { name: 'Create Account' })).toBeInTheDocument();
    });

    it('should set the correct page title', () => {
      renderComponent(RegisterPage);

      const titleElements = document.querySelectorAll('title');
      const hasRegisterTitle = Array.from(titleElements).some(
        title => title.textContent === 'Register - Nexorious'
      );
      expect(hasRegisterTitle).toBe(true);
    });


    it('should have a link to login page', () => {
      renderComponent(RegisterPage);

      expect(screen.getByText('Sign in')).toBeInTheDocument();
      const loginLink = screen.getByText('Sign in').closest('a');
      expect(loginLink?.getAttribute('href')).toBe('/login');
    });

    it('should have proper form structure', () => {
      const { container } = renderComponent(RegisterPage);

      const form = container.querySelector('form');
      expect(form).toBeInTheDocument();
      
      // Check required fields
      const requiredFields = [
        screen.getByLabelText('Email Address'),
        screen.getByLabelText('Username'),
        screen.getByLabelText('Password'),
        screen.getByLabelText('Confirm Password')
      ];

      requiredFields.forEach(field => {
        expect(field).toHaveAttribute('required');
      });
    });
  });

  describe('Form Validation', () => {
    it('should show error for empty required fields', async () => {
      renderComponent(RegisterPage);

      const submitButton = screen.getByRole('button', { name: 'Create Account' });
      await userEvent.click(submitButton);

      await waitFor(() => {
        expect(screen.getByText('Please fill in all required fields')).toBeInTheDocument();
      });
    });

    it('should show error for password mismatch', async () => {
      renderComponent(RegisterPage);

      const emailInput = screen.getByLabelText('Email Address') as HTMLInputElement;
      const usernameInput = screen.getByLabelText('Username') as HTMLInputElement;
      const passwordInput = screen.getByLabelText('Password') as HTMLInputElement;
      const confirmPasswordInput = screen.getByLabelText('Confirm Password') as HTMLInputElement;

      await fillFormField(emailInput, 'test@example.com', userEvent);
      await fillFormField(usernameInput, 'testuser', userEvent);
      await fillFormField(passwordInput, 'password123', userEvent);
      await fillFormField(confirmPasswordInput, 'differentpassword', userEvent);

      const submitButton = screen.getByRole('button', { name: 'Create Account' });
      await userEvent.click(submitButton);

      await waitFor(() => {
        expect(screen.getByText('Passwords do not match')).toBeInTheDocument();
      });
    });

    it('should show error for short password', async () => {
      renderComponent(RegisterPage);

      const emailInput = screen.getByLabelText('Email Address') as HTMLInputElement;
      const usernameInput = screen.getByLabelText('Username') as HTMLInputElement;
      const passwordInput = screen.getByLabelText('Password') as HTMLInputElement;
      const confirmPasswordInput = screen.getByLabelText('Confirm Password') as HTMLInputElement;

      await fillFormField(emailInput, 'test@example.com', userEvent);
      await fillFormField(usernameInput, 'testuser', userEvent);
      await fillFormField(passwordInput, 'short', userEvent);
      await fillFormField(confirmPasswordInput, 'short', userEvent);

      const submitButton = screen.getByRole('button', { name: 'Create Account' });
      await userEvent.click(submitButton);

      await waitFor(() => {
        expect(screen.getByText('Password must be at least 8 characters long')).toBeInTheDocument();
      });
    });

    it('should validate individual required fields', async () => {
      renderComponent(RegisterPage);

      // Test missing email
      const usernameInput = screen.getByLabelText('Username') as HTMLInputElement;
      const passwordInput = screen.getByLabelText('Password') as HTMLInputElement;
      const confirmPasswordInput = screen.getByLabelText('Confirm Password') as HTMLInputElement;

      await fillFormField(usernameInput, 'testuser', userEvent);
      await fillFormField(passwordInput, 'password123', userEvent);
      await fillFormField(confirmPasswordInput, 'password123', userEvent);

      const submitButton = screen.getByRole('button', { name: 'Create Account' });
      await userEvent.click(submitButton);

      await waitFor(() => {
        expect(screen.getByText('Please fill in all required fields')).toBeInTheDocument();
      });
    });

    it('should accept valid form data', async () => {
      mockAuthStore.register.mockResolvedValue(undefined);
      renderComponent(RegisterPage);

      const emailInput = screen.getByLabelText('Email Address') as HTMLInputElement;
      const usernameInput = screen.getByLabelText('Username') as HTMLInputElement;
      const passwordInput = screen.getByLabelText('Password') as HTMLInputElement;
      const confirmPasswordInput = screen.getByLabelText('Confirm Password') as HTMLInputElement;

      await fillFormField(emailInput, 'test@example.com', userEvent);
      await fillFormField(usernameInput, 'testuser', userEvent);
      await fillFormField(passwordInput, 'password123', userEvent);
      await fillFormField(confirmPasswordInput, 'password123', userEvent);

      const submitButton = screen.getByRole('button', { name: 'Create Account' });
      await userEvent.click(submitButton);

      await waitFor(() => {
        expect(mockAuthStore.register).toHaveBeenCalledWith({
          email: 'test@example.com',
          username: 'testuser',
          password: 'password123'
        });
      });
    });
  });

  describe('Form Submission', () => {

    it('should redirect to games page on successful registration', async () => {
      mockAuthStore.register.mockResolvedValue(undefined);
      renderComponent(RegisterPage);

      const emailInput = screen.getByLabelText('Email Address') as HTMLInputElement;
      const usernameInput = screen.getByLabelText('Username') as HTMLInputElement;
      const passwordInput = screen.getByLabelText('Password') as HTMLInputElement;
      const confirmPasswordInput = screen.getByLabelText('Confirm Password') as HTMLInputElement;

      await fillFormField(emailInput, 'test@example.com', userEvent);
      await fillFormField(usernameInput, 'testuser', userEvent);
      await fillFormField(passwordInput, 'password123', userEvent);
      await fillFormField(confirmPasswordInput, 'password123', userEvent);

      const submitButton = screen.getByRole('button', { name: 'Create Account' });
      await userEvent.click(submitButton);

      await waitFor(() => {
        expect(mockGoto).toHaveBeenCalledWith('/games');
      });
    });

    it('should show loading state during registration', async () => {
      mockAuthStore.register.mockImplementation(() => new Promise(() => {}));
      renderComponent(RegisterPage);

      const emailInput = screen.getByLabelText('Email Address') as HTMLInputElement;
      const usernameInput = screen.getByLabelText('Username') as HTMLInputElement;
      const passwordInput = screen.getByLabelText('Password') as HTMLInputElement;
      const confirmPasswordInput = screen.getByLabelText('Confirm Password') as HTMLInputElement;

      await fillFormField(emailInput, 'test@example.com', userEvent);
      await fillFormField(usernameInput, 'testuser', userEvent);
      await fillFormField(passwordInput, 'password123', userEvent);
      await fillFormField(confirmPasswordInput, 'password123', userEvent);

      const submitButton = screen.getByRole('button', { name: 'Create Account' });
      await userEvent.click(submitButton);

      await waitFor(() => {
        expect(screen.getByText('Creating account...')).toBeInTheDocument();
        expect(submitButton).toBeDisabled();
      });
    });

    it('should show error on registration failure', async () => {
      mockAuthStore.register.mockRejectedValue(new Error('Email already exists'));
      renderComponent(RegisterPage);

      const emailInput = screen.getByLabelText('Email Address') as HTMLInputElement;
      const usernameInput = screen.getByLabelText('Username') as HTMLInputElement;
      const passwordInput = screen.getByLabelText('Password') as HTMLInputElement;
      const confirmPasswordInput = screen.getByLabelText('Confirm Password') as HTMLInputElement;

      await fillFormField(emailInput, 'existing@example.com', userEvent);
      await fillFormField(usernameInput, 'testuser', userEvent);
      await fillFormField(passwordInput, 'password123', userEvent);
      await fillFormField(confirmPasswordInput, 'password123', userEvent);

      const submitButton = screen.getByRole('button', { name: 'Create Account' });
      await userEvent.click(submitButton);

      await waitFor(() => {
        expect(screen.getByText('Email already exists')).toBeInTheDocument();
      });
    });

    it('should handle generic registration errors', async () => {
      mockAuthStore.register.mockRejectedValue('Network error');
      renderComponent(RegisterPage);

      const emailInput = screen.getByLabelText('Email Address') as HTMLInputElement;
      const usernameInput = screen.getByLabelText('Username') as HTMLInputElement;
      const passwordInput = screen.getByLabelText('Password') as HTMLInputElement;
      const confirmPasswordInput = screen.getByLabelText('Confirm Password') as HTMLInputElement;

      await fillFormField(emailInput, 'test@example.com', userEvent);
      await fillFormField(usernameInput, 'testuser', userEvent);
      await fillFormField(passwordInput, 'password123', userEvent);
      await fillFormField(confirmPasswordInput, 'password123', userEvent);

      const submitButton = screen.getByRole('button', { name: 'Create Account' });
      await userEvent.click(submitButton);

      await waitFor(() => {
        expect(screen.getByText('Registration failed')).toBeInTheDocument();
      });
    });
  });

  describe('Keyboard Navigation', () => {
    it('should submit form when Enter is pressed in any field', async () => {
      mockAuthStore.register.mockResolvedValue(undefined);
      renderComponent(RegisterPage);

      const emailInput = screen.getByLabelText('Email Address') as HTMLInputElement;
      const usernameInput = screen.getByLabelText('Username') as HTMLInputElement;
      const passwordInput = screen.getByLabelText('Password') as HTMLInputElement;
      const confirmPasswordInput = screen.getByLabelText('Confirm Password') as HTMLInputElement;

      await fillFormField(emailInput, 'test@example.com', userEvent);
      await fillFormField(usernameInput, 'testuser', userEvent);
      await fillFormField(passwordInput, 'password123', userEvent);
      await fillFormField(confirmPasswordInput, 'password123', userEvent);

      await userEvent.keyDown(confirmPasswordInput, 'Enter');

      await waitFor(() => {
        expect(mockAuthStore.register).toHaveBeenCalled();
      });
    });
  });

  describe('Authentication State Handling', () => {
    it('should redirect to games page if already authenticated', () => {
      setAuthenticatedState();
      renderComponent(RegisterPage);

      expect(mockGoto).toHaveBeenCalledWith('/games');
    });

    it('should not redirect if not authenticated', () => {
      setUnauthenticatedState();
      renderComponent(RegisterPage);

      expect(mockGoto).not.toHaveBeenCalled();
    });
  });

  describe('Form Layout', () => {
    it('should have form container', () => {
      const { container } = renderComponent(RegisterPage);

      const form = container.querySelector('form');
      expect(form).toBeInTheDocument();
    });
  });

  describe('Input Types and Validation', () => {
    it('should have correct input types', () => {
      renderComponent(RegisterPage);

      const emailInput = screen.getByLabelText('Email Address');
      const usernameInput = screen.getByLabelText('Username');
      const passwordInput = screen.getByLabelText('Password');
      const confirmPasswordInput = screen.getByLabelText('Confirm Password');

      expect(emailInput).toHaveAttribute('type', 'email');
      expect(usernameInput).toHaveAttribute('type', 'text');
      expect(passwordInput).toHaveAttribute('type', 'password');
      expect(confirmPasswordInput).toHaveAttribute('type', 'password');
    });

    it('should have proper placeholder text', () => {
      renderComponent(RegisterPage);

      expect(screen.getByLabelText('Email Address')).toHaveAttribute('placeholder', 'Enter your email address');
      expect(screen.getByLabelText('Username')).toHaveAttribute('placeholder', 'Choose a unique username');
      expect(screen.getByLabelText('Password')).toHaveAttribute('placeholder', 'Enter a strong password');
      expect(screen.getByLabelText('Confirm Password')).toHaveAttribute('placeholder', 'Confirm your password');
    });
  });

  describe('Error Display', () => {
    it('should display error message', async () => {
      renderComponent(RegisterPage);

      const submitButton = screen.getByRole('button', { name: 'Create Account' });
      await userEvent.click(submitButton);

      await waitFor(() => {
        const errorMessage = screen.getByText('Please fill in all required fields');
        expect(errorMessage).toBeInTheDocument();
      });
    });
  });

  describe('Accessibility', () => {
    it('should have proper accessibility attributes', () => {
      const { container } = renderComponent(RegisterPage);
      testAccessibility(container);
    });

    it('should have proper label associations', () => {
      renderComponent(RegisterPage);

      const inputs = [
        { label: 'Email Address', id: 'email' },
        { label: 'Username', id: 'username' },
        { label: 'Password', id: 'password' },
        { label: 'Confirm Password', id: 'confirmPassword' }
      ];

      inputs.forEach(({ label, id }) => {
        const input = screen.getByLabelText(label);
        expect(input).toHaveAttribute('id', id);
      });
    });

    it('should support keyboard navigation', async () => {
      renderComponent(RegisterPage);

      const focusableElements = [
        screen.getByLabelText('Email Address'),
        screen.getByLabelText('Username'),
        screen.getByLabelText('Password'),
        screen.getByLabelText('Confirm Password'),
        screen.getByRole('button', { name: 'Create Account' }),
        screen.getByText('Sign in')
      ];

      // Test that all elements can receive focus
      focusableElements.forEach(element => {
        element.focus();
        expect(document.activeElement).toBe(element);
      });
    });
  });
});