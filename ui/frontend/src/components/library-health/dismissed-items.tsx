import { useState } from 'react';
import { Button } from '@/components/ui/button';
import { useIgnoredItems, useRestoreSmell } from '@/hooks';
import type { IgnoredItem } from '@/api/library-health';

export interface DismissedItemsProps {
  checkID: string;
}

export function DismissedItems({ checkID }: DismissedItemsProps) {
  const [page, setPage] = useState(1);
  const { data, isLoading, isFetching } = useIgnoredItems(checkID, page, true);
  const restore = useRestoreSmell();

  if (isLoading) return <p className="text-sm text-muted-foreground">Loading dismissed…</p>;

  const items = data?.items ?? [];
  const pages = data?.pages ?? 0;
  const total = data?.total ?? 0;
  if (items.length === 0) {
    return <p className="text-sm text-muted-foreground">No dismissed items.</p>;
  }

  const handleRestore = (userGameId: string) => {
    void restore.mutateAsync({ checkID, userGameIds: [userGameId] }).then(() => {
      // Restoring the last item on a page past the first would leave an empty
      // page that reads as "No dismissed items"; step back instead.
      if (items.length === 1 && page > 1) setPage(page - 1);
    });
  };

  return (
    <div>
      <p className="mb-2 text-sm text-muted-foreground">{total} dismissed</p>
      <ul className="divide-y rounded-md border">
        {items.map((it: IgnoredItem) => (
          <li key={it.user_game_id} className="flex items-center justify-between px-3 py-2">
            <span className="text-sm">{it.title}</span>
            <Button
              size="sm"
              variant="outline"
              disabled={restore.isPending}
              onClick={() => handleRestore(it.user_game_id)}
            >
              Restore
            </Button>
          </li>
        ))}
      </ul>
      {pages > 1 && (
        <div className="mt-2 flex items-center justify-between">
          <Button
            size="sm"
            variant="outline"
            disabled={page <= 1 || isFetching}
            onClick={() => setPage((p) => Math.max(1, p - 1))}
          >
            Previous
          </Button>
          <span className="text-sm text-muted-foreground">
            Page {page} of {pages}
          </span>
          <Button
            size="sm"
            variant="outline"
            disabled={page >= pages || isFetching}
            onClick={() => setPage((p) => p + 1)}
          >
            Next
          </Button>
        </div>
      )}
    </div>
  );
}
