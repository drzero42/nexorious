import { describe, it, expect } from 'vitest';
import { renderHook, waitFor } from '@testing-library/react';
import { http, HttpResponse } from 'msw';
import { server } from '@/test/mocks/server';
import { QueryWrapper } from '@/test/test-utils';
import { useHealthStatus } from './use-health-status';

describe('useHealthStatus', () => {
  it('returns igdb_configured: true from health endpoint', async () => {
    server.use(
      http.get('/health', () =>
        HttpResponse.json({ status: 'ok', igdb_configured: true, backup_available: false })
      )
    );

    const { result } = renderHook(() => useHealthStatus(), { wrapper: QueryWrapper });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data?.igdb_configured).toBe(true);
  });

  it('returns igdb_configured: false when IGDB is not configured', async () => {
    server.use(
      http.get('/health', () =>
        HttpResponse.json({ status: 'ok', igdb_configured: false, backup_available: false })
      )
    );

    const { result } = renderHook(() => useHealthStatus(), { wrapper: QueryWrapper });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data?.igdb_configured).toBe(false);
  });
});
