import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import * as authApi from '@/api/auth';
import type { ApiKey, CreatedApiKey } from '@/api/auth';

export const apiKeysKeys = {
  all: ['api-keys'] as const,
  list: () => [...apiKeysKeys.all, 'list'] as const,
};

export function useApiKeys() {
  return useQuery<ApiKey[]>({
    queryKey: apiKeysKeys.list(),
    queryFn: () => authApi.listApiKeys(),
  });
}

export function useCreateApiKey() {
  const queryClient = useQueryClient();
  return useMutation<
    CreatedApiKey,
    Error,
    { name: string; scopes: 'read' | 'write'; expires_at: string | null }
  >({
    mutationFn: (body) => authApi.createApiKey(body),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: apiKeysKeys.list() });
    },
  });
}

export function useRevokeApiKey() {
  const queryClient = useQueryClient();
  return useMutation<void, Error, string>({
    mutationFn: (id) => authApi.revokeApiKey(id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: apiKeysKeys.list() });
    },
  });
}
