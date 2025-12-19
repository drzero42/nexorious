import { api } from './client';
import type {
  AdminUser,
  AdminUserBackend,
  UserDeletionImpact,
  CreateUserRequest,
  UpdateUserRequest,
  ResetPasswordRequest,
} from '@/types';

function mapBackendUserToFrontend(backendUser: AdminUserBackend): AdminUser {
  return {
    id: backendUser.id,
    username: backendUser.username,
    isActive: backendUser.is_active,
    isAdmin: backendUser.is_admin,
    createdAt: backendUser.created_at,
    updatedAt: backendUser.updated_at,
  };
}

/**
 * Fetch all users (admin only)
 */
export async function getUsers(): Promise<AdminUser[]> {
  const backendUsers = await api.get<AdminUserBackend[]>('/auth/admin/users');
  return backendUsers.map(mapBackendUserToFrontend);
}

/**
 * Fetch a single user by ID (admin only)
 */
export async function getUserById(userId: string): Promise<AdminUser> {
  const backendUser = await api.get<AdminUserBackend>(`/auth/admin/users/${userId}`);
  return mapBackendUserToFrontend(backendUser);
}

/**
 * Create a new user (admin only)
 */
export async function createUser(data: CreateUserRequest): Promise<AdminUser> {
  const backendUser = await api.post<AdminUserBackend>('/auth/admin/users', data);
  return mapBackendUserToFrontend(backendUser);
}

/**
 * Update a user (admin only)
 */
export async function updateUser(userId: string, data: UpdateUserRequest): Promise<AdminUser> {
  const backendUser = await api.put<AdminUserBackend>(`/auth/admin/users/${userId}`, data);
  return mapBackendUserToFrontend(backendUser);
}

/**
 * Reset a user's password (admin only)
 */
export async function resetUserPassword(userId: string, newPassword: string): Promise<void> {
  const data: ResetPasswordRequest = { new_password: newPassword };
  await api.put(`/auth/admin/users/${userId}/password`, data);
}

/**
 * Get the impact of deleting a user (admin only)
 */
export async function getUserDeletionImpact(userId: string): Promise<UserDeletionImpact> {
  return api.get<UserDeletionImpact>(`/auth/admin/users/${userId}/deletion-impact`);
}

/**
 * Delete a user (admin only)
 */
export async function deleteUser(userId: string): Promise<void> {
  await api.delete(`/auth/admin/users/${userId}`);
}
