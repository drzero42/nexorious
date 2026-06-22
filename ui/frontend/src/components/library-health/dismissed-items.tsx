import { Button } from '@/components/ui/button';
import { useIgnoredItems, useRestoreSmell } from '@/hooks';
import type { IgnoredItem } from '@/api/library-health';

export interface DismissedItemsProps {
  checkID: string;
}

export function DismissedItems({ checkID }: DismissedItemsProps) {
  const { data, isLoading } = useIgnoredItems(checkID, true);
  const restore = useRestoreSmell();

  if (isLoading) return <p className="text-sm text-muted-foreground">Loading dismissed…</p>;

  const items = data?.items ?? [];
  if (items.length === 0) {
    return <p className="text-sm text-muted-foreground">No dismissed items.</p>;
  }

  return (
    <ul className="divide-y rounded-md border">
      {items.map((it: IgnoredItem) => (
        <li key={it.user_game_id} className="flex items-center justify-between px-3 py-2">
          <span className="text-sm">{it.title}</span>
          <Button
            size="sm"
            variant="outline"
            disabled={restore.isPending}
            onClick={() => {
              void restore.mutateAsync({ checkID, userGameIds: [it.user_game_id] });
            }}
          >
            Restore
          </Button>
        </li>
      ))}
    </ul>
  );
}
