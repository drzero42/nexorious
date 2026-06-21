import { createRootRoute, Outlet } from '@tanstack/react-router';
import { ThemeProvider } from 'next-themes';
import { Toaster } from '@/components/ui/sonner';
import { QueryProvider, AuthProvider } from '@/providers';
import { DocumentTitle, DocumentTitleProvider } from '@/lib/document-title';
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
