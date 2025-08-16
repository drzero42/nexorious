import { auth } from './auth.svelte';
import { config } from '$lib/env';

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
      console.log('🔄 [STEAM-AVAILABILITY] Starting availability check via API...');
      
      availability.isLoading = true;
      
      try {
        if (!auth.value.accessToken) {
          console.log('❌ [STEAM-AVAILABILITY] No access token available');
          availability.isAvailable = false;
          availability.unavailableReason = 'Not authenticated';
          return;
        }

        console.log('📡 [STEAM-AVAILABILITY] Calling backend availability API...');
        const response = await fetch(`${config.apiUrl}/import/sources/steam/availability`, {
          method: 'GET',
          headers: {
            'Authorization': `Bearer ${auth.value.accessToken}`,
            'Content-Type': 'application/json'
          }
        });

        if (!response.ok) {
          console.error('❌ [STEAM-AVAILABILITY] API request failed:', response.status, response.statusText);
          availability.isAvailable = false;
          availability.unavailableReason = `API request failed: ${response.status}`;
          return;
        }

        const data = await response.json();
        console.log('✅ [STEAM-AVAILABILITY] API response received:', data);
        
        availability.isAvailable = data.available;
        availability.unavailableReason = data.reason;
        
        if (data.available) {
          console.log('🎯 [STEAM-AVAILABILITY] Steam is available!');
        } else {
          console.log('❌ [STEAM-AVAILABILITY] Steam not available:', data.reason);
        }
        
      } catch (error) {
        console.error('❌ [STEAM-AVAILABILITY] Error checking Steam availability:', error);
        availability.isAvailable = false;
        availability.unavailableReason = 'Failed to check Steam availability';
      } finally {
        availability.isLoading = false;
        console.log('✅ [STEAM-AVAILABILITY] Availability check completed');
      }
    }
  };
}

export const steamAvailability = createSteamAvailabilityStore();