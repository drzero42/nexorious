// frontend/src/routes/__root.tsx
import { createRootRoute, Outlet } from '@tanstack/react-router';
import { ThemeProvider } from 'next-themes';
import { Toaster } from '@/components/ui/sonner';
import { QueryProvider } from '@/providers';
import '@fontsource/geist-sans/400.css';
import '@fontsource/geist-sans/700.css';
import '@fontsource/geist-mono/400.css';
import '@/app/globals.css';

export const Route = createRootRoute({
  component: RootComponent,
});

function RootComponent() {
  return (
    <QueryProvider>
      <ThemeProvider attribute="class" defaultTheme="system" enableSystem>
        <Outlet />
        <Toaster />
      </ThemeProvider>
    </QueryProvider>
  );
}
