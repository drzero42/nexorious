import { useEffect } from 'react';
import { createRootRoute, Outlet } from '@tanstack/react-router';
import { ThemeProvider } from 'next-themes';
import { toast } from 'sonner';
import { Toaster } from '@/components/ui/sonner';
import { QueryProvider, AuthProvider } from '@/providers';
import { DocumentTitle, DocumentTitleProvider } from '@/lib/document-title';
import { takeRestoreOutcome } from '@/lib/server-restart';
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
  // A restore ends with the server restarting and the page loading afresh;
  // show the outcome the restore dialog kept for this load.
  useEffect(() => {
    const outcome = takeRestoreOutcome();
    if (outcome) toast[outcome.kind](outcome.message, { duration: 15_000 });
  }, []);

  return (
    <QueryProvider>
      <ThemeProvider attribute="class" defaultTheme="system" enableSystem>
        <AuthProvider>
          <DocumentTitleProvider>
            <DocumentTitle />
            <Outlet />
            <Toaster />
          </DocumentTitleProvider>
        </AuthProvider>
      </ThemeProvider>
    </QueryProvider>
  );
}
