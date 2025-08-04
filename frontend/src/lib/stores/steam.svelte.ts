import { config } from '$lib/env';
import { auth } from './auth.svelte';

// Steam API interfaces based on backend schemas
export interface SteamUserInfo {
  steam_id: string;
  persona_name: string;
  profile_url: string;
  avatar: string;
  avatar_medium: string;
  avatar_full: string;
  persona_state?: number;
  community_visibility_state?: number;
  profile_state?: number;
  last_logoff?: number;
}

export interface SteamConfig {
  has_api_key: boolean;
  api_key_masked?: string;
  steam_id?: string;
  is_verified: boolean;
  configured_at?: Date;
}

export interface SteamVerificationResult {
  is_valid: boolean;
  error_message?: string;
  steam_user_info?: SteamUserInfo;
}

export interface VanityUrlResolveResult {
  success: boolean;
  steam_id?: string;
  error_message?: string;
}

export interface SteamState {
  config: SteamConfig | null;
  isLoading: boolean;
  isVerifying: boolean;
  isResolvingVanity: boolean;
  error: string | null;
  verificationResult: SteamVerificationResult | null;
}

const initialState: SteamState = {
  config: null,
  isLoading: false,
  isVerifying: false,
  isResolvingVanity: false,
  error: null,
  verificationResult: null
};

function createSteamStore() {
  let state = $state<SteamState>(initialState);

  const steamStore = {
    get value() {
      return state;
    },

    reset() {
      state = { ...initialState };
    },

    // Get current Steam configuration
    async getConfig(): Promise<SteamConfig> {
      state = { ...state, isLoading: true, error: null };

      try {
        const response = await fetch(`${config.apiUrl}/steam/config`, {
          headers: {
            'Authorization': `Bearer ${auth.value.accessToken}`
          }
        });

        if (!response.ok) {
          if (response.status === 401) {
            // Try to refresh token
            await auth.refreshAuth();
            return this.getConfig();
          }
          throw new Error('Failed to fetch Steam configuration');
        }

        const configData = await response.json();
        
        state = {
          ...state,
          config: {
            ...configData,
            configured_at: configData.configured_at ? new Date(configData.configured_at) : undefined
          },
          isLoading: false,
          error: null
        };

        return configData;
      } catch (error) {
        const errorMessage = error instanceof Error ? error.message : 'Failed to fetch Steam configuration';
        state = { ...state, isLoading: false, error: errorMessage };
        throw error;
      }
    },

    // Set Steam configuration
    async setConfig(webApiKey: string, steamId?: string): Promise<SteamConfig> {
      state = { ...state, isLoading: true, error: null };

      try {
        const response = await fetch(`${config.apiUrl}/steam/config`, {
          method: 'PUT',
          headers: {
            'Content-Type': 'application/json',
            'Authorization': `Bearer ${auth.value.accessToken}`
          },
          body: JSON.stringify({
            web_api_key: webApiKey,
            steam_id: steamId
          })
        });

        if (!response.ok) {
          if (response.status === 401) {
            await auth.refreshAuth();
            return this.setConfig(webApiKey, steamId);
          }
          const errorData = await response.json().catch(() => ({}));
          throw new Error(errorData.detail || 'Failed to save Steam configuration');
        }

        const configData = await response.json();
        
        state = {
          ...state,
          config: {
            ...configData,
            configured_at: configData.configured_at ? new Date(configData.configured_at) : undefined
          },
          isLoading: false,
          error: null
        };

        return configData;
      } catch (error) {
        const errorMessage = error instanceof Error ? error.message : 'Failed to save Steam configuration';
        state = { ...state, isLoading: false, error: errorMessage };
        throw error;
      }
    },

    // Delete Steam configuration
    async deleteConfig(): Promise<boolean> {
      state = { ...state, isLoading: true, error: null };

      try {
        const response = await fetch(`${config.apiUrl}/steam/config`, {
          method: 'DELETE',
          headers: {
            'Authorization': `Bearer ${auth.value.accessToken}`
          }
        });

        if (!response.ok) {
          if (response.status === 401) {
            await auth.refreshAuth();
            return this.deleteConfig();
          }
          throw new Error('Failed to delete Steam configuration');
        }

        state = {
          ...state,
          config: null,
          isLoading: false,
          error: null,
          verificationResult: null
        };

        return true;
      } catch (error) {
        const errorMessage = error instanceof Error ? error.message : 'Failed to delete Steam configuration';
        state = { ...state, isLoading: false, error: errorMessage };
        throw error;
      }
    },

    // Verify Steam configuration without saving
    async verify(webApiKey: string, steamId?: string): Promise<SteamVerificationResult> {
      state = { ...state, isVerifying: true, error: null, verificationResult: null };

      try {
        const response = await fetch(`${config.apiUrl}/steam/verify`, {
          method: 'POST',
          headers: {
            'Content-Type': 'application/json',
            'Authorization': `Bearer ${auth.value.accessToken}`
          },
          body: JSON.stringify({
            web_api_key: webApiKey,
            steam_id: steamId
          })
        });

        if (!response.ok) {
          if (response.status === 401) {
            await auth.refreshAuth();
            return this.verify(webApiKey, steamId);
          }
          throw new Error('Failed to verify Steam configuration');
        }

        const verificationData = await response.json();
        
        state = {
          ...state,
          isVerifying: false,
          verificationResult: verificationData,
          error: null
        };

        return verificationData;
      } catch (error) {
        const errorMessage = error instanceof Error ? error.message : 'Failed to verify Steam configuration';
        state = { ...state, isVerifying: false, error: errorMessage };
        throw error;
      }
    },

    // Resolve vanity URL to Steam ID
    async resolveVanityUrl(vanityUrl: string): Promise<VanityUrlResolveResult> {
      state = { ...state, isResolvingVanity: true, error: null };

      try {
        const response = await fetch(`${config.apiUrl}/steam/resolve-vanity`, {
          method: 'POST',
          headers: {
            'Content-Type': 'application/json',
            'Authorization': `Bearer ${auth.value.accessToken}`
          },
          body: JSON.stringify({
            vanity_url: vanityUrl
          })
        });

        if (!response.ok) {
          if (response.status === 401) {
            await auth.refreshAuth();
            return this.resolveVanityUrl(vanityUrl);
          }
          throw new Error('Failed to resolve vanity URL');
        }

        const resolveData = await response.json();
        
        state = {
          ...state,
          isResolvingVanity: false,
          error: null
        };

        return resolveData;
      } catch (error) {
        const errorMessage = error instanceof Error ? error.message : 'Failed to resolve vanity URL';
        state = { ...state, isResolvingVanity: false, error: errorMessage };
        throw error;
      }
    },

    // Clear verification result
    clearVerification() {
      state = { ...state, verificationResult: null };
    },

    // Clear error
    clearError() {
      state = { ...state, error: null };
    }
  };

  return steamStore;
}

export const steam = createSteamStore();