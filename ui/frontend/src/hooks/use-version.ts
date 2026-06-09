import { useQuery } from '@tanstack/react-query';

export interface VersionInfo {
  version: string;
  commit: string;
  update_check_enabled: boolean;
  update_available: boolean;
  latest_version: string;
  release_url: string;
}

export function useVersion() {
  return useQuery<VersionInfo>({
    queryKey: ['version'],
    queryFn: () => fetch('/api/version').then((r) => r.json() as Promise<VersionInfo>),
    staleTime: 0,
    gcTime: 5 * 60 * 1000,
  });
}
