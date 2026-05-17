import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import { IGDBSearch } from '@/components/games/igdb-search';
import type { IGDBGameCandidate } from '@/types';

interface IGDBMatchDialogProps {
  open: boolean;
  title?: string;
  initialQuery?: string;
  onClose: () => void;
  onSelect: (candidate: IGDBGameCandidate) => void;
}

export function IGDBMatchDialog({
  open,
  title = 'Find IGDB Match',
  initialQuery,
  onClose,
  onSelect,
}: IGDBMatchDialogProps) {
  return (
    <Dialog open={open} onOpenChange={(o) => { if (!o) onClose(); }}>
      <DialogContent className="sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>{title}</DialogTitle>
        </DialogHeader>
        <IGDBSearch onSelect={onSelect} autoFocus initialQuery={initialQuery} />
      </DialogContent>
    </Dialog>
  );
}
