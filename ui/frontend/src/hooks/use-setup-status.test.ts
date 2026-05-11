import { describe, it, expect, vi, beforeEach } from 'vitest';
import { renderHook, waitFor } from '@testing-library/react';
import { useSetupStatus } from './use-setup-status';

// Mock auth API
const mockCheckSetupStatus = vi.fn();

vi.mock('@/api/auth', () => ({
  checkSetupStatus: () => mockCheckSetupStatus(),
}));

describe('useSetupStatus', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it('returns loading state initially', () => {
    mockCheckSetupStatus.mockImplementation(() => new Promise(() => {}));

    const { result } = renderHook(() => useSetupStatus());

    expect(result.current.isLoading).toBe(true);
    expect(result.current.needsSetup).toBeNull();
    expect(result.current.error).toBeNull();
  });

  it('returns needsSetup=true when setup is needed', async () => {
    mockCheckSetupStatus.mockResolvedValue({ needs_setup: true });

    const { result } = renderHook(() => useSetupStatus());

    await waitFor(() => {
      expect(result.current.isLoading).toBe(false);
    });

    expect(result.current.needsSetup).toBe(true);
    expect(result.current.error).toBeNull();
  });

  it('returns needsSetup=false when setup is complete', async () => {
    mockCheckSetupStatus.mockResolvedValue({ needs_setup: false });

    const { result } = renderHook(() => useSetupStatus());

    await waitFor(() => {
      expect(result.current.isLoading).toBe(false);
    });

    expect(result.current.needsSetup).toBe(false);
    expect(result.current.error).toBeNull();
  });

  it('returns error when API call fails', async () => {
    mockCheckSetupStatus.mockRejectedValue(new Error('Network error'));

    const { result } = renderHook(() => useSetupStatus());

    await waitFor(() => {
      expect(result.current.isLoading).toBe(false);
    });

    expect(result.current.needsSetup).toBeNull();
    expect(result.current.error).toBe('Network error');
  });

  it('returns generic error for non-Error rejections', async () => {
    mockCheckSetupStatus.mockRejectedValue('string error');

    const { result } = renderHook(() => useSetupStatus());

    await waitFor(() => {
      expect(result.current.isLoading).toBe(false);
    });

    expect(result.current.needsSetup).toBeNull();
    expect(result.current.error).toBe('Failed to check setup status');
  });
});
