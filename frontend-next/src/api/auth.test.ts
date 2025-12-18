import { describe, it, expect, vi, beforeEach, afterEach, type Mock } from 'vitest';
import { http, HttpResponse } from 'msw';
import { server } from '@/test/mocks/server';
import { setAuthHandlers } from './client';
import {
  login,
  getMe,
  refreshToken,
  checkSetupStatus,
  createInitialAdmin,
  changeUsername,
  changePassword,
  checkUsernameAvailability,
  updatePreferences,
} from './auth';

const API_URL = '/api';

describe('auth.ts', () => {
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

  describe('login', () => {
    it('successfully logs in and returns tokens', async () => {
      const mockResponse = {
        access_token: 'new-access-token',
        refresh_token: 'new-refresh-token',
        token_type: 'bearer',
        expires_in: 3600,
      };

      server.use(
        http.post(`${API_URL}/auth/login`, async ({ request }) => {
          const body = (await request.json()) as { username: string; password: string };
          expect(body.username).toBe('testuser');
          expect(body.password).toBe('password123');
          return HttpResponse.json(mockResponse);
        })
      );

      const result = await login('testuser', 'password123');

      expect(result).toEqual(mockResponse);
    });

    it('throws error on invalid credentials', async () => {
      server.use(
        http.post(`${API_URL}/auth/login`, () => {
          return HttpResponse.json({ detail: 'Invalid credentials' }, { status: 401 });
        })
      );

      await expect(login('baduser', 'badpass')).rejects.toMatchObject({
        message: 'Invalid credentials',
        status: 401,
      });
    });

    it('does not require authentication (skipAuth)', async () => {
      mockGetAccessToken.mockReturnValue(null);

      server.use(
        http.post(`${API_URL}/auth/login`, () => {
          return HttpResponse.json({
            access_token: 'token',
            refresh_token: 'refresh',
            token_type: 'bearer',
            expires_in: 3600,
          });
        })
      );

      // Should not throw even without auth token
      const result = await login('user', 'pass');
      expect(result.access_token).toBe('token');
    });
  });

  describe('getMe', () => {
    it('returns transformed user data', async () => {
      server.use(
        http.get(`${API_URL}/auth/me`, () => {
          return HttpResponse.json({
            id: 'user-123',
            username: 'testuser',
            is_admin: true,
            preferences: { theme: 'dark' },
          });
        })
      );

      const result = await getMe();

      expect(result).toEqual({
        id: 'user-123',
        username: 'testuser',
        isAdmin: true,
        preferences: { theme: 'dark' },
      });
    });

    it('transforms is_admin to isAdmin correctly', async () => {
      server.use(
        http.get(`${API_URL}/auth/me`, () => {
          return HttpResponse.json({
            id: 'user-456',
            username: 'regularuser',
            is_admin: false,
          });
        })
      );

      const result = await getMe();

      expect(result.isAdmin).toBe(false);
    });

    it('requires authentication', async () => {
      mockGetAccessToken.mockReturnValue(null);

      await expect(getMe()).rejects.toMatchObject({
        message: 'Not authenticated',
        status: 401,
      });
    });
  });

  describe('refreshToken', () => {
    it('refreshes token successfully', async () => {
      const mockResponse = {
        access_token: 'refreshed-access-token',
        refresh_token: 'new-refresh-token',
        token_type: 'bearer',
        expires_in: 3600,
      };

      server.use(
        http.post(`${API_URL}/auth/refresh`, async ({ request }) => {
          const body = (await request.json()) as { refresh_token: string };
          expect(body.refresh_token).toBe('old-refresh-token');
          return HttpResponse.json(mockResponse);
        })
      );

      const result = await refreshToken('old-refresh-token');

      expect(result).toEqual(mockResponse);
    });

    it('does not require authentication (skipAuth)', async () => {
      mockGetAccessToken.mockReturnValue(null);

      server.use(
        http.post(`${API_URL}/auth/refresh`, () => {
          return HttpResponse.json({
            access_token: 'token',
            refresh_token: 'refresh',
            token_type: 'bearer',
            expires_in: 3600,
          });
        })
      );

      const result = await refreshToken('refresh-token');
      expect(result.access_token).toBe('token');
    });

    it('throws error on invalid refresh token', async () => {
      server.use(
        http.post(`${API_URL}/auth/refresh`, () => {
          return HttpResponse.json({ detail: 'Invalid refresh token' }, { status: 401 });
        })
      );

      await expect(refreshToken('bad-token')).rejects.toMatchObject({
        message: 'Invalid refresh token',
        status: 401,
      });
    });
  });

  describe('checkSetupStatus', () => {
    it('returns setup status when setup is needed', async () => {
      server.use(
        http.get(`${API_URL}/auth/setup/status`, () => {
          return HttpResponse.json({ needs_setup: true });
        })
      );

      const result = await checkSetupStatus();

      expect(result).toEqual({ needs_setup: true });
    });

    it('returns setup status when setup is complete', async () => {
      server.use(
        http.get(`${API_URL}/auth/setup/status`, () => {
          return HttpResponse.json({ needs_setup: false });
        })
      );

      const result = await checkSetupStatus();

      expect(result).toEqual({ needs_setup: false });
    });

    it('does not require authentication (skipAuth)', async () => {
      mockGetAccessToken.mockReturnValue(null);

      server.use(
        http.get(`${API_URL}/auth/setup/status`, () => {
          return HttpResponse.json({ needs_setup: true });
        })
      );

      const result = await checkSetupStatus();
      expect(result.needs_setup).toBe(true);
    });
  });

  describe('createInitialAdmin', () => {
    it('creates admin user and returns transformed user', async () => {
      server.use(
        http.post(`${API_URL}/auth/setup/admin`, async ({ request }) => {
          const body = (await request.json()) as { username: string; password: string };
          expect(body.username).toBe('admin');
          expect(body.password).toBe('securepass');
          return HttpResponse.json({
            id: 'admin-123',
            username: 'admin',
            is_admin: true,
          });
        })
      );

      const result = await createInitialAdmin('admin', 'securepass');

      expect(result).toEqual({
        id: 'admin-123',
        username: 'admin',
        isAdmin: true,
        preferences: undefined,
      });
    });

    it('does not require authentication (skipAuth)', async () => {
      mockGetAccessToken.mockReturnValue(null);

      server.use(
        http.post(`${API_URL}/auth/setup/admin`, () => {
          return HttpResponse.json({
            id: 'admin-123',
            username: 'admin',
            is_admin: true,
          });
        })
      );

      const result = await createInitialAdmin('admin', 'pass');
      expect(result.username).toBe('admin');
    });

    it('throws error on invalid setup data', async () => {
      server.use(
        http.post(`${API_URL}/auth/setup/admin`, () => {
          return HttpResponse.json({ detail: 'Invalid setup data' }, { status: 400 });
        })
      );

      await expect(createInitialAdmin('', '')).rejects.toMatchObject({
        message: 'Invalid setup data',
        status: 400,
      });
    });
  });

  describe('changeUsername', () => {
    it('changes username and returns transformed user', async () => {
      server.use(
        http.put(`${API_URL}/auth/username`, async ({ request }) => {
          const body = (await request.json()) as { new_username: string };
          expect(body.new_username).toBe('newusername');
          return HttpResponse.json({
            id: 'user-123',
            username: 'newusername',
            is_admin: false,
          });
        })
      );

      const result = await changeUsername('newusername');

      expect(result).toEqual({
        id: 'user-123',
        username: 'newusername',
        isAdmin: false,
        preferences: undefined,
      });
    });

    it('requires authentication', async () => {
      mockGetAccessToken.mockReturnValue(null);

      await expect(changeUsername('newname')).rejects.toMatchObject({
        message: 'Not authenticated',
        status: 401,
      });
    });

    it('throws error when username is taken', async () => {
      server.use(
        http.put(`${API_URL}/auth/username`, () => {
          return HttpResponse.json({ detail: 'Username already taken' }, { status: 409 });
        })
      );

      await expect(changeUsername('takenname')).rejects.toMatchObject({
        message: 'Username already taken',
        status: 409,
      });
    });
  });

  describe('changePassword', () => {
    it('changes password successfully', async () => {
      server.use(
        http.put(`${API_URL}/auth/change-password`, async ({ request }) => {
          const body = (await request.json()) as {
            current_password: string;
            new_password: string;
          };
          expect(body.current_password).toBe('oldpass');
          expect(body.new_password).toBe('newpass');
          return HttpResponse.json({ message: 'Password changed successfully' });
        })
      );

      // Should not throw
      await expect(changePassword('oldpass', 'newpass')).resolves.toBeUndefined();
    });

    it('requires authentication', async () => {
      mockGetAccessToken.mockReturnValue(null);

      await expect(changePassword('old', 'new')).rejects.toMatchObject({
        message: 'Not authenticated',
        status: 401,
      });
    });

    it('throws error on incorrect current password', async () => {
      server.use(
        http.put(`${API_URL}/auth/change-password`, () => {
          return HttpResponse.json({ detail: 'Current password is incorrect' }, { status: 400 });
        })
      );

      await expect(changePassword('wrongpass', 'newpass')).rejects.toMatchObject({
        message: 'Current password is incorrect',
        status: 400,
      });
    });
  });

  describe('checkUsernameAvailability', () => {
    it('returns availability status for available username', async () => {
      server.use(
        http.get(`${API_URL}/auth/username/check/newuser`, () => {
          return HttpResponse.json({ available: true, username: 'newuser' });
        })
      );

      const result = await checkUsernameAvailability('newuser');

      expect(result).toEqual({ available: true, username: 'newuser' });
    });

    it('returns availability status for taken username', async () => {
      server.use(
        http.get(`${API_URL}/auth/username/check/takenuser`, () => {
          return HttpResponse.json({ available: false, username: 'takenuser' });
        })
      );

      const result = await checkUsernameAvailability('takenuser');

      expect(result).toEqual({ available: false, username: 'takenuser' });
    });

    it('encodes special characters in username', async () => {
      server.use(
        http.get(`${API_URL}/auth/username/check/user%40name`, () => {
          return HttpResponse.json({ available: true, username: 'user@name' });
        })
      );

      const result = await checkUsernameAvailability('user@name');

      expect(result.available).toBe(true);
    });

    it('requires authentication', async () => {
      mockGetAccessToken.mockReturnValue(null);

      await expect(checkUsernameAvailability('username')).rejects.toMatchObject({
        message: 'Not authenticated',
        status: 401,
      });
    });
  });

  describe('updatePreferences', () => {
    it('updates preferences and returns transformed user', async () => {
      const newPreferences = { theme: 'dark', language: 'en' };

      server.use(
        http.put(`${API_URL}/auth/me`, async ({ request }) => {
          const body = (await request.json()) as { preferences: Record<string, unknown> };
          expect(body.preferences).toEqual(newPreferences);
          return HttpResponse.json({
            id: 'user-123',
            username: 'testuser',
            is_admin: false,
            preferences: newPreferences,
          });
        })
      );

      const result = await updatePreferences(newPreferences);

      expect(result).toEqual({
        id: 'user-123',
        username: 'testuser',
        isAdmin: false,
        preferences: newPreferences,
      });
    });

    it('requires authentication', async () => {
      mockGetAccessToken.mockReturnValue(null);

      await expect(updatePreferences({ theme: 'light' })).rejects.toMatchObject({
        message: 'Not authenticated',
        status: 401,
      });
    });

    it('handles empty preferences', async () => {
      server.use(
        http.put(`${API_URL}/auth/me`, async ({ request }) => {
          const body = (await request.json()) as { preferences: Record<string, unknown> };
          expect(body.preferences).toEqual({});
          return HttpResponse.json({
            id: 'user-123',
            username: 'testuser',
            is_admin: false,
            preferences: {},
          });
        })
      );

      const result = await updatePreferences({});

      expect(result.preferences).toEqual({});
    });
  });
});
