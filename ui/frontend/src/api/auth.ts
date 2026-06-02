import { api } from './client';
import type { User } from '@/types';
interface UserApiResponse {
  id: string;
  username: string;
  is_admin: boolean;
  preferences?: Record<string, unknown>;
}

interface UsernameAvailabilityResponse {
  available: boolean;
  username: string;
}

function transformUser(apiUser: UserApiResponse): User {
  return {
    id: apiUser.id,
    username: apiUser.username,
    isAdmin: apiUser.is_admin,
    preferences: apiUser.preferences,
  };
}

export async function login(username: string, password: string): Promise<User> {
  const response = await api.post<UserApiResponse>('/auth/login', { username, password });
  return transformUser(response);
}

export async function logout(): Promise<void> {
  await api.post('/auth/logout');
}

export async function getMe(): Promise<User> {
  const response = await api.get<UserApiResponse>('/auth/me');
  return transformUser(response);
}

export async function changeUsername(newUsername: string): Promise<User> {
  const response = await api.put<UserApiResponse>('/auth/username', {
    new_username: newUsername,
  });
  return transformUser(response);
}

export async function changePassword(currentPassword: string, newPassword: string): Promise<void> {
  await api.put('/auth/change-password', {
    current_password: currentPassword,
    new_password: newPassword,
  });
}

export async function checkUsernameAvailability(
  username: string,
): Promise<UsernameAvailabilityResponse> {
  return api.get<UsernameAvailabilityResponse>(
    `/auth/username/check/${encodeURIComponent(username)}`,
  );
}

export async function updatePreferences(preferences: Record<string, unknown>): Promise<User> {
  const response = await api.put<UserApiResponse>('/auth/me', { preferences });
  return transformUser(response);
}

export interface ApiKey {
  id: string;
  name: string;
  scopes: 'read' | 'write';
  last_used_at: string | null;
  created_at: string;
  expires_at: string | null;
}

// The create response returns the raw key exactly once. It omits last_used_at
// (always null for a brand-new key), so it is not part of this shape.
export interface CreatedApiKey {
  id: string;
  name: string;
  scopes: 'read' | 'write';
  key: string;
  created_at: string;
  expires_at: string | null;
}

export function listApiKeys(): Promise<ApiKey[]> {
  return api.get<ApiKey[]>('/auth/api-keys');
}

export function createApiKey(body: {
  name: string;
  scopes: 'read' | 'write';
  expires_at: string | null;
}): Promise<CreatedApiKey> {
  return api.post<CreatedApiKey>('/auth/api-keys', body);
}

export function revokeApiKey(id: string): Promise<void> {
  return api.delete(`/auth/api-keys/${encodeURIComponent(id)}`);
}
