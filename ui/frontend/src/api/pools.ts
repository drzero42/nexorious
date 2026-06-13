import { api } from './client';
import type { PoolListItem, Pool, PoolDetail, PoolFilter, PoolMembership } from '@/types';

export interface PoolCreateData {
  name: string;
  color?: string | null;
  filter?: PoolFilter | null;
}

export interface PoolUpdateData {
  name?: string;
  color?: string | null;
  filter?: PoolFilter | null;
}

export async function getPools(): Promise<PoolListItem[]> {
  return api.get<PoolListItem[]>('/pools');
}

export async function getPool(id: string): Promise<PoolDetail> {
  return api.get<PoolDetail>(`/pools/${id}`);
}

export async function createPool(data: PoolCreateData): Promise<Pool> {
  return api.post<Pool>('/pools', {
    name: data.name,
    color: data.color,
    filter: data.filter,
  });
}

export async function updatePool(id: string, data: PoolUpdateData): Promise<Pool> {
  // Only send keys that are present so the partial-update semantics hold.
  const body: Record<string, unknown> = {};
  if (data.name !== undefined) body.name = data.name;
  if (data.color !== undefined) body.color = data.color;
  if (data.filter !== undefined) body.filter = data.filter;
  return api.put<Pool>(`/pools/${id}`, body);
}

export async function deletePool(id: string): Promise<void> {
  await api.delete(`/pools/${id}`);
}

export async function reorderPools(ids: string[]): Promise<void> {
  await api.post('/pools/reorder', { ids });
}

export async function addPoolGame(poolId: string, userGameId: string): Promise<void> {
  await api.post(`/pools/${poolId}/games`, { user_game_id: userGameId });
}

export async function bulkAddPoolGames(
  poolId: string,
  userGameIds: string[],
): Promise<{ added: number }> {
  return api.post<{ added: number }>(`/pools/${poolId}/games/bulk`, {
    user_game_ids: userGameIds,
  });
}

export async function removePoolGame(poolId: string, userGameId: string): Promise<void> {
  await api.delete(`/pools/${poolId}/games/${userGameId}`);
}

export async function setQueue(poolId: string, ids: string[]): Promise<void> {
  await api.put(`/pools/${poolId}/queue`, { ids });
}

export async function getGamePoolMemberships(userGameId: string): Promise<PoolMembership[]> {
  return api.get<PoolMembership[]>('/pools/memberships', {
    params: { user_game_id: userGameId },
  });
}
