import { Dialog, DialogContent, DialogHeader, DialogTitle } from '@/components/ui/dialog';
import { IGDBSearch } from '@/components/games/igdb-search';
import { EditionChoice } from './edition-choice';
import type { ExternalGameEdition, IGDBGameCandidate } from '@/types';

interface IGDBMatchDialogProps {
  open: boolean;
  title?: string;
  initialQuery?: string;
  onClose: () => void;
  onSelect: (candidate: IGDBGameCandidate) => void;
  /**
   * When set, scopes the IGDB search to platforms IGDB tags for the given
   * external_game. Used by the sync pending-review manual-match flow
   * (issue #615).
   */
  externalGameId?: string;
  /**
   * The editions of the sync entry being matched. With more than one, the
   * dialog offers to rematch all of them (the default) or one alone (#1452).
   * Absent for import review.
   */
  editions?: ExternalGameEdition[];
  /** The chosen edition's id, or null for all editions. */
  editionId?: string | null;
  onEditionChange?: (editionId: string | null) => void;
}

export function IGDBMatchDialog({
  open,
  title = 'Find IGDB Match',
  initialQuery,
  onClose,
  onSelect,
  externalGameId,
  editions,
  editionId = null,
  onEditionChange,
}: IGDBMatchDialogProps) {
  return (
    <Dialog
      open={open}
      onOpenChange={(o) => {
        if (!o) onClose();
      }}
    >
      <DialogContent className="sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>{title}</DialogTitle>
        </DialogHeader>
        {editions && editions.length > 1 && onEditionChange && (
          <EditionChoice
            name="rematch-edition"
            editions={editions}
            value={editionId}
            onChange={onEditionChange}
          />
        )}
        <IGDBSearch
          onSelect={onSelect}
          autoFocus
          initialQuery={initialQuery}
          externalGameId={externalGameId}
        />
      </DialogContent>
    </Dialog>
  );
}
