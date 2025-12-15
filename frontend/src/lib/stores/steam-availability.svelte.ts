import { auth } from './auth.svelte';
import { config } from '$lib/env';
import { loggers } from '$lib/services/logger';

const log = loggers.steamAvailability;

export interface SteamAvailability {
  /** Whether Steam Games feature is available */
  isAvailable: boolean;
  /** Whether availability check is in progress */
  isLoading: boolean;
  /** Reason why Steam Games is unavailable (null if available) */
  unavailableReason: string | null;
}

/**
 * Simple Steam availability store that uses backend API to check availability.
 * This replaces the complex frontend logic with a simple API call to /api/import/sources/steam/availability.
 * 
 * The backend handles all the complex logic:
 * - User preferences check
 * - PC-Windows platform availability
 * - Steam storefront availability
 */
function createSteamAvailabilityStore() {
  let availability = $state<SteamAvailability>({
    isAvailable: false,
    isLoading: true,
    unavailableReason: null
  });

  return {
    get isAvailable() { 
      return availability.isAvailable; 
    },
    
    get isLoading() { 
      return availability.isLoading; 
    },
    
    get unavailableReason() { 
      return availability.unavailableReason; 
    },
    
    /**
     * Check Steam availability via backend API.
     * This is much simpler and more reliable than the previous approach.
     */
    async checkAvailability(): Promise<void> {
      log.debug('Starting availability check via API');

      availability.isLoading = true;

      try {
        if (!auth.value.accessToken) {
          log.debug('No access token available');
          availability.isAvailable = false;
          availability.unavailableReason = 'Not authenticated';
          return;
        }

        const response = await fetch(`${config.apiUrl}/import/sources/steam/availability`, {
          method: 'GET',
          headers: {
            'Authorization': `Bearer ${auth.value.accessToken}`,
            'Content-Type': 'application/json'
          }
        });

        if (!response.ok) {
          log.error('API request failed', { status: response.status, statusText: response.statusText });
          availability.isAvailable = false;
          availability.unavailableReason = `API request failed: ${response.status}`;
          return;
        }

        const data = await response.json();
        log.debug('API response received', { available: data.available });

        availability.isAvailable = data.available;
        availability.unavailableReason = data.reason;

        if (!data.available) {
          log.debug('Steam not available', { reason: data.reason });
        }

      } catch (error) {
        log.error('Error checking Steam availability', error);
        availability.isAvailable = false;
        availability.unavailableReason = 'Failed to check Steam availability';
      } finally {
        availability.isLoading = false;
      }
    }
  };
}

export const steamAvailability = createSteamAvailabilityStore();