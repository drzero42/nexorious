import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import * as tagsApi from '@/api/tags';
import type { GetTagsParams, TagsListResponse, TagCreateData, TagUpdateData } from '@/api/tags';
import type { Tag } from '@/types';
import { gameKeys } from './use-games';

// Query Keys
export const tagKeys = {
  all: ['tags'] as const,
  lists: () => [...tagKeys.all, 'list'] as const,
  list: (params?: GetTagsParams) => [...tagKeys.lists(), params] as const,
  details: () => [...tagKeys.all, 'detail'] as const,
  detail: (id: string) => [...tagKeys.details(), id] as const,
};

// Query Hooks
export function useTags(params?: GetTagsParams) {
  return useQuery<TagsListResponse, Error>({
    queryKey: tagKeys.list(params),
    queryFn: () => tagsApi.getTags(params),
  });
}

export function useAllTags() {
  return useQuery<Tag[], Error>({
    queryKey: tagKeys.list({ page: 1, perPage: 100, includeGameCount: true }),
    queryFn: () => tagsApi.getAllTags(),
  });
}

export function useTag(id: string | undefined) {
  return useQuery<Tag, Error>({
    queryKey: tagKeys.detail(id ?? ''),
    queryFn: () => tagsApi.getTag(id!),
    enabled: !!id,
  });
}

// Mutation Hooks
export function useCreateTag() {
  const queryClient = useQueryClient();

  return useMutation<Tag, Error, TagCreateData>({
    mutationFn: (data) => tagsApi.createTag(data),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: tagKeys.lists() });
    },
  });
}

export function useCreateOrGetTag() {
  const queryClient = useQueryClient();

  return useMutation<{ tag: Tag; created: boolean }, Error, { name: string; color?: string }>({
    mutationFn: ({ name, color }) => tagsApi.createOrGetTag(name, color),
    onSuccess: (result) => {
      if (result.created) {
        queryClient.invalidateQueries({ queryKey: tagKeys.lists() });
      }
    },
  });
}

export function useUpdateTag() {
  const queryClient = useQueryClient();

  return useMutation<Tag, Error, { id: string; data: TagUpdateData }>({
    mutationFn: ({ id, data }) => tagsApi.updateTag(id, data),
    onSuccess: (_result, { id }) => {
      queryClient.invalidateQueries({ queryKey: tagKeys.lists() });
      queryClient.invalidateQueries({ queryKey: tagKeys.detail(id) });
    },
  });
}

export function useDeleteTag() {
  const queryClient = useQueryClient();

  return useMutation<void, Error, string>({
    mutationFn: (id) => tagsApi.deleteTag(id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: tagKeys.lists() });
    },
  });
}

export function useAssignTagsToGame() {
  const queryClient = useQueryClient();

  return useMutation<
    { message: string; newAssociations: number; totalRequested: number },
    Error,
    { userGameId: string; tagIds: string[] }
  >({
    mutationFn: ({ userGameId, tagIds }) => tagsApi.assignTagsToGame(userGameId, tagIds),
    onSuccess: (_result, { userGameId }) => {
      queryClient.invalidateQueries({ queryKey: gameKeys.detail(userGameId) });
      queryClient.invalidateQueries({ queryKey: tagKeys.lists() });
    },
  });
}

export function useRemoveTagsFromGame() {
  const queryClient = useQueryClient();

  return useMutation<
    { message: string; removedAssociations: number; totalRequested: number },
    Error,
    { userGameId: string; tagIds: string[] }
  >({
    mutationFn: ({ userGameId, tagIds }) => tagsApi.removeTagsFromGame(userGameId, tagIds),
    onSuccess: (_result, { userGameId }) => {
      queryClient.invalidateQueries({ queryKey: gameKeys.detail(userGameId) });
      queryClient.invalidateQueries({ queryKey: tagKeys.lists() });
    },
  });
}
