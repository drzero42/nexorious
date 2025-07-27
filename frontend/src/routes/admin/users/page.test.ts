import { describe, it, expect, vi } from 'vitest';
import { render } from '@testing-library/svelte';
import AdminUsersPage from './+page.svelte';

// Mock navigation
vi.mock('$app/navigation', () => ({
  goto: vi.fn()
}));

// Mock RouteGuard as a simple component that renders its slot
vi.mock('$lib/components', () => ({
  RouteGuard: vi.fn(({ children }: { children: any }) => {
    // In test environment, just render the children
    return children;
  })
}));

// Mock the stores 
vi.mock('$lib/stores', () => ({
  admin: {
    value: {
      users: [
        {
          id: '1',
          username: 'admin',
          isAdmin: true,
          isActive: true,
          createdAt: '2023-01-01T00:00:00Z',
          updatedAt: '2023-01-01T00:00:00Z'
        }
      ],
      statistics: null,
      isLoading: false,
      error: null
    },
    fetchUsers: vi.fn().mockResolvedValue([]),
    clearError: vi.fn(),
    updateUser: vi.fn().mockResolvedValue({})
  },
  auth: {
    value: {
      user: { isAdmin: true },
      accessToken: 'test-token',
      isAuthenticated: true
    }
  }
}));

describe('Admin Users Page', () => {
  it('renders without crashing', () => {
    // This test just ensures the component can be rendered without errors
    expect(() => {
      render(AdminUsersPage);
    }).not.toThrow();
  });

  it('component mounts successfully', () => {
    const { container } = render(AdminUsersPage);
    
    // Check that the component has mounted
    expect(container.firstChild).toBeTruthy();
  });
});