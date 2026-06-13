import { useState } from 'react';
import { toast } from 'sonner';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import { Button } from '@/components/ui/button';
import { Checkbox } from '@/components/ui/checkbox';
import { Input } from '@/components/ui/input';
import { Plus } from 'lucide-react';
import {
  usePools,
  useGamePoolMemberships,
  useAddPoolGame,
  useRemovePoolGame,
  useCreatePool,
} from '@/hooks';
import type { PoolListItem, PoolMembership } from '@/types';

export interface MembershipRow {
  pool: PoolListItem;
  member: boolean;
}

/**
 * Merge the full pool list with this game's memberships into checkbox rows.
 * `memberships` undefined (e.g. #971 read failed) degrades to all-unchecked.
 */
// eslint-disable-next-line react-refresh/only-export-components -- co-located pure helper, unit-tested
export function mergeMembership(
  pools: PoolListItem[],
  memberships: PoolMembership[] | undefined,
): MembershipRow[] {
  const memberIds = new Set((memberships ?? []).map((m) => m.pool_id));
  return pools.map((pool) => ({ pool, member: memberIds.has(pool.id) }));
}

interface AddToPoolDialogProps {
  userGameId: string;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}

export function AddToPoolDialog({ userGameId, open, onOpenChange }: AddToPoolDialogProps) {
  const { data: pools } = usePools();
  const { data: memberships } = useGamePoolMemberships(open ? userGameId : undefined);
  const addGame = useAddPoolGame();
  const removeGame = useRemovePoolGame();
  const createPool = useCreatePool();
  const [newName, setNewName] = useState('');

  const rows = mergeMembership(pools ?? [], memberships);

  const toggle = (poolId: string, nextMember: boolean) => {
    const mutation = nextMember ? addGame : removeGame;
    mutation.mutate(
      { poolId, userGameId },
      { onError: () => toast.error('Failed to update pool membership') },
    );
  };

  const handleCreate = async () => {
    const name = newName.trim();
    if (!name) return;
    try {
      const pool = await createPool.mutateAsync({ name });
      await addGame.mutateAsync({ poolId: pool.id, userGameId });
      toast.success(`Added to ${name}`);
      setNewName('');
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Failed to create pool');
    }
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Add to Pool</DialogTitle>
          <DialogDescription>Toggle which pools this game belongs to.</DialogDescription>
        </DialogHeader>
        <div className="max-h-64 space-y-2 overflow-y-auto py-2">
          {rows.length === 0 ? (
            <p className="text-sm text-muted-foreground">No pools yet — create one below.</p>
          ) : (
            rows.map(({ pool, member }) => (
              <label key={pool.id} className="flex cursor-pointer items-center gap-3 py-1">
                <Checkbox checked={member} onCheckedChange={(v) => toggle(pool.id, v === true)} />
                {pool.color && (
                  <span
                    className="h-3 w-3 rounded-full border"
                    style={{ backgroundColor: pool.color }}
                  />
                )}
                <span className="flex-1">{pool.name}</span>
              </label>
            ))
          )}
        </div>
        <div className="flex items-center gap-2 border-t pt-3">
          <Input
            value={newName}
            onChange={(e) => setNewName(e.target.value)}
            placeholder="New pool name..."
            maxLength={100}
          />
          <Button variant="outline" size="sm" onClick={handleCreate} disabled={!newName.trim()}>
            <Plus className="mr-1 h-4 w-4" /> Create
          </Button>
        </div>
        <DialogFooter>
          <Button onClick={() => onOpenChange(false)}>Done</Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
