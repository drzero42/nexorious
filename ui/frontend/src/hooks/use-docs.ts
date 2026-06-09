import { useQuery } from '@tanstack/react-query';
import { docsApi } from '@/api/docs';

export const docKeys = {
  all: ['docs'] as const,
  detail: (slug: string) => [...docKeys.all, slug] as const,
};

// Embedded docs are static for the lifetime of a binary, so cache them
// aggressively; a page reload (new deploy) refetches.
export function useDoc(slug: string) {
  return useQuery({
    queryKey: docKeys.detail(slug),
    queryFn: () => docsApi.get(slug),
    staleTime: Infinity,
    retry: false, // 403/404 are deterministic; don't hammer the endpoint
  });
}
