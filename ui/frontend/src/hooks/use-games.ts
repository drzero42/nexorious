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
 * Regex to detect the explicit IGDB ID lookup format: igdb:12345 (case-insensitive)
 */
const IGDB_ID_PATTERN = /^igdb:(\d+)$/i;

/**
 * Regex to detect a bare numeric query (e.g. 12345) — treated as an IGDB ID.
 */
const BARE_ID_PATTERN = /^\d+$/;

/**
 * Parse an IGDB ID from the explicit "igdb:12345" prefix format.
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
 * Merge an ID-lookup result (pinned first) with name-search results,
 * de-duplicating by igdb_id so a game found both ways appears once at the top.
 */
function mergeIGDBResults(
  idResults: IGDBGameCandidate[] | undefined,
  nameResults: IGDBGameCandidate[] | undefined,
): IGDBGameCandidate[] {
  const pinned = idResults ?? [];
  const pinnedIds = new Set(pinned.map((g) => g.igdb_id));
  const rest = (nameResults ?? []).filter((g) => !pinnedIds.has(g.igdb_id));
  return [...pinned, ...rest];
}

/**
 * Supports IGDB ID lookup and name search, which can run together:
 * 1. Explicit ID lookup: "igdb:12345" (case-insensitive) — pure ID lookup, no name search.
 * 2. Bare number: "12345" — fires BOTH an ID lookup and a name search, with the
 *    ID-lookup result pinned at the top of the merged, de-duped list. This keeps
 *    purely-numeric game titles (2048, 1942, …) discoverable by name while still
 *    honoring the inferred ID lookup.
 * 3. Name search: any other query (requires 3+ characters).
 */
export function useSearchIGDB(
  query: string,
  options?: { limit?: number; externalGameId?: string },
) {
  const limit = options?.limit;
  const externalGameId = options?.externalGameId;

  const prefixId = parseIGDBIdFromQuery(query);
  const isPrefixLookup = prefixId !== null;
  const bareId = BARE_ID_PATTERN.test(query) ? parseInt(query, 10) : null;
  const igdbId = prefixId ?? bareId;

  // ID lookup: any length (bare number or igdb: prefix).
  const idEnabled = igdbId !== null;
  // Name search: 3+ chars, but never for the pure igdb: prefix form.
  const nameEnabled = !isPrefixLookup && query.length >= 3;

  const idQuery = useQuery<IGDBGameCandidate[], Error>({
    queryKey: gameKeys.igdbById(igdbId ?? 0),
    queryFn: () => gamesApi.getGameByIGDBId(igdbId!),
    enabled: idEnabled,
  });

  const nameQuery = useQuery<IGDBGameCandidate[], Error>({
    queryKey: [...gameKeys.igdbSearch(query), externalGameId ?? null] as const,
    queryFn: () => gamesApi.searchIGDB(query, limit, externalGameId),
    enabled: nameEnabled,
  });

  const activeQueries = [...(idEnabled ? [idQuery] : []), ...(nameEnabled ? [nameQuery] : [])];
  const anyEnabled = activeQueries.length > 0;

  return {
    data: anyEnabled
      ? mergeIGDBResults(
          idEnabled ? idQuery.data : undefined,
          nameEnabled ? nameQuery.data : undefined,
        )
      : undefined,
    isLoading: activeQueries.some((q) => q.isLoading),
    isFetching: activeQueries.some((q) => q.isFetching),
    isError: activeQueries.some((q) => q.isError),
    error: activeQueries.find((q) => q.error)?.error ?? null,
    isSuccess: anyEnabled && activeQueries.every((q) => q.isSuccess),
    isPending: !anyEnabled || activeQueries.some((q) => q.isPending),
  };
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
 * replay games as two parallel calls and merges them (each keeps its own
 * perPage cap). Could collapse into one multi-status call now that the filter
 * accepts an array (#976), but the two-call shape is left as-is here.
 */
export function useActiveGames() {
  return useQuery<UserGamesListResponse, Error>({
    queryKey: ['user-games', 'active'],
    queryFn: async () => {
      const [inProgressData, replayData] = await Promise.all([
        gamesApi.getUserGames({ status: ['in_progress'], perPage: 50 }),
        gamesApi.getUserGames({ status: ['replay'], perPage: 50 }),
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
