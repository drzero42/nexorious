import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { renderHook, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { ReactNode } from 'react';
import {
  useImportMappings,
  useImportMapping,
  useLookupImportMapping,
  useCreateImportMapping,
  useUpdateImportMapping,
  useDeleteImportMapping,
  useBatchImportMappings,
  importMappingKeys,
} from './use-import-mappings';
import * as importMappingsApi from '@/api/import-mappings';
import { MappingType } from '@/types';

// Mock the API
vi.mock('@/api/import-mappings', () => ({
  getImportMappings: vi.fn(),
  getImportMapping: vi.fn(),
  lookupImportMapping: vi.fn(),
  createImportMapping: vi.fn(),
  updateImportMapping: vi.fn(),
  deleteImportMapping: vi.fn(),
  batchImportMappings: vi.fn(),
}));

describe('Import Mapping Hooks', () => {
  let queryClient: QueryClient;

  beforeEach(() => {
    queryClient = new QueryClient({
      defaultOptions: {
        queries: {
          retry: false,
        },
      },
    });
    vi.clearAllMocks();
  });

  afterEach(() => {
    queryClient.clear();
    vi.resetAllMocks();
  });

  const wrapper = ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
  );

  describe('importMappingKeys', () => {
    it('should generate correct query keys', () => {
      expect(importMappingKeys.all).toEqual(['importMappings']);
      expect(importMappingKeys.lists()).toEqual(['importMappings', 'list']);
      expect(importMappingKeys.list('darkadia', MappingType.PLATFORM)).toEqual([
        'importMappings',
        'list',
        { importSource: 'darkadia', mappingType: 'platform' },
      ]);
      expect(importMappingKeys.detail('mapping-1')).toEqual(['importMappings', 'detail', 'mapping-1']);
      expect(importMappingKeys.lookup('darkadia', MappingType.PLATFORM, 'PC')).toEqual([
        'importMappings',
        'lookup',
        { importSource: 'darkadia', mappingType: 'platform', sourceValue: 'PC' },
      ]);
    });
  });

  describe('useImportMappings', () => {
    it('should fetch import mappings', async () => {
      const mockData = {
        items: [
          {
            id: 'mapping-1',
            userId: 'user-1',
            importSource: 'darkadia',
            mappingType: MappingType.PLATFORM,
            sourceValue: 'PC',
            targetId: 'pc-windows',
            createdAt: '2024-01-01T00:00:00Z',
            updatedAt: '2024-01-01T00:00:00Z',
          },
        ],
        total: 1,
      };

      vi.mocked(importMappingsApi.getImportMappings).mockResolvedValueOnce(mockData);

      const { result } = renderHook(() => useImportMappings(), { wrapper });

      await waitFor(() => expect(result.current.isSuccess).toBe(true));

      expect(result.current.data).toEqual(mockData);
      expect(importMappingsApi.getImportMappings).toHaveBeenCalledWith(undefined, undefined);
    });

    it('should apply filters when provided', async () => {
      const mockData = { items: [], total: 0 };
      vi.mocked(importMappingsApi.getImportMappings).mockResolvedValueOnce(mockData);

      const { result } = renderHook(
        () => useImportMappings('darkadia', MappingType.PLATFORM),
        { wrapper }
      );

      await waitFor(() => expect(result.current.isSuccess).toBe(true));

      expect(importMappingsApi.getImportMappings).toHaveBeenCalledWith('darkadia', 'platform');
    });
  });

  describe('useImportMapping', () => {
    it('should fetch a single import mapping', async () => {
      const mockMapping = {
        id: 'mapping-1',
        userId: 'user-1',
        importSource: 'darkadia',
        mappingType: MappingType.PLATFORM,
        sourceValue: 'PC',
        targetId: 'pc-windows',
        createdAt: '2024-01-01T00:00:00Z',
        updatedAt: '2024-01-01T00:00:00Z',
      };

      vi.mocked(importMappingsApi.getImportMapping).mockResolvedValueOnce(mockMapping);

      const { result } = renderHook(() => useImportMapping('mapping-1'), { wrapper });

      await waitFor(() => expect(result.current.isSuccess).toBe(true));

      expect(result.current.data).toEqual(mockMapping);
      expect(importMappingsApi.getImportMapping).toHaveBeenCalledWith('mapping-1');
    });

    it('should not fetch when mappingId is empty', async () => {
      const { result } = renderHook(() => useImportMapping(''), { wrapper });

      expect(result.current.isLoading).toBe(false);
      expect(result.current.fetchStatus).toBe('idle');
      expect(importMappingsApi.getImportMapping).not.toHaveBeenCalled();
    });
  });

  describe('useLookupImportMapping', () => {
    it('should look up an import mapping by source value', async () => {
      const mockMapping = {
        id: 'mapping-1',
        userId: 'user-1',
        importSource: 'darkadia',
        mappingType: MappingType.PLATFORM,
        sourceValue: 'PC',
        targetId: 'pc-windows',
        createdAt: '2024-01-01T00:00:00Z',
        updatedAt: '2024-01-01T00:00:00Z',
      };

      vi.mocked(importMappingsApi.lookupImportMapping).mockResolvedValueOnce(mockMapping);

      const { result } = renderHook(
        () => useLookupImportMapping('darkadia', MappingType.PLATFORM, 'PC'),
        { wrapper }
      );

      await waitFor(() => expect(result.current.isSuccess).toBe(true));

      expect(result.current.data).toEqual(mockMapping);
      expect(importMappingsApi.lookupImportMapping).toHaveBeenCalledWith('darkadia', 'platform', 'PC');
    });

    it('should not fetch when sourceValue is empty', async () => {
      const { result } = renderHook(
        () => useLookupImportMapping('darkadia', MappingType.PLATFORM, ''),
        { wrapper }
      );

      expect(result.current.isLoading).toBe(false);
      expect(result.current.fetchStatus).toBe('idle');
      expect(importMappingsApi.lookupImportMapping).not.toHaveBeenCalled();
    });
  });

  describe('useCreateImportMapping', () => {
    it('should create an import mapping', async () => {
      const mockResponse = {
        id: 'mapping-1',
        userId: 'user-1',
        importSource: 'darkadia',
        mappingType: MappingType.PLATFORM,
        sourceValue: 'PC',
        targetId: 'pc-windows',
        createdAt: '2024-01-01T00:00:00Z',
        updatedAt: '2024-01-01T00:00:00Z',
      };

      vi.mocked(importMappingsApi.createImportMapping).mockResolvedValueOnce(mockResponse);

      const { result } = renderHook(() => useCreateImportMapping(), { wrapper });

      await result.current.mutateAsync({
        importSource: 'darkadia',
        mappingType: MappingType.PLATFORM,
        sourceValue: 'PC',
        targetId: 'pc-windows',
      });

      expect(importMappingsApi.createImportMapping).toHaveBeenCalledWith({
        importSource: 'darkadia',
        mappingType: MappingType.PLATFORM,
        sourceValue: 'PC',
        targetId: 'pc-windows',
      });
    });
  });

  describe('useUpdateImportMapping', () => {
    it('should update an import mapping', async () => {
      const mockResponse = {
        id: 'mapping-1',
        userId: 'user-1',
        importSource: 'darkadia',
        mappingType: MappingType.PLATFORM,
        sourceValue: 'PC',
        targetId: 'pc-linux',
        createdAt: '2024-01-01T00:00:00Z',
        updatedAt: '2024-01-02T00:00:00Z',
      };

      vi.mocked(importMappingsApi.updateImportMapping).mockResolvedValueOnce(mockResponse);

      const { result } = renderHook(() => useUpdateImportMapping(), { wrapper });

      await result.current.mutateAsync({
        mappingId: 'mapping-1',
        targetId: 'pc-linux',
      });

      expect(importMappingsApi.updateImportMapping).toHaveBeenCalledWith('mapping-1', 'pc-linux');
    });
  });

  describe('useDeleteImportMapping', () => {
    it('should delete an import mapping', async () => {
      vi.mocked(importMappingsApi.deleteImportMapping).mockResolvedValueOnce(undefined);

      const { result } = renderHook(() => useDeleteImportMapping(), { wrapper });

      await result.current.mutateAsync('mapping-1');

      expect(importMappingsApi.deleteImportMapping).toHaveBeenCalledWith('mapping-1');
    });
  });

  describe('useBatchImportMappings', () => {
    it('should batch create/update import mappings', async () => {
      const mockResponse = { created: 2, updated: 1 };
      vi.mocked(importMappingsApi.batchImportMappings).mockResolvedValueOnce(mockResponse);

      const { result } = renderHook(() => useBatchImportMappings(), { wrapper });

      const response = await result.current.mutateAsync({
        importSource: 'darkadia',
        mappings: [
          { mappingType: MappingType.PLATFORM, sourceValue: 'PC', targetId: 'pc-windows' },
          { mappingType: MappingType.PLATFORM, sourceValue: 'PS4', targetId: 'playstation-4' },
          { mappingType: MappingType.STOREFRONT, sourceValue: 'Steam', targetId: 'steam' },
        ],
      });

      expect(importMappingsApi.batchImportMappings).toHaveBeenCalledWith('darkadia', [
        { mappingType: MappingType.PLATFORM, sourceValue: 'PC', targetId: 'pc-windows' },
        { mappingType: MappingType.PLATFORM, sourceValue: 'PS4', targetId: 'playstation-4' },
        { mappingType: MappingType.STOREFRONT, sourceValue: 'Steam', targetId: 'steam' },
      ]);
      expect(response.created).toBe(2);
      expect(response.updated).toBe(1);
    });
  });
});
