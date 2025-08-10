import { auth } from './auth.svelte';
import { platforms, type PlatformsState } from './platforms.svelte';
import { isSteamGamesAvailable, getSteamGamesUnavailableReason } from '$lib/utils/steam-utils';

export interface SteamAvailability {
  /** Whether Steam Games feature is available */
  isAvailable: boolean;
  /** Whether platforms/storefronts data is still loading */
  isLoading: boolean;
  /** Error message if data failed to load */
  error: string | null;
  /** Reason why Steam Games is unavailable (null if available) */
  unavailableReason: string | null;
}

/**
 * Reactive store that provides Steam Games availability status.
 * This mirrors the backend verify_steam_games_enabled() dependency logic:
 * 1. User has Steam Games UI feature enabled (default: True)
 * 2. PC-Windows platform exists and is active  
 * 3. Steam storefront exists and is active
 */
function createSteamAvailabilityStore() {
  let platformsState = $state<PlatformsState>({ platforms: [], storefronts: [], isLoading: false, error: null });
  
  // Subscribe to platforms store
  platforms.subscribe((state) => {
    platformsState = state;
  });

  return {
    get isAvailable() {
      const user = auth.value.user;
      const platformsList = platformsState.platforms;
      const storefrontsList = platformsState.storefronts;

      // If platforms data is still loading, not available
      if (platformsState.isLoading && platformsList.length === 0 && storefrontsList.length === 0) {
        return false;
      }

      // If there's an error loading platforms data, not available
      if (platformsState.error) {
        return false;
      }

      // Check Steam Games availability
      return isSteamGamesAvailable(user, platformsList, storefrontsList);
    },

    get isLoading() {
      return platformsState.isLoading && platformsState.platforms.length === 0 && platformsState.storefronts.length === 0;
    },

    get error() {
      return platformsState.error;
    },

    get unavailableReason() {
      if (this.isAvailable) {
        return null;
      }

      const user = auth.value.user;
      const platformsList = platformsState.platforms;
      const storefrontsList = platformsState.storefronts;

      if (platformsState.isLoading && platformsList.length === 0 && storefrontsList.length === 0) {
        return null; // Still loading
      }

      if (platformsState.error) {
        return `Failed to load platforms data: ${platformsState.error}`;
      }

      return getSteamGamesUnavailableReason(user, platformsList, storefrontsList);
    }
  };
}

export const steamAvailability = createSteamAvailabilityStore();