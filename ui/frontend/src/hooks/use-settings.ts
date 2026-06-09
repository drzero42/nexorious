import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import * as settingsApi from '@/api/settings';
import type { Settings } from '@/types/settings';

const settingsKey = ['settings'] as const;

export function useSettings() {
  return useQuery<Settings, Error>({
    queryKey: settingsKey,
    queryFn: settingsApi.getSettings,
    staleTime: 5 * 60 * 1000,
  });
}

export function useUpdateSettings() {
  const queryClient = useQueryClient();
  return useMutation<Settings, Error, Partial<Settings>>({
    mutationFn: settingsApi.updateSettings,
    onSuccess: (updated) => queryClient.setQueryData(settingsKey, updated),
  });
}
