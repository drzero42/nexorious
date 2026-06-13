import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import * as poolsApi from '@/api/pools';
import { getPoolSuggestions, type PoolSuggestionsParams } from '@/api/games';
import type { PoolListItem, PoolDetail, PoolMembership } from '@/types';
import type { UserGamesListResponse } from '@/api/games';

export const poolKeys = {
  all: ['pools'] as const,
  lists: () => [...poolKeys.all, 'list'] as const,
  details: () => [...poolKeys.all, 'detail'] as const,
  detail: (id: string) => [...poolKeys.details(), id] as const,
  // Omit the params element when absent so the broad-invalidation key
  // (`['pools','suggestions',id]`) is a true prefix of the active query key
  // (`[...,id,{sort,page}]`). A trailing `undefined` would NOT partial-match,
  // leaving the suggestions grid stale after add/remove/queue mutations.
  suggestions: (id: string, params?: Omit<PoolSuggestionsParams, 'poolId'>) =>
    params === undefined
      ? ([...poolKeys.all, 'suggestions', id] as const)
      : ([...poolKeys.all, 'suggestions', id, params] as const),
  memberships: (userGameId: string) => [...poolKeys.all, 'memberships', userGameId] as const,
};

export function usePools() {
  return useQuery<PoolListItem[], Error>({
    queryKey: poolKeys.lists(),
    queryFn: poolsApi.getPools,
  });
}

export function usePool(id: string | undefined) {
  return useQuery<PoolDetail, Error>({
    queryKey: poolKeys.detail(id ?? ''),
    queryFn: () => poolsApi.getPool(id!),
    enabled: !!id,
  });
}

export function usePoolSuggestions(params: PoolSuggestionsParams) {
  const { poolId, ...rest } = params;
  return useQuery<UserGamesListResponse, Error>({
    queryKey: poolKeys.suggestions(poolId, rest),
    queryFn: () => getPoolSuggestions(params),
    enabled: !!poolId,
  });
}

export function useGamePoolMemberships(userGameId: string | undefined) {
  return useQuery<PoolMembership[], Error>({
    queryKey: poolKeys.memberships(userGameId ?? ''),
    queryFn: () => poolsApi.getGamePoolMemberships(userGameId!),
    enabled: !!userGameId,
  });
}

export function useCreatePool() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (data: poolsApi.PoolCreateData) => poolsApi.createPool(data),
    onSuccess: () => qc.invalidateQueries({ queryKey: poolKeys.lists() }),
  });
}

export function useUpdatePool() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, data }: { id: string; data: poolsApi.PoolUpdateData }) =>
      poolsApi.updatePool(id, data),
    onSuccess: (_r, { id }) => {
      qc.invalidateQueries({ queryKey: poolKeys.lists() });
      qc.invalidateQueries({ queryKey: poolKeys.detail(id) });
      qc.invalidateQueries({ queryKey: poolKeys.suggestions(id) });
    },
  });
}

export function useDeletePool() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => poolsApi.deletePool(id),
    onSuccess: () => qc.invalidateQueries({ queryKey: poolKeys.lists() }),
  });
}

export function useReorderPools() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (ids: string[]) => poolsApi.reorderPools(ids),
    onSuccess: () => qc.invalidateQueries({ queryKey: poolKeys.lists() }),
  });
}

// Membership + queue mutations all invalidate the affected pool detail, its
// suggestions, and any open per-game membership query. The components layer
// optimistic queue ordering on top via setQueryData before calling these.
export function useAddPoolGame() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ poolId, userGameId }: { poolId: string; userGameId: string }) =>
      poolsApi.addPoolGame(poolId, userGameId),
    onSuccess: (_r, { poolId, userGameId }) => {
      qc.invalidateQueries({ queryKey: poolKeys.detail(poolId) });
      qc.invalidateQueries({ queryKey: poolKeys.suggestions(poolId) });
      qc.invalidateQueries({ queryKey: poolKeys.memberships(userGameId) });
      qc.invalidateQueries({ queryKey: poolKeys.lists() });
    },
  });
}

export function useRemovePoolGame() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ poolId, userGameId }: { poolId: string; userGameId: string }) =>
      poolsApi.removePoolGame(poolId, userGameId),
    onSuccess: (_r, { poolId, userGameId }) => {
      qc.invalidateQueries({ queryKey: poolKeys.detail(poolId) });
      qc.invalidateQueries({ queryKey: poolKeys.suggestions(poolId) });
      qc.invalidateQueries({ queryKey: poolKeys.memberships(userGameId) });
      qc.invalidateQueries({ queryKey: poolKeys.lists() });
    },
  });
}

export function useSetQueue() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ poolId, ids }: { poolId: string; ids: string[] }) =>
      poolsApi.setQueue(poolId, ids),
    onSuccess: (_r, { poolId }) => {
      qc.invalidateQueries({ queryKey: poolKeys.detail(poolId) });
      qc.invalidateQueries({ queryKey: poolKeys.lists() });
    },
  });
}
