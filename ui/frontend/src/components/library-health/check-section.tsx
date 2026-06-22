import { useState } from 'react';
import { toast } from 'sonner';
import { AccordionContent, AccordionItem, AccordionTrigger } from '@/components/ui/accordion';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from '@/components/ui/alert-dialog';
import { Check } from 'lucide-react';
import type { SmellSummaryItem } from '@/api/library-health';
import { useSmellItems, useApplySmell, useApplyAllSmell, useIgnoreSmell } from '@/hooks';
import { FlaggedItemsTable } from './flagged-items-table';
import { DismissedItems } from './dismissed-items';

export interface CheckSectionProps {
  check: SmellSummaryItem;
  onOpenGame: (userGameId: string) => void;
}

export function CheckSection({ check, onOpenGame }: CheckSectionProps) {
  const [expanded, setExpanded] = useState(false);
  const [confirmAll, setConfirmAll] = useState(false);
  const [showDismissed, setShowDismissed] = useState(false);

  const items = useSmellItems(check.id, expanded);
  const apply = useApplySmell();
  const applyAll = useApplyAllSmell();
  const ignore = useIgnoreSmell();
  const busy = apply.isPending || applyAll.isPending || ignore.isPending;

  // Zero-count checks render as a muted, non-expandable "All clear" row.
  if (check.count === 0) {
    return (
      <div className="flex items-center justify-between rounded-md border border-dashed px-4 py-3 text-muted-foreground">
        <span className="flex items-center gap-2">
          <Check className="h-4 w-4 text-green-600" aria-hidden />
          {check.title}
        </span>
        <span className="text-sm">All clear</span>
      </div>
    );
  }

  const flagged = items.data?.items ?? [];
  const total = items.data?.total ?? check.count;

  const handleApply = (userGameId: string) => {
    void apply
      .mutateAsync({ checkID: check.id, userGameIds: [userGameId] })
      .then((r) => toast.success(`Applied (${r.applied}), skipped ${r.skipped}`))
      .catch(() => toast.error('Apply failed'));
  };

  const handleIgnore = (userGameId: string) => {
    void ignore
      .mutateAsync({ checkID: check.id, userGameIds: [userGameId] })
      .catch(() => toast.error('Ignore failed'));
  };

  const handleApplyAll = () => {
    setConfirmAll(false);
    void applyAll
      .mutateAsync({ checkID: check.id })
      .then((r) => toast.success(`Applied ${r.applied}, skipped ${r.skipped}`))
      .catch(() => toast.error('Apply-to-all failed'));
  };

  return (
    <AccordionItem value={check.id}>
      <AccordionTrigger onClick={() => setExpanded(true)}>
        <span className="flex flex-1 items-center justify-between gap-3 pr-2 text-left">
          <span className="flex items-center gap-2">
            <span className="font-medium">{check.title}</span>
            {check.auto_fixable && <Badge variant="secondary">Auto-fix</Badge>}
          </span>
          <Badge>{check.count}</Badge>
        </span>
      </AccordionTrigger>
      <AccordionContent>
        <p className="mb-3 text-sm text-muted-foreground">{check.description}</p>

        {check.auto_fixable && (
          <div className="mb-3">
            <Button size="sm" variant="outline" disabled={busy} onClick={() => setConfirmAll(true)}>
              Apply to all ({check.count})
            </Button>
          </div>
        )}

        {items.isLoading ? (
          <p className="text-sm text-muted-foreground">Loading…</p>
        ) : (
          <>
            <FlaggedItemsTable
              items={flagged}
              autoFixable={check.auto_fixable}
              busy={busy}
              onApply={handleApply}
              onIgnore={handleIgnore}
              onOpenGame={onOpenGame}
            />
            {total > flagged.length && (
              <p className="mt-2 text-xs text-muted-foreground">
                Showing first {flagged.length} of {total}.
              </p>
            )}
          </>
        )}

        <div className="mt-3">
          <Button size="sm" variant="ghost" onClick={() => setShowDismissed((v) => !v)}>
            {showDismissed ? 'Hide dismissed' : 'Show dismissed'}
          </Button>
          {showDismissed && (
            <div className="mt-2">
              <DismissedItems checkID={check.id} />
            </div>
          )}
        </div>
      </AccordionContent>

      <AlertDialog open={confirmAll} onOpenChange={setConfirmAll}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Apply to all flagged games?</AlertDialogTitle>
            <AlertDialogDescription>
              This will apply the suggested fix to all {check.count} games flagged by &quot;
              {check.title}&quot;.
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>Cancel</AlertDialogCancel>
            <AlertDialogAction
              className="bg-destructive text-destructive-foreground hover:bg-destructive/90"
              onClick={handleApplyAll}
            >
              Apply
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </AccordionItem>
  );
}
