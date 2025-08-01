/**
 * Utility functions for formatting various data types for display
 */

/**
 * Formats an ownership status value for display by replacing underscores with spaces
 * and capitalizing each word properly.
 * 
 * @param status - The ownership status string (e.g., "no_longer_owned")
 * @returns Formatted status string (e.g., "No Longer Owned")
 */
export function formatOwnershipStatus(status: string): string {
  return status
    .split('_')
    .map(word => word.charAt(0).toUpperCase() + word.slice(1).toLowerCase())
    .join(' ');
}

/**
 * Converts IGDB rating from integer format (0-100) to decimal format (0.0-10.0).
 * IGDB provides ratings as integers from 0 to 100, but they should be displayed
 * as decimal values from 0.0 to 10.0 for better user understanding.
 * 
 * @param igdbRating - The IGDB rating as an integer (0-100) or null/undefined
 * @returns Decimal rating (0.0-10.0) or null if input is invalid
 * 
 * @example
 * formatIgdbRating(85) // returns 8.5
 * formatIgdbRating(0) // returns 0.0
 * formatIgdbRating(100) // returns 10.0
 * formatIgdbRating(null) // returns null
 */
export function formatIgdbRating(igdbRating: number | null | undefined): number | null {
  // Handle null, undefined, or invalid inputs
  if (igdbRating == null || typeof igdbRating !== 'number' || isNaN(igdbRating)) {
    return null;
  }
  
  // Handle out of range values - clamp to valid IGDB range (0-100)
  const clampedRating = Math.max(0, Math.min(100, igdbRating));
  
  // Convert from 0-100 scale to 0.0-10.0 scale
  // Round to avoid floating point precision issues
  return Math.round((clampedRating / 10) * 10000) / 10000;
}