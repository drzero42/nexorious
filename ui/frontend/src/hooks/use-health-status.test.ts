import { describe, it, expect } from 'vitest';
import { renderHook, waitFor } from '@testing-library/react';
import { http, HttpResponse } from 'msw';
import { server } from '@/test/mocks/server';
import { QueryWrapper } from '@/test/test-utils';
import { useHealthStatus } from './use-health-status';

describe('useHealthStatus', () => {
  it('returns igdb_status: ok from health endpoint', async () => {
    server.use(
      http.get('/health', () =>
        HttpResponse.json({ status: 'ok', igdb_status: 'ok', backup_available: false }),
      ),
    );

    const { result } = renderHook(() => useHealthStatus(), { wrapper: QueryWrapper });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data?.igdb_status).toBe('ok');
  });

  it('returns igdb_status: not_configured when IGDB credentials are absent', async () => {
    server.use(
      http.get('/health', () =>
        HttpResponse.json({ status: 'ok', igdb_status: 'not_configured', backup_available: false }),
      ),
    );

    const { result } = renderHook(() => useHealthStatus(), { wrapper: QueryWrapper });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data?.igdb_status).toBe('not_configured');
  });

  it('returns igdb_status: invalid_credentials when credentials fail auth', async () => {
    server.use(
      http.get('/health', () =>
        HttpResponse.json({
          status: 'ok',
          igdb_status: 'invalid_credentials',
          backup_available: false,
        }),
      ),
    );

    const { result } = renderHook(() => useHealthStatus(), { wrapper: QueryWrapper });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data?.igdb_status).toBe('invalid_credentials');
  });
});
