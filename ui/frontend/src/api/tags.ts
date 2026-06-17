import { api } from './client';
import type { Tag, UserGame } from '@/types';

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
}

export interface TagUpdateData {
  name?: string;
  color?: string;
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

  const response = await api.get<Tag[]>('/tags', {
    params: queryParams,
  });

  return {
    tags: response,
    total: response.length,
    page: 1,
    perPage: response.length,
    totalPages: 1,
  };
}

/**
 * Get all tags (paginate through all pages).
 */
export async function getAllTags(): Promise<Tag[]> {
  const response = await api.get<Tag[]>('/tags');
  return response;
}

/**
 * Get a single tag by ID.
 */
export async function getTag(id: string): Promise<Tag> {
  const response = await api.get<Tag>(`/tags/${id}`);
  return response;
}

/**
 * Create a new tag.
 */
export async function createTag(data: TagCreateData): Promise<Tag> {
  const response = await api.post<Tag>('/tags', {
    name: data.name,
    color: data.color,
  });
  return response;
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

  const response = await api.put<Tag>(`/tags/${id}`, requestBody);
  return response;
}

/**
 * Delete a tag.
 */
export async function deleteTag(id: string): Promise<void> {
  await api.delete(`/tags/${id}`);
}

/**
 * Replace the complete tag set on a user game with the given tag names.
 * The backend resolves or creates each name within the user's tags and
 * reconciles the join table, returning the updated user game.
 */
export async function replaceUserGameTags(userGameId: string, tags: string[]): Promise<UserGame> {
  return api.put<UserGame>(`/user-games/${userGameId}/tags`, { tags });
}
