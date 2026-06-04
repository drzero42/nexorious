import { describe, it, expect, vi, beforeEach } from 'vitest';
import { http, HttpResponse } from 'msw';
import { server } from '@/test/mocks/server';
import {
  login,
  logout,
  getMe,
  changeUsername,
  changePassword,
  checkUsernameAvailability,
} from './auth';

const API_URL = '/api';

describe('auth.ts', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  describe('login', () => {
    it('successfully logs in and returns user data', async () => {
      server.use(
        http.post(`${API_URL}/auth/login`, async ({ request }) => {
          const body = (await request.json()) as { username: string; password: string };
          expect(body.username).toBe('testuser');
          expect(body.password).toBe('password123');
          return HttpResponse.json({
            id: 'user-123',
            username: 'testuser',
            is_admin: false,
            preferences: null,
          });
        }),
      );

      const result = await login('testuser', 'password123');

      expect(result).toEqual({
        id: 'user-123',
        username: 'testuser',
        isAdmin: false,
        preferences: null,
      });
    });

    it('throws error on invalid credentials', async () => {
      server.use(
        http.post(`${API_URL}/auth/login`, () => {
          return HttpResponse.json({ detail: 'Invalid credentials' }, { status: 401 });
        }),
      );

      await expect(login('baduser', 'badpass')).rejects.toMatchObject({
        message: 'Invalid credentials',
        status: 401,
      });
    });
  });

  describe('logout', () => {
    it('logs out successfully', async () => {
      server.use(
        http.post(`${API_URL}/auth/logout`, () => {
          return HttpResponse.json({ message: 'Logged out successfully' });
        }),
      );

      await expect(logout()).resolves.toBeUndefined();
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
        }),
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
        }),
      );

      const result = await getMe();

      expect(result.isAdmin).toBe(false);
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
        }),
      );

      const result = await changeUsername('newusername');

      expect(result).toEqual({
        id: 'user-123',
        username: 'newusername',
        isAdmin: false,
        preferences: undefined,
      });
    });

    it('throws error when username is taken', async () => {
      server.use(
        http.put(`${API_URL}/auth/username`, () => {
          return HttpResponse.json({ detail: 'Username already taken' }, { status: 409 });
        }),
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
        }),
      );

      // Should not throw
      await expect(changePassword('oldpass', 'newpass')).resolves.toBeUndefined();
    });

    it('throws error on incorrect current password', async () => {
      server.use(
        http.put(`${API_URL}/auth/change-password`, () => {
          return HttpResponse.json({ detail: 'Current password is incorrect' }, { status: 400 });
        }),
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
        }),
      );

      const result = await checkUsernameAvailability('newuser');

      expect(result).toEqual({ available: true, username: 'newuser' });
    });

    it('returns availability status for taken username', async () => {
      server.use(
        http.get(`${API_URL}/auth/username/check/takenuser`, () => {
          return HttpResponse.json({ available: false, username: 'takenuser' });
        }),
      );

      const result = await checkUsernameAvailability('takenuser');

      expect(result).toEqual({ available: false, username: 'takenuser' });
    });

    it('encodes special characters in username', async () => {
      server.use(
        http.get(`${API_URL}/auth/username/check/user%40name`, () => {
          return HttpResponse.json({ available: true, username: 'user@name' });
        }),
      );

      const result = await checkUsernameAvailability('user@name');

      expect(result.available).toBe(true);
    });
  });
});
