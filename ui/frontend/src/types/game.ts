import type { Platform, Storefront } from './platform';

// Branded types for type-safe IDs
export type GameId = number & { readonly __brand: 'GameId' };
export type UserGameId = string & { readonly __brand: 'UserGameId' };

// Type guard for game IDs
function isGameId(value: unknown): value is GameId {
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

/**
 * Selection mode for bulk game selection.
 * - manual: Individual games selected by clicking
 * - all-visible: All currently loaded games selected
 * - all-collection: All games in collection selected (fetched from API)
 */
export type SelectionMode = 'manual' | 'all-visible' | 'all-collection';

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
  game_metadata?: string;
  howlongtobeat_main?: number;
  howlongtobeat_extra?: number;
  howlongtobeat_completionist?: number;
  igdb_slug?: string;
  igdb_platform_names?: string;
  game_modes?: string;
  themes?: string;
  player_perspectives?: string;
  created_at: string;
  updated_at: string;
}

export interface UserGamePlatform {
  id: string;
  platform?: string;
  storefront?: string;
  platform_details?: Platform;
  storefront_details?: Storefront;
  store_url?: string;
  is_available: boolean;
  hours_played: number;
  ownership_status: OwnershipStatus;
  acquired_date?: string;
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
  personal_rating?: number | null;
  is_loved: boolean;
  play_status: PlayStatus;
  is_wishlisted: boolean;
  hours_played: number;
  personal_notes?: string;
  platforms: UserGamePlatform[];
  tags?: Tag[];
  created_at: string;
  updated_at: string;
}

export interface IGDBGameCandidate {
  igdb_id: GameId;
  igdb_slug?: string;
  title: string;
  release_date?: string;
  cover_art_url?: string;
  description?: string;
  platforms: string[];
  platform_ids?: number[];
  howlongtobeat_main?: number;
  howlongtobeat_extra?: number;
  howlongtobeat_completionist?: number;
  /**
   * The id of the requesting user's existing library entry for this game, or
   * undefined when the game is not yet in their library. Set by the IGDB search
   * endpoint so the Add Game UI can surface "already in library" and link to the
   * edit page instead of re-adding (#856).
   */
  user_game_id?: string;
}
