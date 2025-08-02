import { writable } from 'svelte/store';
import { auth } from './auth.svelte';
import { config } from '$lib/env';

export interface Platform {
  id: string;
  name: string;
  display_name: string;
  icon_url?: string;
  is_active: boolean;
  source: string;
  version_added?: string;
  default_storefront_id?: string | undefined;
  storefronts?: Storefront[];
  created_at: string;
  updated_at: string;
}

export interface Storefront {
  id: string;
  name: string;
  display_name: string;
  icon_url?: string;
  base_url?: string;
  is_active: boolean;
  source: string;
  version_added?: string;
  created_at: string;
  updated_at: string;
}

export interface PlatformCreateRequest {
  name: string;
  display_name: string;
  icon_url?: string;
  is_active?: boolean;
  default_storefront_id?: string;
}

export interface PlatformUpdateRequest {
  display_name?: string;
  icon_url?: string;
  is_active?: boolean;
  default_storefront_id?: string | null;
}

export interface StorefrontCreateRequest {
  name: string;
  display_name: string;
  icon_url?: string;
  base_url?: string;
  is_active?: boolean;
}

export interface StorefrontUpdateRequest {
  display_name?: string;
  icon_url?: string;
  base_url?: string;
  is_active?: boolean;
}


export interface PlatformsState {
  platforms: Platform[];
  storefronts: Storefront[];
  isLoading: boolean;
  error: string | null;
}

const initialState: PlatformsState = {
  platforms: [],
  storefronts: [],
  isLoading: false,
  error: null
};

