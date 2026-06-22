import { lazy, Suspense, useState } from 'react';
import { useQueryClient } from '@tanstack/react-query';
import { Sparkles } from 'lucide-react';

import { Dialog, DialogContent, DialogHeader, DialogTitle } from '@/components/ui/dialog';
import { Button } from '@/components/ui/button';
import { Skeleton } from '@/components/ui/skeleton';
import { changelogKeys, useChangelogContent, useChangelogUnseen } from '@/hooks';

const MarkdownDoc = lazy(() =>
  import('@/components/docs/markdown-doc').then((m) => ({ default: m.MarkdownDoc })),
);

export function WhatsNew() {
  const queryClient = useQueryClient();
  const { data: unseen } = useChangelogUnseen();
  const [open, setOpen] = useState(false);
  const [showAll, setShowAll] = useState(false);

  // Content fetch is gated on the dialog being open; fetching marks releases
  // seen server-side, so we clear the dot after a successful open.
  const { data, isLoading } = useChangelogContent({ all: showAll }, open);

  const hasUnseen = unseen?.has_unseen === true;

  function openDialog() {
    setShowAll(false);
    setOpen(true);
    // After the since-last fetch marks things seen, refresh the dot.
    void queryClient.invalidateQueries({ queryKey: changelogKeys.unseen() });
  }

  return (
    <>
      <button
        type="button"
        onClick={openDialog}
        className="inline-flex items-center gap-1 underline hover:text-foreground"
      >
        What&apos;s new
        {hasUnseen && (
          <span
            aria-label="new changelog entries"
            className="ml-0.5 inline-block h-2 w-2 rounded-full bg-primary"
          />
        )}
      </button>

      <Dialog open={open} onOpenChange={setOpen}>
        <DialogContent className="max-h-[80vh] overflow-y-auto sm:max-w-2xl">
          <DialogHeader>
            <DialogTitle className="flex items-center gap-2">
              <Sparkles className="h-4 w-4" />
              {showAll ? 'Full changelog' : "What's new"}
            </DialogTitle>
          </DialogHeader>

          {isLoading ? (
            <div className="space-y-3">
              <Skeleton className="h-5 w-48" />
              <Skeleton className="h-4 w-full" />
              <Skeleton className="h-4 w-5/6" />
            </div>
          ) : !data?.available ? (
            <p className="text-sm text-muted-foreground">
              The changelog is unavailable for this build.
            </p>
          ) : data.entries.length === 0 ? (
            <p className="text-sm text-muted-foreground">
              You&apos;re up to date — nothing new since you last looked.
            </p>
          ) : (
            <Suspense fallback={<Skeleton className="h-24 w-full" />}>
              <MarkdownDoc slug="changelog" markdown={data.markdown} />
            </Suspense>
          )}

          {!showAll && !isLoading && data?.available && (
            <div className="pt-2">
              <Button variant="ghost" size="sm" onClick={() => setShowAll(true)}>
                View full changelog
              </Button>
            </div>
          )}
        </DialogContent>
      </Dialog>
    </>
  );
}
