import { vi } from 'vitest';
import type { User, AuthState } from '$lib/stores/auth.svelte';

export const mockUser: User = {
  id: 'test-user-id',
  username: 'testuser',
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
  clearError: vi.fn(),
  checkSetupStatus: vi.fn(),
  createInitialAdmin: vi.fn(),
  checkUsernameAvailability: vi.fn(),
  changeUsername: vi.fn(),
  changePassword: vi.fn()
};

// Mock the auth store
vi.mock('$lib/stores/auth.svelte', () => ({
  auth: mockAuthStore
}));

// Simple UI store mock to avoid circular imports
const mockUIStore = {
  showSuccess: vi.fn(),
  showError: vi.fn(),
  showWarning: vi.fn(),
  showInfo: vi.fn(),
  addNotification: vi.fn(),
  removeNotification: vi.fn(),
  clearNotifications: vi.fn()
};

vi.mock('$lib/stores', () => ({
  auth: mockAuthStore,
  ui: mockUIStore
}));

// Export UI store mock for use in tests
export { mockUIStore };

// Helper functions for test setup
export function setAuthenticatedState(overrides: Partial<AuthState> = {}) {
  mockAuthStore.value = { ...mockAuthenticatedState, ...overrides };
}

export function setAdminState(overrides: Partial<AuthState> = {}) {
  const adminUser = { ...mockUser, isAdmin: true };
  mockAuthStore.value = { 
    ...mockAuthenticatedState, 
    user: adminUser,
    ...overrides 
  };
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
  mockAuthStore.checkSetupStatus.mockClear();
  mockAuthStore.createInitialAdmin.mockClear();
  mockAuthStore.checkUsernameAvailability.mockClear();
  mockAuthStore.changeUsername.mockClear();
  mockAuthStore.changePassword.mockClear();
  
  // Clear UI store mocks
  mockUIStore.showSuccess.mockClear();
  mockUIStore.showError.mockClear();
  mockUIStore.showWarning.mockClear();
  mockUIStore.showInfo.mockClear();
  mockUIStore.addNotification.mockClear();
  mockUIStore.removeNotification.mockClear();
  mockUIStore.clearNotifications.mockClear();
  
  setUnauthenticatedState();
}