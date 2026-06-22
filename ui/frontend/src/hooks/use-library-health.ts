import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import * as smellsApi from '@/api/library-health';
import type {
  SmellSummaryItem,
  FlaggedListResponse,
  IgnoredListResponse,
  ApplyResult,
} from '@/api/library-health';

export const smellKeys = {
  all: ['librarySmells'] as const,
  summary: () => [...smellKeys.all, 'summary'] as const,
  list: (checkID: string) => [...smellKeys.all, 'list', checkID] as const,
  ignored: (checkID: string) => [...smellKeys.all, 'ignored', checkID] as const,
};

export function useSmellSummary() {
  return useQuery<SmellSummaryItem[], Error>({
    queryKey: smellKeys.summary(),
    queryFn: () => smellsApi.getSmellSummary(),
  });
}

export function useSmellItems(checkID: string, enabled: boolean) {
  return useQuery<FlaggedListResponse, Error>({
    queryKey: smellKeys.list(checkID),
    queryFn: () => smellsApi.getSmellItems(checkID),
    enabled,
  });
}

export function useIgnoredItems(checkID: string, enabled: boolean) {
  return useQuery<IgnoredListResponse, Error>({
    queryKey: smellKeys.ignored(checkID),
    queryFn: () => smellsApi.getIgnoredItems(checkID),
    enabled,
  });
}

function useInvalidateSmells() {
  const queryClient = useQueryClient();
  return (checkID: string) => {
    queryClient.invalidateQueries({ queryKey: smellKeys.summary() });
    queryClient.invalidateQueries({ queryKey: smellKeys.list(checkID) });
    queryClient.invalidateQueries({ queryKey: smellKeys.ignored(checkID) });
  };
}

export function useApplySmell() {
  const invalidate = useInvalidateSmells();
  return useMutation<ApplyResult, Error, { checkID: string; userGameIds: string[] }>({
    mutationFn: ({ checkID, userGameIds }) => smellsApi.applySmell(checkID, userGameIds),
    onSuccess: (_res, { checkID }) => invalidate(checkID),
  });
}

export function useApplyAllSmell() {
  const invalidate = useInvalidateSmells();
  return useMutation<ApplyResult, Error, { checkID: string }>({
    mutationFn: ({ checkID }) => smellsApi.applyAllSmell(checkID),
    onSuccess: (_res, { checkID }) => invalidate(checkID),
  });
}

export function useIgnoreSmell() {
  const invalidate = useInvalidateSmells();
  return useMutation<{ ignored: number }, Error, { checkID: string; userGameIds: string[] }>({
    mutationFn: ({ checkID, userGameIds }) => smellsApi.ignoreSmell(checkID, userGameIds),
    onSuccess: (_res, { checkID }) => invalidate(checkID),
  });
}

export function useRestoreSmell() {
  const invalidate = useInvalidateSmells();
  return useMutation<{ restored: number }, Error, { checkID: string; userGameIds: string[] }>({
    mutationFn: ({ checkID, userGameIds }) => smellsApi.restoreSmell(checkID, userGameIds),
    onSuccess: (_res, { checkID }) => invalidate(checkID),
  });
}
