'use client';

import Link from 'next/link';
import { LogOut, User, ChevronDown } from 'lucide-react';
import { Button } from '@/components/ui/button';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu';
import { useAuth } from '@/providers';
import { useNavItems, NavLink, NavSectionCollapsible } from './index';

export function Sidebar() {
  const { user, logout } = useAuth();
  const { mainItems, manageSection, settingsSection, adminSection } = useNavItems();

  return (
    <aside className="hidden md:flex w-64 bg-card border-r flex-col h-screen">
      {/* Logo */}
      <div className="p-4 border-b">
        <Link href="/games" className="block">
          <h1 className="text-xl font-bold">Nexorious</h1>
        </Link>
      </div>

      {/* Navigation */}
      <nav className="flex-1 p-4 overflow-y-auto">
        {/* Main navigation items */}
        <ul className="space-y-1">
          {mainItems.map((item) => (
            <li key={item.href}>
              <NavLink {...item} />
            </li>
          ))}
        </ul>

        {/* Manage section */}
        <div className="mt-6">
          <NavSectionCollapsible {...manageSection} />
        </div>

        {/* Settings section */}
        <div className="mt-4">
          <NavSectionCollapsible {...settingsSection} />
        </div>

        {/* Admin section (admin only) */}
        {user?.isAdmin && (
          <div className="mt-4">
            <NavSectionCollapsible {...adminSection} />
          </div>
        )}
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
            <DropdownMenuItem asChild className="cursor-pointer">
              <Link href="/profile">
                <User className="mr-2 h-4 w-4" />
                <span>Profile</span>
              </Link>
            </DropdownMenuItem>
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
