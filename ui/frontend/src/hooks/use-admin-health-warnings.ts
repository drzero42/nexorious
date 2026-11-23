import { useQuery } from '@tanstack/react-query';
import { getAdminHealthWarnings } from '@/api/admin';

export function useAdminHealthWarnings(enabled: boolean) {
  return useQuery({
    queryKey: ['admin-health-warnings'],
    queryFn: getAdminHealthWarnings,
    enabled,
    staleTime: 5 * 60_000,
  });
}