function createPlatformsStore() {
  const { subscribe, set, update } = writable<PlatformsState>(initialState);

  const apiCall = async (url: string, options: RequestInit = {}) => {
    const authState = auth.value;
    if (!authState.accessToken) {
      throw new Error('Not authenticated');
    }

    const response = await fetch(url, {
      ...options,
      headers: {
        'Content-Type': 'application/json',
        'Authorization': `Bearer ${authState.accessToken}`,
        ...options.headers,
      },
    });

    if (!response.ok) {
      if (response.status === 401) {
        // Try to refresh token
        const refreshed = await auth.refreshAuth();
        if (refreshed) {
          // Retry the request with new token
          return fetch(url, {
            ...options,
            headers: {
              'Content-Type': 'application/json',
              'Authorization': `Bearer ${auth.value.accessToken}`,
              ...options.headers,
            },
          });
        }
      }
      throw new Error(`HTTP ${response.status}: ${response.statusText}`);
    }

    return response;
  };

  return {
    subscribe,

    // Fetch all platforms (loads ALL platforms regardless of active status)
    fetchPlatforms: async () => {
      if (!auth.value.user?.isAdmin) {
        throw new Error('Admin access required');
      }

      update(state => ({ ...state, isLoading: true, error: null }));

      try {
        // Load ALL platforms without active_only filter (active_only=false)
        const response = await apiCall(`${config.apiUrl}/platforms/?active_only=false`);
        const data = await response.json();

        update(state => ({
          ...state,
          platforms: data.platforms,
          isLoading: false
        }));
        return data.platforms;
      } catch (error) {
        const errorMessage = error instanceof Error ? error.message : 'Failed to fetch platforms';
        update(state => ({ ...state, isLoading: false, error: errorMessage }));
        throw error;
      }
    },

    // Fetch all storefronts (loads ALL storefronts regardless of active status)
    fetchStorefronts: async () => {
      if (!auth.value.user?.isAdmin) {
        throw new Error('Admin access required');
      }

      update(state => ({ ...state, isLoading: true, error: null }));

      try {
        // Load ALL storefronts without active_only filter (active_only=false)
        const response = await apiCall(`${config.apiUrl}/platforms/storefronts/?active_only=false`);
        const data = await response.json();

        update(state => ({
          ...state,
          storefronts: data.storefronts,
          isLoading: false
        }));
        return data.storefronts;
      } catch (error) {
        const errorMessage = error instanceof Error ? error.message : 'Failed to fetch storefronts';
        update(state => ({ ...state, isLoading: false, error: errorMessage }));
        throw error;
      }
    },

    // Fetch both platforms and storefronts (accessible to all authenticated users)  
    fetchAll: async () => {
      update(state => ({ ...state, isLoading: true, error: null }));

      try {
        // Load ALL platforms and storefronts in parallel
        const [platformsResponse, storefrontsResponse] = await Promise.all([
          apiCall(`${config.apiUrl}/platforms/?active_only=false`),
          apiCall(`${config.apiUrl}/platforms/storefronts/?active_only=false`)
        ]);

        const [platformsData, storefrontsData] = await Promise.all([
          platformsResponse.json(),
          storefrontsResponse.json()
        ]);

        update(state => ({
          ...state,
          platforms: platformsData.platforms,
          storefronts: storefrontsData.storefronts,
          isLoading: false
        }));

        return {
          platforms: platformsData.platforms,
          storefronts: storefrontsData.storefronts
        };
      } catch (error) {
        const errorMessage = error instanceof Error ? error.message : 'Failed to fetch platforms and storefronts';
        update(state => ({ ...state, isLoading: false, error: errorMessage }));
        throw error;
      }
    },

    // Create a new platform (admin only)
    createPlatform: async (platformData: PlatformCreateRequest) => {
      if (!auth.value.user?.isAdmin) {
        throw new Error('Admin access required');
      }

      update(state => ({ ...state, isLoading: true, error: null }));

      try {
        // Clean the data - convert empty strings to undefined for optional URL fields
        const cleanedData = {
          ...platformData,
          icon_url: platformData.icon_url?.trim() || undefined
        };

        const response = await apiCall(`${config.apiUrl}/platforms/`, {
          method: 'POST',
          body: JSON.stringify(cleanedData),
        });
        
        const platform: Platform = await response.json();

        update(state => ({
          ...state,
          platforms: [...state.platforms, platform],
          isLoading: false
        }));

        return platform;
      } catch (error) {
        const errorMessage = error instanceof Error ? error.message : 'Failed to create platform';
        update(state => ({ ...state, isLoading: false, error: errorMessage }));
        throw error;
      }
    },

    // Update an existing platform (admin only)
    updatePlatform: async (id: string, platformData: PlatformUpdateRequest) => {
      if (!auth.value.user?.isAdmin) {
        throw new Error('Admin access required');
      }

      update(state => ({ ...state, isLoading: true, error: null }));

      try {
        // Clean the data - convert empty strings to undefined for optional URL fields
        const cleanedData = {
          ...platformData,
          icon_url: platformData.icon_url?.trim() || undefined
        };

        const response = await apiCall(`${config.apiUrl}/platforms/${id}`, {
          method: 'PUT',
          body: JSON.stringify(cleanedData),
        });
        
        const updatedPlatform: Platform = await response.json();

        update(state => ({
          ...state,
          platforms: state.platforms.map(platform => 
            platform.id === id ? updatedPlatform : platform
          ),
          isLoading: false
        }));

        return updatedPlatform;
      } catch (error) {
        const errorMessage = error instanceof Error ? error.message : 'Failed to update platform';
        update(state => ({ ...state, isLoading: false, error: errorMessage }));
        throw error;
      }
    },

    // Delete a platform (admin only)
    deletePlatform: async (id: string) => {
      if (!auth.value.user?.isAdmin) {
        throw new Error('Admin access required');
      }

      update(state => ({ ...state, isLoading: true, error: null }));

      try {
        await apiCall(`${config.apiUrl}/platforms/${id}`, {
          method: 'DELETE',
        });

        update(state => ({
          ...state,
          platforms: state.platforms.filter(platform => platform.id !== id),
          isLoading: false
        }));
      } catch (error) {
        const errorMessage = error instanceof Error ? error.message : 'Failed to delete platform';
        update(state => ({ ...state, isLoading: false, error: errorMessage }));
        throw error;
      }
    },

    // Create a new storefront (admin only)
    createStorefront: async (storefrontData: StorefrontCreateRequest) => {
      if (!auth.value.user?.isAdmin) {
        throw new Error('Admin access required');
      }

      update(state => ({ ...state, isLoading: true, error: null }));

      try {
        // Clean the data - convert empty strings to undefined for optional URL fields
        const cleanedData = {
          ...storefrontData,
          icon_url: storefrontData.icon_url?.trim() || undefined,
          base_url: storefrontData.base_url?.trim() || undefined
        };

        const response = await apiCall(`${config.apiUrl}/platforms/storefronts/`, {
          method: 'POST',
          body: JSON.stringify(cleanedData),
        });
        
        const storefront: Storefront = await response.json();

        update(state => ({
          ...state,
          storefronts: [...state.storefronts, storefront],
          isLoading: false
        }));

        return storefront;
      } catch (error) {
        const errorMessage = error instanceof Error ? error.message : 'Failed to create storefront';
        update(state => ({ ...state, isLoading: false, error: errorMessage }));
        throw error;
      }
    },

    // Update an existing storefront (admin only)
    updateStorefront: async (id: string, storefrontData: StorefrontUpdateRequest) => {
      if (!auth.value.user?.isAdmin) {
        throw new Error('Admin access required');
      }

      update(state => ({ ...state, isLoading: true, error: null }));

      try {
        // Clean the data - convert empty strings to undefined for optional URL fields
        const cleanedData = {
          ...storefrontData,
          icon_url: storefrontData.icon_url?.trim() || undefined,
          base_url: storefrontData.base_url?.trim() || undefined
        };

        const response = await apiCall(`${config.apiUrl}/platforms/storefronts/${id}`, {
          method: 'PUT',
          body: JSON.stringify(cleanedData),
        });
        
        const updatedStorefront: Storefront = await response.json();

        update(state => ({
          ...state,
          storefronts: state.storefronts.map(storefront => 
            storefront.id === id ? updatedStorefront : storefront
          ),
          isLoading: false
        }));

        return updatedStorefront;
      } catch (error) {
        const errorMessage = error instanceof Error ? error.message : 'Failed to update storefront';
        update(state => ({ ...state, isLoading: false, error: errorMessage }));
        throw error;
      }
    },

    // Delete a storefront (admin only)
    deleteStorefront: async (id: string) => {
      if (!auth.value.user?.isAdmin) {
        throw new Error('Admin access required');
      }

      update(state => ({ ...state, isLoading: true, error: null }));

      try {
        await apiCall(`${config.apiUrl}/platforms/storefronts/${id}`, {
          method: 'DELETE',
        });

        update(state => ({
          ...state,
          storefronts: state.storefronts.filter(storefront => storefront.id !== id),
          isLoading: false
        }));
      } catch (error) {
        const errorMessage = error instanceof Error ? error.message : 'Failed to delete storefront';
        update(state => ({ ...state, isLoading: false, error: errorMessage }));
        throw error;
      }
    },

    // Get active platforms only (client-side filtering)
    getActivePlatforms: () => {
      // This will need to be used with the store subscription
      // e.g., $platforms.platforms.filter(platform => platform.is_active)
      throw new Error('Use store subscription with client-side filtering instead');
    },

    // Get active storefronts only (client-side filtering)
    getActiveStorefronts: () => {
      // This will need to be used with the store subscription
      // e.g., $platforms.storefronts.filter(storefront => storefront.is_active)
      throw new Error('Use store subscription with client-side filtering instead');
    },

    // Fetch active platforms and storefronts for regular users
    fetchActivePlatformsAndStorefronts: async () => {
      update(state => ({ ...state, isLoading: true, error: null }));
      try {
        // Load only active platforms and storefronts in parallel
        const [platformsResponse, storefrontsResponse] = await Promise.all([
          apiCall(`${config.apiUrl}/platforms/?active_only=true`),
          apiCall(`${config.apiUrl}/platforms/storefronts/?active_only=true`)
        ]);
        const [platformsData, storefrontsData] = await Promise.all([
          platformsResponse.json(),
          storefrontsResponse.json()
        ]);
        
        update(state => ({
          ...state,
          platforms: platformsData.platforms,
          storefronts: storefrontsData.storefronts,
          isLoading: false
        }));
        
        return {
          platforms: platformsData.platforms,
          storefronts: storefrontsData.storefronts
        };
      } catch (error) {
        const errorMessage = error instanceof Error ? error.message : 'Failed to fetch active platforms and storefronts';
        update(state => ({ ...state, isLoading: false, error: errorMessage }));
        throw error;
      }
    },

    // Get platform-storefront associations (admin only)
    getPlatformStorefronts: async (platformId: string) => {
      if (!auth.value.user?.isAdmin) {
        throw new Error('Admin access required');
      }

      try {
        const response = await apiCall(`${config.apiUrl}/platforms/${platformId}/storefronts`);
        const data = await response.json();
        return data.storefronts;
      } catch (error) {
        const errorMessage = error instanceof Error ? error.message : 'Failed to fetch platform storefronts';
        update(state => ({ ...state, error: errorMessage }));
        throw error;
      }
    },

    // Create platform-storefront association (admin only)
    createPlatformStorefrontAssociation: async (platformId: string, storefrontId: string) => {
      if (!auth.value.user?.isAdmin) {
        throw new Error('Admin access required');
      }

      try {
        const response = await apiCall(`${config.apiUrl}/platforms/${platformId}/storefronts/${storefrontId}`, {
          method: 'POST',
        });
        
        const result = await response.json();
        
        // Note: Platform data will refresh automatically on next load
        
        return result;
      } catch (error) {
        const errorMessage = error instanceof Error ? error.message : 'Failed to create platform-storefront association';
        update(state => ({ ...state, error: errorMessage }));
        throw error;
      }
    },

    // Remove platform-storefront association (admin only)
    deletePlatformStorefrontAssociation: async (platformId: string, storefrontId: string) => {
      if (!auth.value.user?.isAdmin) {
        throw new Error('Admin access required');
      }

      try {
        const response = await apiCall(`${config.apiUrl}/platforms/${platformId}/storefronts/${storefrontId}`, {
          method: 'DELETE',
        });
        
        const result = await response.json();
        
        // Note: Platform data will refresh automatically on next load
        
        return result;
      } catch (error) {
        const errorMessage = error instanceof Error ? error.message : 'Failed to remove platform-storefront association';
        update(state => ({ ...state, error: errorMessage }));
        throw error;
      }
    },

    // Clear error
    clearError: () => {
      update(state => ({ ...state, error: null }));
    },

    // Test helper - only use in tests
    __reset: () => {
      set(initialState);
    }
  };
}

export const platforms = createPlatformsStore();