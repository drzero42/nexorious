import { useCallback } from 'react';
import { createFileRoute, Link, Outlet } from '@tanstack/react-router';
import { useQueryClient } from '@tanstack/react-query';
import { RouteGuard } from '@/components/route-guard';
import { ThemeSync } from '@/components/theme/theme-sync';
import { Sidebar, MobileNav } from '@/components/navigation';
import { useHealthStatus } from '@/hooks/use-health-status';
import { useAdminHealthWarnings } from '@/hooks/use-admin-health-warnings';
import { useAuth } from '@/providers';
import { useJobTypeStatus, useJobCompletionEffect } from '@/hooks';
import { gameKeys } from '@/hooks/use-games';
import { JobType } from '@/types';

export const Route = createFileRoute('/_authenticated')({
  component: AuthenticatedLayout,
});

function useInvalidateGamesOnImportComplete() {
  const queryClient = useQueryClient();
  const { data: importStatus } = useJobTypeStatus(JobType.IMPORT);

  const onImportComplete = useCallback(() => {
    queryClient.invalidateQueries({ queryKey: gameKeys.lists() });
    queryClient.invalidateQueries({ queryKey: gameKeys.stats() });
  }, [queryClient]);
  useJobCompletionEffect(importStatus?.activeJobId, onImportComplete);
}

export function AuthenticatedLayout() {
  useInvalidateGamesOnImportComplete();
  const { data: health } = useHealthStatus();
  const { user } = useAuth();
  const { data: adminWarnings } = useAdminHealthWarnings(!!user?.isAdmin);
  const privilegedDbRole = adminWarnings?.find((w) => w.code === 'db_role_privileged');

  return (
    <RouteGuard>
      <ThemeSync />
      <div className="flex h-screen flex-col md:flex-row">
        <MobileNav />
        <Sidebar />
        <div className="flex-1 flex flex-col md:ml-64 min-h-0 min-w-0">
          {health?.igdb_status === 'not_configured' && (
            <div
              role="alert"
              className="bg-amber-50 border-b border-amber-200 px-6 py-3 text-sm text-amber-800 dark:bg-amber-950 dark:border-amber-800 dark:text-amber-200"
            >
              <strong>IGDB is not configured</strong> — game search and scheduled metadata refresh
              are unavailable. An administrator needs to set{' '}
              <code className="font-mono">IGDB_CLIENT_ID</code> and{' '}
              <code className="font-mono">IGDB_CLIENT_SECRET</code>.
            </div>
          )}
          {health?.igdb_status === 'invalid_credentials' && (
            <div
              role="alert"
              className="bg-amber-50 border-b border-amber-200 px-6 py-3 text-sm text-amber-800 dark:bg-amber-950 dark:border-amber-800 dark:text-amber-200"
            >
              <strong>IGDB credentials are invalid</strong> — game search and scheduled metadata
              refresh are unavailable. An administrator needs to verify{' '}
              <code className="font-mono">IGDB_CLIENT_ID</code> and{' '}
              <code className="font-mono">IGDB_CLIENT_SECRET</code> are correct.
            </div>
          )}
          {privilegedDbRole && (
            <div
              role="alert"
              className="bg-amber-50 border-b border-amber-200 px-6 py-3 text-sm text-amber-800 dark:bg-amber-950 dark:border-amber-800 dark:text-amber-200"
            >
              <strong>The database role is a superuser</strong> — Nexorious connects to PostgreSQL
              as <code className="font-mono">{privilegedDbRole.role}</code>, which can run programs
              on the database host, so restoring a crafted backup file could run commands there.{' '}
              <Link
                to="/help/$slug"
                params={{ slug: 'admin-guide' }}
                hash="database-role"
                className="underline"
              >
                How to fix this
              </Link>
            </div>
          )}
          <main className="flex-1 p-6 overflow-auto min-h-0 min-w-0">
            <Outlet />
          </main>
        </div>
      </div>
    </RouteGuard>
  );
}
