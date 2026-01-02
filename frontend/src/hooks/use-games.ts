import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import * as gamesApi from '@/api/games';
import type {
  GetUserGamesParams,
  UserGameCreateData,
  UserGameUpdateData,
  UserGamesListResponse,
  BulkUpdateData,
} from '@/api/games';
import type { UserGame, IGDBGameCandidate, Game, GameId, UserGamePlatform, PlayStatus } from '@/types';

// ============================================================================
// Query Keys
// ============================================================================

export const gameKeys = {
  all: ['userGames'] as const,
  lists: () => [...gameKeys.all, 'list'] as const,
  list: (params?: GetUserGamesParams) => [...gameKeys.lists(), params] as const,
  details: () => [...gameKeys.all, 'detail'] as const,
  detail: (id: string) => [...gameKeys.details(), id] as const,
  stats: () => [...gameKeys.all, 'stats'] as const,
  igdbSearch: (query: string) => ['igdbSearch', query] as const,
};

// ============================================================================
// Query Hooks
// ============================================================================

/**
 * Hook to fetch user's game collection with pagination and filtering.
 */
export function useUserGames(params?: GetUserGamesParams) {
  return useQuery<UserGamesListResponse, Error>({
    queryKey: gameKeys.list(params),
    queryFn: () => gamesApi.getUserGames(params),
  });
}

/**
 * Hook to fetch a single user game by ID.
 */
export function useUserGame(id: string | undefined) {
  return useQuery<UserGame, Error>({
    queryKey: gameKeys.detail(id ?? ''),
    queryFn: () => gamesApi.getUserGame(id!),
    enabled: !!id,
  });
}

/**
 * Regex to detect IGDB ID lookup format: igdb:12345 (case-insensitive)
 */
const IGDB_ID_PATTERN = /^igdb:(\d+)$/i;

/**
 * Parse IGDB ID from query if it matches the igdb:12345 format.
 * Returns the numeric ID or null if not a match.
 */
function parseIGDBIdFromQuery(query: string): number | null {
  const match = query.match(IGDB_ID_PATTERN);
  if (match) {
    return parseInt(match[1], 10);
  }
  return null;
}

/**
 * Hook to search IGDB for games.
 *
 * Supports two modes:
 * 1. Direct ID lookup: Use "igdb:12345" format (case-insensitive)
 * 2. Name search: Any other query (requires 3+ characters)
 */
export function useSearchIGDB(query: string, limit?: number) {
  const igdbId = parseIGDBIdFromQuery(query);
  const isIdLookup = igdbId !== null;

  return useQuery<IGDBGameCandidate[], Error>({
    queryKey: gameKeys.igdbSearch(query),
    queryFn: () => {
      if (isIdLookup) {
        return gamesApi.getGameByIGDBId(igdbId);
      }
      return gamesApi.searchIGDB(query, limit);
    },
    // ID lookup: always enabled (no min chars)
    // Name search: require 3+ characters
    enabled: isIdLookup || query.length >= 3,
  });
}

/**
 * Hook to fetch collection statistics.
 */
export function useCollectionStats() {
  return useQuery({
    queryKey: gameKeys.stats(),
    queryFn: () => gamesApi.getCollectionStats(),
  });
}

/**
 * Hook to fetch unique genres from the user's game collection.
 */
export function useUserGameGenres() {
  return useQuery<string[], Error>({
    queryKey: [...gameKeys.all, 'genres'] as const,
    queryFn: () => gamesApi.getUserGameGenres(),
    staleTime: 5 * 60 * 1000, // 5 minutes
  });
}

/**
 * Hook to fetch active games (IN_PROGRESS and REPLAY statuses).
 * Used for the "Currently Playing" dashboard section.
 * Makes two parallel API calls since backend only supports single status filter.
 */
export function useActiveGames() {
  return useQuery<UserGamesListResponse, Error>({
    queryKey: ['user-games', 'active'],
    queryFn: async () => {
      // Fetch both statuses in parallel
      const [inProgressData, replayData] = await Promise.all([
        gamesApi.getUserGames({ status: 'in_progress' as PlayStatus, perPage: 50 }),
        gamesApi.getUserGames({ status: 'replay' as PlayStatus, perPage: 50 }),
      ]);

      // Merge results
      return {
        items: [...inProgressData.items, ...replayData.items],
        total: inProgressData.total + replayData.total,
        page: 1,
        perPage: 50,
        pages: 1,
      };
    },
    staleTime: 5 * 60 * 1000, // 5 minutes
  });
}

/**
 * Hook to fetch all user game IDs matching filters.
 * Disabled by default - call refetch() to trigger.
 */
export function useUserGameIds(params?: GetUserGamesParams, options?: { enabled?: boolean }) {
  return useQuery<string[], Error>({
    queryKey: [...gameKeys.lists(), 'ids', params] as const,
    queryFn: () => gamesApi.getUserGameIds(params),
    enabled: options?.enabled ?? false,
  });
}

// ============================================================================
// Mutation Hooks
// ============================================================================

/**
 * Hook to create a new user game entry.
 * Invalidates the user games list on success.
 */
