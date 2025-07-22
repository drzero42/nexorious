import { auth } from './auth.svelte';

export interface Platform {
  id: string;
  name: string;
  display_name: string;
  icon_url?: string;
  is_active: boolean;
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
  created_at: string;
  updated_at: string;
}

export interface PlatformCreateRequest {
  name: string;
  display_name: string;
  icon_url?: string;
}

export interface PlatformUpdateRequest {
  display_name?: string;
  icon_url?: string;
  is_active?: boolean;
}

export interface StorefrontCreateRequest {
  name: string;
  display_name: string;
  icon_url?: string;
  base_url?: string;
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
  let state = $state<PlatformsState>(initialState);

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

  const platformsStore = {
    get value() {
      return state;
    },

    // Load all platforms
    loadPlatforms: async () => {
      state = { ...state, isLoading: true, error: null };

      try {
        const response = await apiCall('/api/platforms');
        const data = await response.json();

        state = {
          ...state,
          platforms: data.platforms,
          isLoading: false
        };
      } catch (error) {
        const errorMessage = error instanceof Error ? error.message : 'Failed to load platforms';
        state = { ...state, isLoading: false, error: errorMessage };
        throw error;
      }
    },

    // Load all storefronts
    loadStorefronts: async () => {
      state = { ...state, isLoading: true, error: null };

      try {
        const response = await apiCall('/api/storefronts');
        const data = await response.json();

        state = {
          ...state,
          storefronts: data.storefronts,
          isLoading: false
        };
      } catch (error) {
        const errorMessage = error instanceof Error ? error.message : 'Failed to load storefronts';
        state = { ...state, isLoading: false, error: errorMessage };
        throw error;
      }
    },

    // Load both platforms and storefronts
    loadAll: async () => {
      state = { ...state, isLoading: true, error: null };

      try {
        const store = platformsStore;
        await Promise.all([
          store.loadPlatforms(),
          store.loadStorefronts()
        ]);
      } catch (error) {
        const errorMessage = error instanceof Error ? error.message : 'Failed to load platforms and storefronts';
        state = { ...state, isLoading: false, error: errorMessage };
        throw error;
      }
    },

    // Create a new platform (admin only)
    createPlatform: async (platformData: PlatformCreateRequest) => {
      state = { ...state, isLoading: true, error: null };

      try {
        const response = await apiCall('/api/platforms', {
          method: 'POST',
          body: JSON.stringify(platformData),
        });
        
        const platform: Platform = await response.json();

        state = {
          ...state,
          platforms: [...state.platforms, platform],
          isLoading: false
        };

        return platform;
      } catch (error) {
        const errorMessage = error instanceof Error ? error.message : 'Failed to create platform';
        state = { ...state, isLoading: false, error: errorMessage };
        throw error;
      }
    },

    // Update an existing platform (admin only)
    updatePlatform: async (id: string, platformData: PlatformUpdateRequest) => {
      state = { ...state, isLoading: true, error: null };

      try {
        const response = await apiCall(`/api/platforms/${id}`, {
          method: 'PUT',
          body: JSON.stringify(platformData),
        });
        
        const updatedPlatform: Platform = await response.json();

        state = {
          ...state,
          platforms: state.platforms.map(platform => 
            platform.id === id ? updatedPlatform : platform
          ),
          isLoading: false
        };

        return updatedPlatform;
      } catch (error) {
        const errorMessage = error instanceof Error ? error.message : 'Failed to update platform';
        state = { ...state, isLoading: false, error: errorMessage };
        throw error;
      }
    },

    // Delete a platform (admin only)
    deletePlatform: async (id: string) => {
      state = { ...state, isLoading: true, error: null };

      try {
        await apiCall(`/api/platforms/${id}`, {
          method: 'DELETE',
        });

        state = {
          ...state,
          platforms: state.platforms.filter(platform => platform.id !== id),
          isLoading: false
        };
      } catch (error) {
        const errorMessage = error instanceof Error ? error.message : 'Failed to delete platform';
        state = { ...state, isLoading: false, error: errorMessage };
        throw error;
      }
    },

    // Create a new storefront (admin only)
    createStorefront: async (storefrontData: StorefrontCreateRequest) => {
      state = { ...state, isLoading: true, error: null };

      try {
        const response = await apiCall('/api/storefronts', {
          method: 'POST',
          body: JSON.stringify(storefrontData),
        });
        
        const storefront: Storefront = await response.json();

        state = {
          ...state,
          storefronts: [...state.storefronts, storefront],
          isLoading: false
        };

        return storefront;
      } catch (error) {
        const errorMessage = error instanceof Error ? error.message : 'Failed to create storefront';
        state = { ...state, isLoading: false, error: errorMessage };
        throw error;
      }
    },

    // Update an existing storefront (admin only)
    updateStorefront: async (id: string, storefrontData: StorefrontUpdateRequest) => {
      state = { ...state, isLoading: true, error: null };

      try {
        const response = await apiCall(`/api/storefronts/${id}`, {
          method: 'PUT',
          body: JSON.stringify(storefrontData),
        });
        
        const updatedStorefront: Storefront = await response.json();

        state = {
          ...state,
          storefronts: state.storefronts.map(storefront => 
            storefront.id === id ? updatedStorefront : storefront
          ),
          isLoading: false
        };

        return updatedStorefront;
      } catch (error) {
        const errorMessage = error instanceof Error ? error.message : 'Failed to update storefront';
        state = { ...state, isLoading: false, error: errorMessage };
        throw error;
      }
    },

    // Delete a storefront (admin only)
    deleteStorefront: async (id: string) => {
      state = { ...state, isLoading: true, error: null };

      try {
        await apiCall(`/api/storefronts/${id}`, {
          method: 'DELETE',
        });

        state = {
          ...state,
          storefronts: state.storefronts.filter(storefront => storefront.id !== id),
          isLoading: false
        };
      } catch (error) {
        const errorMessage = error instanceof Error ? error.message : 'Failed to delete storefront';
        state = { ...state, isLoading: false, error: errorMessage };
        throw error;
      }
    },

    // Get active platforms only
    getActivePlatforms: () => {
      return state.platforms.filter(platform => platform.is_active);
    },

    // Get active storefronts only
    getActiveStorefronts: () => {
      return state.storefronts.filter(storefront => storefront.is_active);
    },

    // Get platform by ID
    getPlatformById: (id: string) => {
      return state.platforms.find(platform => platform.id === id);
    },

    // Get storefront by ID
    getStorefrontById: (id: string) => {
      return state.storefronts.find(storefront => storefront.id === id);
    },

    // Clear error
    clearError: () => {
      state = { ...state, error: null };
    }
  };
  
  return platformsStore;
}

export const platforms = createPlatformsStore();