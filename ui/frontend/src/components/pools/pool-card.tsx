import { useSortable } from '@dnd-kit/sortable';
import { CSS } from '@dnd-kit/utilities';
import { Button } from '@/components/ui/button';
import { GripVertical, Pencil, Trash2, ListChecks } from 'lucide-react';
import type { PoolListItem } from '@/types';

interface PoolCardProps {
  pool: PoolListItem;
  onOpen: (id: string) => void;
  onEdit: (pool: PoolListItem) => void;
  onDelete: (pool: PoolListItem) => void;
}

export function PoolCard({ pool, onOpen, onEdit, onDelete }: PoolCardProps) {
  const { attributes, listeners, setNodeRef, transform, transition, isDragging } = useSortable({
    id: pool.id,
  });

  return (
    <div
      ref={setNodeRef}
      style={{ transform: CSS.Transform.toString(transform), transition }}
      className={`flex items-center justify-between gap-3 border-b py-3 ${
        isDragging ? 'opacity-50' : ''
      }`}
    >
      <button
        type="button"
        className="cursor-grab touch-none text-muted-foreground"
        aria-label="Drag to reorder"
        {...attributes}
        {...listeners}
      >
        <GripVertical className="h-4 w-4" />
      </button>
      <button
        type="button"
        className="flex flex-1 items-center gap-3 text-left"
        onClick={() => onOpen(pool.id)}
      >
        <span
          className="h-4 w-4 shrink-0 rounded-full border"
          style={{ backgroundColor: pool.color ?? 'transparent' }}
        />
        <span className="min-w-0 flex-1 truncate font-medium">{pool.name}</span>
        <span className="flex items-center gap-1 text-xs text-muted-foreground">
          <ListChecks className="h-3 w-3" />
          {pool.queue_count} queued · {pool.candidate_count} candidates
        </span>
      </button>
      <div className="flex items-center gap-1">
        <Button variant="ghost" size="sm" onClick={() => onEdit(pool)}>
          <Pencil className="h-4 w-4" />
          <span className="sr-only">Edit</span>
        </Button>
        <Button variant="ghost" size="sm" onClick={() => onDelete(pool)}>
          <Trash2 className="h-4 w-4 text-destructive" />
          <span className="sr-only">Delete</span>
        </Button>
      </div>
    </div>
  );
}
