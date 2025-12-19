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
import type { Platform, Storefront } from '@/types/platform';

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
  estimated_playtime_hours?: number;
  howlongtobeat_main?: number;
  howlongtobeat_extra?: number;
  howlongtobeat_completionist?: number;
  igdb_slug?: string;
  igdb_platform_names?: string;
  created_at: string;
  updated_at: string;
}

interface PlatformApiResponse {
  id: string;
  name: string;
  display_name: string;
  icon_url?: string;
  is_active: boolean;
  source: string;
  default_storefront_id?: string;
  storefronts?: StorefrontApiResponse[];
  created_at: string;
  updated_at: string;
}

interface StorefrontApiResponse {
  id: string;
  name: string;
  display_name: string;
  icon_url?: string;
  base_url?: string;
  is_active: boolean;
  source: string;
  created_at: string;
  updated_at: string;
}

interface UserGamePlatformApiResponse {
  id: string;
  platform_id?: string;
  storefront_id?: string;
  platform?: PlatformApiResponse;
  storefront?: StorefrontApiResponse;
  store_game_id?: string;
  store_url?: string;
  is_available: boolean;
  original_platform_name?: string;
  created_at: string;
}

interface TagApiResponse {
  id: string;
  user_id: string;
  name: string;
  color: string;
  description?: string;
  created_at: string;
  updated_at: string;
  game_count?: number;
}

interface UserGameApiResponse {
  id: string;
  game: GameApiResponse;
  ownership_status: OwnershipStatus;
  personal_rating?: number | null;
  is_loved: boolean;
  play_status: PlayStatus;
  hours_played: number;
  personal_notes?: string;
  acquired_date?: string;
  platforms: UserGamePlatformApiResponse[];
  tags?: TagApiResponse[];
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
  howlongtobeat_main?: number;
  howlongtobeat_extra?: number;
  howlongtobeat_completionist?: number;
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
  platformId?: string;
  storefrontId?: string;
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
  ownershipStatus?: OwnershipStatus;
  playStatus?: PlayStatus;
  personalRating?: number | null;
  isLoved?: boolean;
  hoursPlayed?: number;
  personalNotes?: string;
  acquiredDate?: string;
  platforms?: Array<{
    platformId: string;
    storefrontId?: string;
    storeGameId?: string;
    storeUrl?: string;
    isAvailable?: boolean;
  }>;
}

export interface UserGameUpdateData {
  ownershipStatus?: OwnershipStatus;
  personalRating?: number | null;
  isLoved?: boolean;
  playStatus?: PlayStatus;
  hoursPlayed?: number;
  personalNotes?: string;
  acquiredDate?: string;
}

