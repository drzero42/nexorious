import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import * as gamesApi from '@/api/games';
import type {
  GetUserGamesParams,
  UserGameCreateData,
  UserGameUpdateData,
  UserGamesListResponse,
  BulkUpdateData,
  FilterOptions,
} from '@/api/games';
import type { UserGame, IGDBGameCandidate, Game, GameId, UserGamePlatform } from '@/types';

// Query Keys

export const gameKeys = {
  all: ['userGames'] as const,
  lists: () => [...gameKeys.all, 'list'] as const,
  list: (params?: GetUserGamesParams) => [...gameKeys.lists(), params] as const,
  details: () => [...gameKeys.all, 'detail'] as const,
  detail: (id: string) => [...gameKeys.details(), id] as const,
  stats: () => [...gameKeys.all, 'stats'] as const,
  filterOptions: () => [...gameKeys.all, 'filterOptions'] as const,
  igdbSearch: (query: string) => ['igdbSearch', query] as const,
  igdbById: (id: number) => ['igdbById', id] as const,
};

// Query Hooks

export function useUserGames(params?: GetUserGamesParams) {
  return useQuery<UserGamesListResponse, Error>({
    queryKey: gameKeys.list(params),
    queryFn: () => gamesApi.getUserGames(params),
  });
}

export function useUserGame(id: string | undefined) {
  return useQuery<UserGame, Error>({
    queryKey: gameKeys.detail(id ?? ''),
    queryFn: () => gamesApi.getUserGame(id!),
    enabled: !!id,
  });
}

/**
 * Searches IGDB by name. The IGDB-ID query inference lives on the backend
 * (issue #1153): the server treats "igdb:NNNN" (case-insensitive) as a pure ID
 * lookup and a bare "NNNN" as an ID lookup merged with a name search (ID match
 * pinned first), so every front-end behaves identically. The hook just forwards
 * the query.
 *
 * Enabled for 3+ character queries, plus any purely-numeric query so a short
 * bare IGDB id (e.g. "12") still triggers a lookup.
 */
export function useSearchIGDB(
  query: string,
  options?: { limit?: number; externalGameId?: string },
) {
  const limit = options?.limit;
  const externalGameId = options?.externalGameId;
  const enabled = query.length >= 3 || /^\d+$/.test(query);

  return useQuery<IGDBGameCandidate[], Error>({
    queryKey: [...gameKeys.igdbSearch(query), externalGameId ?? null] as const,
    queryFn: () => gamesApi.searchIGDB(query, limit, externalGameId),
    enabled,
  });
}

/**
 * Used as a fallback on the confirm page when sessionStorage is empty (e.g. page reload).
 */
export function useIGDBGameByID(igdbId: number | null) {
  return useQuery<IGDBGameCandidate[], Error>({
    queryKey: gameKeys.igdbById(igdbId ?? 0),
    queryFn: () => gamesApi.getGameByIGDBId(igdbId!),
    enabled: igdbId !== null,
  });
}

export function useCollectionStats() {
  return useQuery({
    queryKey: gameKeys.stats(),
    queryFn: () => gamesApi.getCollectionStats(),
  });
}

export function useUserGameGenres() {
  return useQuery<string[], Error>({
    queryKey: [...gameKeys.all, 'genres'] as const,
    queryFn: () => gamesApi.getUserGameGenres(),
    staleTime: 5 * 60 * 1000, // 5 minutes
  });
}

/**
 * Returns genres, game modes, themes, and player perspectives from the user's collection.
 */
export function useFilterOptions() {
  return useQuery<FilterOptions, Error>({
    queryKey: gameKeys.filterOptions(),
    queryFn: () => gamesApi.getFilterOptions(),
    staleTime: 5 * 60 * 1000, // 5 minutes
  });
}

/**
 * Used for the "Currently Playing" dashboard section. Fetches in-progress and
 * replay games in a single multi-status call (the play-status filter accepts an
 * array since #976). perPage is 100 to preserve the combined cap of the former
 * two-call shape (50 in-progress + 50 replay).
 */
export function useActiveGames() {
  return useQuery<UserGamesListResponse, Error>({
    queryKey: ['user-games', 'active'],
    queryFn: () => gamesApi.getUserGames({ status: ['in_progress', 'replay'], perPage: 100 }),
    staleTime: 5 * 60 * 1000, // 5 minutes
  });
}

/**
 * Disabled by default - call refetch() to trigger.
 */
export function useUserGameIds(params?: GetUserGamesParams, options?: { enabled?: boolean }) {
  return useQuery<string[], Error>({
    queryKey: [...gameKeys.lists(), 'ids', params] as const,
    queryFn: () => gamesApi.getUserGameIds(params),
    enabled: options?.enabled ?? false,
  });
}

// Mutation Hooks

/**
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
 * Invalidates both the list and the specific game detail on success.
 */
export function useUpdateUserGame() {
  const queryClient = useQueryClient();

  return useMutation<UserGame, Error, { id: string; data: UserGameUpdateData }>({
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

export function useImportFromIGDB() {
  return useMutation<Game, Error, { igdbId: GameId; downloadCoverArt?: boolean }>({
    mutationFn: ({ igdbId, downloadCoverArt }) => gamesApi.importFromIGDB(igdbId, downloadCoverArt),
  });
}

/**
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

export function useAddPlatformToUserGame() {
  const queryClient = useQueryClient();

  return useMutation<
    UserGamePlatform,
    Error,
    { userGameId: string; data: gamesApi.UserGamePlatformData }
  >({
    mutationFn: ({ userGameId, data }) => gamesApi.addPlatformToUserGame(userGameId, data),
    onSuccess: (_result, { userGameId }) => {
      queryClient.invalidateQueries({ queryKey: gameKeys.detail(userGameId) });
      queryClient.invalidateQueries({ queryKey: gameKeys.lists() });
    },
  });
}

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

export function useMoveToLibrary() {
  const queryClient = useQueryClient();

  return useMutation<
    UserGame,
    Error,
    { userGameId: string; platforms: gamesApi.UserGamePlatformData[] }
  >({
    mutationFn: ({ userGameId, platforms }) => gamesApi.moveToLibrary(userGameId, platforms),
    onSuccess: (updated, { userGameId }) => {
      queryClient.invalidateQueries({ queryKey: gameKeys.lists() });
      queryClient.invalidateQueries({ queryKey: gameKeys.stats() });
      queryClient.invalidateQueries({ queryKey: ['user-games', 'active'] });
      queryClient.setQueryData(gameKeys.detail(userGameId), updated);
    },
  });
}

export function useRemovePlatformFromUserGame() {
  const queryClient = useQueryClient();

  return useMutation<void, Error, { userGameId: string; platformAssociationId: string }>({
    mutationFn: ({ userGameId, platformAssociationId }) =>
      gamesApi.removePlatformFromUserGame(userGameId, platformAssociationId),
    onSuccess: (_result, { userGameId }) => {
      queryClient.invalidateQueries({ queryKey: gameKeys.detail(userGameId) });
      queryClient.invalidateQueries({ queryKey: gameKeys.lists() });
    },
  });
}
