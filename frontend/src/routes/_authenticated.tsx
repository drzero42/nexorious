// frontend/src/routes/_authenticated.tsx
import { createFileRoute, Outlet } from '@tanstack/react-router';
import { RouteGuard } from '@/components/route-guard';
import { Sidebar, MobileNav } from '@/components/navigation';

export const Route = createFileRoute('/_authenticated')({
  component: AuthenticatedLayout,
});

function AuthenticatedLayout() {
  return (
    <RouteGuard>
      <div className="flex min-h-screen flex-col md:flex-row">
        <MobileNav />
        <Sidebar />
        <main className="flex-1 p-6 overflow-auto md:ml-64">
          <Outlet />
        </main>
      </div>
    </RouteGuard>
  );
}
