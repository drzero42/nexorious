import { config } from '$lib/env';

/**
 * Resolves relative image URLs to absolute URLs pointing to the backend API
 * @param imageUrl - The image URL (can be relative or absolute)
 * @returns Absolute URL or empty string if input is null/undefined
 */
export function resolveImageUrl(imageUrl?: string | null): string {
  if (!imageUrl) {
    return '';
  }

  // If URL starts with /static/, make it absolute by prefixing with static URL
  if (imageUrl.startsWith('/static/')) {
    return `${config.staticUrl}${imageUrl}`;
  }

  // Return as-is for absolute URLs or other relative URLs
  return imageUrl;
}