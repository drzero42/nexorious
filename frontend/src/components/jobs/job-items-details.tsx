'use client';

import { useState, useCallback, useEffect, useRef } from 'react';
import { toast } from 'sonner';
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from '@/components/ui/collapsible';
import { Button } from '@/components/ui/button';
import { Badge } from '@/components/ui/badge';
import { Skeleton } from '@/components/ui/skeleton';
import { Input } from '@/components/ui/input';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import {
  ChevronDown,
  ChevronRight,
  AlertCircle,
  CheckCircle,
  Clock,
  Loader2,
  RotateCcw,
  Search,
  SkipForward,
  ImageOff,
} from 'lucide-react';
import Link from 'next/link';
import {
  useJobItems,
  useRetryFailedItems,
  useRetryJobItem,
  useResolveJobItem,
  useSkipJobItem,
  useSearchIGDB,
} from '@/hooks';
import { JobItemStatus, getJobItemStatusLabel, getJobItemStatusVariant } from '@/types';
import type { JobItem, IGDBGameCandidate } from '@/types';

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

// Search result item component for the modal
function SearchResultItem({
  result,
  isProcessing,
  onSelect,
}: {
  result: IGDBGameCandidate;
  isProcessing: boolean;
  onSelect: () => void;
}) {
  const releaseYear = result.release_date
    ? new Date(result.release_date).getFullYear()
    : null;

  return (
    <button
      className="flex w-full items-center gap-3 rounded-md p-2 text-left transition-colors hover:bg-muted disabled:opacity-50"
      onClick={onSelect}
      disabled={isProcessing}
    >
      {result.cover_art_url ? (
        <img
          src={result.cover_art_url}
          alt={result.title}
          className="h-12 w-9 rounded object-cover"
        />
      ) : (
        <div className="flex h-12 w-9 items-center justify-center rounded bg-muted">
          <ImageOff className="h-4 w-4 text-muted-foreground" />
        </div>
      )}
      <div className="min-w-0 flex-1">
        <p className="truncate font-medium">
          {result.title}
          {releaseYear && (
            <span className="ml-1 text-muted-foreground">({releaseYear})</span>
          )}
        </p>
        {result.platforms.length > 0 && (
          <p className="truncate text-xs text-muted-foreground">
            {result.platforms.slice(0, 3).join(', ')}
            {result.platforms.length > 3 && ` +${result.platforms.length - 3}`}
          </p>
        )}
      </div>
    </button>
  );
}

