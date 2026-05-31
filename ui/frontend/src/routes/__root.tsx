import { createRootRoute, HeadContent, Outlet } from '@tanstack/react-router';
import { ThemeProvider } from 'next-themes';
import { Toaster } from '@/components/ui/sonner';
import { QueryProvider, AuthProvider } from '@/providers';
import '@fontsource/geist-sans/400.css';
import '@fontsource/geist-sans/700.css';
import '@fontsource/geist-mono/400.css';
import '@/styles/globals.css';

export const Route = createRootRoute({
  head: () => ({
    meta: [{ title: 'Nexorious' }],
  }),
  component: RootComponent,
});

function RootComponent() {
  return (
    <QueryProvider>
      <ThemeProvider attribute="class" defaultTheme="system" enableSystem>
        <AuthProvider>
          <HeadContent />
          <Outlet />
          <Toaster />
        </AuthProvider>
      </ThemeProvider>
    </QueryProvider>
  );
}
