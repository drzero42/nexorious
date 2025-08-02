import { writable } from 'svelte/store';
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

export interface UserDeletionImpact {
  user_id: string;
  username: string;
  total_games: number;
  total_tags: number;
  total_wishlist_items: number;
  total_import_jobs: number;
  total_sessions: number;
  warning: string;
}

export interface SystemStatistics {
  totalUsers: number;
  totalAdmins: number;
  totalGames: number;
  recentUsers: AdminUser[];
}

export interface SeedDataResponse {
  platforms_added: number;
  storefronts_added: number;
  mappings_created: number;
  total_changes: number;
  message: string;
}

export interface AdminState {
  users: AdminUser[];
  statistics: SystemStatistics | null;
  isLoading: boolean;
  error: string | null;
  seedDataResult: SeedDataResponse | null;
  isSeedDataLoading: boolean;
}

const initialState: AdminState = {
  users: [],
  statistics: null,
  isLoading: false,
  error: null,
  seedDataResult: null,
  isSeedDataLoading: false
};

function mapBackendUserToFrontend(backendUser: any): AdminUser {
  return {
    id: backendUser.id,
    username: backendUser.username,
    isActive: backendUser.is_active,
    isAdmin: backendUser.is_admin,
    createdAt: backendUser.created_at,
    updatedAt: backendUser.updated_at
  };
}

