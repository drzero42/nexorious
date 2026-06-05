import { useInfiniteQuery } from '@tanstack/react-query';
import { eventsApi } from '@/api/events';
import type { AdminEventFilters } from '@/types';

const eventKeys = {
  all: ['admin-events'] as const,
  lists: () => [...eventKeys.all, 'list'] as const,
  list: (filters: AdminEventFilters) => [...eventKeys.lists(), filters] as const,
};

const PAGE_SIZE = 50;

export function useAdminEvents(filters: AdminEventFilters, enabled = true) {
  return useInfiniteQuery({
    queryKey: eventKeys.list(filters),
    queryFn: ({ pageParam }) => eventsApi.list(filters, pageParam as string | undefined, PAGE_SIZE),
    initialPageParam: undefined as string | undefined,
    getNextPageParam: (lastPage) => lastPage.nextCursor ?? undefined,
    enabled,
  });
}
