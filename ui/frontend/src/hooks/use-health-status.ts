import { useQuery } from '@tanstack/react-query';

export interface HealthStatus {
  status: string;
  igdb_status: 'ok' | 'not_configured' | 'invalid_credentials';
  backup_available: boolean;
}

export function useHealthStatus() {
  return useQuery<HealthStatus>({
    queryKey: ['health'],
    queryFn: () => fetch('/health').then((r) => r.json() as Promise<HealthStatus>),
    staleTime: 60_000,
    refetchOnWindowFocus: true,
  });
}
