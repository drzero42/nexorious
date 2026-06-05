import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { http, HttpResponse } from 'msw';
import { server } from '@/test/mocks/server';
import { ApiErrorException, apiCall, api } from './client';

// In test environment, NODE_ENV is 'test' so apiUrl defaults to '/api'
// MSW intercepts relative URLs with the origin prepended
const API_URL = '/api';

describe('client.ts', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  describe('ApiErrorException', () => {
    it('creates error with message and status', () => {
      const error = new ApiErrorException('Not found', 404);

      expect(error).toBeInstanceOf(Error);
      expect(error.message).toBe('Not found');
      expect(error.status).toBe(404);
      expect(error.details).toBeUndefined();
      expect(error.name).toBe('ApiErrorException');
    });

    it('creates error with details', () => {
      const details = { field: 'email', reason: 'invalid' };
      const error = new ApiErrorException('Validation failed', 400, details);

      expect(error.message).toBe('Validation failed');
      expect(error.status).toBe(400);
      expect(error.details).toEqual(details);
    });
  });

  describe('apiCall', () => {
    describe('URL construction', () => {
      it('constructs URL with path starting with slash', async () => {
        server.use(
          http.get(`${API_URL}/test-path`, () => {
            return HttpResponse.json({ success: true });
          }),
        );

        const response = await apiCall('/test-path');
        const data = await response.json();

        expect(data.success).toBe(true);
      });

      it('constructs URL with path not starting with slash', async () => {
        server.use(
          http.get(`${API_URL}/test-path`, () => {
            return HttpResponse.json({ success: true });
          }),
        );

        const response = await apiCall('test-path');
        const data = await response.json();

        expect(data.success).toBe(true);
      });

      it('appends query params to URL', async () => {
        server.use(
          http.get(`${API_URL}/search`, ({ request }) => {
            const url = new URL(request.url);
            return HttpResponse.json({
              query: url.searchParams.get('query'),
              limit: url.searchParams.get('limit'),
            });
          }),
        );

        const response = await apiCall('/search', {
          params: { query: 'test', limit: 10 },
        });
        const data = await response.json();

        expect(data.query).toBe('test');
        expect(data.limit).toBe('10');
      });

      it('excludes undefined params', async () => {
        server.use(
          http.get(`${API_URL}/search`, ({ request }) => {
            const url = new URL(request.url);
            return HttpResponse.json({
              hasQuery: url.searchParams.has('query'),
              hasUndefined: url.searchParams.has('undefined_param'),
            });
          }),
        );

        const response = await apiCall('/search', {
          params: { query: 'test', undefined_param: undefined },
        });
        const data = await response.json();

        expect(data.hasQuery).toBe(true);
        expect(data.hasUndefined).toBe(false);
      });

      it('handles boolean params', async () => {
        server.use(
          http.get(`${API_URL}/items`, ({ request }) => {
            const url = new URL(request.url);
            return HttpResponse.json({
              active: url.searchParams.get('active'),
            });
          }),
        );

        const response = await apiCall('/items', {
          params: { active: true },
        });
        const data = await response.json();

        expect(data.active).toBe('true');
      });
    });

    describe('headers', () => {
      it('sets Content-Type to application/json by default', async () => {
        server.use(
          http.get(`${API_URL}/test`, ({ request }) => {
            return HttpResponse.json({
              contentType: request.headers.get('Content-Type'),
            });
          }),
        );

        const response = await apiCall('/test');
        const data = await response.json();

        expect(data.contentType).toBe('application/json');
      });

      it('allows custom headers', async () => {
        server.use(
          http.get(`${API_URL}/test`, ({ request }) => {
            return HttpResponse.json({
              customHeader: request.headers.get('X-Custom-Header'),
            });
          }),
        );

        const response = await apiCall('/test', {
          headers: { 'X-Custom-Header': 'custom-value' },
        });
        const data = await response.json();

        expect(data.customHeader).toBe('custom-value');
      });
    });

    describe('error handling', () => {
      it('throws ApiErrorException on non-ok response', async () => {
        server.use(
          http.get(`${API_URL}/error`, () => {
            return HttpResponse.json({ detail: 'Something went wrong' }, { status: 500 });
          }),
        );

        await expect(apiCall('/error')).rejects.toThrow(ApiErrorException);
        await expect(apiCall('/error')).rejects.toMatchObject({
          message: 'Something went wrong',
          status: 500,
        });
      });

      it('extracts error message from detail field', async () => {
        server.use(
          http.get(`${API_URL}/error`, () => {
            return HttpResponse.json({ detail: 'Detailed error message' }, { status: 400 });
          }),
        );

        await expect(apiCall('/error')).rejects.toMatchObject({
          message: 'Detailed error message',
        });
      });

      it('extracts error message from message field', async () => {
        server.use(
          http.get(`${API_URL}/error`, () => {
            return HttpResponse.json({ message: 'Error message field' }, { status: 400 });
          }),
        );

        await expect(apiCall('/error')).rejects.toMatchObject({
          message: 'Error message field',
        });
      });

      it('extracts error message from error field', async () => {
        server.use(
          http.get(`${API_URL}/error`, () => {
            return HttpResponse.json({ error: 'invalid or expired token' }, { status: 401 });
          }),
        );

        await expect(apiCall('/error')).rejects.toMatchObject({
          message: 'invalid or expired token',
          status: 401,
        });
      });

      it('uses default error message when response is not JSON', async () => {
        server.use(
          http.get(`${API_URL}/error`, () => {
            return new HttpResponse('Server Error', {
              status: 500,
              statusText: 'Internal Server Error',
            });
          }),
        );

        await expect(apiCall('/error')).rejects.toMatchObject({
          message: 'HTTP 500: Internal Server Error',
          status: 500,
        });
      });

      it('preserves error details in exception', async () => {
        const errorDetails = {
          detail: 'Validation failed',
          errors: [
            { field: 'email', message: 'Invalid email' },
            { field: 'password', message: 'Too short' },
          ],
        };

        server.use(
          http.get(`${API_URL}/error`, () => {
            return HttpResponse.json(errorDetails, { status: 422 });
          }),
        );

        try {
          await apiCall('/error');
          expect.fail('Should have thrown');
        } catch (error) {
          expect(error).toBeInstanceOf(ApiErrorException);
          const apiError = error as ApiErrorException;
          expect(apiError.details).toEqual(errorDetails);
        }
      });
    });

    describe('app-state redirects (issue #771)', () => {
      let assignSpy: ReturnType<typeof vi.fn>;
      let originalLocation: Location;

      beforeEach(() => {
        originalLocation = window.location;
        assignSpy = vi.fn();
        Object.defineProperty(window, 'location', {
          configurable: true,
          value: {
            href: originalLocation.href,
            origin: originalLocation.origin,
            pathname: '/',
            assign: assignSpy,
            replace: vi.fn(),
          },
        });
      });

      afterEach(() => {
        Object.defineProperty(window, 'location', {
          configurable: true,
          value: originalLocation,
        });
      });

      it('redirects to /migrate on 503 app_state=needs_migration', async () => {
        server.use(
          http.get(`${API_URL}/games`, () => {
            return HttpResponse.json({ app_state: 'needs_migration' }, { status: 503 });
          }),
        );

        await expect(apiCall('/games')).rejects.toThrow(ApiErrorException);
        expect(assignSpy).toHaveBeenCalledWith('/migrate');
      });

      it('redirects to /migrate on 503 app_state=migrating', async () => {
        server.use(
          http.get(`${API_URL}/games`, () => {
            return HttpResponse.json({ app_state: 'migrating' }, { status: 503 });
          }),
        );

        await expect(apiCall('/games')).rejects.toThrow(ApiErrorException);
        expect(assignSpy).toHaveBeenCalledWith('/migrate');
      });

      it('redirects to /db-error on 503 app_state=db_unavailable', async () => {
        server.use(
          http.get(`${API_URL}/games`, () => {
            return HttpResponse.json({ app_state: 'db_unavailable' }, { status: 503 });
          }),
        );

        await expect(apiCall('/games')).rejects.toThrow(ApiErrorException);
        expect(assignSpy).toHaveBeenCalledWith('/db-error');
      });

      it('redirects to /setup on 503 app_state=needs_setup', async () => {
        server.use(
          http.get(`${API_URL}/games`, () => {
            return HttpResponse.json({ app_state: 'needs_setup' }, { status: 503 });
          }),
        );

        await expect(apiCall('/games')).rejects.toThrow(ApiErrorException);
        expect(assignSpy).toHaveBeenCalledWith('/setup');
      });

      it('does not redirect when already on the target page', async () => {
        Object.defineProperty(window, 'location', {
          configurable: true,
          value: {
            href: originalLocation.href,
            origin: originalLocation.origin,
            pathname: '/migrate',
            assign: assignSpy,
            replace: vi.fn(),
          },
        });
        server.use(
          http.get(`${API_URL}/games`, () => {
            return HttpResponse.json({ app_state: 'needs_migration' }, { status: 503 });
          }),
        );

        await expect(apiCall('/games')).rejects.toThrow(ApiErrorException);
        expect(assignSpy).not.toHaveBeenCalled();
      });

      it('does not redirect on a plain 503 without app_state', async () => {
        server.use(
          http.get(`${API_URL}/games`, () => {
            return HttpResponse.json({ detail: 'temporarily unavailable' }, { status: 503 });
          }),
        );

        await expect(apiCall('/games')).rejects.toThrow(ApiErrorException);
        expect(assignSpy).not.toHaveBeenCalled();
      });
    });

    describe('HTTP methods', () => {
      it('supports GET method', async () => {
        server.use(
          http.get(`${API_URL}/resource`, () => {
            return HttpResponse.json({ method: 'GET' });
          }),
        );

        const response = await apiCall('/resource', { method: 'GET' });
        const data = await response.json();

        expect(data.method).toBe('GET');
      });

      it('supports POST method with body', async () => {
        server.use(
          http.post(`${API_URL}/resource`, async ({ request }) => {
            const body = await request.json();
            return HttpResponse.json({ method: 'POST', body });
          }),
        );

        const response = await apiCall('/resource', {
          method: 'POST',
          body: JSON.stringify({ name: 'test' }),
        });
        const data = await response.json();

        expect(data.method).toBe('POST');
        expect(data.body).toEqual({ name: 'test' });
      });

      it('supports PUT method', async () => {
        server.use(
          http.put(`${API_URL}/resource/1`, async ({ request }) => {
            const body = await request.json();
            return HttpResponse.json({ method: 'PUT', body });
          }),
        );

        const response = await apiCall('/resource/1', {
          method: 'PUT',
          body: JSON.stringify({ name: 'updated' }),
        });
        const data = await response.json();

        expect(data.method).toBe('PUT');
        expect(data.body).toEqual({ name: 'updated' });
      });

      it('supports DELETE method', async () => {
        server.use(
          http.delete(`${API_URL}/resource/1`, () => {
            return HttpResponse.json({ method: 'DELETE' });
          }),
        );

        const response = await apiCall('/resource/1', { method: 'DELETE' });
        const data = await response.json();

        expect(data.method).toBe('DELETE');
      });
    });
  });

  describe('api helper object', () => {
    describe('api.get', () => {
      it('makes GET request and returns parsed JSON', async () => {
        server.use(
          http.get(`${API_URL}/items`, () => {
            return HttpResponse.json({ items: [1, 2, 3] });
          }),
        );

        const data = await api.get<{ items: number[] }>('/items');

        expect(data.items).toEqual([1, 2, 3]);
      });

      it('passes options to apiCall', async () => {
        server.use(
          http.get(`${API_URL}/items`, ({ request }) => {
            const url = new URL(request.url);
            return HttpResponse.json({
              page: url.searchParams.get('page'),
            });
          }),
        );

        const data = await api.get<{ page: string }>('/items', {
          params: { page: 2 },
        });

        expect(data.page).toBe('2');
      });
    });

    describe('api.post', () => {
      it('makes POST request with body and returns parsed JSON', async () => {
        server.use(
          http.post(`${API_URL}/items`, async ({ request }) => {
            const body = (await request.json()) as { name: string };
            return HttpResponse.json({ id: 1, name: body.name });
          }),
        );

        const data = await api.post<{ id: number; name: string }>('/items', {
          name: 'New Item',
        });

        expect(data).toEqual({ id: 1, name: 'New Item' });
      });

      it('makes POST request without body', async () => {
        server.use(
          http.post(`${API_URL}/action`, () => {
            return HttpResponse.json({ success: true });
          }),
        );

        const data = await api.post<{ success: boolean }>('/action');

        expect(data.success).toBe(true);
      });
    });

    describe('api.put', () => {
      it('makes PUT request with body and returns parsed JSON', async () => {
        server.use(
          http.put(`${API_URL}/items/1`, async ({ request }) => {
            const body = (await request.json()) as { name: string };
            return HttpResponse.json({ id: 1, name: body.name });
          }),
        );

        const data = await api.put<{ id: number; name: string }>('/items/1', {
          name: 'Updated Item',
        });

        expect(data).toEqual({ id: 1, name: 'Updated Item' });
      });

      it('makes PUT request without body', async () => {
        server.use(
          http.put(`${API_URL}/items/1/activate`, () => {
            return HttpResponse.json({ activated: true });
          }),
        );

        const data = await api.put<{ activated: boolean }>('/items/1/activate');

        expect(data.activated).toBe(true);
      });
    });

    describe('api.patch', () => {
      it('makes PATCH request with partial body and returns parsed JSON', async () => {
        server.use(
          http.patch(`${API_URL}/items/1`, async ({ request }) => {
            const body = (await request.json()) as { status: string };
            return HttpResponse.json({ id: 1, name: 'Original', status: body.status });
          }),
        );

        const data = await api.patch<{ id: number; name: string; status: string }>('/items/1', {
          status: 'active',
        });

        expect(data).toEqual({ id: 1, name: 'Original', status: 'active' });
      });

      it('makes PATCH request without body', async () => {
        server.use(
          http.patch(`${API_URL}/items/1/touch`, () => {
            return HttpResponse.json({ touched: true });
          }),
        );

        const data = await api.patch<{ touched: boolean }>('/items/1/touch');

        expect(data.touched).toBe(true);
      });
    });

    describe('api.delete', () => {
      it('makes DELETE request and returns parsed JSON', async () => {
        server.use(
          http.delete(`${API_URL}/items/1`, () => {
            return HttpResponse.json({ deleted: true });
          }),
        );

        const data = await api.delete<{ deleted: boolean }>('/items/1');

        expect(data.deleted).toBe(true);
      });

      it('handles 204 No Content response', async () => {
        server.use(
          http.delete(`${API_URL}/items/1`, () => {
            return new HttpResponse(null, { status: 204 });
          }),
        );

        const data = await api.delete('/items/1');

        expect(data).toBeUndefined();
      });

      it('passes options to apiCall', async () => {
        server.use(
          http.delete(`${API_URL}/items`, ({ request }) => {
            const url = new URL(request.url);
            return HttpResponse.json({
              ids: url.searchParams.get('ids'),
            });
          }),
        );

        const data = await api.delete<{ ids: string }>('/items', {
          params: { ids: '1,2,3' },
        });

        expect(data.ids).toBe('1,2,3');
      });
    });
  });
});
