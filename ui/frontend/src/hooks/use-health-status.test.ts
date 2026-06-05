import { describe, it, expect } from 'vitest';
import { renderHook, waitFor } from '@testing-library/react';
import { http, HttpResponse } from 'msw';
import { server } from '@/test/mocks/server';
import { QueryWrapper } from '@/test/test-utils';
import { useHealthStatus } from './use-health-status';

describe('useHealthStatus', () => {
  it('returns the health payload from the endpoint', async () => {
    server.use(
      http.get('/health', () =>
        HttpResponse.json({
          status: 'ok',
          igdb_status: 'invalid_credentials',
          backup_available: true,
        }),
      ),
    );

    const { result } = renderHook(() => useHealthStatus(), { wrapper: QueryWrapper });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data).toEqual({
      status: 'ok',
      igdb_status: 'invalid_credentials',
      backup_available: true,
    });
  });
});
