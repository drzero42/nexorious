import { describe, it, expect, beforeEach, vi } from 'vitest';
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
    // Reset the admin store state
    admin.clearError();
    // Reset the users array
    admin.value.users = [];
    admin.value.statistics = null;
    admin.value.isLoading = false;
  });

  describe('fetchUsers', () => {
    it('should fetch users successfully', async () => {
      const mockUsers = [
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
        json: async () => mockUsers
      });

      const result = await admin.fetchUsers();

      expect(mockFetch).toHaveBeenCalledWith('http://localhost:8000/auth/admin/users', {
        headers: {
          'Authorization': 'Bearer mock-token'
        }
      });

      expect(result).toEqual(mockUsers);
      expect(admin.value.users).toEqual(mockUsers);
      expect(admin.value.isLoading).toBe(false);
      expect(admin.value.error).toBeNull();
    });

    it('should handle fetch users error', async () => {
      mockFetch.mockResolvedValueOnce({
        ok: false,
        json: async () => ({ detail: 'Unauthorized' })
      });

      await expect(admin.fetchUsers()).rejects.toThrow('Unauthorized');
      expect(admin.value.error).toBe('Unauthorized');
      expect(admin.value.isLoading).toBe(false);
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
      const mockUsers = [
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
        json: async () => mockUsers
      });

      const result = await admin.fetchStatistics();

      expect(result.totalUsers).toBe(2);
      expect(result.totalAdmins).toBe(1);
      expect(result.totalGames).toBe(0);
      expect(result.recentUsers).toEqual(mockUsers.slice(0, 5));
      expect(admin.value.statistics).toEqual(result);
    });

    it('should handle fetch statistics error', async () => {
      mockFetch.mockResolvedValueOnce({
        ok: false,
        json: async () => ({ detail: 'Server error' })
      });

      await expect(admin.fetchStatistics()).rejects.toThrow('Failed to fetch users');
      expect(admin.value.error).toBe('Failed to fetch users');
    });
  });

  describe('createUser', () => {
    it('should create user successfully', async () => {
      const mockNewUser = {
        id: '3',
        username: 'newuser',
        isAdmin: false,
        isActive: true,
        createdAt: '2023-01-03T00:00:00Z',
        updatedAt: '2023-01-03T00:00:00Z'
      };

      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: async () => mockNewUser
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

      expect(result).toEqual(mockNewUser);
      expect(admin.value.users).toHaveLength(1);
      expect(admin.value.users[0]).toEqual(mockNewUser);
    });

    it('should handle create user error', async () => {
      mockFetch.mockResolvedValueOnce({
        ok: false,
        json: async () => ({ detail: 'Username already taken' })
      });

      await expect(admin.createUser('existing', 'password123')).rejects.toThrow('Username already taken');
      expect(admin.value.error).toBe('Username already taken');
    });
  });

  describe('updateUser', () => {
    it('should update user successfully', async () => {
      // Set initial users
      admin.value.users = [
        {
          id: '1',
          username: 'testuser',
          isAdmin: false,
          isActive: true,
          createdAt: '2023-01-01T00:00:00Z',
          updatedAt: '2023-01-01T00:00:00Z'
        }
      ];

      const mockUpdatedUser = {
        id: '1',
        username: 'updateduser',
        isAdmin: false,
        isActive: true,
        createdAt: '2023-01-01T00:00:00Z',
        updatedAt: '2023-01-03T00:00:00Z'
      };

      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: async () => mockUpdatedUser
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

      expect(result).toEqual(mockUpdatedUser);
      expect(admin.value.users[0]).toEqual(mockUpdatedUser);
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
      // Set initial users
      admin.value.users = [
        {
          id: '1',
          username: 'user1',
          isAdmin: false,
          isActive: true,
          createdAt: '2023-01-01T00:00:00Z',
          updatedAt: '2023-01-01T00:00:00Z'
        },
        {
          id: '2',
          username: 'user2',
          isAdmin: false,
          isActive: true,
          createdAt: '2023-01-02T00:00:00Z',
          updatedAt: '2023-01-02T00:00:00Z'
        }
      ];

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
      expect(admin.value.users).toHaveLength(1);
      expect(admin.value.users[0].id).toBe('2');
    });
  });

  describe('clearError', () => {
    it('should clear error', () => {
      // Set an error first
      admin.value.error = 'Some error';
      
      admin.clearError();
      
      expect(admin.value.error).toBeNull();
    });
  });
});