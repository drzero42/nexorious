/**
 * Core game type definitions with integer IGDB IDs as primary keys
 * 
 * The backend now uses integer IGDB IDs as primary keys for games,
 * replacing the previous UUID system.
 */

// Branded type for type-safe game IDs
export type GameId = number & { readonly __brand: 'GameId' };

// Type guard for game IDs
export function isGameId(value: unknown): value is GameId {
  return typeof value === 'number' && Number.isInteger(value) && value > 0;
}

// Convert a value to GameId with validation
export function toGameId(value: unknown): GameId {
  if (typeof value === 'string') {
    const parsed = parseInt(value, 10);
    if (!isNaN(parsed) && isGameId(parsed)) {
      return parsed as GameId;
    }
  } else if (isGameId(value)) {
    return value as GameId;
  }
  throw new Error(`Invalid game ID: ${value}`);
}

// Safe conversion that returns null on invalid input
export function toGameIdOrNull(value: unknown): GameId | null {
  try {
    return toGameId(value);
  } catch {
    return null;
  }
}

// Parse game ID from route parameters (which are always strings in SvelteKit)
export function parseGameIdParam(param: string | undefined): GameId | null {
  if (!param) return null;
  return toGameIdOrNull(param);
}

// User game IDs remain as UUIDs
export type UserGameId = string & { readonly __brand: 'UserGameId' };

// Type guard for user game IDs (UUIDs)
export function isUserGameId(value: unknown): value is UserGameId {
  if (typeof value !== 'string') return false;
  // Basic UUID v4 regex pattern
  const uuidRegex = /^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i;
  return uuidRegex.test(value);
}

// Convert a value to UserGameId with validation
export function toUserGameId(value: unknown): UserGameId {
  if (isUserGameId(value)) {
    return value as UserGameId;
  }
  throw new Error(`Invalid user game ID (UUID expected): ${value}`);
}

// Safe conversion that returns null on invalid input
export function toUserGameIdOrNull(value: unknown): UserGameId | null {
  try {
    return toUserGameId(value);
  } catch {
    return null;
  }
}