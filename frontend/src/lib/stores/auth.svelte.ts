import { browser } from '$app/environment';
import { config } from '$lib/env';

export interface User {
  id: string;
  username: string;
  isAdmin: boolean;
}

export interface SetupStatusResponse {
  needs_setup: boolean;
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
          user: {
            ...user,
            isAdmin: user.is_admin // Map backend snake_case to frontend camelCase
          },
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




    checkUsernameAvailability: async (username: string) => {
      if (!username.trim() || username.length < 3) {
        return { available: false, username };
      }

      try {
        const response = await fetch(`${config.apiUrl}/auth/username/check/${encodeURIComponent(username)}`, {
          method: 'GET',
          headers: {
            'Authorization': `Bearer ${state.accessToken}`
          }
        });

        if (!response.ok) {
          const errorData = await response.json().catch(() => ({}));
          throw new Error(errorData.detail || 'Failed to check username availability');
        }

        const data = await response.json();
        return data;
      } catch (error) {
        console.error('Username availability check failed:', error);
        return { available: false, username };
      }
    },

    changeUsername: async (newUsername: string) => {
      if (!state.accessToken) {
        throw new Error('Not authenticated');
      }

      state = { ...state, isLoading: true, error: null };
      
      try {
        const response = await fetch(`${config.apiUrl}/auth/username`, {
          method: 'PUT',
          headers: {
            'Content-Type': 'application/json',
            'Authorization': `Bearer ${state.accessToken}`
          },
          body: JSON.stringify({ new_username: newUsername }),
        });

        if (!response.ok) {
          const errorData = await response.json().catch(() => ({}));
          throw new Error(errorData.detail || 'Failed to change username');
        }

        const updatedUser = await response.json();
        const newState = {
          ...state,
          user: {
            ...updatedUser,
            isAdmin: updatedUser.is_admin // Map backend snake_case to frontend camelCase
          },
          isLoading: false,
          error: null
        };

        state = newState;
        
        if (browser) {
          localStorage.setItem('auth', JSON.stringify(newState));
        }
        
        return updatedUser;
      } catch (error) {
        const errorMessage = error instanceof Error ? error.message : 'Failed to change username';
        state = { ...state, isLoading: false, error: errorMessage };
        throw error;
      }
    },

    changePassword: async (currentPassword: string, newPassword: string) => {
      if (!state.accessToken) {
        throw new Error('Not authenticated');
      }

      state = { ...state, isLoading: true, error: null };
      
      try {
        const response = await fetch(`${config.apiUrl}/auth/change-password`, {
          method: 'PUT',
          headers: {
            'Content-Type': 'application/json',
            'Authorization': `Bearer ${state.accessToken}`
          },
          body: JSON.stringify({ 
            current_password: currentPassword, 
            new_password: newPassword 
          }),
        });

        if (!response.ok) {
          const errorData = await response.json().catch(() => ({}));
          throw new Error(errorData.detail || 'Failed to change password');
        }

        const data = await response.json();
        
        // Password change invalidates all sessions, so logout user
        state = initialState;
        if (browser) {
          localStorage.removeItem('auth');
        }
        
        return data;
      } catch (error) {
        const errorMessage = error instanceof Error ? error.message : 'Failed to change password';
        state = { ...state, isLoading: false, error: errorMessage };
        throw error;
      }
    },

    checkSetupStatus: async (): Promise<SetupStatusResponse> => {
      try {
        const response = await fetch(`${config.apiUrl}/auth/setup/status`, {
          method: 'GET',
          headers: {
            'Content-Type': 'application/json',
          },
        });

        if (!response.ok) {
          throw new Error('Failed to check setup status');
        }

        const data = await response.json();
        return data;
      } catch (error) {
        console.error('Setup status check failed:', error);
        // Default to not needing setup on error
        return { needs_setup: false };
      }
    },

    createInitialAdmin: async (username: string, password: string) => {
      state = { ...state, isLoading: true, error: null };
      
      try {
        const response = await fetch(`${config.apiUrl}/auth/setup/admin`, {
          method: 'POST',
          headers: {
            'Content-Type': 'application/json',
          },
          body: JSON.stringify({ username, password }),
        });

        if (!response.ok) {
          const errorData = await response.json().catch(() => ({}));
          throw new Error(errorData.detail || 'Failed to create initial admin');
        }

        const adminUser = await response.json();
        state = { ...state, isLoading: false, error: null };
        
        return adminUser;
      } catch (error) {
        const errorMessage = error instanceof Error ? error.message : 'Failed to create initial admin';
        state = { ...state, isLoading: false, error: errorMessage };
        throw error;
      }
    }
  };

  return authStore;
}

export const auth = createAuthStore();