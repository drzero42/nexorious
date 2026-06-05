import { useState, useEffect, useRef } from 'react';
import { toast } from 'sonner';
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from '@/components/ui/collapsible';
import { Button } from '@/components/ui/button';
import { Badge } from '@/components/ui/badge';
import { Skeleton } from '@/components/ui/skeleton';
import {
  ChevronDown,
  ChevronRight,
  AlertCircle,
  CheckCircle,
  Clock,
  Loader2,
  RotateCcw,
  Search,
  Ban,
} from 'lucide-react';
import { Link } from '@tanstack/react-router';
import {
  useJobItems,
  useRetryFailedItems,
  useRetryJobItem,
  useResolveJobItem,
  useSkipJobItem,
} from '@/hooks';
import { IGDBMatchDialog } from '@/components/sync/igdb-match-dialog';
import { JobItemStatus, getJobItemStatusLabel, getJobItemStatusVariant } from '@/types';

interface JobItemsDetailsProps {
  jobId: string;
  progress: {
    pending: number;
    processing: number;
    completed: number;
    pendingReview: number;
    skipped: number;
    failed: number;
  };
  isTerminal: boolean;
}

interface StatusSectionProps {
  jobId: string;
  status: JobItemStatus;
  count: number;
  defaultOpen?: boolean;
  isTerminal: boolean;
}

function StatusSection({
  jobId,
  status,
  count,
  defaultOpen = false,
  isTerminal,
}: StatusSectionProps) {
  const [isOpen, setIsOpen] = useState(defaultOpen);
  const [page, setPage] = useState(1);
  const prevCountRef = useRef(count);
  const { data, isLoading, refetch } = useJobItems(jobId, status, page, 20, {
    enabled: isOpen && count > 0,
  });

  // Refetch items when count changes (triggered by parent job polling)
  // This avoids duplicate polling - parent polls job, we react to count changes
  useEffect(() => {
    if (isOpen && count !== prevCountRef.current) {
      refetch();
    }
    prevCountRef.current = count;
  }, [count, isOpen, refetch]);

  // Retry mutations
  const retryAllMutation = useRetryFailedItems();
  const retryItemMutation = useRetryJobItem();

  // Review actions (pending_review import items)
  const resolveMutation = useResolveJobItem();
  const skipMutation = useSkipJobItem();
  const [matchItem, setMatchItem] = useState<{ id: string; title: string } | null>(null);

  // Determine section behavior
  const isFailedSection = status === JobItemStatus.FAILED;
  const isReviewSection = status === JobItemStatus.PENDING_REVIEW;
  const canRetryAll = isFailedSection && isTerminal;
  const canRetryItem = canRetryAll;

  const handleSkip = async (itemId: string) => {
    try {
      await skipMutation.mutateAsync(itemId);
      toast.success('Item skipped');
      refetch();
    } catch (error) {
      toast.error(error instanceof Error ? error.message : 'Failed to skip item');
    }
  };

  const handleSelectMatch = async (itemId: string, igdbId: number) => {
    try {
      await resolveMutation.mutateAsync({ itemId, igdbId });
      toast.success('Match applied');
      setMatchItem(null);
      refetch();
    } catch (error) {
      toast.error(error instanceof Error ? error.message : 'Failed to apply match');
    }
  };

  const handleRetryAll = async () => {
    try {
      const result = await retryAllMutation.mutateAsync(jobId);
      toast.success(result.message);
    } catch (error) {
      toast.error(error instanceof Error ? error.message : 'Failed to retry items');
    }
  };

  const handleRetryItem = async (itemId: string) => {
    try {
      await retryItemMutation.mutateAsync(itemId);
      toast.success('Item queued for retry');
      refetch();
    } catch (error) {
      toast.error(error instanceof Error ? error.message : 'Failed to retry item');
    }
  };

  // Use the items query total when section is open and data is loaded,
  // ensuring the count badge stays in sync with the displayed items
  const displayCount = isOpen && data ? data.total : count;

  if (count === 0) return null;

  const iconMap: Record<JobItemStatus, React.ReactNode> = {
    [JobItemStatus.PENDING]: <Clock className="h-4 w-4 text-muted-foreground" />,
    [JobItemStatus.PROCESSING]: <Loader2 className="h-4 w-4 animate-spin text-blue-500" />,
    [JobItemStatus.COMPLETED]: <CheckCircle className="h-4 w-4 text-green-600" />,
    [JobItemStatus.PENDING_REVIEW]: <AlertCircle className="h-4 w-4 text-yellow-600" />,
    [JobItemStatus.SKIPPED]: <Clock className="h-4 w-4 text-muted-foreground" />,
    [JobItemStatus.FAILED]: <AlertCircle className="h-4 w-4 text-red-600" />,
  };

  return (
    <Collapsible open={isOpen} onOpenChange={setIsOpen}>
      <CollapsibleTrigger asChild>
        <Button variant="ghost" className="w-full justify-between px-4 py-2 h-auto">
          <div className="flex items-center gap-2">
            {isOpen ? <ChevronDown className="h-4 w-4" /> : <ChevronRight className="h-4 w-4" />}
            {iconMap[status]}
            <span>{getJobItemStatusLabel(status)}</span>
          </div>
          <div className="flex items-center gap-2">
            {canRetryAll && (
              <Button
                variant="outline"
                size="sm"
                onClick={(e) => {
                  e.stopPropagation();
                  handleRetryAll();
                }}
                disabled={retryAllMutation.isPending}
                className="h-7"
              >
                {retryAllMutation.isPending ? (
                  <Loader2 className="h-3 w-3 animate-spin mr-1" />
                ) : (
                  <RotateCcw className="h-3 w-3 mr-1" />
                )}
                Retry All
              </Button>
            )}
            <Badge variant={getJobItemStatusVariant(status)}>{displayCount}</Badge>
          </div>
        </Button>
      </CollapsibleTrigger>
      <CollapsibleContent>
        <div className="border-l-2 border-muted ml-6 pl-4 py-2 space-y-2">
          {isLoading ? (
            <div className="space-y-2">
              <Skeleton className="h-10 w-full" />
              <Skeleton className="h-10 w-full" />
              <Skeleton className="h-10 w-full" />
            </div>
          ) : (
            <>
              {data?.items.map((item) => (
                <div
                  key={item.id}
                  className="flex items-start justify-between rounded-md border p-3 text-sm"
                >
                  <div className="min-w-0 flex-1">
                    {item.resultUserGameId ? (
                      <Link
                        to="/games/$id"
                        params={{ id: String(item.resultUserGameId) }}
                        className="font-medium truncate hover:underline text-primary block"
                      >
                        {item.resultGameTitle || item.sourceTitle}
                      </Link>
                    ) : (
                      <div className="font-medium truncate">
                        {item.resultGameTitle || item.sourceTitle}
                      </div>
                    )}
                    {item.errorMessage && (
                      <div className="text-xs mt-1 text-red-600">{item.errorMessage}</div>
                    )}
                  </div>
                  {canRetryItem && (
                    <Button
                      variant="ghost"
                      size="sm"
                      onClick={() => handleRetryItem(item.id)}
                      disabled={retryItemMutation.isPending}
                      className="ml-2 h-8"
                      title="Retry"
                    >
                      {retryItemMutation.isPending ? (
                        <Loader2 className="h-3 w-3 animate-spin" />
                      ) : (
                        <RotateCcw className="h-3 w-3" />
                      )}
                    </Button>
                  )}
                  {isReviewSection && (
                    <div className="ml-2 flex gap-1">
                      <Button
                        variant="outline"
                        size="sm"
                        className="h-8"
                        onClick={() => setMatchItem({ id: item.id, title: item.sourceTitle })}
                      >
                        <Search className="h-3 w-3 mr-1" />
                        Find Match
                      </Button>
                      <Button
                        variant="ghost"
                        size="sm"
                        className="h-8"
                        onClick={() => handleSkip(item.id)}
                        disabled={skipMutation.isPending}
                        title="Skip"
                      >
                        <Ban className="h-3 w-3" />
                      </Button>
                    </div>
                  )}
                </div>
              ))}
              {data && data.pages > 1 && (
                <div className="flex items-center justify-center gap-2 pt-2">
                  <Button
                    variant="outline"
                    size="sm"
                    onClick={() => setPage((p) => Math.max(1, p - 1))}
                    disabled={page <= 1}
                  >
                    Previous
                  </Button>
                  <span className="text-sm text-muted-foreground">
                    Page {page} of {data.pages}
                  </span>
                  <Button
                    variant="outline"
                    size="sm"
                    onClick={() => setPage((p) => Math.min(data.pages, p + 1))}
                    disabled={page >= data.pages}
                  >
                    Next
                  </Button>
                </div>
              )}
            </>
          )}
        </div>
      </CollapsibleContent>
      {matchItem && (
        <IGDBMatchDialog
          open
          title="Find IGDB Match"
          initialQuery={matchItem.title}
          onClose={() => setMatchItem(null)}
          onSelect={(candidate) => handleSelectMatch(matchItem.id, candidate.igdb_id)}
        />
      )}
    </Collapsible>
  );
}

