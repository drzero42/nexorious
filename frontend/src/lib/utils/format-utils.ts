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