// frontend/src/app/(main)/layout.tsx
'use client';

import { RouteGuard } from '@/components';
import { Sidebar, MobileNav } from '@/components/navigation';

export default function MainLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  return (
    <RouteGuard>
      <div className="flex min-h-screen flex-col md:flex-row">
        {/* Mobile header */}
        <MobileNav />

        {/* Desktop sidebar */}
        <Sidebar />

        {/* Main content */}
        <main className="flex-1 p-6 overflow-auto">{children}</main>
      </div>
    </RouteGuard>
  );
}
