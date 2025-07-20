import { browser } from '$app/environment';
import { config } from '$lib/env';

export interface User {
  id: string;
  username: string;
  email: string;
  isAdmin: boolean;
}

export interface AuthState {
  user: User | null;
  accessToken: string | null;
  refreshToken: string | null;
  isLoading: boolean;
  error: string | null;
}

const initialState: AuthState = {
  user: null,
  accessToken: null,
  refreshToken: null,
  isLoading: false,
  error: null
};

function createAuthStore() {
  let state = $state<AuthState>(initialState);

  // Load auth state from localStorage on initialization
  if (browser) {
    const storedAuth = localStorage.getItem('auth');
    if (storedAuth) {
      try {
        const parsedAuth = JSON.parse(storedAuth);
        state = parsedAuth;
      } catch (error) {
        console.error('Failed to parse stored auth:', error);
        localStorage.removeItem('auth');
      }
    }
  }

  const authStore = {
    get value() {
      return state;
    },
    
    login: async (username: string, password: string) => {
      state = { ...state, isLoading: true, error: null };
      
      try {
        const response = await fetch(`${config.apiUrl}/auth/login`, {
          method: 'POST',
          headers: {
            'Content-Type': 'application/json',
          },
          body: JSON.stringify({ username: username, password }),
        });

        if (!response.ok) {
          const errorData = await response.json().catch(() => ({}));
          throw new Error(errorData.detail || 'Login failed');
        }

        const data = await response.json();
        
        // Fetch user profile after successful login
        const userResponse = await fetch(`${config.apiUrl}/auth/me`, {
          headers: {
            'Authorization': `Bearer ${data.access_token}`
          }
        });

        if (!userResponse.ok) {
          throw new Error('Failed to fetch user profile');
        }

        const user = await userResponse.json();
        const newState = {
          user: user,
          accessToken: data.access_token,
          refreshToken: data.refresh_token,
          isLoading: false,
          error: null
        };

        state = newState;
        
        if (browser) {
          localStorage.setItem('auth', JSON.stringify(newState));
        }
        
        return { ...data, user };
      } catch (error) {
        const errorMessage = error instanceof Error ? error.message : 'Login failed';
        state = { ...state, isLoading: false, error: errorMessage };
        throw error;
      }
    },

    register: async (userData: {
      email: string;
      username: string;
      password: string;
    }) => {
      state = { ...state, isLoading: true, error: null };
      
      try {
        const response = await fetch(`${config.apiUrl}/auth/register`, {
          method: 'POST',
          headers: {
            'Content-Type': 'application/json',
          },
          body: JSON.stringify(userData),
        });

        if (!response.ok) {
          const errorData = await response.json().catch(() => ({}));
          throw new Error(errorData.detail || 'Registration failed');
        }

        const userProfile = await response.json();
        
        // After successful registration, automatically log in the user
        await authStore.login(userData.username, userData.password);
        
        return userProfile;
      } catch (error) {
        const errorMessage = error instanceof Error ? error.message : 'Registration failed';
        state = { ...state, isLoading: false, error: errorMessage };
        throw error;
      }
    },

    logout: () => {
      state = initialState;
      if (browser) {
        localStorage.removeItem('auth');
      }
    },

    refreshAuth: async () => {
      if (!state.refreshToken) {
        return false;
      }

      try {
        const response = await fetch(`${config.apiUrl}/auth/refresh`, {
          method: 'POST',
          headers: {
            'Content-Type': 'application/json',
          },
          body: JSON.stringify({ refresh_token: state.refreshToken }),
        });

        if (!response.ok) {
          throw new Error('Token refresh failed');
        }

        const data = await response.json();
        const newState = {
          ...state,
          accessToken: data.access_token,
          refreshToken: data.refresh_token || state.refreshToken,
        };

        state = newState;
        
        if (browser) {
          localStorage.setItem('auth', JSON.stringify(newState));
        }
        
        return true;
      } catch (error) {
        console.error('Token refresh failed:', error);
        state = initialState;
        if (browser) {
          localStorage.removeItem('auth');
        }
        return false;
      }
    },

    clearError: () => {
      state = { ...state, error: null };
    },

    forgotPassword: async (email: string) => {
      state = { ...state, isLoading: true, error: null };
      
      try {
        const response = await fetch(`${config.apiUrl}/auth/forgot-password`, {
          method: 'POST',
          headers: {
            'Content-Type': 'application/json',
          },
          body: JSON.stringify({ email }),
        });

        if (!response.ok) {
          const errorData = await response.json().catch(() => ({}));
          throw new Error(errorData.detail || 'Failed to send password reset email');
        }

        const data = await response.json();
        state = { ...state, isLoading: false, error: null };
        return data;
      } catch (error) {
        const errorMessage = error instanceof Error ? error.message : 'Failed to send password reset email';
        state = { ...state, isLoading: false, error: errorMessage };
        throw error;
      }
    },

    resetPassword: async (token: string, newPassword: string) => {
      state = { ...state, isLoading: true, error: null };
      
      try {
        const response = await fetch(`${config.apiUrl}/auth/reset-password`, {
          method: 'POST',
          headers: {
            'Content-Type': 'application/json',
          },
          body: JSON.stringify({ token, new_password: newPassword }),
        });

        if (!response.ok) {
          const errorData = await response.json().catch(() => ({}));
          throw new Error(errorData.detail || 'Failed to reset password');
        }

        const data = await response.json();
        state = { ...state, isLoading: false, error: null };
        return data;
      } catch (error) {
        const errorMessage = error instanceof Error ? error.message : 'Failed to reset password';
        state = { ...state, isLoading: false, error: errorMessage };
        throw error;
      }
    },

    validateResetToken: async (token: string) => {
      state = { ...state, isLoading: true, error: null };
      
      try {
        const response = await fetch(`${config.apiUrl}/auth/reset-password/validate/${token}`, {
          method: 'GET',
          headers: {
            'Content-Type': 'application/json',
          },
        });

        if (!response.ok) {
          const errorData = await response.json().catch(() => ({}));
          throw new Error(errorData.detail || 'Invalid or expired reset token');
        }

        const data = await response.json();
        state = { ...state, isLoading: false, error: null };
        return data;
      } catch (error) {
        const errorMessage = error instanceof Error ? error.message : 'Invalid or expired reset token';
        state = { ...state, isLoading: false, error: errorMessage };
        throw error;
      }
    }
  };

  return authStore;
}

export const auth = createAuthStore();