export interface UserGamePlatformData {
  platformId: string;
  storefrontId?: string;
  storeGameId?: string;
  storeUrl?: string;
  isAvailable?: boolean;
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

function transformPlatform(apiPlatform: PlatformApiResponse): Platform {
  return {
    id: apiPlatform.id,
    name: apiPlatform.name,
    display_name: apiPlatform.display_name,
    icon_url: apiPlatform.icon_url,
    is_active: apiPlatform.is_active,
    source: apiPlatform.source,
    default_storefront_id: apiPlatform.default_storefront_id,
    storefronts: apiPlatform.storefronts?.map(transformStorefront),
    created_at: apiPlatform.created_at,
    updated_at: apiPlatform.updated_at,
  };
}

function transformStorefront(apiStorefront: StorefrontApiResponse): Storefront {
  return {
    id: apiStorefront.id,
    name: apiStorefront.name,
    display_name: apiStorefront.display_name,
    icon_url: apiStorefront.icon_url,
    base_url: apiStorefront.base_url,
    is_active: apiStorefront.is_active,
    source: apiStorefront.source,
    created_at: apiStorefront.created_at,
    updated_at: apiStorefront.updated_at,
  };
}

function transformUserGamePlatform(
  apiPlatform: UserGamePlatformApiResponse
): UserGamePlatform {
  return {
    id: apiPlatform.id,
    platform_id: apiPlatform.platform_id,
    storefront_id: apiPlatform.storefront_id,
    platform: apiPlatform.platform
      ? transformPlatform(apiPlatform.platform)
      : undefined,
    storefront: apiPlatform.storefront
      ? transformStorefront(apiPlatform.storefront)
      : undefined,
    store_game_id: apiPlatform.store_game_id,
    store_url: apiPlatform.store_url,
    is_available: apiPlatform.is_available,
    original_platform_name: apiPlatform.original_platform_name,
    created_at: apiPlatform.created_at,
  };
}

function transformTag(apiTag: TagApiResponse): Tag {
  return {
    id: apiTag.id,
    user_id: apiTag.user_id,
    name: apiTag.name,
    color: apiTag.color,
    description: apiTag.description,
    created_at: apiTag.created_at,
    updated_at: apiTag.updated_at,
    game_count: apiTag.game_count,
  };
}

function transformGame(apiGame: GameApiResponse): Game {
  return {
    id: apiGame.id as GameId,
    title: apiGame.title,
    description: apiGame.description,
    genre: apiGame.genre,
    developer: apiGame.developer,
    publisher: apiGame.publisher,
    release_date: apiGame.release_date,
    cover_art_url: apiGame.cover_art_url,
    rating_average: apiGame.rating_average,
    rating_count: apiGame.rating_count,
    game_metadata: apiGame.game_metadata,
    estimated_playtime_hours: apiGame.estimated_playtime_hours,
    howlongtobeat_main: apiGame.howlongtobeat_main,
    howlongtobeat_extra: apiGame.howlongtobeat_extra,
    howlongtobeat_completionist: apiGame.howlongtobeat_completionist,
    igdb_slug: apiGame.igdb_slug,
    igdb_platform_names: apiGame.igdb_platform_names,
    created_at: apiGame.created_at,
    updated_at: apiGame.updated_at,
  };
}

function transformUserGame(apiUserGame: UserGameApiResponse): UserGame {
  return {
    id: apiUserGame.id as UserGameId,
    game: transformGame(apiUserGame.game),
    ownership_status: apiUserGame.ownership_status,
    personal_rating: apiUserGame.personal_rating,
    is_loved: apiUserGame.is_loved,
    play_status: apiUserGame.play_status,
    hours_played: apiUserGame.hours_played,
    personal_notes: apiUserGame.personal_notes,
    acquired_date: apiUserGame.acquired_date,
    platforms: apiUserGame.platforms.map(transformUserGamePlatform),
    tags: apiUserGame.tags?.map(transformTag),
    created_at: apiUserGame.created_at,
    updated_at: apiUserGame.updated_at,
  };
}

function transformIGDBGameCandidate(
  apiCandidate: IGDBGameCandidateApiResponse
): IGDBGameCandidate {
  return {
    igdb_id: apiCandidate.igdb_id as GameId,
    igdb_slug: apiCandidate.igdb_slug,
    title: apiCandidate.title,
    release_date: apiCandidate.release_date,
    cover_art_url: apiCandidate.cover_art_url,
    description: apiCandidate.description,
    platforms: apiCandidate.platforms,
    howlongtobeat_main: apiCandidate.howlongtobeat_main,
    howlongtobeat_extra: apiCandidate.howlongtobeat_extra,
    howlongtobeat_completionist: apiCandidate.howlongtobeat_completionist,
  };
}

// ============================================================================
// Helper: Convert camelCase params to snake_case for API
// ============================================================================

function buildUserGamesQueryParams(
  params?: GetUserGamesParams
): Record<string, string | number | boolean | undefined> {
  if (!params) return {};

  return {
    play_status: params.status,
    ownership_status: params.ownershipStatus,
    platform_id: params.platformId,
    storefront_id: params.storefrontId,
    q: params.search,
    sort_by: params.sortBy,
    sort_order: params.sortOrder,
    page: params.page,
    per_page: params.perPage,
    limit: params.limit,
    is_loved: params.isLoved,
    rating_min: params.ratingMin,
    rating_max: params.ratingMax,
    has_notes: params.hasNotes,
    fuzzy_threshold: params.fuzzyThreshold,
  };
}

// ============================================================================
// API Functions
// ============================================================================

/**
 * Get user's game collection with optional filtering and pagination.
 */
export async function getUserGames(
  params?: GetUserGamesParams
): Promise<UserGamesListResponse> {
  const queryParams = buildUserGamesQueryParams(params);
  const response = await api.get<UserGameListApiResponse>('/user-games/', {
    params: queryParams,
  });

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
    ownership_status: data.ownershipStatus,
    play_status: data.playStatus,
    personal_rating: data.personalRating,
    is_loved: data.isLoved,
    hours_played: data.hoursPlayed,
    personal_notes: data.personalNotes,
    acquired_date: data.acquiredDate,
    platforms: data.platforms?.map((p) => ({
      platform_id: p.platformId,
      storefront_id: p.storefrontId,
      store_game_id: p.storeGameId,
      store_url: p.storeUrl,
      is_available: p.isAvailable ?? true,
    })),
  };

