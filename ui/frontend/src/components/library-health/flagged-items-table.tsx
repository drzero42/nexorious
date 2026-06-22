import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table';
import { Button } from '@/components/ui/button';
import type { FlaggedItem } from '@/api/library-health';

export interface FlaggedItemsTableProps {
  items: FlaggedItem[];
  autoFixable: boolean;
  busy?: boolean;
  onApply: (userGameId: string) => void;
  onIgnore: (userGameId: string) => void;
  onView: (userGameId: string) => void;
  onEdit: (userGameId: string) => void;
}

export function FlaggedItemsTable({
  items,
  autoFixable,
  busy = false,
  onApply,
  onIgnore,
  onView,
  onEdit,
}: FlaggedItemsTableProps) {
  return (
    <Table>
      <TableHeader>
        <TableRow>
          <TableHead>Game</TableHead>
          <TableHead className="text-right">Actions</TableHead>
        </TableRow>
      </TableHeader>
      <TableBody>
        {items.map((item) => (
          <TableRow key={item.user_game_id}>
            <TableCell>
              <button
                type="button"
                className="text-left font-medium text-primary hover:underline"
                onClick={() => onView(item.user_game_id)}
              >
                {item.title}
              </button>
              {/* Only impossible-acquired-date sets a detail; show it under the title. */}
              {item.detail ? (
                <div className="text-sm text-muted-foreground">{item.detail}</div>
              ) : null}
            </TableCell>
            <TableCell className="space-x-2 text-right">
              {autoFixable ? (
                <Button size="sm" disabled={busy} onClick={() => onApply(item.user_game_id)}>
                  Apply
                </Button>
              ) : (
                <Button
                  size="sm"
                  variant="outline"
                  disabled={busy}
                  onClick={() => onEdit(item.user_game_id)}
                >
                  Edit
                </Button>
              )}
              <Button
                size="sm"
                variant="ghost"
                disabled={busy}
                onClick={() => onIgnore(item.user_game_id)}
              >
                Ignore
              </Button>
            </TableCell>
          </TableRow>
        ))}
      </TableBody>
    </Table>
  );
}
