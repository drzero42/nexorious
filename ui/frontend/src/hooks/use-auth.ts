import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import * as authApi from '@/api/auth';
import type { User } from '@/types';

// Query Keys

export const authKeys = {
  all: ['auth'] as const,
  me: () => [...authKeys.all, 'me'] as const,
};

// Query Hooks

export function useCurrentUser() {
  return useQuery<User>({
    queryKey: authKeys.me(),
    queryFn: () => authApi.getMe(),
  });
}

// Mutation Hooks

export function useUpdateProfile() {
  const queryClient = useQueryClient();

  return useMutation<User, Error, { preferences?: Record<string, unknown> }>({
    mutationFn: (data) => authApi.updatePreferences(data.preferences ?? {}),
    onSuccess: (user) => {
      queryClient.setQueryData(authKeys.me(), user);
    },
  });
}
