import { api } from './client';
import type { Tag } from '@/types';

// ============================================================================
// API Response Types (snake_case from backend)
// ============================================================================

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

interface TagListApiResponse {
  tags: TagApiResponse[];
  total: number;
  page: number;
  per_page: number;
  total_pages: number;
}

interface TagCreateOrGetApiResponse {
  tag: TagApiResponse;
  created: boolean;
}

interface TagAssignApiResponse {
  message: string;
  new_associations: number;
  total_requested: number;
}

interface TagRemoveApiResponse {
  message: string;
  removed_associations: number;
  total_requested: number;
}

// ============================================================================
// Request Parameter Types
// ============================================================================

export interface GetTagsParams {
  page?: number;
  perPage?: number;
  includeGameCount?: boolean;
}

export interface TagCreateData {
  name: string;
  color?: string;
  description?: string;
}

export interface TagUpdateData {
  name?: string;
  color?: string;
  description?: string;
}

// ============================================================================
// Response Types
// ============================================================================

export interface TagsListResponse {
  tags: Tag[];
  total: number;
  page: number;
  perPage: number;
  totalPages: number;
}

export interface TagCreateOrGetResponse {
  tag: Tag;
  created: boolean;
}

export interface TagAssignResponse {
  message: string;
  newAssociations: number;
  totalRequested: number;
}

export interface TagRemoveResponse {
  message: string;
  removedAssociations: number;
  totalRequested: number;
}

// ============================================================================
// Transformation Functions
// ============================================================================

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

// ============================================================================
// API Functions
// ============================================================================

/**
 * Get user's tags with optional pagination.
 */
export async function getTags(params?: GetTagsParams): Promise<TagsListResponse> {
  const queryParams: Record<string, string | number | boolean | undefined> = {};

  if (params?.page !== undefined) {
    queryParams.page = params.page;
  }
  if (params?.perPage !== undefined) {
    queryParams.per_page = params.perPage;
  }
  if (params?.includeGameCount !== undefined) {
    queryParams.include_game_count = params.includeGameCount;
  }

  const response = await api.get<TagListApiResponse>('/tags/', {
    params: queryParams,
  });

  return {
    tags: response.tags.map(transformTag),
    total: response.total,
    page: response.page,
    perPage: response.per_page,
    totalPages: response.total_pages,
  };
}

/**
 * Get all tags (paginate through all pages).
 */
export async function getAllTags(): Promise<Tag[]> {
  const allTags: Tag[] = [];
  let page = 1;
  let hasMore = true;

  while (hasMore) {
    const response = await getTags({ page, perPage: 100, includeGameCount: true });
    allTags.push(...response.tags);
    hasMore = page < response.totalPages;
    page++;
  }

  return allTags;
}

/**
 * Get a single tag by ID.
 */
export async function getTag(id: string): Promise<Tag> {
  const response = await api.get<TagApiResponse>(`/tags/${id}`);
  return transformTag(response);
}

/**
 * Create a new tag.
 */
export async function createTag(data: TagCreateData): Promise<Tag> {
  const response = await api.post<TagApiResponse>('/tags/', {
    name: data.name,
    color: data.color,
    description: data.description,
  });
  return transformTag(response);
}

/**
 * Create a tag or get existing one by name (for inline tag creation).
 */
export async function createOrGetTag(
  name: string,
  color?: string
): Promise<TagCreateOrGetResponse> {
  const queryParams: Record<string, string | undefined> = { name };
  if (color) {
    queryParams.color = color;
  }

  const response = await api.post<TagCreateOrGetApiResponse>(
    '/tags/create-or-get',
    undefined,
    { params: queryParams }
  );

  return {
    tag: transformTag(response.tag),
    created: response.created,
  };
}

/**
 * Update an existing tag.
 */
export async function updateTag(id: string, data: TagUpdateData): Promise<Tag> {
  const requestBody: Record<string, unknown> = {};

  if (data.name !== undefined) {
    requestBody.name = data.name;
  }
  if (data.color !== undefined) {
    requestBody.color = data.color;
  }
  if (data.description !== undefined) {
    requestBody.description = data.description;
  }

  const response = await api.put<TagApiResponse>(`/tags/${id}`, requestBody);
  return transformTag(response);
}

/**
 * Delete a tag.
 */
export async function deleteTag(id: string): Promise<void> {
  await api.delete(`/tags/${id}`);
}

/**
 * Assign tags to a user game.
 */
export async function assignTagsToGame(
  userGameId: string,
  tagIds: string[]
): Promise<TagAssignResponse> {
  const response = await api.post<TagAssignApiResponse>(
    `/tags/assign/${userGameId}`,
    { tag_ids: tagIds }
  );

  return {
    message: response.message,
    newAssociations: response.new_associations,
    totalRequested: response.total_requested,
  };
}

/**
 * Remove tags from a user game.
 */
export async function removeTagsFromGame(
  userGameId: string,
  tagIds: string[]
): Promise<TagRemoveResponse> {
  const response = await api.delete<TagRemoveApiResponse>(
    `/tags/remove/${userGameId}`,
    { body: JSON.stringify({ tag_ids: tagIds }) }
  );

  return {
    message: response.message,
    removedAssociations: response.removed_associations,
    totalRequested: response.total_requested,
  };
}

/**
 * Bulk assign tags to multiple games.
 */
export async function bulkAssignTags(
  userGameIds: string[],
  tagIds: string[]
): Promise<{ message: string; totalNewAssociations: number; gamesProcessed: number }> {
  const response = await api.post<{
    message: string;
    total_new_associations: number;
    games_processed: number;
  }>('/tags/bulk-assign', {
    user_game_ids: userGameIds,
    tag_ids: tagIds,
  });

  return {
    message: response.message,
    totalNewAssociations: response.total_new_associations,
    gamesProcessed: response.games_processed,
  };
}

/**
 * Bulk remove tags from multiple games.
 */
export async function bulkRemoveTags(
  userGameIds: string[],
  tagIds: string[]
): Promise<{ message: string; totalRemovedAssociations: number; gamesProcessed: number }> {
  const response = await api.delete<{
    message: string;
    total_removed_associations: number;
    games_processed: number;
  }>('/tags/bulk-remove', {
    body: JSON.stringify({
      user_game_ids: userGameIds,
      tag_ids: tagIds,
    }),
  });

  return {
    message: response.message,
    totalRemovedAssociations: response.total_removed_associations,
    gamesProcessed: response.games_processed,
  };
}
