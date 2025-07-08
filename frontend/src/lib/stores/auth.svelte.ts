import { browser } from '$app/environment';

export interface User {
  id: string;
  username: string;
  email: string;
  firstName?: string;
  lastName?: string;
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

  return {
    get value() {
      return state;
    },
    
    login: async (username: string, password: string) => {
      state = { ...state, isLoading: true, error: null };
      
      try {
        const response = await fetch('/api/auth/login', {
          method: 'POST',
          headers: {
            'Content-Type': 'application/json',
          },
          body: JSON.stringify({ username, password }),
        });

        if (!response.ok) {
          throw new Error('Login failed');
        }

        const data = await response.json();
        const newState = {
          user: data.user,
          accessToken: data.access_token,
          refreshToken: data.refresh_token,
          isLoading: false,
          error: null
        };

        state = newState;
        
        if (browser) {
          localStorage.setItem('auth', JSON.stringify(newState));
        }
        
        return data;
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
        const response = await fetch('/api/auth/refresh', {
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
    }
  };
}

export const auth = createAuthStore();