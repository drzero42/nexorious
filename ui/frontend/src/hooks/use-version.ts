import { useQuery } from '@tanstack/react-query';

export interface VersionInfo {
  version: string;
  commit: string;
}

export function useVersion() {
  return useQuery<VersionInfo>({
    queryKey: ['version'],
    queryFn: () => fetch('/api/version').then((r) => r.json() as Promise<VersionInfo>),
    staleTime: 60 * 60 * 1000,
    gcTime: 24 * 60 * 60 * 1000,
  });
}
