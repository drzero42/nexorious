import { useState, useEffect } from 'react';
import * as authApi from '@/api/auth';

interface UseSetupStatusResult {
  needsSetup: boolean | null;
  isLoading: boolean;
  error: string | null;
}

export function useSetupStatus(): UseSetupStatusResult {
  const [needsSetup, setNeedsSetup] = useState<boolean | null>(null);
  const [isLoading, setIsLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    const checkSetup = async () => {
      try {
        const status = await authApi.checkSetupStatus();
        setNeedsSetup(status.needs_setup);
      } catch (err) {
        setError(err instanceof Error ? err.message : 'Failed to check setup status');
      } finally {
        setIsLoading(false);
      }
    };

    checkSetup();
  }, []);

  return { needsSetup, isLoading, error };
}
