import { api } from './client';
import type {
  UserGame,
  UserGameId,
  Game,
  GameId,
  PlayStatus,
  OwnershipStatus,
  UserGamePlatform,
  Tag,
  IGDBGameCandidate,
} from '@/types';
import {
  transformPlatform,
  transformStorefront,
  type PlatformApiResponse,
  type StorefrontApiResponse,
} from './platforms';

// ============================================================================
// API Response Types (snake_case from backend)
// ============================================================================

interface GameApiResponse {
  id: number;
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

interface FilterOptionsApiResponse {
  genres: string[];
  game_modes: string[];
  themes: string[];
  player_perspectives: string[];
}

interface UserGamePlatformApiResponse {
  id: string;
  platform?: string;
  storefront?: string;
  platform_details?: PlatformApiResponse;
  storefront_details?: StorefrontApiResponse;
  is_available: boolean;
  hours_played: number;
  ownership_status: OwnershipStatus;
  acquired_date?: string;
  store_url?: string;
  created_at: string;
}

interface UserGameApiResponse {
  id: string;
  game: GameApiResponse;
  personal_rating?: number | null;
  is_loved: boolean;
  play_status: PlayStatus;
  hours_played: number;
  personal_notes?: string;
  platforms: UserGamePlatformApiResponse[];
  tags?: Tag[];
  created_at: string;
  updated_at: string;
}

interface UserGameListApiResponse {
  user_games: UserGameApiResponse[];
  total: number;
  page: number;
  per_page: number;
  pages: number;
}

interface IGDBGameCandidateApiResponse {
  igdb_id: number;
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
  user_game_id?: string;
}

interface IGDBSearchApiResponse {
  games: IGDBGameCandidateApiResponse[];
  total: number;
}

// ============================================================================
// Request Parameter Types
// ============================================================================

export interface GetUserGamesParams {
  status?: PlayStatus;
  ownershipStatus?: OwnershipStatus;
  platform?: string | string[];
  storefront?: string | string[];
  genre?: string[];
  gameMode?: string[];
  theme?: string[];
  playerPerspective?: string[];
  tags?: string[];
  search?: string;
  sortBy?: string;
  sortOrder?: 'asc' | 'desc';
  page?: number;
  perPage?: number;
  limit?: number;
  isLoved?: boolean;
  ratingMin?: number;
  ratingMax?: number;
  hasNotes?: boolean;
  fuzzyThreshold?: number;
}

export interface UserGameCreateData {
  gameId: GameId;
  playStatus?: PlayStatus;
  personalRating?: number | null;
  isLoved?: boolean;
  hoursPlayed?: number;
  personalNotes?: string;
  platforms?: Array<{
    platform: string;
    storefront?: string;
    isAvailable?: boolean;
    hoursPlayed?: number;
    ownershipStatus?: OwnershipStatus;
    acquiredDate?: string;
  }>;
}

export interface UserGameUpdateData {
  personalRating?: number | null;
  isLoved?: boolean;
  playStatus?: PlayStatus;
  hoursPlayed?: number;
  personalNotes?: string;
}

export interface UserGamePlatformData {
  platform: string;
  storefront?: string;
  isAvailable?: boolean;
  hoursPlayed?: number;
  ownershipStatus?: OwnershipStatus;
  acquiredDate?: string;
}

export interface BulkUpdateData {
  playStatus?: PlayStatus;
  ownershipStatus?: OwnershipStatus;
  personalRating?: number;
  isLoved?: boolean;
}

// ============================================================================
// Response Types
// ============================================================================

export interface UserGamesListResponse {
  items: UserGame[];
  total: number;
  page: number;
  perPage: number;
  pages: number;
}

// ============================================================================
// Transformation Functions
// ============================================================================

function transformUserGamePlatform(apiPlatform: UserGamePlatformApiResponse): UserGamePlatform {
  return {
    ...apiPlatform,
    platform_details: apiPlatform.platform_details
      ? transformPlatform(apiPlatform.platform_details)
      : undefined,
    storefront_details: apiPlatform.storefront_details
      ? transformStorefront(apiPlatform.storefront_details)
      : undefined,
  };
}

function transformGame(apiGame: GameApiResponse): Game {
  return { ...apiGame, id: apiGame.id as GameId };
}

function transformUserGame(apiUserGame: UserGameApiResponse): UserGame {
  return {
    ...apiUserGame,
    id: apiUserGame.id as UserGameId,
    game: transformGame(apiUserGame.game),
    platforms: (apiUserGame.platforms ?? []).map(transformUserGamePlatform),
    tags: apiUserGame.tags,
  };
}

function transformIGDBGameCandidate(apiCandidate: IGDBGameCandidateApiResponse): IGDBGameCandidate {
  return { ...apiCandidate, igdb_id: apiCandidate.igdb_id as GameId };
}

// ============================================================================
// Helper: Convert camelCase params to snake_case for API
// ============================================================================

/**
 * Helper function to append a value or array of values to URLSearchParams.
 * For arrays, appends each value with the same key (e.g., ?platform=windows&platform=ps5)
 */
function appendParam(
  searchParams: URLSearchParams,
  key: string,
  value: string | string[] | number | boolean | undefined | null,
): void {
  if (value === undefined || value === null) return;

  if (Array.isArray(value)) {
    value.forEach((v) => searchParams.append(key, String(v)));
  } else {
    searchParams.append(key, String(value));
  }
}

/**
 * Build query string for user games API requests.
 * Supports multi-value params by appending the same key multiple times.
 * Returns undefined if no params provided or all params are undefined.
 */
function buildUserGamesQueryParams(params?: GetUserGamesParams): string | undefined {
  if (!params) return undefined;

  const searchParams = new URLSearchParams();

  appendParam(searchParams, 'play_status', params.status);
  appendParam(searchParams, 'ownership_status', params.ownershipStatus);
  appendParam(searchParams, 'platform', params.platform);
  appendParam(searchParams, 'storefront', params.storefront);
  appendParam(searchParams, 'genre', params.genre);
  appendParam(searchParams, 'game_mode', params.gameMode);
  appendParam(searchParams, 'theme', params.theme);
  appendParam(searchParams, 'player_perspective', params.playerPerspective);
  appendParam(searchParams, 'tag', params.tags);
  appendParam(searchParams, 'q', params.search);
  appendParam(searchParams, 'sort_by', params.sortBy);
  appendParam(searchParams, 'sort_order', params.sortOrder);
  appendParam(searchParams, 'page', params.page);
  appendParam(searchParams, 'per_page', params.perPage);
  appendParam(searchParams, 'limit', params.limit);
  appendParam(searchParams, 'is_loved', params.isLoved);
  appendParam(searchParams, 'rating_min', params.ratingMin);
  appendParam(searchParams, 'rating_max', params.ratingMax);
  appendParam(searchParams, 'has_notes', params.hasNotes);
  appendParam(searchParams, 'fuzzy_threshold', params.fuzzyThreshold);

  const queryString = searchParams.toString();
  return queryString || undefined;
}

// ============================================================================
// API Functions
// ============================================================================

/**
 * Get user's game collection with optional filtering and pagination.
 */
export async function getUserGames(params?: GetUserGamesParams): Promise<UserGamesListResponse> {
  const queryString = buildUserGamesQueryParams(params);
  const path = queryString ? `/user-games?${queryString}` : '/user-games';
  const response = await api.get<UserGameListApiResponse>(path);

  return {
    items: response.user_games.map(transformUserGame),
    total: response.total,
    page: response.page,
    perPage: response.per_page,
    pages: response.pages,
  };
}

/**
 * Get a single user game by ID.
 */
export async function getUserGame(id: string): Promise<UserGame> {
  const response = await api.get<UserGameApiResponse>(`/user-games/${id}`);
  return transformUserGame(response);
}

/**
 * Add a game to the user's collection.
 */
export async function createUserGame(data: UserGameCreateData): Promise<UserGame> {
  const requestBody = {
    game_id: data.gameId,
    play_status: data.playStatus,
    personal_rating: data.personalRating,
    is_loved: data.isLoved,
    hours_played: data.hoursPlayed,
    personal_notes: data.personalNotes,
    platforms: data.platforms?.map((p) => ({
      platform: p.platform,
      storefront: p.storefront,
      is_available: p.isAvailable ?? true,
      hours_played: p.hoursPlayed,
      ownership_status: p.ownershipStatus,
      acquired_date: p.acquiredDate,
    })),
  };

  const response = await api.post<UserGameApiResponse>('/user-games', requestBody);
  return transformUserGame(response);
}

/**
 * Update a user game entry.
 */
export async function updateUserGame(id: string, data: UserGameUpdateData): Promise<UserGame> {
  const requestBody: Record<string, unknown> = {};

  if (data.personalRating !== undefined) {
    requestBody.personal_rating = data.personalRating;
  }
  if (data.isLoved !== undefined) {
    requestBody.is_loved = data.isLoved;
  }
  if (data.playStatus !== undefined) {
    requestBody.play_status = data.playStatus;
  }
  if (data.hoursPlayed !== undefined) {
    requestBody.hours_played = data.hoursPlayed;
  }
  if (data.personalNotes !== undefined) {
    requestBody.personal_notes = data.personalNotes;
  }

  const response = await api.put<UserGameApiResponse>(`/user-games/${id}`, requestBody);
  return transformUserGame(response);
}

/**
 * Remove a game from the user's collection.
 */
export async function deleteUserGame(id: string): Promise<void> {
  await api.delete(`/user-games/${id}`);
}

/**
 * Remove all games from the authenticated user's library.
 */
export async function clearLibrary(): Promise<{ deleted: number }> {
  return api.delete<{ deleted: number }>('/user-games');
}

/**
 * Search for games in the IGDB database.
 */
export async function searchIGDB(
  query: string,
  limit?: number,
  externalGameId?: string,
): Promise<IGDBGameCandidate[]> {
  const body: { query: string; limit: number; external_game_id?: string } = {
    query,
    limit: limit ?? 10,
  };
  if (externalGameId) {
    body.external_game_id = externalGameId;
  }
  const response = await api.post<IGDBSearchApiResponse>('/games/search/igdb', body);

  return response.games.map(transformIGDBGameCandidate);
}

/**
 * Get a game from IGDB by its ID.
 * Returns the same format as searchIGDB for consistency.
 */
export async function getGameByIGDBId(igdbId: number): Promise<IGDBGameCandidate[]> {
  const response = await api.get<IGDBSearchApiResponse>(`/games/igdb/${igdbId}`);
  return response.games.map(transformIGDBGameCandidate);
}

/**
 * Import a game from IGDB to the database.
 */
export async function importFromIGDB(igdbId: GameId, downloadCoverArt?: boolean): Promise<Game> {
  const response = await api.post<GameApiResponse>(
    '/games/igdb-import',
    { igdb_id: igdbId },
    {
      params: {
        download_cover_art: downloadCoverArt ?? true,
      },
    },
  );

  return transformGame(response);
}

/**
 * Bulk update multiple user games.
 */
export async function bulkUpdateUserGames(
  ids: string[],
  updates: BulkUpdateData,
): Promise<{ message: string; updatedCount: number; failedCount: number }> {
  const requestBody = {
    user_game_ids: ids,
    play_status: updates.playStatus,
    ownership_status: updates.ownershipStatus,
    personal_rating: updates.personalRating,
    is_loved: updates.isLoved,
  };

  const response = await api.put<{
    message: string;
    updated_count: number;
    failed_count: number;
  }>('/user-games/bulk-update', requestBody);

  return {
    message: response.message,
    updatedCount: response.updated_count,
    failedCount: response.failed_count,
  };
}

/**
 * Bulk delete multiple user games.
 */
export async function bulkDeleteUserGames(
  ids: string[],
): Promise<{ message: string; deletedCount: number; failedCount: number }> {
  const response = await api.delete<{
    message: string;
    deleted_count: number;
    failed_count: number;
  }>('/user-games/bulk-delete', { body: JSON.stringify({ user_game_ids: ids }) });

  return {
    message: response.message,
    deletedCount: response.deleted_count,
    failedCount: response.failed_count,
  };
}

/**
 * Get collection statistics for the current user.
 */
export async function getCollectionStats(): Promise<{
  totalGames: number;
  completionStats: Record<PlayStatus, number>;
  ownershipStats: Record<OwnershipStatus, number>;
  platformStats: Record<string, number>;
  genreStats: Record<string, number>;
  pileOfShame: number;
  completionRate: number;
  averageRating: number | null;
  totalHoursPlayed: number;
}> {
  const response = await api.get<{
    total_games: number;
    completion_stats: Record<PlayStatus, number>;
    ownership_stats: Record<OwnershipStatus, number>;
    platform_stats: Record<string, number>;
    genre_stats: Record<string, number>;
    pile_of_shame: number;
    completion_rate: number;
    average_rating: number | null;
    total_hours_played: number;
  }>('/user-games/stats');

  return {
    totalGames: response.total_games,
    completionStats: response.completion_stats,
    ownershipStats: response.ownership_stats,
    platformStats: response.platform_stats,
    genreStats: response.genre_stats,
    pileOfShame: response.pile_of_shame,
    completionRate: response.completion_rate,
    averageRating: response.average_rating,
    totalHoursPlayed: response.total_hours_played,
  };
}

/**
 * Get unique genres from the user's game collection.
 */
export async function getUserGameGenres(): Promise<string[]> {
  const response = await api.get<{ genres: string[] }>('/user-games/genres');
  return response.genres;
}

/**
 * Get all user game IDs matching filters (for bulk selection).
 */
export async function getUserGameIds(params?: GetUserGamesParams): Promise<string[]> {
  const queryString = buildUserGamesQueryParams(params);
  const path = queryString ? `/user-games/ids?${queryString}` : '/user-games/ids';
  const response = await api.get<{ ids: string[] }>(path);

  return response.ids;
}

/**
 * Add a platform association to a user game.
 */
export async function addPlatformToUserGame(
  userGameId: string,
  data: UserGamePlatformData,
): Promise<UserGamePlatform> {
  const requestBody = {
    platform: data.platform,
    storefront: data.storefront,
    is_available: data.isAvailable ?? true,
    hours_played: data.hoursPlayed ?? 0,
    ownership_status: data.ownershipStatus,
    acquired_date: data.acquiredDate,
  };

  const response = await api.post<UserGamePlatformApiResponse>(
    `/user-games/${userGameId}/platforms`,
    requestBody,
  );
  return transformUserGamePlatform(response);
}

/**
 * Update a platform association on a user game.
 */
export async function updatePlatformAssociation(
  userGameId: string,
  platformAssociationId: string,
  data: UserGamePlatformData,
): Promise<UserGamePlatform> {
  const requestBody = {
    platform: data.platform,
    storefront: data.storefront,
    is_available: data.isAvailable ?? true,
    hours_played: data.hoursPlayed ?? 0,
    ownership_status: data.ownershipStatus,
    acquired_date: data.acquiredDate,
  };

  const response = await api.put<UserGamePlatformApiResponse>(
    `/user-games/${userGameId}/platforms/${platformAssociationId}`,
    requestBody,
  );
  return transformUserGamePlatform(response);
}

/**
 * Remove a platform association from a user game.
 */
export async function removePlatformFromUserGame(
  userGameId: string,
  platformAssociationId: string,
): Promise<void> {
  await api.delete(`/user-games/${userGameId}/platforms/${platformAssociationId}`);
}

/**
 * Filter options response type for frontend consumption.
 */
export interface FilterOptions {
  genres: string[];
  gameModes: string[];
  themes: string[];
  playerPerspectives: string[];
}

/**
 * Get filter options (unique values) from the user's game collection.
 * Returns only values that exist in the user's collection for each filter type.
 */
export async function getFilterOptions(): Promise<FilterOptions> {
  const response = await api.get<FilterOptionsApiResponse>('/user-games/filter-options');
  return {
    genres: response.genres,
    gameModes: response.game_modes,
    themes: response.themes,
    playerPerspectives: response.player_perspectives,
  };
}
