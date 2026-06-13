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
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Checkbox } from '@/components/ui/checkbox';
import { MultiSelectFilter } from '@/components/ui/multi-select-filter';
import { Plus, Trash2, Loader2 } from 'lucide-react';
import {
  useUpdatePool,
  useAllPlatforms,
  useAllStorefronts,
  useFilterOptions,
  useAllTags,
} from '@/hooks';
import { sanitizeFilter, isValidFilter } from '@/lib/pool-filter';
import { statusLabels } from '@/lib/play-status';
import { PlayStatus } from '@/types';
import type { FilterCard, PoolFilter } from '@/types';

// PlayStatus is an enum in types/game.ts; enumerate its values (multi-select).
const playStatusOpts = Object.values(PlayStatus).map((s) => ({
  value: s,
  label: statusLabels[s],
}));

interface PoolFilterEditorProps {
  poolId: string;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  initialFilter: PoolFilter | null;
}

function emptyCard(): FilterCard {
  return {};
}

interface PoolFilterBodyProps {
  poolId: string;
  initialFilter: PoolFilter | null;
  onOpenChange: (open: boolean) => void;
}

/** Inner form — mounted fresh on each open via `key`, so useState initializers
 *  act as the reset mechanism without needing a setState-in-effect. */
function PoolFilterBody({ poolId, initialFilter, onOpenChange }: PoolFilterBodyProps) {
  const updatePool = useUpdatePool();
  const { data: platforms } = useAllPlatforms();
  const { data: storefronts } = useAllStorefronts();
  const { data: options } = useFilterOptions();
  const { data: tags } = useAllTags();

  const [cards, setCards] = useState<FilterCard[]>(
    initialFilter?.filters.length ? initialFilter.filters : [emptyCard()],
  );

  const updateCard = (idx: number, patch: Partial<FilterCard>) => {
    setCards((prev) => prev.map((c, i) => (i === idx ? { ...c, ...patch } : c)));
  };

  const platformOpts = (platforms ?? []).map((p) => ({
    value: p.name,
    label: p.display_name ?? p.name,
  }));
  const storefrontOpts = (storefronts ?? []).map((s) => ({
    value: s.name,
    label: s.display_name ?? s.name,
  }));
  const genreOpts = (options?.genres ?? []).map((g) => ({ value: g, label: g }));
  const themeOpts = (options?.themes ?? []).map((t) => ({ value: t, label: t }));
  const modeOpts = (options?.gameModes ?? []).map((m) => ({ value: m, label: m }));
  const perspectiveOpts = (options?.playerPerspectives ?? []).map((p) => ({ value: p, label: p }));
  const tagOpts = (tags ?? []).map((t) => ({ value: t.id, label: t.name }));

  const handleSave = async () => {
    const filter = sanitizeFilter({ filters: cards });
    if (!isValidFilter(filter)) {
      toast.error('Add at least one facet to a card before saving.');
      return;
    }
    try {
      await updatePool.mutateAsync({ id: poolId, data: { filter } });
      toast.success('Filter saved');
      onOpenChange(false);
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Failed to save filter');
    }
  };

  const handleClear = async () => {
    try {
      await updatePool.mutateAsync({ id: poolId, data: { filter: null } });
      toast.success('Filter cleared');
      onOpenChange(false);
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Failed to clear filter');
    }
  };

  return (
    <>
      <DialogHeader>
        <DialogTitle>Pool Filter</DialogTitle>
        <DialogDescription>
          A game is suggested if it matches ANY card below. Within a card, all facets must match.
        </DialogDescription>
      </DialogHeader>

      <div className="space-y-4 py-2">
        {cards.map((card, idx) => (
          <div key={idx} className="space-y-3 rounded-md border p-3">
            <div className="flex items-center justify-between">
              <span className="text-sm font-medium">Card {idx + 1}</span>
              <Button
                variant="ghost"
                size="sm"
                onClick={() => setCards((prev) => prev.filter((_, i) => i !== idx))}
                disabled={cards.length === 1}
                aria-label="Remove card"
              >
                <Trash2 className="h-4 w-4 text-destructive" />
              </Button>
            </div>

            <div className="grid grid-cols-2 gap-2">
              <MultiSelectFilter
                label="Genre"
                options={genreOpts}
                selected={card.genre ?? []}
                onChange={(v) => updateCard(idx, { genre: v })}
              />
              <MultiSelectFilter
                label="Theme"
                options={themeOpts}
                selected={card.theme ?? []}
                onChange={(v) => updateCard(idx, { theme: v })}
              />
              <MultiSelectFilter
                label="Platform"
                options={platformOpts}
                selected={card.platform ?? []}
                onChange={(v) => updateCard(idx, { platform: v })}
              />
              <MultiSelectFilter
                label="Storefront"
                options={storefrontOpts}
                selected={card.storefront ?? []}
                onChange={(v) => updateCard(idx, { storefront: v })}
              />
              <MultiSelectFilter
                label="Game Mode"
                options={modeOpts}
                selected={card.game_mode ?? []}
                onChange={(v) => updateCard(idx, { game_mode: v })}
              />
              <MultiSelectFilter
                label="Perspective"
                options={perspectiveOpts}
                selected={card.player_perspective ?? []}
                onChange={(v) => updateCard(idx, { player_perspective: v })}
              />
              <MultiSelectFilter
                label="Tag"
                options={tagOpts}
                selected={card.tag ?? []}
                onChange={(v) => updateCard(idx, { tag: v })}
              />
              <MultiSelectFilter
                label="Play status"
                options={playStatusOpts}
                selected={card.play_status ?? []}
                onChange={(v) => updateCard(idx, { play_status: v })}
              />
            </div>

            <div className="grid grid-cols-2 gap-2">
              <div className="space-y-1">
                <Label className="text-xs">Rating min</Label>
                <Input
                  type="number"
                  value={card.rating_min ?? ''}
                  onChange={(e) =>
                    updateCard(idx, {
                      rating_min: e.target.value === '' ? undefined : Number(e.target.value),
                    })
                  }
                />
              </div>
              <div className="space-y-1">
                <Label className="text-xs">Rating max</Label>
                <Input
                  type="number"
                  value={card.rating_max ?? ''}
                  onChange={(e) =>
                    updateCard(idx, {
                      rating_max: e.target.value === '' ? undefined : Number(e.target.value),
                    })
                  }
                />
              </div>
              <div className="space-y-1">
                <Label className="text-xs">Time to beat min (h)</Label>
                <Input
                  type="number"
                  value={card.time_to_beat_min ?? ''}
                  onChange={(e) =>
                    updateCard(idx, {
                      time_to_beat_min: e.target.value === '' ? undefined : Number(e.target.value),
                    })
                  }
                />
              </div>
              <div className="space-y-1">
                <Label className="text-xs">Time to beat max (h)</Label>
                <Input
                  type="number"
                  value={card.time_to_beat_max ?? ''}
                  onChange={(e) =>
                    updateCard(idx, {
                      time_to_beat_max: e.target.value === '' ? undefined : Number(e.target.value),
                    })
                  }
                />
              </div>
            </div>

            <div className="flex items-center gap-2">
              <Checkbox
                id={`loved-${idx}`}
                checked={card.is_loved === true}
                onCheckedChange={(v) =>
                  updateCard(idx, { is_loved: v === true ? true : undefined })
                }
              />
              <Label htmlFor={`loved-${idx}`}>Loved only</Label>
            </div>
          </div>
        ))}

        <Button
          variant="outline"
          size="sm"
          onClick={() => setCards((prev) => [...prev, emptyCard()])}
        >
          <Plus className="mr-1 h-4 w-4" /> Add card (OR)
        </Button>
      </div>

      <DialogFooter className="flex items-center justify-between">
        <Button variant="ghost" onClick={handleClear} disabled={updatePool.isPending}>
          Clear filter
        </Button>
        <div className="flex gap-2">
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            Cancel
          </Button>
          <Button onClick={handleSave} disabled={updatePool.isPending}>
            {updatePool.isPending && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
            Save
          </Button>
        </div>
      </DialogFooter>
    </>
  );
}

export function PoolFilterEditor({
  poolId,
  open,
  onOpenChange,
  initialFilter,
}: PoolFilterEditorProps) {
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-h-[85vh] max-w-2xl overflow-y-auto">
        {open && (
          <PoolFilterBody
            key={`${poolId}-${String(initialFilter != null)}`}
            poolId={poolId}
            initialFilter={initialFilter}
            onOpenChange={onOpenChange}
          />
        )}
      </DialogContent>
    </Dialog>
  );
}