export function useCreateUserGame() {
  const queryClient = useQueryClient();

  return useMutation<UserGame, Error, UserGameCreateData>({
    mutationFn: (data: UserGameCreateData) => gamesApi.createUserGame(data),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: gameKeys.lists() });
      queryClient.invalidateQueries({ queryKey: gameKeys.stats() });
    },
  });
}

/**
 * Hook to update an existing user game.
 * Invalidates both the list and the specific game detail on success.
 */
export function useUpdateUserGame() {
  const queryClient = useQueryClient();

  return useMutation<
    UserGame,
    Error,
    { id: string; data: UserGameUpdateData }
  >({
    mutationFn: ({ id, data }) => gamesApi.updateUserGame(id, data),
    onSuccess: (updatedGame, { id }) => {
      queryClient.invalidateQueries({ queryKey: gameKeys.lists() });
      queryClient.invalidateQueries({ queryKey: gameKeys.detail(id) });
      queryClient.invalidateQueries({ queryKey: gameKeys.stats() });
      queryClient.invalidateQueries({ queryKey: ['user-games', 'active'] });
      // Optionally set the updated data directly in the cache
      queryClient.setQueryData(gameKeys.detail(id), updatedGame);
    },
  });
}

/**
 * Hook to delete a user game from the collection.
 * Invalidates the user games list on success.
 */
export function useDeleteUserGame() {
  const queryClient = useQueryClient();

  return useMutation<void, Error, string>({
    mutationFn: (id: string) => gamesApi.deleteUserGame(id),
    onSuccess: (_data, id) => {
      queryClient.invalidateQueries({ queryKey: gameKeys.lists() });
      queryClient.invalidateQueries({ queryKey: gameKeys.stats() });
      queryClient.invalidateQueries({ queryKey: ['user-games', 'active'] });
      // Remove the specific game from cache
      queryClient.removeQueries({ queryKey: gameKeys.detail(id) });
    },
  });
}

/**
 * Hook to import a game from IGDB.
 */
export function useImportFromIGDB() {
  return useMutation<Game, Error, { igdbId: GameId; downloadCoverArt?: boolean }>({
    mutationFn: ({ igdbId, downloadCoverArt }) =>
      gamesApi.importFromIGDB(igdbId, downloadCoverArt),
  });
}

/**
 * Hook to bulk update multiple user games.
 * Invalidates the user games list on success.
 */
export function useBulkUpdateUserGames() {
  const queryClient = useQueryClient();

  return useMutation<
    { message: string; updatedCount: number; failedCount: number },
    Error,
    { ids: string[]; updates: BulkUpdateData }
  >({
    mutationFn: ({ ids, updates }) => gamesApi.bulkUpdateUserGames(ids, updates),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: gameKeys.lists() });
      queryClient.invalidateQueries({ queryKey: gameKeys.stats() });
      queryClient.invalidateQueries({ queryKey: ['user-games', 'active'] });
    },
  });
}

/**
 * Hook to bulk delete multiple user games.
 * Invalidates the user games list on success.
 */
export function useBulkDeleteUserGames() {
  const queryClient = useQueryClient();

  return useMutation<
    { message: string; deletedCount: number; failedCount: number },
    Error,
    string[]
  >({
    mutationFn: (ids: string[]) => gamesApi.bulkDeleteUserGames(ids),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: gameKeys.lists() });
      queryClient.invalidateQueries({ queryKey: gameKeys.stats() });
      queryClient.invalidateQueries({ queryKey: ['user-games', 'active'] });
    },
  });
}

/**
 * Hook to add a platform to a user game.
 */
export function useAddPlatformToUserGame() {
  const queryClient = useQueryClient();

  return useMutation<
    UserGamePlatform,
    Error,
    { userGameId: string; data: gamesApi.UserGamePlatformData }
  >({
    mutationFn: ({ userGameId, data }) =>
      gamesApi.addPlatformToUserGame(userGameId, data),
    onSuccess: (_result, { userGameId }) => {
      queryClient.invalidateQueries({ queryKey: gameKeys.detail(userGameId) });
      queryClient.invalidateQueries({ queryKey: gameKeys.lists() });
    },
  });
}

/**
 * Hook to update a platform association.
 */
export function useUpdatePlatformAssociation() {
  const queryClient = useQueryClient();

  return useMutation<
    UserGamePlatform,
    Error,
    {
      userGameId: string;
      platformAssociationId: string;
      data: gamesApi.UserGamePlatformData;
    }
  >({
    mutationFn: ({ userGameId, platformAssociationId, data }) =>
      gamesApi.updatePlatformAssociation(userGameId, platformAssociationId, data),
    onSuccess: (_result, { userGameId }) => {
      queryClient.invalidateQueries({ queryKey: gameKeys.detail(userGameId) });
    },
  });
}

/**
 * Hook to remove a platform from a user game.
 */
export function useRemovePlatformFromUserGame() {
  const queryClient = useQueryClient();

  return useMutation<
    void,
    Error,
    { userGameId: string; platformAssociationId: string }
  >({
    mutationFn: ({ userGameId, platformAssociationId }) =>
      gamesApi.removePlatformFromUserGame(userGameId, platformAssociationId),
    onSuccess: (_result, { userGameId }) => {
      queryClient.invalidateQueries({ queryKey: gameKeys.detail(userGameId) });
      queryClient.invalidateQueries({ queryKey: gameKeys.lists() });
    },
  });
}
