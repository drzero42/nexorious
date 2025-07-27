import { config } from '$lib/env';
import { auth } from './auth.svelte';

export interface AdminUser {
  id: string;
  username: string;
  isAdmin: boolean;
  isActive: boolean;
  createdAt: string;
  updatedAt: string;
}

export interface SystemStatistics {
  totalUsers: number;
  totalAdmins: number;
  totalGames: number;
  recentUsers: AdminUser[];
}

export interface AdminState {
  users: AdminUser[];
  statistics: SystemStatistics | null;
  isLoading: boolean;
  error: string | null;
}

const initialState: AdminState = {
  users: [],
  statistics: null,
  isLoading: false,
  error: null
};

function createAdminStore() {
  let state = $state<AdminState>(initialState);

  const adminStore = {
    get value() {
      return state;
    },

    fetchUsers: async () => {
      if (!auth.value.user?.isAdmin) {
        throw new Error('Admin access required');
      }

      state = { ...state, isLoading: true, error: null };

      try {
        const response = await fetch(`${config.apiUrl}/auth/admin/users`, {
          headers: {
            'Authorization': `Bearer ${auth.value.accessToken}`
          }
        });

        if (!response.ok) {
          const errorData = await response.json().catch(() => ({}));
          throw new Error(errorData.detail || 'Failed to fetch users');
        }

        const users = await response.json();
        state = { ...state, users, isLoading: false };
        return users;
      } catch (error) {
        const errorMessage = error instanceof Error ? error.message : 'Failed to fetch users';
        state = { ...state, isLoading: false, error: errorMessage };
        throw error;
      }
    },

    fetchStatistics: async () => {
      if (!auth.value.user?.isAdmin) {
        throw new Error('Admin access required');
      }

      state = { ...state, isLoading: true, error: null };

      try {
        // Fetch users first to calculate statistics
        const usersResponse = await fetch(`${config.apiUrl}/auth/admin/users`, {
          headers: {
            'Authorization': `Bearer ${auth.value.accessToken}`
          }
        });

        if (!usersResponse.ok) {
          throw new Error('Failed to fetch users');
        }

        const users: AdminUser[] = await usersResponse.json();
        
        // For now, we'll calculate statistics from the users data
        // In the future, we might want a dedicated statistics endpoint
        const totalUsers = users.length;
        const totalAdmins = users.filter(u => u.isAdmin).length;
        
        // Get recent users (last 5, sorted by creation date)
        const recentUsers = users
          .sort((a, b) => new Date(b.createdAt).getTime() - new Date(a.createdAt).getTime())
          .slice(0, 5);

        const statistics: SystemStatistics = {
          totalUsers,
          totalAdmins,
          totalGames: 0, // TODO: Implement when games statistics endpoint is available
          recentUsers
        };

        state = { ...state, statistics, isLoading: false };
        return statistics;
      } catch (error) {
        const errorMessage = error instanceof Error ? error.message : 'Failed to fetch statistics';
        state = { ...state, isLoading: false, error: errorMessage };
        throw error;
      }
    },

    createUser: async (username: string, password: string, isAdmin: boolean = false) => {
      if (!auth.value.user?.isAdmin) {
        throw new Error('Admin access required');
      }

      state = { ...state, isLoading: true, error: null };

      try {
        const response = await fetch(`${config.apiUrl}/auth/admin/users`, {
          method: 'POST',
          headers: {
            'Content-Type': 'application/json',
            'Authorization': `Bearer ${auth.value.accessToken}`
          },
          body: JSON.stringify({ username, password, is_admin: isAdmin })
        });

        if (!response.ok) {
          const errorData = await response.json().catch(() => ({}));
          throw new Error(errorData.detail || 'Failed to create user');
        }

        const newUser = await response.json();
        state = { ...state, users: [...state.users, newUser], isLoading: false };
        return newUser;
      } catch (error) {
        const errorMessage = error instanceof Error ? error.message : 'Failed to create user';
        state = { ...state, isLoading: false, error: errorMessage };
        throw error;
      }
    },

    updateUser: async (userId: string, updates: { username?: string; isActive?: boolean; isAdmin?: boolean }) => {
      if (!auth.value.user?.isAdmin) {
        throw new Error('Admin access required');
      }

      state = { ...state, isLoading: true, error: null };

      try {
        const response = await fetch(`${config.apiUrl}/auth/admin/users/${userId}`, {
          method: 'PUT',
          headers: {
            'Content-Type': 'application/json',
            'Authorization': `Bearer ${auth.value.accessToken}`
          },
          body: JSON.stringify({
            username: updates.username,
            is_active: updates.isActive,
            is_admin: updates.isAdmin
          })
        });

        if (!response.ok) {
          const errorData = await response.json().catch(() => ({}));
          throw new Error(errorData.detail || 'Failed to update user');
        }

        const updatedUser = await response.json();
        state = {
          ...state,
          users: state.users.map(u => u.id === userId ? updatedUser : u),
          isLoading: false
        };
        return updatedUser;
      } catch (error) {
        const errorMessage = error instanceof Error ? error.message : 'Failed to update user';
        state = { ...state, isLoading: false, error: errorMessage };
        throw error;
      }
    },

    resetUserPassword: async (userId: string, newPassword: string) => {
      if (!auth.value.user?.isAdmin) {
        throw new Error('Admin access required');
      }

      state = { ...state, isLoading: true, error: null };

      try {
        const response = await fetch(`${config.apiUrl}/auth/admin/users/${userId}/password`, {
          method: 'PUT',
          headers: {
            'Content-Type': 'application/json',
            'Authorization': `Bearer ${auth.value.accessToken}`
          },
          body: JSON.stringify({ new_password: newPassword })
        });

        if (!response.ok) {
          const errorData = await response.json().catch(() => ({}));
          throw new Error(errorData.detail || 'Failed to reset password');
        }

        state = { ...state, isLoading: false };
        return await response.json();
      } catch (error) {
        const errorMessage = error instanceof Error ? error.message : 'Failed to reset password';
        state = { ...state, isLoading: false, error: errorMessage };
        throw error;
      }
    },

    deleteUser: async (userId: string) => {
      if (!auth.value.user?.isAdmin) {
        throw new Error('Admin access required');
      }

      state = { ...state, isLoading: true, error: null };

      try {
        const response = await fetch(`${config.apiUrl}/auth/admin/users/${userId}`, {
          method: 'DELETE',
          headers: {
            'Authorization': `Bearer ${auth.value.accessToken}`
          }
        });

        if (!response.ok) {
          const errorData = await response.json().catch(() => ({}));
          throw new Error(errorData.detail || 'Failed to delete user');
        }

        state = {
          ...state,
          users: state.users.filter(u => u.id !== userId),
          isLoading: false
        };
        return await response.json();
      } catch (error) {
        const errorMessage = error instanceof Error ? error.message : 'Failed to delete user';
        state = { ...state, isLoading: false, error: errorMessage };
        throw error;
      }
    },

    clearError: () => {
      state = { ...state, error: null };
    }
  };

  return adminStore;
}

export const admin = createAdminStore();