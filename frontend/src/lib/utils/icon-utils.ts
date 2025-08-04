import { config } from '$lib/env';

/**
 * Builds a complete URL for platform or storefront icons from the backend
 * @param iconUrl - The icon URL path from the backend (e.g., "/static/logos/platforms/pc-windows/pc-windows-icon-light.svg")
 * @returns Complete URL to the icon, or null if iconUrl is empty
 */
export function buildIconUrl(iconUrl: string | null | undefined): string | null {
  if (!iconUrl || iconUrl.trim() === '') {
    return null;
  }
  
  // Remove leading slash if present since config.staticUrl may or may not have trailing slash
  const cleanPath = iconUrl.startsWith('/') ? iconUrl.slice(1) : iconUrl;
  
  // Ensure proper URL construction
  const baseUrl = config.staticUrl.endsWith('/') ? config.staticUrl.slice(0, -1) : config.staticUrl;
  
  return `${baseUrl}/${cleanPath}`;
}

/**
 * Gets a fallback icon for platforms when the backend icon is unavailable
 */
export function getPlatformFallbackIcon(): string {
  return '🎮'; // Generic gaming icon
}

/**
 * Gets a fallback icon for storefronts when the backend icon is unavailable  
 */
export function getStorefrontFallbackIcon(): string {
  return '🏪'; // Generic store icon
}