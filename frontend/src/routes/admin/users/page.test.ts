import { describe, it, expect, beforeEach, vi } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/svelte';
import AdminUsersPage from './+page.svelte';
import { admin, auth } from '$lib/stores';
import type { AdminUser } from '$lib/stores/admin.svelte';

// Mock stores
vi.mock('$lib/stores', () => ({
  admin: {
    value: {
      users: [],
      statistics: null,
      isLoading: false,
      error: null
    },
    fetchUsers: vi.fn(),
    clearError: vi.fn(),
    updateUser: vi.fn()
  },
  auth: {
    value: {
      user: { isAdmin: true },
      accessToken: 'test-token'
    }
  }
}));

// Mock navigation
vi.mock('$app/navigation', () => ({
  goto: vi.fn()
}));

describe('Admin Users Page', () => {
  const mockUsers: AdminUser[] = [
    {
      id: '1',
      username: 'admin',
      isAdmin: true,
      isActive: true,
      createdAt: '2023-01-01T00:00:00Z',
      updatedAt: '2023-01-01T00:00:00Z'
    },
    {
      id: '2',
      username: 'user1',
      isAdmin: false,
      isActive: true,
      createdAt: '2023-01-02T00:00:00Z',
      updatedAt: '2023-01-02T00:00:00Z'
    },
    {
      id: '3',
      username: 'user2',
      isAdmin: false,
      isActive: false,
      createdAt: '2023-01-03T00:00:00Z',
      updatedAt: '2023-01-03T00:00:00Z'
    }
  ];

  beforeEach(() => {
    vi.clearAllMocks();
    (admin as any).value = {
      users: mockUsers,
      statistics: null,
      isLoading: false,
      error: null
    };
  });

  it('renders the user management page', () => {
    render(AdminUsersPage);
    
    expect(screen.getByText('User Management')).toBeInTheDocument();
    expect(screen.getByText('Create User')).toBeInTheDocument();
  });

  it('displays users in the table', () => {
    render(AdminUsersPage);
    
    expect(screen.getByText('admin')).toBeInTheDocument();
    expect(screen.getByText('user1')).toBeInTheDocument();
    expect(screen.getByText('user2')).toBeInTheDocument();
  });

  it('shows admin badges for admin users', () => {
    render(AdminUsersPage);
    
    const adminBadges = screen.getAllByText('Admin');
    expect(adminBadges).toHaveLength(1);
  });

  it('shows inactive badges for inactive users', () => {
    render(AdminUsersPage);
    
    const inactiveBadges = screen.getAllByText('Inactive');
    expect(inactiveBadges).toHaveLength(1);
  });

  it('filters users by search query', async () => {
    render(AdminUsersPage);
    
    const searchInput = screen.getByPlaceholderText('Search by username...');
    await fireEvent.input(searchInput, { target: { value: 'admin' } });
    
    expect(screen.getByText('admin')).toBeInTheDocument();
    expect(screen.queryByText('user1')).not.toBeInTheDocument();
  });

  it('filters users by status', async () => {
    render(AdminUsersPage);
    
    const statusFilter = screen.getByDisplayValue('All Users');
    await fireEvent.change(statusFilter, { target: { value: 'admin' } });
    
    expect(screen.getByText('admin')).toBeInTheDocument();
    expect(screen.queryByText('user1')).not.toBeInTheDocument();
  });
});