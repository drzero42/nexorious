import {
  createContext,
  useContext,
  useState,
  useEffect,
  useCallback,
  useRef,
  type ReactNode,
} from 'react';
import { useNavigate } from '@tanstack/react-router';
import type { User } from '@/types';
import * as authApi from '@/api/auth';
import { setAuthHandlers } from '@/api/client';

interface AuthContextValue {
  user: User | null;
  isAuthenticated: boolean;
  isLoading: boolean;
  error: string | null;
  login: (username: string, password: string) => Promise<void>;
  logout: () => void;
  clearError: () => void;
}

const AuthContext = createContext<AuthContextValue | null>(null);

const STORAGE_KEY = 'auth';

interface StoredAuth {
  accessToken: string;
  refreshToken: string;
  user: User;
}

function getStoredAuth(): StoredAuth | null {
  if (typeof window === 'undefined') return null;

  try {
    const stored = localStorage.getItem(STORAGE_KEY);
    if (!stored) return null;

    const parsed = JSON.parse(stored) as StoredAuth;
    if (parsed.accessToken && parsed.refreshToken && parsed.user) {
      return parsed;
    }
    return null;
  } catch {
    localStorage.removeItem(STORAGE_KEY);
    return null;
  }
}

function setStoredAuth(auth: StoredAuth): void {
  if (typeof window === 'undefined') return;
  localStorage.setItem(STORAGE_KEY, JSON.stringify(auth));
}

function clearStoredAuth(): void {
  if (typeof window === 'undefined') return;
  localStorage.removeItem(STORAGE_KEY);
}

export function AuthProvider({ children }: { children: ReactNode }) {
  const navigate = useNavigate();
  const [user, setUser] = useState<User | null>(null);
  const [accessToken, setAccessToken] = useState<string | null>(null);
  const [refreshToken, setRefreshToken] = useState<string | null>(null);
  const [isLoading, setIsLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  // Ref to track current tokens for use in callbacks
  const tokensRef = useRef({ accessToken, refreshToken });
  tokensRef.current = { accessToken, refreshToken };

  // Ref for refresh deduplication
  const refreshPromiseRef = useRef<Promise<boolean> | null>(null);

  const isAuthenticated = !!user && !!accessToken;

  // Token getter for API client
  const getAccessTokenFn = useCallback(() => {
    return tokensRef.current.accessToken;
  }, []);

  // Logout handler
  const logout = useCallback(() => {
    clearStoredAuth();
    setUser(null);
    setAccessToken(null);
    setRefreshToken(null);
    setError(null);
    navigate({ to: '/login' });
  }, [navigate]);

  // Token refresh with deduplication
  const refreshTokensFn = useCallback(async (): Promise<boolean> => {
    const currentRefreshToken = tokensRef.current.refreshToken;
    if (!currentRefreshToken) {
      return false;
    }

    // Return existing promise if refresh is already in progress
    if (refreshPromiseRef.current) {
      return refreshPromiseRef.current;
    }

    // Create new refresh promise
    refreshPromiseRef.current = (async () => {
      try {
        const response = await authApi.refreshToken(currentRefreshToken);
        const newAccessToken = response.access_token;
        const newRefreshToken = response.refresh_token || currentRefreshToken;

        setAccessToken(newAccessToken);
        setRefreshToken(newRefreshToken);

        // Update stored auth
        const storedAuth = getStoredAuth();
        if (storedAuth) {
          setStoredAuth({
            ...storedAuth,
            accessToken: newAccessToken,
            refreshToken: newRefreshToken,
          });
        }

        return true;
      } catch {
        // Refresh failed, clear auth state
        clearStoredAuth();
        setUser(null);
        setAccessToken(null);
        setRefreshToken(null);
        return false;
      } finally {
        refreshPromiseRef.current = null;
      }
    })();

    return refreshPromiseRef.current;
  }, []);

  // Register auth handlers with API client
  useEffect(() => {
    setAuthHandlers(getAccessTokenFn, refreshTokensFn, logout);
  }, [getAccessTokenFn, refreshTokensFn, logout]);

  // Initialize auth state from localStorage
  useEffect(() => {
    const initializeAuth = async () => {
      const storedAuth = getStoredAuth();

      if (!storedAuth) {
        setIsLoading(false);
        return;
      }

      // Update ref immediately so API calls can use the token
      tokensRef.current = {
        accessToken: storedAuth.accessToken,
        refreshToken: storedAuth.refreshToken
      };

      // Set state for React rendering
      setAccessToken(storedAuth.accessToken);
      setRefreshToken(storedAuth.refreshToken);
      setUser(storedAuth.user);

      // Validate token by calling getMe
      try {
        const currentUser = await authApi.getMe();
        setUser(currentUser);
        // Update stored auth with potentially updated user data
        setStoredAuth({
          ...storedAuth,
          user: currentUser,
        });
      } catch {
        // Token is invalid, clear auth state
        clearStoredAuth();
        tokensRef.current = { accessToken: null, refreshToken: null };
        setUser(null);
        setAccessToken(null);
        setRefreshToken(null);
      } finally {
        setIsLoading(false);
      }
    };

    initializeAuth();
  }, []);

  // Login function
  const login = useCallback(async (username: string, password: string): Promise<void> => {
    setIsLoading(true);
    setError(null);

    try {
      // Call login API
      const loginResponse = await authApi.login(username, password);
      const newAccessToken = loginResponse.access_token;
      const newRefreshToken = loginResponse.refresh_token;

      // Update ref immediately so getMe can use the new token
      // (setState is async, ref update is sync)
      tokensRef.current = { accessToken: newAccessToken, refreshToken: newRefreshToken };

      // Set state for React rendering
      setAccessToken(newAccessToken);
      setRefreshToken(newRefreshToken);

      // Fetch user data (now uses token from ref)
      const currentUser = await authApi.getMe();
      setUser(currentUser);

      // Store auth data
      setStoredAuth({
        accessToken: newAccessToken,
        refreshToken: newRefreshToken,
        user: currentUser,
      });
    } catch (err) {
      const errorMessage = err instanceof Error ? err.message : 'Login failed';
      setError(errorMessage);
      // Clear any partial state
      tokensRef.current = { accessToken: null, refreshToken: null };
      setAccessToken(null);
      setRefreshToken(null);
      setUser(null);
      throw err;
    } finally {
      setIsLoading(false);
    }
  }, []);

  // Clear error function
  const clearError = useCallback(() => {
    setError(null);
  }, []);

  const value: AuthContextValue = {
    user,
    isAuthenticated,
    isLoading,
    error,
    login,
    logout,
    clearError,
  };

  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>;
}

// eslint-disable-next-line react-refresh/only-export-components
export function useAuth(): AuthContextValue {
  const context = useContext(AuthContext);
  if (!context) {
    throw new Error('useAuth must be used within an AuthProvider');
  }
  return context;
}
