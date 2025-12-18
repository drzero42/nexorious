'use client';

import { useAuth } from '@/providers';
import { RouteGuard } from '@/components';
import { Button } from '@/components/ui/button';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu';
import { cn } from '@/lib/utils';
import Link from 'next/link';
import { usePathname } from 'next/navigation';
import { Library, LogOut, User, ChevronDown, LayoutDashboard, RefreshCw } from 'lucide-react';

interface NavItem {
  href: string;
  label: string;
  icon: React.ReactNode;
}

function Sidebar() {
  const pathname = usePathname();
  const { user, logout } = useAuth();

  const navItems: NavItem[] = [
    { href: '/dashboard', label: 'Dashboard', icon: <LayoutDashboard className="h-4 w-4" /> },
    { href: '/games', label: 'Library', icon: <Library className="h-4 w-4" /> },
    { href: '/sync', label: 'Sync', icon: <RefreshCw className="h-4 w-4" /> },
  ];

  return (
    <aside className="w-64 bg-card border-r flex flex-col h-screen">
      {/* Logo */}
      <div className="p-4 border-b">
        <Link href="/games" className="block">
          <h1 className="text-xl font-bold">Nexorious</h1>
        </Link>
      </div>

      {/* Navigation */}
      <nav className="flex-1 p-4">
        <ul className="space-y-2">
          {navItems.map((item) => (
            <li key={item.href}>
              <Link
                href={item.href}
                className={cn(
                  'flex items-center gap-2 px-3 py-2 rounded-md transition-colors',
                  pathname === item.href ||
                    (pathname.startsWith(item.href) && item.href !== '/')
                    ? 'bg-primary text-primary-foreground'
                    : 'hover:bg-muted'
                )}
              >
                {item.icon}
                <span>{item.label}</span>
              </Link>
            </li>
          ))}
        </ul>
      </nav>

      {/* User menu at bottom */}
      <div className="p-4 border-t">
        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <Button variant="ghost" className="w-full justify-between">
              <span className="flex items-center gap-2">
                <User className="h-4 w-4" />
                <span className="truncate">{user?.username}</span>
              </span>
              <ChevronDown className="h-4 w-4 opacity-50" />
            </Button>
          </DropdownMenuTrigger>
          <DropdownMenuContent align="start" className="w-56">
            <DropdownMenuItem onClick={logout} className="cursor-pointer">
              <LogOut className="mr-2 h-4 w-4" />
              <span>Log out</span>
            </DropdownMenuItem>
          </DropdownMenuContent>
        </DropdownMenu>
      </div>
    </aside>
  );
}

export default function MainLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  return (
    <RouteGuard>
      <div className="flex min-h-screen">
        <Sidebar />
        <main className="flex-1 p-6 overflow-auto">{children}</main>
      </div>
    </RouteGuard>
  );
}
