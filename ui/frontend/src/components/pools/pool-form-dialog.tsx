import { useState } from 'react';
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
import { ColorPicker, COLOR_PALETTE } from '@/components/ui/color-picker';
import { Loader2 } from 'lucide-react';
import type { PoolListItem } from '@/types';

export interface PoolFormValues {
  name: string;
  color: string | null;
}

interface PoolFormDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  /** When set, the dialog is in edit mode and prefills from this row. */
  editing?: PoolListItem | null;
  onSubmit: (values: PoolFormValues) => Promise<void>;
  pending: boolean;
}

interface PoolFormBodyProps {
  editing?: PoolListItem | null;
  onOpenChange: (open: boolean) => void;
  onSubmit: (values: PoolFormValues) => Promise<void>;
  pending: boolean;
}

/** Inner form — mounted fresh on each open via `key`, so useState initializers
 *  act as the reset mechanism without needing a setState-in-effect. */
function PoolFormBody({ editing, onOpenChange, onSubmit, pending }: PoolFormBodyProps) {
  const [name, setName] = useState(editing?.name ?? '');
  const [useColor, setUseColor] = useState(editing?.color != null);
  const [color, setColor] = useState<string>(editing?.color ?? COLOR_PALETTE[0]);

  const handleSubmit = async () => {
    await onSubmit({ name: name.trim(), color: useColor ? color : null });
  };

  return (
    <>
      <DialogHeader>
        <DialogTitle>{editing ? 'Edit Pool' : 'Create Pool'}</DialogTitle>
        <DialogDescription>
          Pools group games you plan to play. Add a filter later to get suggestions.
        </DialogDescription>
      </DialogHeader>
      <div className="space-y-4 py-4">
        <div className="space-y-2">
          <Label htmlFor="pool-name">Name *</Label>
          <Input
            id="pool-name"
            value={name}
            onChange={(e) => setName(e.target.value)}
            placeholder="Enter pool name..."
            maxLength={100}
          />
        </div>
        <div className="space-y-2">
          <div className="flex items-center gap-2">
            <Checkbox
              id="pool-use-color"
              checked={useColor}
              onCheckedChange={(v) => setUseColor(v === true)}
            />
            <Label htmlFor="pool-use-color">Use a color</Label>
          </div>
          {useColor && <ColorPicker value={color} onChange={setColor} />}
        </div>
      </div>
      <DialogFooter>
        <Button variant="outline" onClick={() => onOpenChange(false)}>
          Cancel
        </Button>
        <Button onClick={handleSubmit} disabled={pending || !name.trim()}>
          {pending && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
          {editing ? 'Save' : 'Create'}
        </Button>
      </DialogFooter>
    </>
  );
}

export function PoolFormDialog({
  open,
  onOpenChange,
  editing,
  onSubmit,
  pending,
}: PoolFormDialogProps) {
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        {open && (
          <PoolFormBody
            key={editing?.id ?? 'new'}
            editing={editing}
            onOpenChange={onOpenChange}
            onSubmit={onSubmit}
            pending={pending}
          />
        )}
      </DialogContent>
    </Dialog>
  );
}
