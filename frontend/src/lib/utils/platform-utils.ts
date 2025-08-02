import type { UserGamePlatform } from '$lib/stores/user-games.svelte';
import type { Platform } from '$lib/stores/platforms.svelte';

export interface GroupedPlatform {
  platform: Platform;
  storefronts: UserGamePlatform[];
}

/**
 * Groups UserGamePlatform entries by platform to avoid displaying duplicate platform names
 * when a game is available on multiple storefronts for the same platform.
 * 
 * @param platforms Array of UserGamePlatform objects
 * @returns Array of grouped platforms with their associated storefronts
 */
export function groupPlatformsByPlatform(platforms: UserGamePlatform[]): GroupedPlatform[] {
  if (!platforms || platforms.length === 0) {
    return [];
  }

  // Create a map to group platforms by their ID
  const platformMap = new Map<string, GroupedPlatform>();

  for (const userGamePlatform of platforms) {
    // Defensive check: ensure platform exists and has required properties
    if (!userGamePlatform.platform || !userGamePlatform.platform.id) {
      console.warn('groupPlatformsByPlatform: Skipping invalid UserGamePlatform with missing platform data:', userGamePlatform);
      continue;
    }
    
    const platformId = userGamePlatform.platform.id;
    
    if (!platformMap.has(platformId)) {
      platformMap.set(platformId, {
        platform: userGamePlatform.platform,
        storefronts: []
      });
    }
    
    platformMap.get(platformId)!.storefronts.push(userGamePlatform);
  }

  // Convert map to array and sort by platform display name
  return Array.from(platformMap.values()).sort((a, b) => 
    a.platform.display_name.localeCompare(b.platform.display_name)
  );
}

/**
 * Creates a compact display string for platforms and their storefronts
 * Format: "PC: Steam, Epic Games Store | PlayStation 5: PlayStation Store"
 * 
 * @param platforms Array of UserGamePlatform objects
 * @returns Compact string representation
 */
export function createCompactPlatformDisplay(platforms: UserGamePlatform[]): string {
  const grouped = groupPlatformsByPlatform(platforms);
  
  return grouped.map(group => {
    const platformName = group.platform.display_name;
    const storefrontNames = group.storefronts
      .map(sf => sf.storefront?.display_name)
      .filter(name => name !== undefined && name !== null)
      .join(', ');
    
    return storefrontNames ? `${platformName}: ${storefrontNames}` : platformName;
  }).join(' | ');
}