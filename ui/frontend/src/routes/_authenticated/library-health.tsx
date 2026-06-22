import { createFileRoute, useNavigate } from '@tanstack/react-router';
import { Stethoscope, RefreshCw } from 'lucide-react';
import { Accordion } from '@/components/ui/accordion';
import { Button } from '@/components/ui/button';
import { Card, CardContent } from '@/components/ui/card';
import { Skeleton } from '@/components/ui/skeleton';
import { useSmellSummary } from '@/hooks';
import type { SmellSummaryItem, SmellTier } from '@/api/library-health';
import { CheckSection } from '@/components/library-health/check-section';

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
  { tier: 'nudge', label: 'Nudges', blurb: 'You might want to update these.' },
];

function TierBlock({
  label,
  blurb,
  checks,
  onOpenGame,
}: {
  label: string;
  blurb: string;
  checks: SmellSummaryItem[];
  onOpenGame: (id: string) => void;
}) {
  if (checks.length === 0) return null;
  return (
    <section className="space-y-2">
      <div>
        <h2 className="text-lg font-semibold">{label}</h2>
        <p className="text-sm text-muted-foreground">{blurb}</p>
      </div>
      <Accordion type="multiple" className="space-y-2">
        {checks.map((check) => (
          <CheckSection key={check.id} check={check} onOpenGame={onOpenGame} />
        ))}
      </Accordion>
    </section>
  );
}

function LibraryHealthPage() {
  const navigate = useNavigate();
  const { data, isLoading, isError, error, refetch } = useSmellSummary();

  const onOpenGame = (userGameId: string) => {
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
        <Button variant="outline" onClick={() => void refetch()} disabled={isLoading}>
          <RefreshCw className="mr-2 h-4 w-4" />
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
            <Button onClick={() => void refetch()}>Try again</Button>
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
              onOpenGame={onOpenGame}
            />
          ))}
        </>
      )}
    </div>
  );
}
