import { describe, it, expect, vi, beforeEach, afterEach, type Mock } from 'vitest';
import { http, HttpResponse } from 'msw';
import { server } from '@/test/mocks/server';
import { setAuthHandlers } from './client';
import {
  getUsers,
  getUserById,
  createUser,
  updateUser,
  resetUserPassword,
  getUserDeletionImpact,
  deleteUser,
} from './admin';

const API_URL = '/api';

describe('admin.ts', () => {
  let mockGetAccessToken: Mock<() => string | null>;
  let mockRefreshTokens: Mock<() => Promise<boolean>>;
  let mockLogout: Mock<() => void>;

  beforeEach(() => {
    vi.clearAllMocks();

    mockGetAccessToken = vi.fn<() => string | null>().mockReturnValue('test-access-token');
    mockRefreshTokens = vi.fn<() => Promise<boolean>>().mockResolvedValue(false);
    mockLogout = vi.fn<() => void>();

    setAuthHandlers(mockGetAccessToken, mockRefreshTokens, mockLogout);
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  describe('getUsers', () => {
    it('returns transformed user list', async () => {
      server.use(
        http.get(`${API_URL}/auth/admin/users`, () => {
          return HttpResponse.json([
            {
              id: 'user-1',
              username: 'admin',
              is_admin: true,
              is_active: true,
              created_at: '2024-01-01T00:00:00Z',
              updated_at: '2024-01-02T00:00:00Z',
            },
            {
              id: 'user-2',
              username: 'regular',
              is_admin: false,
              is_active: true,
              created_at: '2024-01-03T00:00:00Z',
              updated_at: '2024-01-04T00:00:00Z',
            },
          ]);
        })
      );

      const result = await getUsers();

      expect(result).toHaveLength(2);
      expect(result[0]).toEqual({
        id: 'user-1',
        username: 'admin',
        isAdmin: true,
        isActive: true,
        createdAt: '2024-01-01T00:00:00Z',
        updatedAt: '2024-01-02T00:00:00Z',
      });
      expect(result[1]).toEqual({
        id: 'user-2',
        username: 'regular',
        isAdmin: false,
        isActive: true,
        createdAt: '2024-01-03T00:00:00Z',
        updatedAt: '2024-01-04T00:00:00Z',
      });
    });

    it('returns empty array when no users', async () => {
      server.use(
        http.get(`${API_URL}/auth/admin/users`, () => {
          return HttpResponse.json([]);
        })
      );

      const result = await getUsers();
      expect(result).toEqual([]);
    });

    it('requires authentication', async () => {
      mockGetAccessToken.mockReturnValue(null);

      await expect(getUsers()).rejects.toMatchObject({
        message: 'Not authenticated',
        status: 401,
      });
    });

    it('throws error on server error', async () => {
      server.use(
        http.get(`${API_URL}/auth/admin/users`, () => {
          return HttpResponse.json({ detail: 'Admin access required' }, { status: 403 });
        })
      );

      await expect(getUsers()).rejects.toMatchObject({
        message: 'Admin access required',
        status: 403,
      });
    });
  });

  describe('getUserById', () => {
    it('returns transformed user', async () => {
      server.use(
        http.get(`${API_URL}/auth/admin/users/user-123`, () => {
          return HttpResponse.json({
            id: 'user-123',
            username: 'testuser',
            is_admin: false,
            is_active: true,
            created_at: '2024-01-01T00:00:00Z',
            updated_at: '2024-01-02T00:00:00Z',
          });
        })
      );

      const result = await getUserById('user-123');

      expect(result).toEqual({
        id: 'user-123',
        username: 'testuser',
        isAdmin: false,
        isActive: true,
        createdAt: '2024-01-01T00:00:00Z',
        updatedAt: '2024-01-02T00:00:00Z',
      });
    });

    it('requires authentication', async () => {
      mockGetAccessToken.mockReturnValue(null);

      await expect(getUserById('user-123')).rejects.toMatchObject({
        message: 'Not authenticated',
        status: 401,
      });
    });

    it('throws error when user not found', async () => {
      server.use(
        http.get(`${API_URL}/auth/admin/users/nonexistent`, () => {
          return HttpResponse.json({ detail: 'User not found' }, { status: 404 });
        })
      );

      await expect(getUserById('nonexistent')).rejects.toMatchObject({
        message: 'User not found',
        status: 404,
      });
    });
  });

  describe('createUser', () => {
    it('creates user and returns transformed result', async () => {
      server.use(
        http.post(`${API_URL}/auth/admin/users`, async ({ request }) => {
          const body = (await request.json()) as {
            username: string;
            password: string;
            is_admin?: boolean;
          };
          expect(body.username).toBe('newuser');
          expect(body.password).toBe('password123');
          expect(body.is_admin).toBe(true);

          return HttpResponse.json({
            id: 'new-user-id',
            username: 'newuser',
            is_admin: true,
            is_active: true,
            created_at: '2024-01-01T00:00:00Z',
            updated_at: '2024-01-01T00:00:00Z',
          });
        })
      );

      const result = await createUser({
        username: 'newuser',
        password: 'password123',
        is_admin: true,
      });

      expect(result).toEqual({
        id: 'new-user-id',
        username: 'newuser',
        isAdmin: true,
        isActive: true,
        createdAt: '2024-01-01T00:00:00Z',
        updatedAt: '2024-01-01T00:00:00Z',
      });
    });

    it('creates non-admin user by default', async () => {
      server.use(
        http.post(`${API_URL}/auth/admin/users`, async ({ request }) => {
          const body = (await request.json()) as {
            username: string;
            password: string;
            is_admin?: boolean;
          };
          expect(body.is_admin).toBeUndefined();

          return HttpResponse.json({
            id: 'new-user-id',
            username: body.username,
            is_admin: false,
            is_active: true,
            created_at: '2024-01-01T00:00:00Z',
            updated_at: '2024-01-01T00:00:00Z',
          });
        })
      );

      const result = await createUser({
        username: 'regularuser',
        password: 'password123',
      });

      expect(result.isAdmin).toBe(false);
    });

    it('requires authentication', async () => {
      mockGetAccessToken.mockReturnValue(null);

      await expect(
        createUser({ username: 'test', password: 'test123' })
      ).rejects.toMatchObject({
        message: 'Not authenticated',
        status: 401,
      });
    });

    it('throws error when username is taken', async () => {
      server.use(
        http.post(`${API_URL}/auth/admin/users`, () => {
          return HttpResponse.json({ detail: 'Username already exists' }, { status: 409 });
        })
      );

      await expect(
        createUser({ username: 'existing', password: 'test123' })
      ).rejects.toMatchObject({
        message: 'Username already exists',
        status: 409,
      });
    });
  });

  describe('updateUser', () => {
    it('updates user and returns transformed result', async () => {
      server.use(
        http.put(`${API_URL}/auth/admin/users/user-123`, async ({ request }) => {
          const body = (await request.json()) as {
            username?: string;
            is_active?: boolean;
            is_admin?: boolean;
          };
          expect(body.username).toBe('updated');
          expect(body.is_active).toBe(false);
          expect(body.is_admin).toBe(true);

          return HttpResponse.json({
            id: 'user-123',
            username: 'updated',
            is_admin: true,
            is_active: false,
            created_at: '2024-01-01T00:00:00Z',
            updated_at: '2024-01-02T00:00:00Z',
          });
        })
      );

      const result = await updateUser('user-123', {
        username: 'updated',
        is_active: false,
        is_admin: true,
      });

      expect(result).toEqual({
        id: 'user-123',
        username: 'updated',
        isAdmin: true,
        isActive: false,
        createdAt: '2024-01-01T00:00:00Z',
        updatedAt: '2024-01-02T00:00:00Z',
      });
    });

    it('allows partial updates', async () => {
      server.use(
        http.put(`${API_URL}/auth/admin/users/user-123`, async ({ request }) => {
          const body = (await request.json()) as {
            username?: string;
            is_active?: boolean;
            is_admin?: boolean;
          };
          expect(body.is_active).toBe(false);
          expect(body.username).toBeUndefined();
          expect(body.is_admin).toBeUndefined();

          return HttpResponse.json({
            id: 'user-123',
            username: 'unchanged',
            is_admin: false,
            is_active: false,
            created_at: '2024-01-01T00:00:00Z',
            updated_at: '2024-01-02T00:00:00Z',
          });
        })
      );

      const result = await updateUser('user-123', { is_active: false });

      expect(result.isActive).toBe(false);
    });

    it('requires authentication', async () => {
      mockGetAccessToken.mockReturnValue(null);

      await expect(
        updateUser('user-123', { username: 'test' })
      ).rejects.toMatchObject({
        message: 'Not authenticated',
        status: 401,
      });
    });
  });

  describe('resetUserPassword', () => {
    it('resets password successfully', async () => {
      server.use(
        http.put(`${API_URL}/auth/admin/users/user-123/password`, async ({ request }) => {
          const body = (await request.json()) as { new_password: string };
          expect(body.new_password).toBe('newpassword123');

          return HttpResponse.json({ message: 'Password reset successfully' });
        })
      );

      await expect(resetUserPassword('user-123', 'newpassword123')).resolves.toBeUndefined();
    });

    it('requires authentication', async () => {
      mockGetAccessToken.mockReturnValue(null);

      await expect(
        resetUserPassword('user-123', 'newpass')
      ).rejects.toMatchObject({
        message: 'Not authenticated',
        status: 401,
      });
    });

    it('throws error when user not found', async () => {
      server.use(
        http.put(`${API_URL}/auth/admin/users/nonexistent/password`, () => {
          return HttpResponse.json({ detail: 'User not found' }, { status: 404 });
        })
      );

      await expect(
        resetUserPassword('nonexistent', 'newpass')
      ).rejects.toMatchObject({
        message: 'User not found',
        status: 404,
      });
    });
  });

  describe('getUserDeletionImpact', () => {
    it('returns deletion impact data', async () => {
      const mockImpact = {
        user_id: 'user-123',
        username: 'testuser',
        total_games: 10,
        total_tags: 5,
        total_wishlist_items: 3,
        total_import_jobs: 2,
        total_sessions: 1,
        warning: 'This action cannot be undone',
      };

      server.use(
        http.get(`${API_URL}/auth/admin/users/user-123/deletion-impact`, () => {
          return HttpResponse.json(mockImpact);
        })
      );

      const result = await getUserDeletionImpact('user-123');

      expect(result).toEqual(mockImpact);
    });

    it('requires authentication', async () => {
      mockGetAccessToken.mockReturnValue(null);

      await expect(getUserDeletionImpact('user-123')).rejects.toMatchObject({
        message: 'Not authenticated',
        status: 401,
      });
    });
  });

  describe('deleteUser', () => {
    it('deletes user successfully', async () => {
      server.use(
        http.delete(`${API_URL}/auth/admin/users/user-123`, () => {
          return new HttpResponse(null, { status: 204 });
        })
      );

      await expect(deleteUser('user-123')).resolves.toBeUndefined();
    });

    it('requires authentication', async () => {
      mockGetAccessToken.mockReturnValue(null);

      await expect(deleteUser('user-123')).rejects.toMatchObject({
        message: 'Not authenticated',
        status: 401,
      });
    });

    it('throws error when user not found', async () => {
      server.use(
        http.delete(`${API_URL}/auth/admin/users/nonexistent`, () => {
          return HttpResponse.json({ detail: 'User not found' }, { status: 404 });
        })
      );

      await expect(deleteUser('nonexistent')).rejects.toMatchObject({
        message: 'User not found',
        status: 404,
      });
    });

    it('throws error when deleting self', async () => {
      server.use(
        http.delete(`${API_URL}/auth/admin/users/current-user`, () => {
          return HttpResponse.json({ detail: 'Cannot delete yourself' }, { status: 400 });
        })
      );

      await expect(deleteUser('current-user')).rejects.toMatchObject({
        message: 'Cannot delete yourself',
        status: 400,
      });
    });
  });
});
