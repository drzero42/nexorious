import { lazy, Suspense } from 'react';
import { createFileRoute } from '@tanstack/react-router';
import { useDoc } from '@/hooks';
import { Skeleton } from '@/components/ui/skeleton';
import { ApiErrorException } from '@/api/client';

// Lazy-load the renderer so the heavy markdown stack (react-markdown + the
// micromark/mdast/hast ecosystem) is code-split into its own chunk, fetched
// only when a help page is opened rather than bundled into the main entry.
const MarkdownDoc = lazy(() =>
  import('@/components/docs/markdown-doc').then((m) => ({ default: m.MarkdownDoc })),
);

export const Route = createFileRoute('/_authenticated/help/$slug')({
  component: HelpDocPage,
});

function DocSkeleton() {
  return (
    <div className="mx-auto max-w-5xl space-y-4 p-6">
      <Skeleton className="h-8 w-64" />
      <Skeleton className="h-4 w-full" />
      <Skeleton className="h-4 w-5/6" />
      <Skeleton className="h-4 w-4/6" />
    </div>
  );
}

function HelpDocPage() {
  const { slug } = Route.useParams();
  const { data, isLoading, error } = useDoc(slug);

  if (isLoading) {
    return <DocSkeleton />;
  }

  if (error) {
    const status = error instanceof ApiErrorException ? error.status : 0;
    const message =
      status === 403
        ? 'You are not authorized to view this guide.'
        : status === 404
          ? 'That guide could not be found.'
          : 'Something went wrong loading this guide.';
    return (
      <div className="mx-auto max-w-5xl p-6">
        <p className="text-muted-foreground">{message}</p>
      </div>
    );
  }

  return (
    <div className="mx-auto max-w-5xl p-6">
      <Suspense fallback={<DocSkeleton />}>
        <MarkdownDoc slug={slug} markdown={data ?? ''} />
      </Suspense>
    </div>
  );
}
