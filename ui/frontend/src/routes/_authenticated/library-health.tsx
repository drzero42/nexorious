import { useEffect, useState } from 'react';
import { createFileRoute, useNavigate } from '@tanstack/react-router';
import { useQueryClient } from '@tanstack/react-query';
import { Stethoscope, RefreshCw } from 'lucide-react';
import { Accordion } from '@/components/ui/accordion';
import { Button } from '@/components/ui/button';
import { Card, CardContent } from '@/components/ui/card';
import { Skeleton } from '@/components/ui/skeleton';
import { useSmellSummary, smellKeys } from '@/hooks';
import { setGameReturn } from '@/lib/game-return';
import type { SmellSummaryItem, SmellTier } from '@/api/library-health';
import { CheckSection } from '@/components/library-health/check-section';
import { loadExpandedChecks, saveExpandedChecks } from '@/lib/library-health-prefs';

export const Route = createFileRoute('/_authenticated/library-health')({
  head: () => ({ meta: [{ title: 'Library Health | Nexorious' }] }),
  component: LibraryHealthPage,
});

const TIER_ORDER: { tier: SmellTier; label: string; blurb: string }[] = [
  {
    tier: 'inconsistency',
    label: 'Inconsistencies',
    blurb: 'Something looks wrong and probably needs fixing.',
  },
  { tier: 'nudge', label: 'Suggestions', blurb: 'You might want to update these.' },
];

function TierBlock({
  label,
  blurb,
  checks,
  expanded,
  onExpandedChange,
  onView,
  onEdit,
}: {
  label: string;
  blurb: string;
  checks: SmellSummaryItem[];
  // Global set of expanded check IDs (shared across tiers) and a setter for it.
  expanded: string[];
  onExpandedChange: (next: string[]) => void;
  onView: (id: string) => void;
  onEdit: (id: string) => void;
}) {
  if (checks.length === 0) return null;
  // Check IDs are globally unique, so this tier owns exactly its own IDs; merge
  // its accordion selection back into the global set without touching others'.
  const tierIds = checks.map((c) => c.id);
  const value = expanded.filter((id) => tierIds.includes(id));
  const handleValueChange = (vals: string[]) => {
    onExpandedChange([...expanded.filter((id) => !tierIds.includes(id)), ...vals]);
  };
  return (
    <section className="space-y-2">
      <div>
        <h2 className="text-lg font-semibold">{label}</h2>
        <p className="text-sm text-muted-foreground">{blurb}</p>
      </div>
      <Accordion
        type="multiple"
        className="space-y-2"
        value={value}
        onValueChange={handleValueChange}
      >
        {checks.map((check) => (
          <CheckSection
            key={check.id}
            check={check}
            expanded={value.includes(check.id)}
            onView={onView}
            onEdit={onEdit}
          />
        ))}
      </Accordion>
    </section>
  );
}

export function LibraryHealthPage() {
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const { data, isLoading, isFetching, isError, error } = useSmellSummary();

  // Which checks have their flagged-item lists expanded. Seeded from (and
  // mirrored back to) sessionStorage so the expansion survives navigating away
  // and back — e.g. editing a flagged game — within the browser session.
  const [expanded, setExpanded] = useState<string[]>(loadExpandedChecks);
  const handleExpandedChange = (next: string[]) => {
    setExpanded(next);
    saveExpandedChecks(next);
  };

  // Re-run every check (summary + per-check listings + dismissed) by invalidating
  // the whole smells tree. Used by the Refresh button and the error retry.
  const handleRefresh = () => {
    void queryClient.invalidateQueries({ queryKey: smellKeys.all });
  };

  // Smells are an on-demand scan: re-run them whenever the page is opened so a
  // fix made elsewhere (e.g. editing a game, which invalidates the games cache
  // but not this one) is reflected on return.
  useEffect(() => {
    void queryClient.invalidateQueries({ queryKey: smellKeys.all });
  }, [queryClient]);

  // Title opens the game's details page; the Edit action opens its edit form.
  // Both record Library Health as the referrer so the game's back button (and
  // edit → detail → back) returns here rather than to the games library.
  const onView = (userGameId: string) => {
    setGameReturn({ to: '/library-health', label: 'Library Health' });
    void navigate({ to: '/games/$id', params: { id: userGameId } });
  };
  const onEdit = (userGameId: string) => {
    setGameReturn({ to: '/library-health', label: 'Library Health' });
    void navigate({ to: '/games/$id/edit', params: { id: userGameId } });
  };

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="flex items-center gap-2 text-2xl font-bold">
            <Stethoscope className="h-6 w-6" />
            Library Health
          </h1>
          <p className="text-muted-foreground">Data-quality checks across your collection.</p>
        </div>
        <Button variant="outline" onClick={handleRefresh} disabled={isFetching}>
          <RefreshCw className={`mr-2 h-4 w-4 ${isFetching ? 'animate-spin' : ''}`} />
          Refresh
        </Button>
      </div>

      {isLoading && (
        <div className="space-y-2">
          <Skeleton className="h-6 w-40" />
          <Skeleton className="h-16 w-full" />
          <Skeleton className="h-16 w-full" />
        </div>
      )}

      {isError && (
        <Card>
          <CardContent className="flex flex-col items-center gap-3 py-10 text-center">
            <p className="font-semibold">Failed to load library health</p>
            <p className="text-sm text-muted-foreground">{error?.message}</p>
            <Button onClick={handleRefresh}>Try again</Button>
          </CardContent>
        </Card>
      )}

      {data && (
        <>
          {data.every((c) => c.count === 0) && (
            <Card>
              <CardContent className="py-10 text-center">
                <div className="mb-2 text-4xl">🎉</div>
                <p className="font-semibold">Your library is in great shape</p>
                <p className="text-sm text-muted-foreground">No issues found across all checks.</p>
              </CardContent>
            </Card>
          )}
          {TIER_ORDER.map(({ tier, label, blurb }) => (
            <TierBlock
              key={tier}
              label={label}
              blurb={blurb}
              checks={data.filter((c) => c.tier === tier)}
              expanded={expanded}
              onExpandedChange={handleExpandedChange}
              onView={onView}
              onEdit={onEdit}
            />
          ))}
        </>
      )}
    </div>
  );
}
