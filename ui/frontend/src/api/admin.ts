import { api } from './client';
import type {
  AdminUser,
  AdminUserBackend,
  UserDeletionImpact,
  CreateUserRequest,
  UpdateUserRequest,
  ResetPasswordRequest,
  AdminStatistics,
  SeedDataResult,
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

/**
 * Get admin statistics (computed from users list)
 * Note: This computes stats from user data as there's no dedicated stats endpoint
 */
export async function getAdminStatistics(): Promise<AdminStatistics> {
  const users = await getUsers();

  const totalUsers = users.length;
  const totalAdmins = users.filter(u => u.isAdmin).length;

  // Sort by creation date (newest first) and take top 5
  const recentUsers = [...users]
    .sort((a, b) => new Date(b.createdAt).getTime() - new Date(a.createdAt).getTime())
    .slice(0, 5);

  return {
    totalUsers,
    totalAdmins,
    totalGames: 0, // Would need a separate endpoint
    recentUsers,
  };
}

/**
 * Load seed data for platforms and storefronts (admin only)
 */
export async function loadSeedData(version?: string): Promise<SeedDataResult> {
  const response = await api.post<{
    platforms_added: number;
    storefronts_added: number;
    mappings_created: number;
    total_changes: number;
    message: string;
  }>('/platforms/seed', { version: version ?? '1.0.0' });

  return {
    platformsAdded: response.platforms_added,
    storefrontsAdded: response.storefronts_added,
    mappingsCreated: response.mappings_created,
    totalChanges: response.total_changes,
    message: response.message,
  };
}

/**
 * Response from starting a metadata refresh job
 */
export interface MetadataRefreshJobResult {
  success: boolean;
  message: string;
  jobId: string;
}

/**
 * Start a metadata refresh job to update game metadata from IGDB (admin only)
 * Uses fan-out pattern to process games in parallel via background workers.
 */
export async function startMetadataRefreshJob(gameIds?: string[]): Promise<MetadataRefreshJobResult> {
  const response = await api.post<{
    success: boolean;
    message: string;
    job_id: string;
  }>('/games/metadata/refresh-job', { game_ids: gameIds ?? null });

  return {
    success: response.success,
    message: response.message,
    jobId: response.job_id,
  };
}
