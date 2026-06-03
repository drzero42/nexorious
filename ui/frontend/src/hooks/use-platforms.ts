import { useQuery } from '@tanstack/react-query';
import * as platformsApi from '@/api/platforms';
import type {
  GetPlatformsParams,
  GetStorefrontsParams,
  PlatformsListResponse,
  StorefrontsListResponse,
} from '@/api/platforms';
import type { Platform, Storefront } from '@/types/platform';

// Query Keys

export const platformKeys = {
  all: ['platforms'] as const,
  lists: () => [...platformKeys.all, 'list'] as const,
  list: (params?: GetPlatformsParams) => [...platformKeys.lists(), params] as const,
  details: () => [...platformKeys.all, 'detail'] as const,
  detail: (id: string) => [...platformKeys.details(), id] as const,
  storefronts: (platformId: string) => [...platformKeys.all, 'storefronts', platformId] as const,
  names: () => [...platformKeys.all, 'names'] as const,
};

export const storefrontKeys = {
  all: ['storefronts'] as const,
  lists: () => [...storefrontKeys.all, 'list'] as const,
  list: (params?: GetStorefrontsParams) => [...storefrontKeys.lists(), params] as const,
  details: () => [...storefrontKeys.all, 'detail'] as const,
  detail: (id: string) => [...storefrontKeys.details(), id] as const,
  names: () => [...storefrontKeys.all, 'names'] as const,
};

// Platform Query Hooks

/**
 * Uses Infinity staleTime since platforms rarely change.
 */
export function usePlatforms(params?: GetPlatformsParams) {
  return useQuery<PlatformsListResponse, Error>({
    queryKey: platformKeys.list(params),
    queryFn: () => platformsApi.getPlatforms(params),
    staleTime: Infinity,
  });
}

/**
 * Uses Infinity staleTime since platforms rarely change.
 */
export function useAllPlatforms(params?: Omit<GetPlatformsParams, 'page' | 'perPage'>) {
  return useQuery<Platform[], Error>({
    queryKey: platformKeys.list({ ...params, page: 1, perPage: 100 }),
    queryFn: () => platformsApi.getAllPlatforms(params),
    staleTime: Infinity,
  });
}

export function usePlatform(id: string | undefined) {
  return useQuery<Platform, Error>({
    queryKey: platformKeys.detail(id ?? ''),
    queryFn: () => platformsApi.getPlatform(id!),
    enabled: !!id,
    staleTime: Infinity,
  });
}

export function usePlatformStorefronts(platformId: string | undefined, activeOnly?: boolean) {
  return useQuery<Storefront[], Error>({
    queryKey: platformKeys.storefronts(platformId ?? ''),
    queryFn: () => platformsApi.getPlatformStorefronts(platformId!, activeOnly),
    enabled: !!platformId,
    staleTime: Infinity,
  });
}

/**
 * Uses Infinity staleTime since platforms rarely change.
 */
export function usePlatformNames(activeOnly?: boolean) {
  return useQuery<string[], Error>({
    queryKey: platformKeys.names(),
    queryFn: () => platformsApi.getPlatformNames(activeOnly),
    staleTime: Infinity,
  });
}

// Storefront Query Hooks

/**
 * Uses Infinity staleTime since storefronts rarely change.
 */
export function useStorefronts(params?: GetStorefrontsParams) {
  return useQuery<StorefrontsListResponse, Error>({
    queryKey: storefrontKeys.list(params),
    queryFn: () => platformsApi.getStorefronts(params),
    staleTime: Infinity,
  });
}

/**
 * Uses Infinity staleTime since storefronts rarely change.
 */
export function useAllStorefronts(params?: Omit<GetStorefrontsParams, 'page' | 'perPage'>) {
  return useQuery<Storefront[], Error>({
    queryKey: storefrontKeys.list({ ...params, page: 1, perPage: 100 }),
    queryFn: () => platformsApi.getAllStorefronts(params),
    staleTime: Infinity,
  });
}

export function useStorefront(id: string | undefined) {
  return useQuery<Storefront, Error>({
    queryKey: storefrontKeys.detail(id ?? ''),
    queryFn: () => platformsApi.getStorefront(id!),
    enabled: !!id,
    staleTime: Infinity,
  });
}

/**
 * Uses Infinity staleTime since storefronts rarely change.
 */
export function useStorefrontNames(activeOnly?: boolean) {
  return useQuery<string[], Error>({
    queryKey: storefrontKeys.names(),
    queryFn: () => platformsApi.getStorefrontNames(activeOnly),
    staleTime: Infinity,
  });
}
