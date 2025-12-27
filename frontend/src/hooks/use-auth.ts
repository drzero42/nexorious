import { useMutation, useQueryClient } from '@tanstack/react-query';
import * as authApi from '@/api/auth';
import type { User } from '@/types';

// ============================================================================
// Query Keys
// ============================================================================

export const authKeys = {
  all: ['auth'] as const,
  me: () => [...authKeys.all, 'me'] as const,
};

// ============================================================================
// Mutation Hooks
// ============================================================================

/**
 * Hook to update user profile/preferences.
 */
export function useUpdateProfile() {
  const queryClient = useQueryClient();

  return useMutation<User, Error, { preferences?: Record<string, unknown> }>({
    mutationFn: (data) => authApi.updatePreferences(data.preferences ?? {}),
    onSuccess: (user) => {
      queryClient.setQueryData(authKeys.me(), user);
    },
  });
}
