import { vi } from 'vitest';
import type { User, AuthState } from '$lib/stores/auth.svelte';

export const mockUser: User = {
  id: 'test-user-id',
  username: 'testuser',
  email: 'test@example.com',
  firstName: 'Test',
  lastName: 'User',
  isAdmin: false
};

export const mockAuthenticatedState: AuthState = {
  user: mockUser,
  accessToken: 'mock-access-token',
  refreshToken: 'mock-refresh-token',
  isLoading: false,
  error: null
};

export const mockUnauthenticatedState: AuthState = {
  user: null,
  accessToken: null,
  refreshToken: null,
  isLoading: false,
  error: null
};

export const mockAuthStore = {
  value: mockUnauthenticatedState,
  login: vi.fn(),
  register: vi.fn(),
  logout: vi.fn(),
  refreshAuth: vi.fn(),
  clearError: vi.fn()
};

// Mock the auth store
vi.mock('$lib/stores/auth.svelte', () => ({
  auth: mockAuthStore
}));

// Also mock the main stores module
vi.mock('$lib/stores', () => ({
  auth: mockAuthStore
}));

// Helper functions for test setup
export function setAuthenticatedState(overrides: Partial<AuthState> = {}) {
  mockAuthStore.value = { ...mockAuthenticatedState, ...overrides };
}

export function setUnauthenticatedState() {
  mockAuthStore.value = { ...mockUnauthenticatedState };
}

export function setLoadingState() {
  mockAuthStore.value = {
    ...mockUnauthenticatedState,
    isLoading: true
  };
}

export function setErrorState(error: string) {
  mockAuthStore.value = {
    ...mockUnauthenticatedState,
    error
  };
}

export function resetAuthMocks() {
  mockAuthStore.login.mockClear();
  mockAuthStore.register.mockClear();
  mockAuthStore.logout.mockClear();
  mockAuthStore.refreshAuth.mockClear();
  mockAuthStore.clearError.mockClear();
  setUnauthenticatedState();
}