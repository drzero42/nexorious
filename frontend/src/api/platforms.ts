import { api } from './client';
import type { Platform, Storefront } from '@/types/platform';

// ============================================================================
// API Response Types (snake_case from backend)
// ============================================================================

interface PlatformApiResponse {
  id: string;
  name: string;
  display_name: string;
  icon_url?: string;
  is_active: boolean;
  source: string;
  default_storefront_id?: string;
  storefronts?: StorefrontApiResponse[];
  created_at: string;
  updated_at: string;
}

interface StorefrontApiResponse {
  id: string;
  name: string;
  display_name: string;
  icon_url?: string;
  base_url?: string;
  is_active: boolean;
  source: string;
  created_at: string;
  updated_at: string;
}

interface PlatformListApiResponse {
  platforms: PlatformApiResponse[];
  total: number;
  page: number;
  per_page: number;
  pages: number;
}

interface StorefrontListApiResponse {
  storefronts: StorefrontApiResponse[];
  total: number;
  page: number;
  per_page: number;
  pages: number;
}

// ============================================================================
// Request Parameter Types
// ============================================================================

export interface GetPlatformsParams {
  activeOnly?: boolean;
  source?: 'official' | 'custom';
  page?: number;
  perPage?: number;
}

export interface GetStorefrontsParams {
  activeOnly?: boolean;
  source?: 'official' | 'custom';
  page?: number;
  perPage?: number;
}

// ============================================================================
// Response Types
// ============================================================================

export interface PlatformsListResponse {
  platforms: Platform[];
  total: number;
  page: number;
  perPage: number;
  pages: number;
}

export interface StorefrontsListResponse {
  storefronts: Storefront[];
  total: number;
  page: number;
  perPage: number;
  pages: number;
}

// ============================================================================
// Transformation Functions
// ============================================================================

function transformStorefront(apiStorefront: StorefrontApiResponse): Storefront {
  return {
    id: apiStorefront.id,
    name: apiStorefront.name,
    display_name: apiStorefront.display_name,
    icon_url: apiStorefront.icon_url,
    base_url: apiStorefront.base_url,
    is_active: apiStorefront.is_active,
    source: apiStorefront.source,
    created_at: apiStorefront.created_at,
    updated_at: apiStorefront.updated_at,
  };
}

function transformPlatform(apiPlatform: PlatformApiResponse): Platform {
  return {
    id: apiPlatform.id,
    name: apiPlatform.name,
    display_name: apiPlatform.display_name,
    icon_url: apiPlatform.icon_url,
    is_active: apiPlatform.is_active,
    source: apiPlatform.source,
    default_storefront_id: apiPlatform.default_storefront_id,
    storefronts: apiPlatform.storefronts?.map(transformStorefront),
    created_at: apiPlatform.created_at,
    updated_at: apiPlatform.updated_at,
  };
}

// ============================================================================
// API Functions
// ============================================================================

/**
 * Get a paginated list of platforms.
 */
export async function getPlatforms(
  params?: GetPlatformsParams
): Promise<PlatformsListResponse> {
  const queryParams: Record<string, string | number | boolean | undefined> = {
    active_only: params?.activeOnly ?? true,
    source: params?.source,
    page: params?.page ?? 1,
    per_page: params?.perPage ?? 100, // Default to 100 for dropdown use cases
  };

  const response = await api.get<PlatformListApiResponse>('/platforms/', {
    params: queryParams,
  });

  return {
    platforms: response.platforms.map(transformPlatform),
    total: response.total,
    page: response.page,
    perPage: response.per_page,
    pages: response.pages,
  };
}

/**
 * Get all platforms (convenience function that fetches all pages).
 */
export async function getAllPlatforms(
  params?: Omit<GetPlatformsParams, 'page' | 'perPage'>
): Promise<Platform[]> {
  const response = await getPlatforms({
    ...params,
    page: 1,
    perPage: 100,
  });
  return response.platforms;
}

/**
 * Get a single platform by ID.
 */
export async function getPlatform(id: string): Promise<Platform> {
  const response = await api.get<PlatformApiResponse>(`/platforms/${id}`);
  return transformPlatform(response);
}

/**
 * Get storefronts associated with a specific platform.
 */
export async function getPlatformStorefronts(
  platformId: string,
  activeOnly?: boolean
): Promise<Storefront[]> {
  const response = await api.get<{
    platform_id: string;
    platform_name: string;
    platform_display_name: string;
    storefronts: StorefrontApiResponse[];
    total_storefronts: number;
  }>(`/platforms/${platformId}/storefronts`, {
    params: { active_only: activeOnly ?? true },
  });

  return response.storefronts.map(transformStorefront);
}

/**
 * Get a paginated list of storefronts.
 */
