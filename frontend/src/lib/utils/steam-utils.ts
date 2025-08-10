import type { Platform, Storefront } from '$lib/stores/platforms.svelte';
import type { User } from '$lib/stores/auth.svelte';

/**
 * Checks if the PC-Windows platform exists and is active
 */
export function isPCWindowsPlatformActive(platforms: Platform[]): boolean {
  const pcWindowsPlatform = platforms.find(platform => platform.name === 'pc-windows');
  return pcWindowsPlatform?.is_active ?? false;
}

/**
 * Checks if the Steam storefront exists and is active
 */
export function isSteamStorefrontActive(storefronts: Storefront[]): boolean {
  const steamStorefront = storefronts.find(storefront => storefront.name === 'steam');
  return steamStorefront?.is_active ?? false;
}

/**
 * Checks if user has Steam Games feature enabled in their preferences
 */
export function isSteamGamesUserPreferenceEnabled(user: User | null): boolean {
  if (!user) return false;
  
  const preferences = user.preferences || {};
  const uiPreferences = preferences.ui || {};
  return uiPreferences.steam_games_visible !== false; // Default: enabled
}

/**
 * Comprehensive check for Steam Games feature availability
 * Mirrors the backend verify_steam_games_enabled() dependency logic:
 * 1. User has Steam Games UI feature enabled (default: True)
 * 2. PC-Windows platform exists and is active
 * 3. Steam storefront exists and is active
 */
export function isSteamGamesAvailable(
  user: User | null,
  platforms: Platform[],
  storefronts: Storefront[]
): boolean {
  // Check user preference
  if (!isSteamGamesUserPreferenceEnabled(user)) {
    return false;
  }

  // Check PC-Windows platform
  if (!isPCWindowsPlatformActive(platforms)) {
    return false;
  }

  // Check Steam storefront
  if (!isSteamStorefrontActive(storefronts)) {
    return false;
  }

  return true;
}

/**
 * Get the reason why Steam Games is not available (for debugging/error messages)
 */
export function getSteamGamesUnavailableReason(
  user: User | null,
  platforms: Platform[],
  storefronts: Storefront[]
): string | null {
  if (!user) {
    return 'User not authenticated';
  }

  if (!isSteamGamesUserPreferenceEnabled(user)) {
    return 'Steam Games feature is disabled in user preferences';
  }

  if (!isPCWindowsPlatformActive(platforms)) {
    const pcWindowsPlatform = platforms.find(platform => platform.name === 'pc-windows');
    if (!pcWindowsPlatform) {
      return 'PC-Windows platform not found';
    }
    return 'PC-Windows platform is inactive';
  }

  if (!isSteamStorefrontActive(storefronts)) {
    const steamStorefront = storefronts.find(storefront => storefront.name === 'steam');
    if (!steamStorefront) {
      return 'Steam storefront not found';
    }
    return 'Steam storefront is inactive';
  }

  return null; // Available
}