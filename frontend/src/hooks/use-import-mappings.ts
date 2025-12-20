import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import {
  getImportMappings,
  getImportMapping,
  lookupImportMapping,
  createImportMapping,
  updateImportMapping,
  deleteImportMapping,
  batchImportMappings,
} from '@/api';
import type {
  ImportMapping,
  ImportMappingListResponse,
  CreateImportMappingRequest,
  BatchMappingItem,
  BatchImportMappingResponse,
  MappingType,
} from '@/types';

// ============================================================================
// Query Keys
// ============================================================================

export const importMappingKeys = {
  all: ['importMappings'] as const,
  lists: () => [...importMappingKeys.all, 'list'] as const,
  list: (importSource?: string, mappingType?: MappingType) =>
    [...importMappingKeys.lists(), { importSource, mappingType }] as const,
  detail: (id: string) => [...importMappingKeys.all, 'detail', id] as const,
  lookup: (importSource: string, mappingType: MappingType, sourceValue: string) =>
    [...importMappingKeys.all, 'lookup', { importSource, mappingType, sourceValue }] as const,
};

// ============================================================================
// Query Hooks
// ============================================================================

/**
 * Hook to fetch import mappings with optional filters.
 */
export function useImportMappings(
  importSource?: string,
  mappingType?: MappingType
): ReturnType<typeof useQuery<ImportMappingListResponse>> {
  return useQuery({
    queryKey: importMappingKeys.list(importSource, mappingType),
    queryFn: () => getImportMappings(importSource, mappingType),
  });
}

/**
 * Hook to fetch a single import mapping by ID.
 */
export function useImportMapping(
  mappingId: string
): ReturnType<typeof useQuery<ImportMapping>> {
  return useQuery({
    queryKey: importMappingKeys.detail(mappingId),
    queryFn: () => getImportMapping(mappingId),
    enabled: !!mappingId,
  });
}

/**
 * Hook to look up an import mapping by source value.
 */
export function useLookupImportMapping(
  importSource: string,
  mappingType: MappingType,
  sourceValue: string
): ReturnType<typeof useQuery<ImportMapping>> {
  return useQuery({
    queryKey: importMappingKeys.lookup(importSource, mappingType, sourceValue),
    queryFn: () => lookupImportMapping(importSource, mappingType, sourceValue),
    enabled: !!importSource && !!mappingType && !!sourceValue,
  });
}

// ============================================================================
// Mutation Hooks
// ============================================================================

/**
 * Hook to create a new import mapping.
 */
export function useCreateImportMapping() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: (request: CreateImportMappingRequest): Promise<ImportMapping> =>
      createImportMapping(request),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: importMappingKeys.all });
    },
  });
}

/**
 * Hook to update an existing import mapping.
 */
export function useUpdateImportMapping() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: ({ mappingId, targetId }: { mappingId: string; targetId: string }): Promise<ImportMapping> =>
      updateImportMapping(mappingId, targetId),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: importMappingKeys.all });
    },
  });
}

/**
 * Hook to delete an import mapping.
 */
export function useDeleteImportMapping() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: (mappingId: string): Promise<void> => deleteImportMapping(mappingId),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: importMappingKeys.all });
    },
  });
}

/**
 * Hook to batch create/update import mappings.
 */
export function useBatchImportMappings() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: ({
      importSource,
      mappings,
    }: {
      importSource: string;
      mappings: BatchMappingItem[];
    }): Promise<BatchImportMappingResponse> => batchImportMappings(importSource, mappings),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: importMappingKeys.all });
    },
  });
}
