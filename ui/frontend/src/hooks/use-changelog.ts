import { useQuery } from '@tanstack/react-query';

import { changelogApi } from '@/api/changelog';

export const changelogKeys = {
  all: ['changelog'] as const,
  unseen: () => [...changelogKeys.all, 'unseen'] as const,
  content: (mode: string) => [...changelogKeys.all, 'content', mode] as const,
};

// Cheap "is there anything new?" signal for the sidebar dot. Captures the
// per-user baseline server-side on first call.
export function useChangelogUnseen() {
  return useQuery({
    queryKey: changelogKeys.unseen(),
    queryFn: changelogApi.unseen,
    staleTime: 5 * 60 * 1000,
    retry: false,
  });
}

// Changelog content for the dialog. Fetching the default/all modes marks the
// releases seen server-side, so this is gated behind `enabled` (only runs when
// the dialog opens).
export function useChangelogContent(params: { all?: boolean; since?: string }, enabled: boolean) {
  const mode = params.since ? `since:${params.since}` : params.all ? 'all' : 'since-last';
  return useQuery({
    queryKey: changelogKeys.content(mode),
    queryFn: () => changelogApi.get(params),
    enabled,
    staleTime: 0,
    retry: false,
  });
}