  const response = await api.post<UserGameApiResponse>('/user-games/', requestBody);
  return transformUserGame(response);
}

/**
 * Update a user game entry.
 */
export async function updateUserGame(
  id: string,
  data: UserGameUpdateData
): Promise<UserGame> {
  const requestBody: Record<string, unknown> = {};

  if (data.ownershipStatus !== undefined) {
    requestBody.ownership_status = data.ownershipStatus;
  }
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
  if (data.acquiredDate !== undefined) {
    requestBody.acquired_date = data.acquiredDate;
  }

  const response = await api.put<UserGameApiResponse>(
    `/user-games/${id}`,
    requestBody
  );
  return transformUserGame(response);
}

/**
 * Remove a game from the user's collection.
 */
export async function deleteUserGame(id: string): Promise<void> {
  await api.delete(`/user-games/${id}`);
}

/**
 * Search for games in the IGDB database.
 */
export async function searchIGDB(
  query: string,
  limit?: number
): Promise<IGDBGameCandidate[]> {
  const response = await api.post<IGDBSearchApiResponse>('/games/search/igdb', {
    query,
    limit: limit ?? 10,
  });

  return response.games.map(transformIGDBGameCandidate);
}

/**
 * Import a game from IGDB to the database.
 */
export async function importFromIGDB(
  igdbId: GameId,
  downloadCoverArt?: boolean
): Promise<Game> {
  const response = await api.post<GameApiResponse>(
    '/games/igdb-import',
    { igdb_id: igdbId },
    {
      params: {
        download_cover_art: downloadCoverArt ?? true,
      },
    }
  );

  return transformGame(response);
}

/**
 * Bulk update multiple user games.
 */
export async function bulkUpdateUserGames(
  ids: string[],
  updates: BulkUpdateData
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
  ids: string[]
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
 * Add a platform association to a user game.
 */
export async function addPlatformToUserGame(
  userGameId: string,
  data: UserGamePlatformData
): Promise<UserGamePlatform> {
  const requestBody = {
    platform_id: data.platformId,
    storefront_id: data.storefrontId,
    store_game_id: data.storeGameId,
    store_url: data.storeUrl,
    is_available: data.isAvailable ?? true,
  };

  const response = await api.post<UserGamePlatformApiResponse>(
    `/user-games/${userGameId}/platforms`,
    requestBody
  );
  return transformUserGamePlatform(response);
}

/**
 * Update a platform association on a user game.
 */
export async function updatePlatformAssociation(
  userGameId: string,
  platformAssociationId: string,
  data: UserGamePlatformData
): Promise<UserGamePlatform> {
  const requestBody = {
    platform_id: data.platformId,
    storefront_id: data.storefrontId,
    store_game_id: data.storeGameId,
    store_url: data.storeUrl,
    is_available: data.isAvailable ?? true,
  };

  const response = await api.put<UserGamePlatformApiResponse>(
    `/user-games/${userGameId}/platforms/${platformAssociationId}`,
    requestBody
  );
  return transformUserGamePlatform(response);
}

/**
 * Remove a platform association from a user game.
 */
export async function removePlatformFromUserGame(
  userGameId: string,
  platformAssociationId: string
): Promise<void> {
  await api.delete(`/user-games/${userGameId}/platforms/${platformAssociationId}`);
}
