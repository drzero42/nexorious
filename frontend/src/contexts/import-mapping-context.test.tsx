import { describe, it, expect, vi } from 'vitest';
import { renderHook, act } from '@testing-library/react';
import { ImportMappingProvider, useImportMapping } from './import-mapping-context';

describe('ImportMappingContext', () => {
  const wrapper = ({ children }: { children: React.ReactNode }) => (
    <ImportMappingProvider>{children}</ImportMappingProvider>
  );

  it('should provide empty mappings initially', () => {
    const { result } = renderHook(() => useImportMapping(), { wrapper });

    expect(result.current.platformMappings).toEqual({});
    expect(result.current.storefrontMappings).toEqual({});
    expect(result.current.jobId).toBeNull();
  });

  it('should set job ID', () => {
    const { result } = renderHook(() => useImportMapping(), { wrapper });

    act(() => {
      result.current.setJobId('test-job-123');
    });

    expect(result.current.jobId).toBe('test-job-123');
  });

  it('should set platform mapping', () => {
    const { result } = renderHook(() => useImportMapping(), { wrapper });

    act(() => {
      result.current.setPlatformMapping('PC', 'pc-windows');
    });

    expect(result.current.platformMappings).toEqual({ PC: 'pc-windows' });
  });

  it('should set storefront mapping', () => {
    const { result } = renderHook(() => useImportMapping(), { wrapper });

    act(() => {
      result.current.setStorefrontMapping('Steam', 'steam');
    });

    expect(result.current.storefrontMappings).toEqual({ Steam: 'steam' });
  });

  it('should clear all mappings', () => {
    const { result } = renderHook(() => useImportMapping(), { wrapper });

    act(() => {
      result.current.setJobId('test-job');
      result.current.setPlatformMapping('PC', 'pc-windows');
      result.current.setStorefrontMapping('Steam', 'steam');
    });

    act(() => {
      result.current.clearMappings();
    });

    expect(result.current.jobId).toBeNull();
    expect(result.current.platformMappings).toEqual({});
    expect(result.current.storefrontMappings).toEqual({});
  });

  it('should throw error when used outside provider', () => {
    const consoleSpy = vi.spyOn(console, 'error').mockImplementation(() => {});

    expect(() => {
      renderHook(() => useImportMapping());
    }).toThrow('useImportMapping must be used within an ImportMappingProvider');

    consoleSpy.mockRestore();
  });
});
