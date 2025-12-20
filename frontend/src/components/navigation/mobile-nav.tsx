// frontend/src/components/navigation/mobile-nav.tsx
'use client';

import { useState } from 'react';
import Link from 'next/link';
import { Menu, LogOut } from 'lucide-react';
import { Button } from '@/components/ui/button';
import {
  Sheet,
  SheetContent,
  SheetHeader,
  SheetTitle,
  SheetTrigger,
} from '@/components/ui/sheet';
import { Avatar, AvatarFallback } from '@/components/ui/avatar';
import { useAuth } from '@/providers';
import { useNavItems, NavLink, NavSectionCollapsible } from './index';

export function MobileNav() {
  const [open, setOpen] = useState(false);
  const { user, logout } = useAuth();
  const { mainItems, manageSection, settingsSection, adminSection } = useNavItems();

  const handleNavigate = () => {
    setOpen(false);
  };

  const handleLogout = () => {
    setOpen(false);
    logout();
  };

  return (
    <div className="flex md:hidden items-center justify-between p-4 border-b bg-card">
      {/* Hamburger and Logo */}
      <div className="flex items-center gap-3">
        <Sheet open={open} onOpenChange={setOpen}>
          <SheetTrigger asChild>
            <Button variant="ghost" size="icon">
              <Menu className="h-5 w-5" />
              <span className="sr-only">Toggle menu</span>
            </Button>
          </SheetTrigger>
          <SheetContent side="left" className="w-72 p-0">
            <SheetHeader className="p-4 border-b">
              <SheetTitle>
                <Link href="/games" onClick={handleNavigate}>
                  Nexorious
                </Link>
              </SheetTitle>
            </SheetHeader>

            <nav className="flex-1 p-4 overflow-y-auto">
              {/* Main navigation items */}
              <ul className="space-y-1">
                {mainItems.map((item) => (
                  <li key={item.href}>
                    <NavLink {...item} onNavigate={handleNavigate} />
                  </li>
                ))}
              </ul>

              {/* Manage section */}
              <div className="mt-6">
                <NavSectionCollapsible
                  {...manageSection}
                  onNavigate={handleNavigate}
                />
              </div>

              {/* Settings section */}
              <div className="mt-4">
                <NavSectionCollapsible
                  {...settingsSection}
                  onNavigate={handleNavigate}
                />
              </div>

              {/* Admin section (admin only) */}
              {user?.isAdmin && (
                <div className="mt-4">
                  <NavSectionCollapsible
                    {...adminSection}
                    onNavigate={handleNavigate}
                  />
                </div>
              )}

              {/* Account section */}
              <div className="mt-6 pt-4 border-t">
                <p className="px-3 mb-2 text-xs font-semibold uppercase text-muted-foreground">
                  Account
                </p>
                <ul className="space-y-1">
                  <li>
                    <Link
                      href="/profile"
                      onClick={handleNavigate}
                      className="flex items-center gap-2 px-3 py-2 rounded-md hover:bg-muted transition-colors"
                    >
                      <Avatar className="h-6 w-6">
                        <AvatarFallback className="text-xs">
                          {user?.username?.charAt(0).toUpperCase()}
                        </AvatarFallback>
                      </Avatar>
                      <span>{user?.username}</span>
                    </Link>
                  </li>
                  <li>
                    <button
                      onClick={handleLogout}
                      className="flex w-full items-center gap-2 px-3 py-2 rounded-md hover:bg-muted transition-colors text-left"
                    >
                      <LogOut className="h-4 w-4" />
                      <span>Sign out</span>
                    </button>
                  </li>
                </ul>
              </div>
            </nav>
          </SheetContent>
        </Sheet>

        <Link href="/games" className="font-bold text-lg">
          Nexorious
        </Link>
      </div>

      {/* Avatar on right */}
      <Link href="/profile">
        <Avatar className="h-8 w-8">
          <AvatarFallback>
            {user?.username?.charAt(0).toUpperCase()}
          </AvatarFallback>
        </Avatar>
      </Link>
    </div>
  );
}