// Inline review widget for a single PENDING_REVIEW item
function ReviewItemWidget({
  item,
  isProcessing,
  onProcessingChange,
}: {
  item: JobItem;
  isProcessing: boolean;
  onProcessingChange: (processing: boolean) => void;
}) {
  const [showSearchModal, setShowSearchModal] = useState(false);
  const [searchQuery, setSearchQuery] = useState('');

  const resolveMutation = useResolveJobItem();
  const skipMutation = useSkipJobItem();
  const { data: searchResults, isLoading: isSearching } = useSearchIGDB(searchQuery);

  // Parse IGDB candidates from the item
  const candidates: IGDBGameCandidate[] = (() => {
    try {
      const parsed = JSON.parse(item.igdbCandidatesJson || '[]');
      return parsed.map((c: Record<string, unknown>) => ({
        igdb_id: c.igdb_id as number,
        title: (c.name || c.title) as string,
        release_date: c.release_date as string | null,
        cover_art_url: c.cover_art_url as string | null,
        platforms: (c.platforms || []) as string[],
      }));
    } catch {
      return [];
    }
  })();

  const handleMatch = useCallback(
    async (igdbId: number) => {
      onProcessingChange(true);
      try {
        await resolveMutation.mutateAsync({ itemId: item.id, igdbId });
        toast.success(`Matched "${item.sourceTitle}"`);
      } catch (err) {
        toast.error(err instanceof Error ? err.message : 'Failed to match');
      } finally {
        onProcessingChange(false);
      }
    },
    [item.id, item.sourceTitle, resolveMutation, onProcessingChange]
  );

  const handleSkip = useCallback(async () => {
    onProcessingChange(true);
    try {
      await skipMutation.mutateAsync({ itemId: item.id });
      toast.success(`Skipped "${item.sourceTitle}"`);
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Failed to skip');
    } finally {
      onProcessingChange(false);
    }
  }, [item.id, item.sourceTitle, skipMutation, onProcessingChange]);

  const handleSearchMatch = useCallback(
    async (igdbId: number) => {
      onProcessingChange(true);
      try {
        await resolveMutation.mutateAsync({ itemId: item.id, igdbId });
        toast.success(`Matched "${item.sourceTitle}"`);
        setShowSearchModal(false);
        setSearchQuery('');
      } catch (err) {
        toast.error(err instanceof Error ? err.message : 'Failed to match');
      } finally {
        onProcessingChange(false);
      }
    },
    [item.id, item.sourceTitle, resolveMutation, onProcessingChange]
  );

  return (
    <div className="rounded-md border p-3 space-y-3">
      <div className="flex items-start justify-between gap-2">
        <div className="min-w-0 flex-1">
          <div className="font-medium truncate">{item.sourceTitle}</div>
        </div>
        <div className="flex gap-1 shrink-0">
          <Button
            variant="outline"
            size="sm"
            onClick={() => {
              setSearchQuery(item.sourceTitle);
              setShowSearchModal(true);
            }}
            disabled={isProcessing}
            className="h-7"
          >
            <Search className="h-3 w-3 mr-1" />
            Search
          </Button>
          <Button
            variant="ghost"
            size="sm"
            onClick={handleSkip}
            disabled={isProcessing}
            className="h-7"
          >
            <SkipForward className="h-3 w-3 mr-1" />
            Skip
          </Button>
        </div>
      </div>

      {/* Suggested matches */}
      {candidates.length > 0 && (
        <div className="space-y-1">
          <div className="text-xs text-muted-foreground">Suggested matches:</div>
          <div className="flex flex-wrap gap-1">
            {candidates.slice(0, 3).map((candidate) => (
              <Button
                key={candidate.igdb_id}
                variant="secondary"
                size="sm"
                onClick={() => handleMatch(candidate.igdb_id)}
                disabled={isProcessing}
                className="h-auto py-1 px-2 text-xs"
              >
                {candidate.title}
                <span className="ml-1 text-muted-foreground">
                  (ID: {candidate.igdb_id})
                </span>
              </Button>
            ))}
          </div>
        </div>
      )}

      {/* Search Modal */}
      <Dialog open={showSearchModal} onOpenChange={setShowSearchModal}>
        <DialogContent className="max-w-2xl">
          <DialogHeader>
            <DialogTitle>Search: {item.sourceTitle}</DialogTitle>
            <DialogDescription>
              Search IGDB to find the correct match
            </DialogDescription>
          </DialogHeader>

          <div className="pt-2">
            <div className="relative">
              <Input
                placeholder="Search IGDB..."
                value={searchQuery}
                onChange={(e) => setSearchQuery(e.target.value)}
              />
              {searchQuery.length >= 3 && (
                <div className="absolute left-0 right-0 top-full z-50 mt-1 max-h-64 overflow-y-auto rounded-md border bg-popover shadow-lg">
                  {isSearching ? (
                    <div className="flex items-center justify-center p-4">
                      <Loader2 className="h-4 w-4 animate-spin" />
                      <span className="ml-2 text-sm text-muted-foreground">
                        Searching...
                      </span>
                    </div>
                  ) : searchResults && searchResults.length > 0 ? (
                    <div className="p-1">
                      {searchResults.map((result) => (
                        <SearchResultItem
                          key={result.igdb_id}
                          result={result}
                          isProcessing={isProcessing}
                          onSelect={() => handleSearchMatch(result.igdb_id)}
                        />
                      ))}
                    </div>
                  ) : (
                    <div className="p-4 text-center text-sm text-muted-foreground">
                      No games found for &ldquo;{searchQuery}&rdquo;
                    </div>
                  )}
                </div>
              )}
            </div>
          </div>

          <div className="flex justify-end gap-2 border-t pt-4">
            <Button
              variant="outline"
              onClick={handleSkip}
              disabled={isProcessing}
            >
              Skip
            </Button>
            <Button variant="ghost" onClick={() => setShowSearchModal(false)}>
              Cancel
            </Button>
          </div>
        </DialogContent>
      </Dialog>
    </div>
  );
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
  const [processingItemId, setProcessingItemId] = useState<string | null>(null);
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

  // Determine section behavior
  const isFailedSection = status === JobItemStatus.FAILED;
  const isPendingReviewSection = status === JobItemStatus.PENDING_REVIEW;
  const canRetry = isFailedSection && isTerminal;

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

  if (count === 0 && !isOpen) return null;

  const iconMap: Record<JobItemStatus, React.ReactNode> = {
    [JobItemStatus.PENDING]: <Clock className="h-4 w-4 text-muted-foreground" />,
    [JobItemStatus.PROCESSING]: (
      <Loader2 className="h-4 w-4 animate-spin text-blue-500" />
    ),
    [JobItemStatus.COMPLETED]: <CheckCircle className="h-4 w-4 text-green-600" />,
    [JobItemStatus.PENDING_REVIEW]: (
      <AlertCircle className="h-4 w-4 text-yellow-600" />
    ),
    [JobItemStatus.SKIPPED]: <Clock className="h-4 w-4 text-muted-foreground" />,
    [JobItemStatus.FAILED]: <AlertCircle className="h-4 w-4 text-red-600" />,
  };

  return (
    <Collapsible open={isOpen} onOpenChange={setIsOpen}>
      <CollapsibleTrigger asChild>
        <Button
          variant="ghost"
          className="w-full justify-between px-4 py-2 h-auto"
        >
          <div className="flex items-center gap-2">
            {isOpen ? (
              <ChevronDown className="h-4 w-4" />
            ) : (
              <ChevronRight className="h-4 w-4" />
            )}
            {iconMap[status]}
            <span>{getJobItemStatusLabel(status)}</span>
          </div>
          <div className="flex items-center gap-2">
            {canRetry && (
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
              {data?.items.map((item) =>
                isPendingReviewSection ? (
                  <ReviewItemWidget
                    key={item.id}
                    item={item}
                    isProcessing={processingItemId === item.id}
                    onProcessingChange={(processing) =>
                      setProcessingItemId(processing ? item.id : null)
                    }
                  />
                ) : (
                  <div
                    key={item.id}
                    className="flex items-start justify-between rounded-md border p-3 text-sm"
                  >
                    <div className="min-w-0 flex-1">
                      {item.resultUserGameId ? (
                        <Link
                          href={`/games/${item.resultUserGameId}`}
                          className="font-medium truncate hover:underline text-primary block"
                        >
                          {item.resultGameTitle || item.sourceTitle}
                        </Link>
                      ) : (
                        <div className="font-medium truncate">{item.resultGameTitle || item.sourceTitle}</div>
                      )}
                      {item.errorMessage && (
                        <div className="text-red-600 text-xs mt-1">
                          {item.errorMessage}
                        </div>
                      )}
                    </div>
                    {canRetry && (
                      <Button
                        variant="ghost"
                        size="sm"
                        onClick={() => handleRetryItem(item.id)}
                        disabled={retryItemMutation.isPending}
                        className="ml-2 h-8"
                      >
                        {retryItemMutation.isPending ? (
                          <Loader2 className="h-3 w-3 animate-spin" />
                        ) : (
                          <RotateCcw className="h-3 w-3" />
                        )}
                      </Button>
                    )}
                  </div>
                )
              )}
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
    </Collapsible>
  );
}

export function JobItemsDetails({ jobId, progress, isTerminal }: JobItemsDetailsProps) {
  // Order sections: needs review first (action required), then failed, then others
  const sections = [
    {
      status: JobItemStatus.PENDING_REVIEW,
      count: progress.pendingReview,
      defaultOpen: progress.pendingReview > 0,  // Auto-expand when items need review
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
