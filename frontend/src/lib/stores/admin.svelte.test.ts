import { describe, it, expect, beforeEach, vi } from 'vitest';
import { get } from 'svelte/store';
import { admin } from './admin.svelte';
import { auth } from './auth.svelte';

// Mock the auth store
vi.mock('./auth.svelte', () => ({
  auth: {
    value: {
      user: { id: '1', username: 'admin', isAdmin: true },
      accessToken: 'mock-token',
      refreshToken: 'mock-refresh-token',
      isLoading: false,
      error: null
    }
  }
}));

// Mock the config
vi.mock('$lib/env', () => ({
  config: {
    apiUrl: 'http://localhost:8000'
  }
}));

// Mock fetch
const mockFetch = vi.fn();
global.fetch = mockFetch;

describe('Admin Store', () => {
  beforeEach(() => {
    mockFetch.mockClear();
    // Reset the admin store state to initial state
    admin.__reset();
  });

  describe('fetchUsers', () => {
    it('should fetch users successfully', async () => {
      const mockBackendUsers = [
        {
          id: '1',
          username: 'admin',
          is_admin: true,
          is_active: true,
          created_at: '2023-01-01T00:00:00Z',
          updated_at: '2023-01-01T00:00:00Z'
        },
        {
          id: '2',
          username: 'user',
          is_admin: false,
          is_active: true,
          created_at: '2023-01-02T00:00:00Z',
          updated_at: '2023-01-02T00:00:00Z'
        }
      ];

      const expectedFrontendUsers = [
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
          username: 'user',
          isAdmin: false,
          isActive: true,
          createdAt: '2023-01-02T00:00:00Z',
          updatedAt: '2023-01-02T00:00:00Z'
        }
      ];

      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: async () => mockBackendUsers
      });

      const result = await admin.fetchUsers();

      expect(mockFetch).toHaveBeenCalledWith('http://localhost:8000/auth/admin/users', {
        headers: {
          'Authorization': 'Bearer mock-token'
        }
      });

      expect(result).toEqual(expectedFrontendUsers);
      const state = get(admin);
      expect(state.users).toEqual(expectedFrontendUsers);
      expect(state.isLoading).toBe(false);
      expect(state.error).toBeNull();
    });

    it('should handle fetch users error', async () => {
      mockFetch.mockResolvedValueOnce({
        ok: false,
        json: async () => ({ detail: 'Unauthorized' })
      });

      await expect(admin.fetchUsers()).rejects.toThrow('Unauthorized');
      expect(get(admin).error).toBe('Unauthorized');
      expect(get(admin).isLoading).toBe(false);
    });

    it('should throw error if user is not admin', async () => {
      // Mock non-admin user
      vi.mocked(auth.value).user = { id: '1', username: 'user', isAdmin: false };

      await expect(admin.fetchUsers()).rejects.toThrow('Admin access required');

      // Restore admin user
      vi.mocked(auth.value).user = { id: '1', username: 'admin', isAdmin: true };
    });
  });

  describe('fetchStatistics', () => {
    it('should fetch and calculate statistics successfully', async () => {
      const mockBackendUsers = [
        {
          id: '1',
          username: 'admin',
          is_admin: true,
          is_active: true,
          created_at: '2023-01-01T00:00:00Z',
          updated_at: '2023-01-01T00:00:00Z'
        },
        {
          id: '2',
          username: 'user',
          is_admin: false,
          is_active: true,
          created_at: '2023-01-02T00:00:00Z',
          updated_at: '2023-01-02T00:00:00Z'
        }
      ];

      const expectedFrontendUsers = [
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
          username: 'user',
          isAdmin: false,
          isActive: true,
          createdAt: '2023-01-02T00:00:00Z',
          updatedAt: '2023-01-02T00:00:00Z'
        }
      ];

      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: async () => mockBackendUsers
      });

      const result = await admin.fetchStatistics();

      expect(result.totalUsers).toBe(2);
      expect(result.totalAdmins).toBe(1);
      expect(result.totalGames).toBe(0);
      // Recent users should be sorted by creation date descending (most recent first)
      const expectedRecentUsers = expectedFrontendUsers
        .sort((a, b) => new Date(b.createdAt).getTime() - new Date(a.createdAt).getTime())
        .slice(0, 5);
      expect(result.recentUsers).toEqual(expectedRecentUsers);
      expect(get(admin).statistics).toEqual(result);
    });

    it('should handle fetch statistics error', async () => {
      mockFetch.mockResolvedValueOnce({
        ok: false,
        json: async () => ({ detail: 'Server error' })
      });

      await expect(admin.fetchStatistics()).rejects.toThrow('Failed to fetch users');
      expect(get(admin).error).toBe('Failed to fetch users');
    });
  });

  describe('createUser', () => {
    it('should create user successfully', async () => {
      const mockBackendUser = {
        id: '3',
        username: 'newuser',
        is_admin: false,
        is_active: true,
        created_at: '2023-01-03T00:00:00Z',
        updated_at: '2023-01-03T00:00:00Z'
      };

      const expectedFrontendUser = {
        id: '3',
        username: 'newuser',
        isAdmin: false,
        isActive: true,
        createdAt: '2023-01-03T00:00:00Z',
        updatedAt: '2023-01-03T00:00:00Z'
      };

      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: async () => mockBackendUser
      });

      const result = await admin.createUser('newuser', 'password123', false);

      expect(mockFetch).toHaveBeenCalledWith('http://localhost:8000/auth/admin/users', {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          'Authorization': 'Bearer mock-token'
        },
        body: JSON.stringify({
          username: 'newuser',
          password: 'password123',
          is_admin: false
        })
      });

      expect(result).toEqual(expectedFrontendUser);
      const state = get(admin);
      expect(state.users).toHaveLength(1);
      expect(state.users[0]).toEqual(expectedFrontendUser);
    });

    it('should handle create user error', async () => {
      mockFetch.mockResolvedValueOnce({
        ok: false,
        json: async () => ({ detail: 'Username already taken' })
      });

      await expect(admin.createUser('existing', 'password123')).rejects.toThrow('Username already taken');
      expect(get(admin).error).toBe('Username already taken');
    });
  });

  describe('updateUser', () => {
    it('should update user successfully', async () => {
      // Set initial users by mocking fetchUsers response
      const initialUsers = [{
        id: '1',
        username: 'testuser',
        is_admin: false,
        is_active: true,
        created_at: '2023-01-01T00:00:00Z',
        updated_at: '2023-01-01T00:00:00Z'
      }];
      
      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: async () => initialUsers
      });
      
      await admin.fetchUsers();

      const mockBackendUpdatedUser = {
        id: '1',
        username: 'updateduser',
        is_admin: false,
        is_active: true,
        created_at: '2023-01-01T00:00:00Z',
        updated_at: '2023-01-03T00:00:00Z'
      };

      const expectedFrontendUpdatedUser = {
        id: '1',
        username: 'updateduser',
        isAdmin: false,
        isActive: true,
        createdAt: '2023-01-01T00:00:00Z',
        updatedAt: '2023-01-03T00:00:00Z'
      };

      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: async () => mockBackendUpdatedUser
      });

      const result = await admin.updateUser('1', { username: 'updateduser' });

      expect(mockFetch).toHaveBeenCalledWith('http://localhost:8000/auth/admin/users/1', {
        method: 'PUT',
        headers: {
          'Content-Type': 'application/json',
          'Authorization': 'Bearer mock-token'
        },
        body: JSON.stringify({
          username: 'updateduser',
          is_active: undefined,
          is_admin: undefined
        })
      });

      expect(result).toEqual(expectedFrontendUpdatedUser);
      expect(get(admin).users[0]).toEqual(expectedFrontendUpdatedUser);
    });
  });

  describe('resetUserPassword', () => {
    it('should reset user password successfully', async () => {
      const mockResponse = { message: 'Password reset successfully' };

      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: async () => mockResponse
      });

      const result = await admin.resetUserPassword('1', 'newpassword123');

      expect(mockFetch).toHaveBeenCalledWith('http://localhost:8000/auth/admin/users/1/password', {
        method: 'PUT',
        headers: {
          'Content-Type': 'application/json',
          'Authorization': 'Bearer mock-token'
        },
        body: JSON.stringify({
          new_password: 'newpassword123'
        })
      });

      expect(result).toEqual(mockResponse);
    });
  });

  describe('deleteUser', () => {
    it('should delete user successfully', async () => {
      // Set initial users by mocking fetchUsers response
      const initialUsers = [
        {
          id: '1',
          username: 'user1',
          is_admin: false,
          is_active: true,
          created_at: '2023-01-01T00:00:00Z',
          updated_at: '2023-01-01T00:00:00Z'
        },
        {
          id: '2',
          username: 'user2',
          is_admin: false,
          is_active: true,
          created_at: '2023-01-02T00:00:00Z',
          updated_at: '2023-01-02T00:00:00Z'
        }
      ];
      
      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: async () => initialUsers
      });
      
      await admin.fetchUsers();

      const mockResponse = { message: 'User deleted successfully' };

      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: async () => mockResponse
      });

      const result = await admin.deleteUser('1');

      expect(mockFetch).toHaveBeenCalledWith('http://localhost:8000/auth/admin/users/1', {
        method: 'DELETE',
        headers: {
          'Authorization': 'Bearer mock-token'
        }
      });

      expect(result).toEqual(mockResponse);
      const state = get(admin);
      expect(state.users).toHaveLength(1);
      expect(state.users[0]?.id).toBe('2');
    });
  });

  describe('getUserById', () => {
    it('should fetch user by ID successfully', async () => {
      const mockBackendUser = {
        id: '1',
        username: 'testuser',
        is_admin: false,
        is_active: true,
        created_at: '2023-01-01T00:00:00Z',
        updated_at: '2023-01-01T00:00:00Z'
      };

      const expectedFrontendUser = {
        id: '1',
        username: 'testuser',
        isAdmin: false,
        isActive: true,
        createdAt: '2023-01-01T00:00:00Z',
        updatedAt: '2023-01-01T00:00:00Z'
      };

      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: async () => mockBackendUser
      });

      const result = await admin.getUserById('1');

      expect(mockFetch).toHaveBeenCalledWith('http://localhost:8000/auth/admin/users/1', {
        headers: {
          'Authorization': 'Bearer mock-token'
        }
      });

      expect(result).toEqual(expectedFrontendUser);
      const state = get(admin);
      expect(state.isLoading).toBe(false);
      expect(state.error).toBeNull();
    });

    it('should handle fetch user by ID error', async () => {
      mockFetch.mockResolvedValueOnce({
        ok: false,
        json: async () => ({ detail: 'User not found' })
      });

      await expect(admin.getUserById('999')).rejects.toThrow('User not found');
      expect(get(admin).error).toBe('User not found');
      expect(get(admin).isLoading).toBe(false);
    });

    it('should throw error if user is not admin', async () => {
      // Mock non-admin user
      vi.mocked(auth.value).user = { id: '1', username: 'user', isAdmin: false };

      await expect(admin.getUserById('1')).rejects.toThrow('Admin access required');

      // Restore admin user
      vi.mocked(auth.value).user = { id: '1', username: 'admin', isAdmin: true };
    });

    it('should handle network error', async () => {
      mockFetch.mockRejectedValueOnce(new Error('Network error'));

      await expect(admin.getUserById('1')).rejects.toThrow('Network error');
      expect(get(admin).error).toBe('Network error');
      expect(get(admin).isLoading).toBe(false);
    });

    it('should handle malformed JSON response', async () => {
      mockFetch.mockResolvedValueOnce({
        ok: false,
        json: async () => { throw new Error('Invalid JSON'); }
      });

      await expect(admin.getUserById('1')).rejects.toThrow('Failed to fetch user');
      expect(get(admin).error).toBe('Failed to fetch user');
    });
  });

  describe('clearError', () => {
    it('should clear error', async () => {
      // Set an error first by triggering a failed request
      mockFetch.mockResolvedValueOnce({
        ok: false,
        json: async () => ({ detail: 'Test error' })
      });
      
      try {
        await admin.fetchUsers();
      } catch (e) {
        // Expected to throw
      }
      
      expect(get(admin).error).toBe('Test error');
      
      admin.clearError();
      
      expect(get(admin).error).toBeNull();
    });
  });
});