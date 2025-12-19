'use client';

import { useEffect, useState } from 'react';
import { useRouter } from 'next/navigation';
import { useAuth } from '@/providers';
import * as authApi from '@/api/auth';

interface RouteGuardProps {
  children: React.ReactNode;
}

export function RouteGuard({ children }: RouteGuardProps) {
  const router = useRouter();
  const { isAuthenticated, isLoading: authLoading } = useAuth();
  const [isCheckingSetup, setIsCheckingSetup] = useState(true);
  const [needsSetup, setNeedsSetup] = useState(false);

  // Check setup status on mount
  useEffect(() => {
    const checkSetup = async () => {
      try {
        const status = await authApi.checkSetupStatus();
        if (status.needs_setup) {
          setNeedsSetup(true);
          router.replace('/setup');
          return;
        }
      } catch {
        // If setup check fails, continue to auth check
        // (we'll redirect to login if not authenticated)
      } finally {
        setIsCheckingSetup(false);
      }
    };

    checkSetup();
  }, [router]);

  // Handle auth redirect after setup check completes
  useEffect(() => {
    if (!isCheckingSetup && !needsSetup && !authLoading && !isAuthenticated) {
      router.replace('/login');
    }
  }, [isCheckingSetup, needsSetup, authLoading, isAuthenticated, router]);

  // Show loading spinner while checking setup or auth
  if (isCheckingSetup || authLoading) {
    return (
      <div className="flex min-h-screen items-center justify-center">
        <div className="animate-spin h-8 w-8 border-4 border-primary border-t-transparent rounded-full" />
      </div>
    );
  }

  // Don't render anything while redirecting to setup
  if (needsSetup) {
    return null;
  }

  // Don't render anything while redirecting to login
  if (!isAuthenticated) {
    return null;
  }

  return <>{children}</>;
}
