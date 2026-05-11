import { createFileRoute, Outlet } from '@tanstack/react-router';
import { RouteGuard } from '@/components/route-guard';
import { Sidebar, MobileNav } from '@/components/navigation';
import { useHealthStatus } from '@/hooks/use-health-status';

export const Route = createFileRoute('/_authenticated')({
  component: AuthenticatedLayout,
});

export function AuthenticatedLayout() {
  const { data: health } = useHealthStatus();

  return (
    <RouteGuard>
      <div className="flex min-h-screen flex-col md:flex-row">
        <MobileNav />
        <Sidebar />
        <div className="flex-1 flex flex-col md:ml-64">
          {health?.igdb_configured === false && (
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
          <main className="flex-1 p-6 overflow-auto">
            <Outlet />
          </main>
        </div>
      </div>
    </RouteGuard>
  );
}