function createAdminStore() {
  const { subscribe, set, update } = writable<AdminState>(initialState);

  return {
    subscribe,

    fetchUsers: async () => {
      if (!auth.value.user?.isAdmin) {
        throw new Error('Admin access required');
      }

      update(state => ({ ...state, isLoading: true, error: null }));

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

        const backendUsers = await response.json();
        const users = backendUsers.map(mapBackendUserToFrontend);
        update(state => ({ ...state, users, isLoading: false }));
        return users;
      } catch (error) {
        const errorMessage = error instanceof Error ? error.message : 'Failed to fetch users';
        update(state => ({ ...state, isLoading: false, error: errorMessage }));
        throw error;
      }
    },

    fetchStatistics: async () => {
      if (!auth.value.user?.isAdmin) {
        throw new Error('Admin access required');
      }

      update(state => ({ ...state, isLoading: true, error: null }));

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

        const backendUsers = await usersResponse.json();
        const users: AdminUser[] = backendUsers.map(mapBackendUserToFrontend);
        
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

        update(state => ({ ...state, statistics, isLoading: false }));
        return statistics;
      } catch (error) {
        const errorMessage = error instanceof Error ? error.message : 'Failed to fetch statistics';
        update(state => ({ ...state, isLoading: false, error: errorMessage }));
        throw error;
      }
    },

    createUser: async (username: string, password: string, isAdmin: boolean = false) => {
      if (!auth.value.user?.isAdmin) {
        throw new Error('Admin access required');
      }

      update(state => ({ ...state, isLoading: true, error: null }));

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

        const backendUser = await response.json();
        const newUser = mapBackendUserToFrontend(backendUser);
        update(state => ({ ...state, users: [...state.users, newUser], isLoading: false }));
        return newUser;
      } catch (error) {
        const errorMessage = error instanceof Error ? error.message : 'Failed to create user';
        update(state => ({ ...state, isLoading: false, error: errorMessage }));
        throw error;
      }
    },

    updateUser: async (userId: string, updates: { username?: string; isActive?: boolean; isAdmin?: boolean }) => {
      if (!auth.value.user?.isAdmin) {
        throw new Error('Admin access required');
      }

      update(state => ({ ...state, isLoading: true, error: null }));

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

        const backendUser = await response.json();
        const updatedUser = mapBackendUserToFrontend(backendUser);
        update(state => ({
          ...state,
          users: state.users.map(u => u.id === userId ? updatedUser : u),
          isLoading: false
        }));
        return updatedUser;
      } catch (error) {
        const errorMessage = error instanceof Error ? error.message : 'Failed to update user';
        update(state => ({ ...state, isLoading: false, error: errorMessage }));
        throw error;
      }
    },

    resetUserPassword: async (userId: string, newPassword: string) => {
      if (!auth.value.user?.isAdmin) {
        throw new Error('Admin access required');
      }

      update(state => ({ ...state, isLoading: true, error: null }));

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

        update(state => ({ ...state, isLoading: false }));
        return await response.json();
      } catch (error) {
        const errorMessage = error instanceof Error ? error.message : 'Failed to reset password';
        update(state => ({ ...state, isLoading: false, error: errorMessage }));
        throw error;
      }
    },

    getUserDeletionImpact: async (userId: string): Promise<UserDeletionImpact> => {
      if (!auth.value.user?.isAdmin) {
        throw new Error('Admin access required');
      }

      try {
        const response = await fetch(`${config.apiUrl}/auth/admin/users/${userId}/deletion-impact`, {
          headers: {
            'Authorization': `Bearer ${auth.value.accessToken}`
          }
        });

        if (!response.ok) {
          const errorData = await response.json().catch(() => ({}));
          throw new Error(errorData.detail || 'Failed to get deletion impact');
        }

        return await response.json();
      } catch (error) {
        throw error;
      }
    },

    deleteUser: async (userId: string) => {
      if (!auth.value.user?.isAdmin) {
        throw new Error('Admin access required');
      }

      update(state => ({ ...state, isLoading: true, error: null }));

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

        update(state => ({
          ...state,
          users: state.users.filter(u => u.id !== userId),
          isLoading: false
        }));
        return await response.json();
      } catch (error) {
        const errorMessage = error instanceof Error ? error.message : 'Failed to delete user';
        update(state => ({ ...state, isLoading: false, error: errorMessage }));
        throw error;
      }
    },

    getUserById: async (userId: string) => {
      if (!auth.value.user?.isAdmin) {
        throw new Error('Admin access required');
      }

      update(state => ({ ...state, isLoading: true, error: null }));

      try {
        const response = await fetch(`${config.apiUrl}/auth/admin/users/${userId}`, {
          headers: {
            'Authorization': `Bearer ${auth.value.accessToken}`
          }
        });

        if (!response.ok) {
          const errorData = await response.json().catch(() => ({}));
          throw new Error(errorData.detail || 'Failed to fetch user');
        }

        const backendUser = await response.json();
        const user = mapBackendUserToFrontend(backendUser);
        update(state => ({ ...state, isLoading: false }));
        return user;
      } catch (error) {
        const errorMessage = error instanceof Error ? error.message : 'Failed to fetch user';
        update(state => ({ ...state, isLoading: false, error: errorMessage }));
        throw error;
      }
    },

    loadSeedData: async () => {
      if (!auth.value.user?.isAdmin) {
        throw new Error('Admin access required');
      }

      update(state => ({ ...state, isSeedDataLoading: true, error: null, seedDataResult: null }));

      try {
        const response = await fetch(`${config.apiUrl}/platforms/seed`, {
          method: 'POST',
          headers: {
            'Authorization': `Bearer ${auth.value.accessToken}`
          }
        });

        if (!response.ok) {
          const errorData = await response.json().catch(() => ({}));
          throw new Error(errorData.detail || 'Failed to load seed data');
        }

        const seedDataResult: SeedDataResponse = await response.json();
        update(state => ({ ...state, seedDataResult, isSeedDataLoading: false }));
        return seedDataResult;
      } catch (error) {
        const errorMessage = error instanceof Error ? error.message : 'Failed to load seed data';
        update(state => ({ ...state, isSeedDataLoading: false, error: errorMessage }));
        throw error;
      }
    },

    clearError: () => {
      update(state => ({ ...state, error: null }));
    },

    clearSeedDataResult: () => {
      update(state => ({ ...state, seedDataResult: null }));
    },
    
    // Test helper - only use in tests
    __reset: () => {
      set(initialState);
    }
  };
}

export const admin = createAdminStore();