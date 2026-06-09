import { describe, it, expect } from 'vitest';
import { http, HttpResponse } from 'msw';
import { server } from '@/test/mocks/server';
import { docsApi } from './docs';

const API_URL = '/api';

describe('docsApi', () => {
  it('requests /api/docs/<slug> (no doubled /api prefix) and returns the raw body', async () => {
    server.use(
      http.get(`${API_URL}/docs/user-guide`, () => {
        return new HttpResponse('# User Guide\n\nhello', {
          headers: { 'Content-Type': 'text/markdown' },
        });
      }),
    );

    const md = await docsApi.get('user-guide');
    expect(md).toBe('# User Guide\n\nhello');
  });

  it('encodes the slug into the path', async () => {
    let requestedPath = '';
    server.use(
      http.get(`${API_URL}/docs/:slug`, ({ request }) => {
        requestedPath = new URL(request.url).pathname;
        return new HttpResponse('ok', { headers: { 'Content-Type': 'text/markdown' } });
      }),
    );

    await docsApi.get('admin-guide');
    expect(requestedPath).toBe('/api/docs/admin-guide');
  });
});