export async function getStorefronts(
  params?: GetStorefrontsParams
): Promise<StorefrontsListResponse> {
  const queryParams: Record<string, string | number | boolean | undefined> = {
    active_only: params?.activeOnly ?? true,
    source: params?.source,
    page: params?.page ?? 1,
    per_page: params?.perPage ?? 100, // Default to 100 for dropdown use cases
  };

  const response = await api.get<StorefrontListApiResponse>(
    '/platforms/storefronts/',
    {
      params: queryParams,
    }
  );

  return {
    storefronts: response.storefronts.map(transformStorefront),
    total: response.total,
    page: response.page,
    perPage: response.per_page,
    pages: response.pages,
  };
}

/**
 * Get all storefronts (convenience function that fetches all pages).
 */
export async function getAllStorefronts(
  params?: Omit<GetStorefrontsParams, 'page' | 'perPage'>
): Promise<Storefront[]> {
  const response = await getStorefronts({
    ...params,
    page: 1,
    perPage: 100,
  });
  return response.storefronts;
}

/**
 * Get a single storefront by ID.
 */
export async function getStorefront(id: string): Promise<Storefront> {
  const response = await api.get<StorefrontApiResponse>(
    `/platforms/storefronts/${id}`
  );
  return transformStorefront(response);
}

/**
 * Get simple list of platform names for dropdowns.
 */
export async function getPlatformNames(activeOnly?: boolean): Promise<string[]> {
  return api.get<string[]>('/platforms/simple-list', {
    params: { active_only: activeOnly ?? true },
  });
}

/**
 * Get simple list of storefront names for dropdowns.
 */
export async function getStorefrontNames(activeOnly?: boolean): Promise<string[]> {
  return api.get<string[]>('/platforms/storefronts/simple-list', {
    params: { active_only: activeOnly ?? true },
  });
}

// ============================================================================
// Admin CRUD Types
// ============================================================================

export interface PlatformCreateData {
  name: string;
  display_name: string;
  icon_url?: string;
  is_active?: boolean;
  default_storefront_id?: string;
}

export interface PlatformUpdateData {
  display_name?: string;
  icon_url?: string | null;
  is_active?: boolean;
  default_storefront_id?: string | null;
}

export interface StorefrontCreateData {
  name: string;
  display_name: string;
  icon_url?: string;
  base_url?: string;
  is_active?: boolean;
}

export interface StorefrontUpdateData {
  display_name?: string;
  icon_url?: string | null;
  base_url?: string | null;
  is_active?: boolean;
}

// ============================================================================
// Admin Platform CRUD Operations
// ============================================================================

/**
 * Create a new platform (admin only).
 */
export async function createPlatform(data: PlatformCreateData): Promise<Platform> {
  const response = await api.post<PlatformApiResponse>('/platforms/', data);
  return transformPlatform(response);
}

/**
 * Update an existing platform (admin only).
 */
export async function updatePlatform(
  id: string,
  data: PlatformUpdateData
): Promise<Platform> {
  const response = await api.put<PlatformApiResponse>(`/platforms/${id}`, data);
  return transformPlatform(response);
}

/**
 * Delete a platform (admin only).
 */
export async function deletePlatform(id: string): Promise<void> {
  await api.delete(`/platforms/${id}`);
}

// ============================================================================
// Admin Storefront CRUD Operations
// ============================================================================

/**
 * Create a new storefront (admin only).
 */
export async function createStorefront(data: StorefrontCreateData): Promise<Storefront> {
  const response = await api.post<StorefrontApiResponse>('/platforms/storefronts/', data);
  return transformStorefront(response);
}

/**
 * Update an existing storefront (admin only).
 */
export async function updateStorefront(
  id: string,
  data: StorefrontUpdateData
): Promise<Storefront> {
  const response = await api.put<StorefrontApiResponse>(
    `/platforms/storefronts/${id}`,
    data
  );
  return transformStorefront(response);
}

/**
 * Delete a storefront (admin only).
 */
export async function deleteStorefront(id: string): Promise<void> {
  await api.delete(`/platforms/storefronts/${id}`);
}

// ============================================================================
// Admin Platform-Storefront Association Operations
// ============================================================================

/**
 * Create a platform-storefront association (admin only).
 */
export async function createPlatformStorefrontAssociation(
  platformId: string,
  storefrontId: string
): Promise<void> {
  await api.post(`/platforms/${platformId}/storefronts/${storefrontId}`);
}

/**
 * Delete a platform-storefront association (admin only).
 */
export async function deletePlatformStorefrontAssociation(
  platformId: string,
  storefrontId: string
): Promise<void> {
  await api.delete(`/platforms/${platformId}/storefronts/${storefrontId}`);
}
