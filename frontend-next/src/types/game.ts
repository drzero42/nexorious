import type { Platform, Storefront } from './platform';

// Branded types for type-safe IDs
export type GameId = number & { readonly __brand: 'GameId' };
export type UserGameId = string & { readonly __brand: 'UserGameId' };

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

export enum OwnershipStatus {
  OWNED = 'owned',
  BORROWED = 'borrowed',
  RENTED = 'rented',
  SUBSCRIPTION = 'subscription',
  NO_LONGER_OWNED = 'no_longer_owned',
}

export enum PlayStatus {
  NOT_STARTED = 'not_started',
  IN_PROGRESS = 'in_progress',
  COMPLETED = 'completed',
  MASTERED = 'mastered',
  DOMINATED = 'dominated',
  SHELVED = 'shelved',
  DROPPED = 'dropped',
  REPLAY = 'replay',
}

export interface Game {
  id: GameId;
  title: string;
  description?: string;
  genre?: string;
  developer?: string;
  publisher?: string;
  release_date?: string;
  cover_art_url?: string;
  rating_average?: number;
  rating_count: number;
  estimated_playtime_hours?: number;
  howlongtobeat_main?: number;
  howlongtobeat_extra?: number;
  howlongtobeat_completionist?: number;
  igdb_slug?: string;
  igdb_platform_names?: string;
  created_at: string;
  updated_at: string;
}

export interface UserGamePlatform {
  id: string;
  platform: Platform;
  storefront?: Storefront;
  store_game_id?: string;
  store_url?: string;
  is_available: boolean;
  created_at: string;
}

export interface Tag {
  id: string;
  user_id: string;
  name: string;
  color: string;
  description?: string;
  created_at: string;
  updated_at: string;
  game_count?: number;
}

export interface UserGame {
  id: UserGameId;
  game: Game;
  ownership_status: OwnershipStatus;
  is_physical: boolean;
  physical_location?: string;
  personal_rating?: number | null;
  is_loved: boolean;
  play_status: PlayStatus;
  hours_played: number;
  personal_notes?: string;
  acquired_date?: string;
  platforms: UserGamePlatform[];
  tags?: Tag[];
  created_at: string;
  updated_at: string;
}

export interface UserGameFilters {
  q?: string;
  play_status?: PlayStatus;
  ownership_status?: OwnershipStatus;
  platform_id?: string;
  tag_id?: string;
  is_loved?: boolean;
  sort_by?: string;
  sort_order?: 'asc' | 'desc';
  page?: number;
  per_page?: number;
}

export interface PaginatedResponse<T> {
  items: T[];
  total: number;
  page: number;
  per_page: number;
  pages: number;
}

export interface IGDBGameCandidate {
  igdb_id: GameId;
  igdb_slug?: string;
  title: string;
  release_date?: string;
  cover_art_url?: string;
  description?: string;
  platforms: string[];
  howlongtobeat_main?: number;
  howlongtobeat_extra?: number;
  howlongtobeat_completionist?: number;
}

export interface UserGameCreateRequest {
  game_id: GameId;
  ownership_status?: OwnershipStatus;
  play_status?: PlayStatus;
  platforms?: Array<{
    platform_id: string;
    storefront_id?: string;
  }>;
}

export interface UserGameUpdateRequest {
  ownership_status?: OwnershipStatus;
  is_physical?: boolean;
  physical_location?: string;
  personal_rating?: number | null;
  is_loved?: boolean;
  play_status?: PlayStatus;
  hours_played?: number;
  personal_notes?: string;
  acquired_date?: string;
}
