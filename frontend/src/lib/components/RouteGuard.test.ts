import { describe, it, expect, beforeEach, vi } from 'vitest';
import { render } from '@testing-library/svelte';

// Hoisted mocks
const { mockGoto, mockAuthStore } = vi.hoisted(() => {
  const mockGoto = vi.fn();
  const mockAuthStore = {
    value: {
      user: null as any,
      accessToken: null as any,
      refreshToken: null as any
    },
    refreshAuth: vi.fn(),
    login: vi.fn(),
    logout: vi.fn()
  };
  
  return { mockGoto, mockAuthStore };
});

vi.mock('$lib/stores', () => ({
  auth: mockAuthStore
}));

vi.mock('$app/navigation', () => ({
  goto: mockGoto
}));

vi.mock('$app/environment', () => ({
  browser: true,
  dev: false
}));

// Import component after mocks
import RouteGuard from './RouteGuard.svelte';

// Helper functions
function setAuthenticatedState(authData: { user: { id: string; username: string; isAdmin: boolean } }) {
  mockAuthStore.value = {
    ...mockAuthStore.value,
    ...authData
  };
}

function setUnauthenticatedState() {
  mockAuthStore.value = {
    user: null,
    accessToken: null,
    refreshToken: null
  };
}

function resetMocks() {
  vi.clearAllMocks();
  mockGoto.mockClear();
  mockAuthStore.refreshAuth.mockClear();
  mockAuthStore.login.mockClear();
  mockAuthStore.logout.mockClear();
}

describe('RouteGuard', () => {
  beforeEach(() => {
    resetMocks();
    setUnauthenticatedState();
  });

  it('should render loading state initially', () => {
    setUnauthenticatedState();
    const { container } = render(RouteGuard, { props: { requireAuth: false } });
    
    // Component should render (it shows loading divs)
    expect(container.firstChild).toBeTruthy();
  });

  it('should render when not require auth and user not authenticated', async () => {
    setUnauthenticatedState();
    
    const { container } = render(RouteGuard, { 
      props: { requireAuth: false }
    });
    
    // Should not redirect when requireAuth is false
    expect(mockGoto).not.toHaveBeenCalledWith('/login');
    expect(container.firstChild).toBeTruthy();
  });

  it('should redirect to login when requireAuth=true and user not authenticated', async () => {
    setUnauthenticatedState();
    
    render(RouteGuard, { props: { requireAuth: true } });
    
    // Wait for onMount to complete
    await vi.waitFor(() => {
      expect(mockGoto).toHaveBeenCalledWith('/login');
    });
  });

  it('should redirect to custom path when specified', async () => {
    setUnauthenticatedState();
    
    render(RouteGuard, { props: { requireAuth: true, redirectTo: '/custom-login' } });
    
    await vi.waitFor(() => {
      expect(mockGoto).toHaveBeenCalledWith('/custom-login');
    });
  });

  it('should not redirect when user is authenticated', async () => {
    setAuthenticatedState({
      user: { id: '1', username: 'testuser', isAdmin: false }
    });
    
    const { container } = render(RouteGuard, { 
      props: { requireAuth: true }
    });
    
    // Should not redirect when user is authenticated
    expect(mockGoto).not.toHaveBeenCalledWith('/login');
    expect(container.firstChild).toBeTruthy();
  });

  it('should redirect non-admin users when requireAdmin=true', async () => {
    setAuthenticatedState({
      user: { id: '1', username: 'testuser', isAdmin: false }
    });
    
    render(RouteGuard, { props: { requireAuth: true, requireAdmin: true } });
    
    await vi.waitFor(() => {
      expect(mockGoto).toHaveBeenCalledWith('/');
    });
  });

  it('should not redirect admin users when requireAdmin=true', async () => {
    setAuthenticatedState({
      user: { id: '1', username: 'admin', isAdmin: true }
    });
    
    const { container } = render(RouteGuard, { 
      props: { requireAuth: true, requireAdmin: true }
    });
    
    // Should not redirect admin users
    expect(mockGoto).not.toHaveBeenCalledWith('/');
    expect(container.firstChild).toBeTruthy();
  });

  it('should redirect authenticated users when requireAuth=false', async () => {
    setAuthenticatedState({
      user: { id: '1', username: 'testuser', isAdmin: false }
    });
    
    render(RouteGuard, { props: { requireAuth: false } });
    
    await vi.waitFor(() => {
      expect(mockGoto).toHaveBeenCalledWith('/games');
    });
  });

  it('should handle auth refresh failure', async () => {
    // Set state with tokens but no user (simulating expired tokens)
    mockAuthStore.value = {
      user: null,
      accessToken: 'expired-token',
      refreshToken: 'refresh-token'
    };
    mockAuthStore.refreshAuth.mockRejectedValue(new Error('Refresh failed'));
    
    render(RouteGuard, { props: { requireAuth: true } });
    
    await vi.waitFor(() => {
      expect(mockAuthStore.refreshAuth).toHaveBeenCalled();
      expect(mockGoto).toHaveBeenCalledWith('/login');
    });
  });

  it('should handle successful auth refresh', async () => {
    // Set state with tokens but no user
    mockAuthStore.value = {
      user: null,
      accessToken: 'expired-token',
      refreshToken: 'refresh-token'
    };
    
    // Mock successful refresh that sets user
    mockAuthStore.refreshAuth.mockImplementation(async () => {
      mockAuthStore.value.user = { id: '1', username: 'testuser', isAdmin: false };
      return true;
    });
    
    const { container } = render(RouteGuard, { 
      props: { requireAuth: true }
    });
    
    await vi.waitFor(() => {
      expect(mockAuthStore.refreshAuth).toHaveBeenCalled();
      expect(container.firstChild).toBeTruthy();
    });
  });

  it('should handle auth state without tokens', async () => {
    // Test case where there are no tokens at all
    mockAuthStore.value = {
      user: null,
      accessToken: null,
      refreshToken: null
    };
    
    render(RouteGuard, { props: { requireAuth: true } });
    
    await vi.waitFor(() => {
      expect(mockGoto).toHaveBeenCalledWith('/login');
    });
  });

  it('should handle multiple prop combinations', async () => {
    // Test various prop combinations
    setAuthenticatedState({
      user: { id: '1', username: 'testuser', isAdmin: false }
    });
    
    const { container } = render(RouteGuard, { 
      props: { 
        requireAuth: true, 
        requireAdmin: false,
        redirectTo: '/custom-login'
      }
    });
    
    // Should render without redirecting since user is authenticated but not admin required
    expect(mockGoto).not.toHaveBeenCalled();
    expect(container.firstChild).toBeTruthy();
  });
});