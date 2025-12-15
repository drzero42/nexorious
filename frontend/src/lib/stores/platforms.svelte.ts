import { auth } from './auth.svelte';
import { config } from '$lib/env';
import { loggers } from '$lib/services/logger';

const log = loggers.platforms;
import type {
  PlatformSuggestionsRequest,
  PlatformSuggestionsResponse,
  PlatformResolutionRequest,
  BulkPlatformResolutionRequest,
  BulkPlatformResolutionResponse,
  PendingResolutionsListResponse,
  PlatformResolutionResult,
  StorefrontSuggestionsRequest,
  StorefrontSuggestionsResponse,
  StorefrontResolutionRequest,
  BulkStorefrontResolutionRequest,
  BulkStorefrontResolutionResponse,
  PendingStorefrontsListResponse,
  StorefrontResolutionResult
} from '$lib/types/platform-resolution';

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

  return {
    get value() {
      return state;
    },
    
    // Subscribe method for compatibility with Svelte store interface
    subscribe: (run: (value: PlatformsState) => void) => {
      run(state); // Initial call
      return () => {}; // Return unsubscribe function directly
    },

    // Fetch all platforms (loads ALL platforms regardless of active status)
    fetchPlatforms: async () => {
      if (!auth.value.user?.isAdmin) {
        throw new Error('Admin access required');
      }

      state = { ...state, isLoading: true, error: null };

      try {
        // Load ALL platforms without active_only filter (active_only=false)
        const response = await apiCall(`${config.apiUrl}/platforms/?active_only=false`);
        const data = await response.json();

        state = {
          ...state,
          platforms: data.platforms,
          isLoading: false
        };
        return data.platforms;
      } catch (error) {
        const errorMessage = error instanceof Error ? error.message : 'Failed to fetch platforms';
        state = { ...state, isLoading: false, error: errorMessage };
        throw error;
      }
    },

    // Fetch all storefronts (loads ALL storefronts regardless of active status)
    fetchStorefronts: async () => {
      if (!auth.value.user?.isAdmin) {
        throw new Error('Admin access required');
      }

      state = { ...state, isLoading: true, error: null };

      try {
        // Load ALL storefronts without active_only filter (active_only=false)
        const response = await apiCall(`${config.apiUrl}/platforms/storefronts/?active_only=false`);
        const data = await response.json();

        state = {
          ...state,
          storefronts: data.storefronts,
          isLoading: false
        };
        return data.storefronts;
      } catch (error) {
        const errorMessage = error instanceof Error ? error.message : 'Failed to fetch storefronts';
        state = { ...state, isLoading: false, error: errorMessage };
        throw error;
      }
    },

    // Fetch both platforms and storefronts (accessible to all authenticated users)
    fetchAll: async () => {
      log.debug('Starting fetchAll');
      state = { ...state, isLoading: true, error: null };

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

        log.debug('Fetched platforms and storefronts', {
          platformsCount: platformsData.platforms?.length || 0,
          storefrontsCount: storefrontsData.storefronts?.length || 0
        });

        state = {
          ...state,
          platforms: platformsData.platforms,
          storefronts: storefrontsData.storefronts,
          isLoading: false
        };

        return {
          platforms: platformsData.platforms,
          storefronts: storefrontsData.storefronts
        };
      } catch (error) {
        const errorMessage = error instanceof Error ? error.message : 'Failed to fetch platforms and storefronts';
        log.error('Error in fetchAll', error);
        state = { ...state, isLoading: false, error: errorMessage };
        throw error;
      }
    },

    // Create a new platform (admin only)
    createPlatform: async (platformData: PlatformCreateRequest) => {
      if (!auth.value.user?.isAdmin) {
        throw new Error('Admin access required');
      }

      state = { ...state, isLoading: true, error: null };

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
      if (!auth.value.user?.isAdmin) {
        throw new Error('Admin access required');
      }

      state = { ...state, isLoading: true, error: null };

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
      if (!auth.value.user?.isAdmin) {
        throw new Error('Admin access required');
      }

      state = { ...state, isLoading: true, error: null };

      try {
        await apiCall(`${config.apiUrl}/platforms/${id}`, {
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
      if (!auth.value.user?.isAdmin) {
        throw new Error('Admin access required');
      }

      state = { ...state, isLoading: true, error: null };

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
      if (!auth.value.user?.isAdmin) {
        throw new Error('Admin access required');
      }

      state = { ...state, isLoading: true, error: null };

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
      if (!auth.value.user?.isAdmin) {
        throw new Error('Admin access required');
      }

      state = { ...state, isLoading: true, error: null };

      try {
        await apiCall(`${config.apiUrl}/platforms/storefronts/${id}`, {
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
      state = { ...state, isLoading: true, error: null };
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
        
        state = {
          ...state,
          platforms: platformsData.platforms,
          storefronts: storefrontsData.storefronts,
          isLoading: false
        };
        
        return {
          platforms: platformsData.platforms,
          storefronts: storefrontsData.storefronts
        };
      } catch (error) {
        const errorMessage = error instanceof Error ? error.message : 'Failed to fetch active platforms and storefronts';
        state = { ...state, isLoading: false, error: errorMessage };
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
        state = { ...state, error: errorMessage };
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
        state = { ...state, error: errorMessage };
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
        state = { ...state, error: errorMessage };
        throw error;
      }
    },

    // Clear error
    clearError: () => {
      state = { ...state, error: null };
    },

    // Platform Resolution Methods

    // Get fuzzy matching suggestions for unknown platform names
    getSuggestions: async (request: PlatformSuggestionsRequest): Promise<PlatformSuggestionsResponse> => {
      try {
        const response = await apiCall(`${config.apiUrl}/platforms/resolution/suggestions`, {
          method: 'POST',
          body: JSON.stringify(request),
        });
        
        return await response.json();
      } catch (error) {
        const errorMessage = error instanceof Error ? error.message : 'Failed to get platform suggestions';
        state = { ...state, error: errorMessage };
        throw error;
      }
    },

    // Get pending platform resolutions for the current user
    getPendingResolutions: async (page: number = 1, perPage: number = 20): Promise<PendingResolutionsListResponse> => {
      try {
        const response = await apiCall(
          `${config.apiUrl}/platforms/resolution/pending?page=${page}&per_page=${perPage}`
        );
        
        return await response.json();
      } catch (error) {
        const errorMessage = error instanceof Error ? error.message : 'Failed to fetch pending platform resolutions';
        state = { ...state, error: errorMessage };
        throw error;
      }
    },

    // Resolve a single platform mapping
    resolvePlatform: async (request: PlatformResolutionRequest): Promise<PlatformResolutionResult> => {
      try {
        const response = await apiCall(`${config.apiUrl}/platforms/resolution/resolve`, {
          method: 'POST',
          body: JSON.stringify(request),
        });
        
        const result = await response.json();
        
        // If the resolution was successful and a platform was created, add it to our store
        if (result.success && result.resolved_platform) {
          // Check if we already have this platform in our store
          const existingPlatform = state.platforms.find(p => p.id === result.resolved_platform.id);
          if (!existingPlatform) {
            // Add the new platform to our store (it was likely just created)
            const newPlatform: Platform = {
              ...result.resolved_platform,
              is_active: true,
              source: 'custom',
              storefronts: [],
              created_at: new Date().toISOString(),
              updated_at: new Date().toISOString()
            };
            state = {
              ...state,
              platforms: [...state.platforms, newPlatform]
            };
          }
        }
        
        return result;
      } catch (error) {
        const errorMessage = error instanceof Error ? error.message : 'Failed to resolve platform';
        state = { ...state, error: errorMessage };
        throw error;
      }
    },

    // Resolve multiple platforms in a bulk operation
    bulkResolvePlatforms: async (request: BulkPlatformResolutionRequest): Promise<BulkPlatformResolutionResponse> => {
      try {
        const response = await apiCall(`${config.apiUrl}/platforms/resolution/bulk-resolve`, {
          method: 'POST',
          body: JSON.stringify(request),
        });
        
        const result = await response.json();
        
        // Add any newly created platforms to our store
        const newPlatforms: Platform[] = [];
        for (const resolutionResult of result.results) {
          if (resolutionResult.success && resolutionResult.resolved_platform) {
            const existingPlatform = state.platforms.find(p => p.id === resolutionResult.resolved_platform!.id);
            if (!existingPlatform) {
              const newPlatform: Platform = {
                ...resolutionResult.resolved_platform,
                is_active: true,
                source: 'custom',
                storefronts: [],
                created_at: new Date().toISOString(),
                updated_at: new Date().toISOString()
              };
              newPlatforms.push(newPlatform);
            }
          }
        }
        
        if (newPlatforms.length > 0) {
          state = {
            ...state,
            platforms: [...state.platforms, ...newPlatforms]
          };
        }
        
        return result;
      } catch (error) {
        const errorMessage = error instanceof Error ? error.message : 'Failed to bulk resolve platforms';
        state = { ...state, error: errorMessage };
        throw error;
      }
    },

    // Populate platform suggestions for a specific import record
    populateSuggestions: async (importId: string, minConfidence: number = 0.6): Promise<void> => {
      try {
        await apiCall(
          `${config.apiUrl}/platforms/resolution/populate-suggestions/${importId}?min_confidence=${minConfidence}`,
          { method: 'POST' }
        );
      } catch (error) {
        const errorMessage = error instanceof Error ? error.message : 'Failed to populate platform suggestions';
        state = { ...state, error: errorMessage };
        throw error;
      }
    },

    // Create platform from resolution UI (extends existing createPlatform with resolution context)
    createPlatformFromResolution: async (
      platformData: PlatformCreateRequest,
      importId?: string
    ): Promise<Platform> => {
      // Create the platform first
      if (!auth.value.user?.isAdmin) {
        throw new Error('Admin access required');
      }

      state = { ...state, isLoading: true, error: null };

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

        state = {
          ...state,
          platforms: [...state.platforms, platform],
          isLoading: false
        };

        // If we have an import ID, automatically resolve it to the new platform
        if (importId) {
          try {
            await apiCall(`${config.apiUrl}/platforms/resolution/resolve`, {
              method: 'POST',
              body: JSON.stringify({
                import_id: importId,
                resolved_platform_id: platform.id,
                user_notes: `Auto-resolved to newly created platform: ${platform.display_name}`
              }),
            });
          } catch (error) {
            // Log error but don't throw - platform was created successfully
            log.warn('Platform created but auto-resolution failed', error);
          }
        }

        return platform;
      } catch (error) {
        const errorMessage = error instanceof Error ? error.message : 'Failed to create platform from resolution';
        state = { ...state, isLoading: false, error: errorMessage };
        throw error;
      }
    },

    // Storefront Resolution Methods (Task 15)

    // Get fuzzy matching suggestions for unknown storefront names with platform context
    getStorefrontSuggestions: async (request: StorefrontSuggestionsRequest): Promise<StorefrontSuggestionsResponse> => {
      try {
        const response = await apiCall(`${config.apiUrl}/platforms/resolution/storefront-suggestions`, {
          method: 'POST',
          body: JSON.stringify(request),
        });
        
        return await response.json();
      } catch (error) {
        const errorMessage = error instanceof Error ? error.message : 'Failed to get storefront suggestions';
        state = { ...state, error: errorMessage };
        throw error;
      }
    },

    // Get pending storefront resolutions for the current user
    getPendingStorefrontResolutions: async (page: number = 1, perPage: number = 20): Promise<PendingStorefrontsListResponse> => {
      try {
        const response = await apiCall(
          `${config.apiUrl}/platforms/resolution/pending-storefronts?page=${page}&per_page=${perPage}`
        );
        
        return await response.json();
      } catch (error) {
        const errorMessage = error instanceof Error ? error.message : 'Failed to fetch pending storefront resolutions';
        state = { ...state, error: errorMessage };
        throw error;
      }
    },

    // Resolve a single storefront mapping
    resolveStorefront: async (request: StorefrontResolutionRequest): Promise<StorefrontResolutionResult> => {
      try {
        const response = await apiCall(`${config.apiUrl}/platforms/resolution/resolve-storefront`, {
          method: 'POST',
          body: JSON.stringify(request),
        });
        
        const result = await response.json();
        
        // If the resolution was successful and a storefront was created, add it to our store
        if (result.success && result.resolved_storefront) {
          // Check if we already have this storefront in our store
          const existingStorefront = state.storefronts.find(s => s.id === result.resolved_storefront.id);
          if (!existingStorefront) {
            // Add the new storefront to our store (it was likely just created)
            const newStorefront: Storefront = {
              ...result.resolved_storefront,
              is_active: true,
              source: 'custom',
              base_url: null,
              created_at: new Date().toISOString(),
              updated_at: new Date().toISOString()
            };
            state = {
              ...state,
              storefronts: [...state.storefronts, newStorefront]
            };
          }
        }
        
        return result;
      } catch (error) {
        const errorMessage = error instanceof Error ? error.message : 'Failed to resolve storefront';
        state = { ...state, error: errorMessage };
        throw error;
      }
    },

    // Resolve multiple storefronts in a bulk operation
    bulkResolveStorefronts: async (request: BulkStorefrontResolutionRequest): Promise<BulkStorefrontResolutionResponse> => {
      try {
        const response = await apiCall(`${config.apiUrl}/platforms/resolution/bulk-resolve-storefronts`, {
          method: 'POST',
          body: JSON.stringify(request),
        });
        
        const result = await response.json();
        
        // Add any newly created storefronts to our store
        const newStorefronts: Storefront[] = [];
        for (const resolutionResult of result.results) {
          if (resolutionResult.success && resolutionResult.resolved_storefront) {
            const existingStorefront = state.storefronts.find(s => s.id === resolutionResult.resolved_storefront!.id);
            if (!existingStorefront) {
              const newStorefront: Storefront = {
                ...resolutionResult.resolved_storefront,
                is_active: true,
                source: 'custom',
                base_url: null,
                created_at: new Date().toISOString(),
                updated_at: new Date().toISOString()
              };
              newStorefronts.push(newStorefront);
            }
          }
        }
        
        if (newStorefronts.length > 0) {
          state = {
            ...state,
            storefronts: [...state.storefronts, ...newStorefronts]
          };
        }
        
        return result;
      } catch (error) {
        const errorMessage = error instanceof Error ? error.message : 'Failed to bulk resolve storefronts';
        state = { ...state, error: errorMessage };
        throw error;
      }
    },

    // Create storefront from resolution UI (extends existing createStorefront with resolution context)
    createStorefrontFromResolution: async (
      storefrontData: StorefrontCreateRequest,
      importId?: string
    ): Promise<Storefront> => {
      // Create the storefront first
      if (!auth.value.user?.isAdmin) {
        throw new Error('Admin access required');
      }

      state = { ...state, isLoading: true, error: null };

      try {
        // Clean the data - convert empty strings to undefined for optional URL fields
        const cleanedData = {
          ...storefrontData,
          icon_url: storefrontData.icon_url?.trim() || undefined,
          base_url: storefrontData.base_url?.trim() || undefined
        };

        const response = await apiCall(`${config.apiUrl}/storefronts/`, {
          method: 'POST',
          body: JSON.stringify(cleanedData),
        });
        
        const storefront: Storefront = await response.json();

        state = {
          ...state,
          storefronts: [...state.storefronts, storefront],
          isLoading: false
        };

        // If we have an import ID, automatically resolve it to the new storefront
        if (importId) {
          try {
            await apiCall(`${config.apiUrl}/platforms/resolution/resolve-storefront`, {
              method: 'POST',
              body: JSON.stringify({
                import_id: importId,
                resolved_storefront_id: storefront.id,
                user_notes: `Auto-resolved to newly created storefront: ${storefront.display_name}`
              }),
            });
          } catch (error) {
            // Log error but don't throw - storefront was created successfully
            log.warn('Storefront created but auto-resolution failed', error);
          }
        }

        return storefront;
      } catch (error) {
        const errorMessage = error instanceof Error ? error.message : 'Failed to create storefront from resolution';
        state = { ...state, isLoading: false, error: errorMessage };
        throw error;
      }
    },

    // Test helper - only use in tests
    __reset: () => {
      state = { ...initialState };
    }
  };
}

export const platforms = createPlatformsStore();