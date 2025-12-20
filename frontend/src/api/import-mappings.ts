import { api } from './client';
import type {
  ImportMapping,
  ImportMappingListResponse,
  CreateImportMappingRequest,
  BatchMappingItem,
  BatchImportMappingResponse,
  MappingType,
} from '@/types';

// ============================================================================
// API Response Types (snake_case from backend)
// ============================================================================

interface ImportMappingApiResponse {
  id: string;
  user_id: string;
  import_source: string;
  mapping_type: string;
  source_value: string;
  target_id: string;
  created_at: string;
  updated_at: string;
}

interface ImportMappingListApiResponse {
  items: ImportMappingApiResponse[];
  total: number;
}

interface BatchImportMappingApiResponse {
  created: number;
  updated: number;
}

// ============================================================================
// Transformation Functions
// ============================================================================

function transformImportMapping(apiMapping: ImportMappingApiResponse): ImportMapping {
  return {
    id: apiMapping.id,
    userId: apiMapping.user_id,
    importSource: apiMapping.import_source,
    mappingType: apiMapping.mapping_type as MappingType,
    sourceValue: apiMapping.source_value,
    targetId: apiMapping.target_id,
    createdAt: apiMapping.created_at,
    updatedAt: apiMapping.updated_at,
  };
}

// ============================================================================
// API Functions
// ============================================================================

/**
 * Get all import mappings for the current user.
 * Optionally filter by import source and/or mapping type.
 */
export async function getImportMappings(
  importSource?: string,
  mappingType?: MappingType
): Promise<ImportMappingListResponse> {
  const params: Record<string, string> = {};
  if (importSource) params.import_source = importSource;
  if (mappingType) params.mapping_type = mappingType;

  const response = await api.get<ImportMappingListApiResponse>('/import-mappings/', { params });

  return {
    items: response.items.map(transformImportMapping),
    total: response.total,
  };
}

/**
 * Get a specific import mapping by ID.
 */
export async function getImportMapping(mappingId: string): Promise<ImportMapping> {
  const response = await api.get<ImportMappingApiResponse>(`/import-mappings/${mappingId}`);
  return transformImportMapping(response);
}

/**
 * Look up a specific import mapping by source value.
 */
export async function lookupImportMapping(
  importSource: string,
  mappingType: MappingType,
  sourceValue: string
): Promise<ImportMapping> {
  const response = await api.get<ImportMappingApiResponse>('/import-mappings/lookup', {
    params: {
      import_source: importSource,
      mapping_type: mappingType,
      source_value: sourceValue,
    },
  });
  return transformImportMapping(response);
}

/**
 * Create a new import mapping.
 */
export async function createImportMapping(
  request: CreateImportMappingRequest
): Promise<ImportMapping> {
  const response = await api.post<ImportMappingApiResponse>('/import-mappings/', {
    import_source: request.importSource,
    mapping_type: request.mappingType,
    source_value: request.sourceValue,
    target_id: request.targetId,
  });
  return transformImportMapping(response);
}

/**
 * Update an existing import mapping.
 * Only the target_id can be updated.
 */
export async function updateImportMapping(
  mappingId: string,
  targetId: string
): Promise<ImportMapping> {
  const response = await api.put<ImportMappingApiResponse>(`/import-mappings/${mappingId}`, {
    target_id: targetId,
  });
  return transformImportMapping(response);
}

/**
 * Delete an import mapping.
 */
export async function deleteImportMapping(mappingId: string): Promise<void> {
  await api.delete(`/import-mappings/${mappingId}`);
}

/**
 * Create or update multiple import mappings at once.
 * This is an upsert operation - existing mappings will be updated,
 * new mappings will be created.
 */
export async function batchImportMappings(
  importSource: string,
  mappings: BatchMappingItem[]
): Promise<BatchImportMappingResponse> {
  const response = await api.post<BatchImportMappingApiResponse>('/import-mappings/batch', {
    import_source: importSource,
    mappings: mappings.map((m) => ({
      mapping_type: m.mappingType,
      source_value: m.sourceValue,
      target_id: m.targetId,
    })),
  });
  return {
    created: response.created,
    updated: response.updated,
  };
}
