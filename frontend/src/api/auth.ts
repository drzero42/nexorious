import { api } from './client';
import type { User, LoginResponse, SetupStatusResponse } from '@/types';
import { config } from '@/lib/env';

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

export async function login(username: string, password: string): Promise<LoginResponse> {
  return api.post<LoginResponse>('/auth/login', { username, password }, { skipAuth: true });
}

export async function getMe(): Promise<User> {
  const response = await api.get<UserApiResponse>('/auth/me');
  return transformUser(response);
}

export async function refreshToken(refreshTokenValue: string): Promise<LoginResponse> {
  return api.post<LoginResponse>(
    '/auth/refresh',
    { refresh_token: refreshTokenValue },
    { skipAuth: true }
  );
}

export async function checkSetupStatus(): Promise<SetupStatusResponse> {
  return api.get<SetupStatusResponse>('/auth/setup/status', { skipAuth: true });
}

export async function createInitialAdmin(username: string, password: string): Promise<User> {
  const response = await api.post<UserApiResponse>(
    '/auth/setup/admin',
    { username, password },
    { skipAuth: true }
  );
  return transformUser(response);
}

export async function changeUsername(newUsername: string): Promise<User> {
  const response = await api.put<UserApiResponse>('/auth/username', {
    new_username: newUsername,
  });
  return transformUser(response);
}

export async function changePassword(
  currentPassword: string,
  newPassword: string
): Promise<void> {
  await api.put('/auth/change-password', {
    current_password: currentPassword,
    new_password: newPassword,
  });
}

export async function checkUsernameAvailability(
  username: string
): Promise<UsernameAvailabilityResponse> {
  return api.get<UsernameAvailabilityResponse>(
    `/auth/username/check/${encodeURIComponent(username)}`
  );
}

export async function updatePreferences(
  preferences: Record<string, unknown>
): Promise<User> {
  const response = await api.put<UserApiResponse>('/auth/me', { preferences });
  return transformUser(response);
}

interface SetupRestoreResponse {
  success: boolean;
  message: string;
}

export async function setupRestore(file: File): Promise<SetupRestoreResponse> {
  const formData = new FormData();
  formData.append('file', file);

  const response = await fetch(`${config.apiUrl}/auth/setup/restore`, {
    method: 'POST',
    body: formData,
  });

  if (!response.ok) {
    const errorData = await response.json().catch(() => ({}));
    const message = errorData.detail || `HTTP ${response.status}: ${response.statusText}`;
    throw new Error(message);
  }

  return response.json();
}
