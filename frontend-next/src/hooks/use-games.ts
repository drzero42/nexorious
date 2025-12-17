import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import * as gamesApi from '@/api/games';
import type {
  GetUserGamesParams,
  UserGameCreateData,
  UserGameUpdateData,
  UserGamesListResponse,
  BulkUpdateData,
} from '@/api/games';
import type { UserGame, IGDBGameCandidate, Game, GameId, UserGamePlatform } from '@/types';

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
 * Hook to search IGDB for games.
 * Only enabled when query has at least 3 characters.
 */
export function useSearchIGDB(query: string, limit?: number) {
  return useQuery<IGDBGameCandidate[], Error>({
    queryKey: gameKeys.igdbSearch(query),
    queryFn: () => gamesApi.searchIGDB(query, limit),
    enabled: query.length >= 3,
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
