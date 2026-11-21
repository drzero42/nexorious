import { useEffect, useRef } from 'react';
import { useNavigate } from '@tanstack/react-router';
import { useAuth } from '@/providers';

interface RouteGuardProps {
  children: React.ReactNode;
}

export function RouteGuard({ children }: RouteGuardProps) {
  const navigate = useNavigate();
  const { isAuthenticated, isLoading } = useAuth();
  // The live region is rendered empty and "Loading" is written into it after
  // mount: assistive technology announces text inserted into a live region
  // that already exists, and can miss text the region was created with.
  const liveRef = useRef<HTMLSpanElement>(null);
  useEffect(() => {
    if (liveRef.current) liveRef.current.textContent = isLoading ? 'Loading' : '';
  }, [isLoading]);

  useEffect(() => {
    if (!isLoading && !isAuthenticated) {
      navigate({ to: '/login', replace: true });
    }
  }, [isLoading, isAuthenticated, navigate]);

  if (isLoading) {
    return (
      <div className="flex min-h-screen items-center justify-center" role="status">
        <span ref={liveRef} className="sr-only" />
        <div
          aria-hidden="true"
          className="animate-spin h-8 w-8 border-4 border-primary border-t-transparent rounded-full"
        />
      </div>
    );
  }

  if (!isAuthenticated) {
    return null;
  }

  return <>{children}</>;
}
