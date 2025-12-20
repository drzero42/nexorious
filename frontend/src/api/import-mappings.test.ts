import { vi, describe, it, expect, beforeEach, afterEach } from 'vitest';
import {
  getImportMappings,
  getImportMapping,
  lookupImportMapping,
  createImportMapping,
  updateImportMapping,
  deleteImportMapping,
  batchImportMappings,
} from './import-mappings';
import { api } from './client';
import { MappingType } from '@/types';

// Mock the api client
vi.mock('./client', () => ({
  api: {
    get: vi.fn(),
    post: vi.fn(),
    put: vi.fn(),
    delete: vi.fn(),
  },
}));

describe('Import Mappings API', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  afterEach(() => {
    vi.resetAllMocks();
  });

  describe('getImportMappings', () => {
    it('should fetch all import mappings without filters', async () => {
      const mockResponse = {
        items: [
          {
            id: 'mapping-1',
            user_id: 'user-1',
            import_source: 'darkadia',
            mapping_type: 'platform',
            source_value: 'PC',
            target_id: 'pc-windows',
            created_at: '2024-01-01T00:00:00Z',
            updated_at: '2024-01-01T00:00:00Z',
          },
        ],
        total: 1,
      };

      vi.mocked(api.get).mockResolvedValueOnce(mockResponse);

      const result = await getImportMappings();

      expect(api.get).toHaveBeenCalledWith('/import-mappings/', { params: {} });
      expect(result.items).toHaveLength(1);
      expect(result.items[0].id).toBe('mapping-1');
      expect(result.items[0].sourceValue).toBe('PC');
      expect(result.items[0].mappingType).toBe('platform');
    });

    it('should apply filters when provided', async () => {
      const mockResponse = {
        items: [],
        total: 0,
      };

      vi.mocked(api.get).mockResolvedValueOnce(mockResponse);

      await getImportMappings('darkadia', MappingType.PLATFORM);

      expect(api.get).toHaveBeenCalledWith('/import-mappings/', {
        params: { import_source: 'darkadia', mapping_type: 'platform' },
      });
    });
  });

  describe('getImportMapping', () => {
    it('should fetch a single import mapping by ID', async () => {
      const mockResponse = {
        id: 'mapping-1',
        user_id: 'user-1',
        import_source: 'darkadia',
        mapping_type: 'platform',
        source_value: 'PC',
        target_id: 'pc-windows',
        created_at: '2024-01-01T00:00:00Z',
        updated_at: '2024-01-01T00:00:00Z',
      };

      vi.mocked(api.get).mockResolvedValueOnce(mockResponse);

      const result = await getImportMapping('mapping-1');

      expect(api.get).toHaveBeenCalledWith('/import-mappings/mapping-1');
      expect(result.id).toBe('mapping-1');
      expect(result.targetId).toBe('pc-windows');
    });
  });

  describe('lookupImportMapping', () => {
    it('should look up a mapping by source value', async () => {
      const mockResponse = {
        id: 'mapping-1',
        user_id: 'user-1',
        import_source: 'darkadia',
        mapping_type: 'platform',
        source_value: 'PC',
        target_id: 'pc-windows',
        created_at: '2024-01-01T00:00:00Z',
        updated_at: '2024-01-01T00:00:00Z',
      };

      vi.mocked(api.get).mockResolvedValueOnce(mockResponse);

      const result = await lookupImportMapping('darkadia', MappingType.PLATFORM, 'PC');

      expect(api.get).toHaveBeenCalledWith('/import-mappings/lookup', {
        params: {
          import_source: 'darkadia',
          mapping_type: 'platform',
          source_value: 'PC',
        },
      });
      expect(result.targetId).toBe('pc-windows');
    });
  });

  describe('createImportMapping', () => {
    it('should create a new import mapping', async () => {
      const mockResponse = {
        id: 'mapping-1',
        user_id: 'user-1',
        import_source: 'darkadia',
        mapping_type: 'platform',
        source_value: 'PC',
        target_id: 'pc-windows',
        created_at: '2024-01-01T00:00:00Z',
        updated_at: '2024-01-01T00:00:00Z',
      };

      vi.mocked(api.post).mockResolvedValueOnce(mockResponse);

      const result = await createImportMapping({
        importSource: 'darkadia',
        mappingType: MappingType.PLATFORM,
        sourceValue: 'PC',
        targetId: 'pc-windows',
      });

      expect(api.post).toHaveBeenCalledWith('/import-mappings/', {
        import_source: 'darkadia',
        mapping_type: 'platform',
        source_value: 'PC',
        target_id: 'pc-windows',
      });
      expect(result.id).toBe('mapping-1');
    });
  });

  describe('updateImportMapping', () => {
    it('should update an existing import mapping', async () => {
      const mockResponse = {
        id: 'mapping-1',
        user_id: 'user-1',
        import_source: 'darkadia',
        mapping_type: 'platform',
        source_value: 'PC',
        target_id: 'pc-linux',
        created_at: '2024-01-01T00:00:00Z',
        updated_at: '2024-01-02T00:00:00Z',
      };

      vi.mocked(api.put).mockResolvedValueOnce(mockResponse);

      const result = await updateImportMapping('mapping-1', 'pc-linux');

      expect(api.put).toHaveBeenCalledWith('/import-mappings/mapping-1', {
        target_id: 'pc-linux',
      });
      expect(result.targetId).toBe('pc-linux');
    });
  });

  describe('deleteImportMapping', () => {
    it('should delete an import mapping', async () => {
      vi.mocked(api.delete).mockResolvedValueOnce(undefined);

      await deleteImportMapping('mapping-1');

      expect(api.delete).toHaveBeenCalledWith('/import-mappings/mapping-1');
    });
  });

  describe('batchImportMappings', () => {
    it('should batch create/update import mappings', async () => {
      const mockResponse = {
        created: 2,
        updated: 1,
      };

      vi.mocked(api.post).mockResolvedValueOnce(mockResponse);

      const result = await batchImportMappings('darkadia', [
        { mappingType: MappingType.PLATFORM, sourceValue: 'PC', targetId: 'pc-windows' },
        { mappingType: MappingType.PLATFORM, sourceValue: 'PS4', targetId: 'playstation-4' },
        { mappingType: MappingType.STOREFRONT, sourceValue: 'Steam', targetId: 'steam' },
      ]);

      expect(api.post).toHaveBeenCalledWith('/import-mappings/batch', {
        import_source: 'darkadia',
        mappings: [
          { mapping_type: 'platform', source_value: 'PC', target_id: 'pc-windows' },
          { mapping_type: 'platform', source_value: 'PS4', target_id: 'playstation-4' },
          { mapping_type: 'storefront', source_value: 'Steam', target_id: 'steam' },
        ],
      });
      expect(result.created).toBe(2);
      expect(result.updated).toBe(1);
    });
  });
});