export function JobItemsDetails({ jobId, progress, isTerminal }: JobItemsDetailsProps) {
  // Order sections: needs review first (action required), then failed, then others
  const sections = [
    {
      status: JobItemStatus.PENDING_REVIEW,
      count: progress.pendingReview,
      defaultOpen: progress.pendingReview > 0,
    },
    { status: JobItemStatus.FAILED, count: progress.failed, defaultOpen: progress.failed > 0 },
    { status: JobItemStatus.PROCESSING, count: progress.processing, defaultOpen: false },
    { status: JobItemStatus.PENDING, count: progress.pending, defaultOpen: false },
    { status: JobItemStatus.COMPLETED, count: progress.completed, defaultOpen: false },
    { status: JobItemStatus.SKIPPED, count: progress.skipped, defaultOpen: false },
  ];

  const hasItems = sections.some((s) => s.count > 0);

  if (!hasItems) {
    return null;
  }

  return (
    <div className="rounded-lg border">
      <div className="border-b p-3">
        <h3 className="font-medium">Item Details</h3>
      </div>
      <div className="divide-y">
        {sections.map(({ status, count, defaultOpen }) => (
          <StatusSection
            key={status}
            jobId={jobId}
            status={status}
            count={count}
            defaultOpen={defaultOpen}
            isTerminal={isTerminal}
          />
        ))}
      </div>
    </div>
  );
}
