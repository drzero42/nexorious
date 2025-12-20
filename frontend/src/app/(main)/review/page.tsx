'use client';

import { useState, useCallback, useEffect } from 'react';
import { useRouter, useSearchParams } from 'next/navigation';
import Link from 'next/link';
import { toast } from 'sonner';
import { Card, CardContent } from '@/components/ui/card';
import { Button } from '@/components/ui/button';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import { Skeleton } from '@/components/ui/skeleton';
import {
  Pagination,
  PaginationContent,
  PaginationItem,
  PaginationLink,
  PaginationNext,
  PaginationPrevious,
} from '@/components/ui/pagination';
import {
  AlertCircle,
  ArrowRight,
  Check,
  CheckCircle,
  Loader2,
  RefreshCw,
  X,
  ImageOff,
} from 'lucide-react';
import { ReviewItemCard } from '@/components/review';
import {
  useReviewItems,
  useReviewSummary,
  useMatchReviewItem,
  useSkipReviewItem,
  useKeepReviewItem,
  useRemoveReviewItem,
  useSearchIGDB,
  useFinalizeImport,
  usePlatformSummary,
} from '@/hooks';
import { useImportMapping } from '@/contexts/import-mapping-context';
import type { ReviewItem, ReviewFilters, IGDBCandidate, IGDBGameCandidate } from '@/types';
import { ReviewItemStatus, ReviewSource, formatReleaseYear } from '@/types';

const ITEMS_PER_PAGE = 20;

function ReviewPageSkeleton() {
  return (
    <div className="space-y-6">
      <div>
        <Skeleton className="mb-2 h-8 w-48" />
        <Skeleton className="h-4 w-64" />
      </div>
      <div className="grid grid-cols-2 gap-4 sm:grid-cols-4">
        {[1, 2, 3, 4].map((i) => (
          <Skeleton key={i} className="h-24" />
        ))}
      </div>
      <Card>
        <CardContent className="grid gap-4 p-4 sm:grid-cols-2">
          <Skeleton className="h-10" />
          <Skeleton className="h-10" />
        </CardContent>
      </Card>
      <div className="space-y-4">
        {[1, 2, 3].map((i) => (
          <Skeleton key={i} className="h-48" />
        ))}
      </div>
    </div>
  );
}

export default function ReviewPage() {
  const router = useRouter();
  const searchParams = useSearchParams();

  // Get filter values from URL
  const jobIdFromUrl = searchParams.get('job_id');
  const sourceFromUrl = searchParams.get('source');

  // Import mapping context
  const { platformMappings, storefrontMappings, jobId: contextJobId, clearMappings } = useImportMapping();
  const effectiveJobId = jobIdFromUrl || contextJobId;
  const hasMappings = Object.keys(platformMappings).length > 0 || Object.keys(storefrontMappings).length > 0;
  const canFinalize = effectiveJobId && hasMappings;

  const [page, setPage] = useState(1);
  const [filters, setFilters] = useState<ReviewFilters>(() => {
    const initial: ReviewFilters = {};
    if (jobIdFromUrl) initial.jobId = jobIdFromUrl;
    if (sourceFromUrl === 'import') initial.source = ReviewSource.IMPORT;
    if (sourceFromUrl === 'sync') initial.source = ReviewSource.SYNC;
    return initial;
  });
  const [selectedItem, setSelectedItem] = useState<ReviewItem | null>(null);
  const [processingItemId, setProcessingItemId] = useState<string | null>(null);
  const [searchQuery, setSearchQuery] = useState('');

  const { data, isLoading, error, refetch, isFetching } = useReviewItems(
    filters,
    page,
    ITEMS_PER_PAGE
  );
  const { data: summary } = useReviewSummary();

  // Find the first import job from review items that might need platform mapping
  const firstImportJobId = data?.items.find(
    (item) => item.jobSource === 'DARKADIA' || item.jobSource === 'darkadia'
  )?.jobId || null;

  // Check if this job has unresolved platform/storefront mappings
  const { data: platformSummary } = usePlatformSummary(firstImportJobId);

  // Smart default: show pending items if there are any and no explicit status filter
  useEffect(() => {
    const statusFromUrl = searchParams.get('status');
    if (!statusFromUrl && summary && summary.totalPending > 0 && filters.status === undefined) {
      setFilters((prev) => ({ ...prev, status: ReviewItemStatus.PENDING }));
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps -- intentionally only run when summary.totalPending changes
  }, [summary?.totalPending, searchParams]);

  const matchMutation = useMatchReviewItem();
  const skipMutation = useSkipReviewItem();
  const keepMutation = useKeepReviewItem();
  const removeMutation = useRemoveReviewItem();
  const finalizeMutation = useFinalizeImport();
  const { data: searchResults, isLoading: isSearching, error: searchError } = useSearchIGDB(searchQuery);

  const hasFilters =
    filters.status !== undefined ||
    filters.jobId !== undefined ||
    filters.source !== undefined;

  const handleFilterChange = (key: keyof ReviewFilters, value: string | undefined) => {
    setFilters((prev) => ({
      ...prev,
      [key]: value === 'all' ? undefined : value,
    }));
    setPage(1);
  };

  const clearFilters = () => {
    setFilters({});
    setPage(1);
    router.replace('/review');
  };

  const handleMatch = useCallback(
    async (item: ReviewItem, igdbId: number) => {
      setProcessingItemId(item.id);
      try {
        await matchMutation.mutateAsync({ itemId: item.id, igdbId });
        toast.success(`Matched "${item.sourceTitle}" to IGDB`);
      } catch (err) {
        toast.error(err instanceof Error ? err.message : 'Failed to match item');
      } finally {
        setProcessingItemId(null);
      }
    },
    [matchMutation]
  );

  const handleSkip = useCallback(
    async (item: ReviewItem) => {
      setProcessingItemId(item.id);
      try {
        await skipMutation.mutateAsync(item.id);
        toast.success(`Skipped "${item.sourceTitle}"`);
      } catch (err) {
        toast.error(err instanceof Error ? err.message : 'Failed to skip item');
      } finally {
        setProcessingItemId(null);
      }
    },
    [skipMutation]
  );

  const handleKeep = useCallback(
    async (item: ReviewItem) => {
      setProcessingItemId(item.id);
      try {
        await keepMutation.mutateAsync(item.id);
        toast.success(`Kept "${item.sourceTitle}" in collection`);
      } catch (err) {
        toast.error(err instanceof Error ? err.message : 'Failed to keep item');
      } finally {
        setProcessingItemId(null);
      }
    },
    [keepMutation]
  );

  const handleRemove = useCallback(
    async (item: ReviewItem) => {
      setProcessingItemId(item.id);
      try {
        await removeMutation.mutateAsync(item.id);
        toast.success(`Removed "${item.sourceTitle}" from collection`);
      } catch (err) {
        toast.error(err instanceof Error ? err.message : 'Failed to remove item');
      } finally {
        setProcessingItemId(null);
      }
    },
    [removeMutation]
  );

  const handleView = useCallback((item: ReviewItem) => {
    setSelectedItem(item);
  }, []);

  const handleModalMatch = useCallback(
    async (igdbId: number) => {
      if (!selectedItem) return;
      setProcessingItemId(selectedItem.id);
      try {
        await matchMutation.mutateAsync({ itemId: selectedItem.id, igdbId });
        toast.success(`Matched "${selectedItem.sourceTitle}" to IGDB`);
        setSelectedItem(null);
      } catch (err) {
        toast.error(err instanceof Error ? err.message : 'Failed to match item');
      } finally {
        setProcessingItemId(null);
      }
    },
    [selectedItem, matchMutation]
  );

  const handleModalSkip = useCallback(async () => {
    if (!selectedItem) return;
    setProcessingItemId(selectedItem.id);
    try {
      await skipMutation.mutateAsync(selectedItem.id);
      toast.success(`Skipped "${selectedItem.sourceTitle}"`);
      setSelectedItem(null);
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Failed to skip item');
    } finally {
      setProcessingItemId(null);
    }
  }, [selectedItem, skipMutation]);

  const handleSearchResultMatch = useCallback(
    async (igdbId: number) => {
      if (!selectedItem) return;
      setProcessingItemId(selectedItem.id);
      try {
        await matchMutation.mutateAsync({ itemId: selectedItem.id, igdbId });
        toast.success(`Matched "${selectedItem.sourceTitle}" to IGDB`);
        setSelectedItem(null);
        setSearchQuery('');
      } catch (err) {
        toast.error(err instanceof Error ? err.message : 'Failed to match item');
      } finally {
        setProcessingItemId(null);
      }
    },
    [selectedItem, matchMutation]
  );

  const handleFinalize = useCallback(async () => {
    if (!effectiveJobId) return;
    try {
      const result = await finalizeMutation.mutateAsync({
        jobId: effectiveJobId,
        platformMappings,
        storefrontMappings,
      });
      toast.success(result.message);
      clearMappings();
      // Optionally navigate to games page
      router.push('/games');
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Failed to finalize import');
    }
  }, [effectiveJobId, platformMappings, storefrontMappings, finalizeMutation, clearMappings, router]);

  if (isLoading) {
    return <ReviewPageSkeleton />;
  }

  return (
    <div className="space-y-6">
      {/* Header */}
      <div className="flex items-center justify-between">
        <div>
          <nav className="mb-2 flex items-center text-sm text-muted-foreground">
            <Link href="/dashboard" className="hover:text-foreground">
              Dashboard
            </Link>
            <span className="mx-2">/</span>
            <span className="text-foreground">Review Queue</span>
          </nav>
          <h1 className="text-2xl font-bold">Review Queue</h1>
          <p className="text-muted-foreground">
            Match unmatched games from your imports and syncs
          </p>
        </div>
        <div className="flex items-center gap-4">
          <Button variant="outline" size="sm" onClick={() => refetch()} disabled={isFetching}>
            {isFetching ? (
              <Loader2 className="mr-2 h-4 w-4 animate-spin" />
            ) : (
              <RefreshCw className="mr-2 h-4 w-4" />
            )}
            Refresh
          </Button>
          {canFinalize && (
            <Button
              onClick={handleFinalize}
              disabled={finalizeMutation.isPending}
              className="bg-green-600 hover:bg-green-700"
              size="sm"
            >
              {finalizeMutation.isPending ? (
                <>
                  <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                  Finalizing...
                </>
              ) : (
                <>
                  <Check className="mr-2 h-4 w-4" />
                  Finalize Import
                </>
              )}
            </Button>
          )}
          {summary && (
            <div className="text-right">
              <div className="text-2xl font-bold text-primary">{summary.totalPending}</div>
              <div className="text-sm text-muted-foreground">pending items</div>
            </div>
          )}
        </div>
      </div>

      {/* Platform/Storefront Mapping Info */}
      {platformSummary && firstImportJobId && (
        <Alert
          className={
            platformSummary.allResolved
              ? 'border-green-200 bg-green-50 dark:border-green-800 dark:bg-green-900/20'
              : 'border-orange-200 bg-orange-50 dark:border-orange-800 dark:bg-orange-900/20'
          }
        >
          {platformSummary.allResolved ? (
            <CheckCircle className="h-4 w-4 text-green-600 dark:text-green-400" />
          ) : (
            <AlertCircle className="h-4 w-4 text-orange-600 dark:text-orange-400" />
          )}
          <AlertTitle
            className={
              platformSummary.allResolved
                ? 'text-green-800 dark:text-green-200'
                : 'text-orange-800 dark:text-orange-200'
            }
          >
            {platformSummary.allResolved
              ? 'Platform/Storefront Mappings Ready'
              : 'Platform/Storefront Mapping Required'}
          </AlertTitle>
          <AlertDescription className="flex items-center justify-between">
            <span
              className={
                platformSummary.allResolved
                  ? 'text-green-700 dark:text-green-300'
                  : 'text-orange-700 dark:text-orange-300'
              }
            >
              {platformSummary.allResolved
                ? 'All platforms and storefronts have been mapped. You can review the mappings before finalizing.'
                : 'Some platforms or storefronts from your import need to be mapped before you can finalize.'}
            </span>
            <Button
              variant="outline"
              size="sm"
              className={
                platformSummary.allResolved
                  ? 'ml-4 border-green-300 text-green-700 hover:bg-green-100 dark:border-green-700 dark:text-green-300 dark:hover:bg-green-900/40'
                  : 'ml-4 border-orange-300 text-orange-700 hover:bg-orange-100 dark:border-orange-700 dark:text-orange-300 dark:hover:bg-orange-900/40'
              }
              onClick={() => router.push(`/import/mapping?job_id=${firstImportJobId}`)}
            >
              {platformSummary.allResolved ? 'View Mappings' : 'Go to Mapping'}
              <ArrowRight className="ml-2 h-4 w-4" />
            </Button>
          </AlertDescription>
        </Alert>
      )}

      {/* Summary Stats */}
      {summary && (
        <div className="grid grid-cols-2 gap-4 sm:grid-cols-4">
          <Card className="border-yellow-200 bg-yellow-50 dark:border-yellow-800 dark:bg-yellow-900/20">
            <CardContent className="p-4">
              <div className="text-2xl font-bold text-yellow-600 dark:text-yellow-400">
                {summary.totalPending}
              </div>
              <div className="text-sm text-yellow-700 dark:text-yellow-300">Pending</div>
            </CardContent>
          </Card>
          <Card className="border-green-200 bg-green-50 dark:border-green-800 dark:bg-green-900/20">
            <CardContent className="p-4">
              <div className="text-2xl font-bold text-green-600 dark:text-green-400">
                {summary.totalMatched}
              </div>
              <div className="text-sm text-green-700 dark:text-green-300">Matched</div>
            </CardContent>
          </Card>
          <Card>
            <CardContent className="p-4">
              <div className="text-2xl font-bold text-muted-foreground">{summary.totalSkipped}</div>
              <div className="text-sm text-muted-foreground">Skipped</div>
            </CardContent>
          </Card>
          <Card className="border-red-200 bg-red-50 dark:border-red-800 dark:bg-red-900/20">
            <CardContent className="p-4">
              <div className="text-2xl font-bold text-red-600 dark:text-red-400">
                {summary.totalRemoval}
              </div>
              <div className="text-sm text-red-700 dark:text-red-300">Removed</div>
            </CardContent>
          </Card>
        </div>
      )}

      {/* Filters */}
      <Card>
        <CardContent className="p-4">
          {/* Source Filter Toggle */}
          <div className="mb-4">
            <Label className="mb-2 block">Source</Label>
            <div className="inline-flex rounded-lg border bg-muted/50 p-1">
              <Button
                variant={filters.source === undefined ? 'secondary' : 'ghost'}
                size="sm"
                onClick={() => handleFilterChange('source', undefined)}
              >
                All
              </Button>
              <Button
                variant={filters.source === ReviewSource.IMPORT ? 'secondary' : 'ghost'}
                size="sm"
                onClick={() => handleFilterChange('source', ReviewSource.IMPORT)}
              >
                Imports
              </Button>
              <Button
                variant={filters.source === ReviewSource.SYNC ? 'secondary' : 'ghost'}
                size="sm"
                onClick={() => handleFilterChange('source', ReviewSource.SYNC)}
              >
                Syncs
              </Button>
            </div>
          </div>

          <div className="grid gap-4 sm:grid-cols-2">
            {/* Status Filter */}
            <div className="space-y-2">
              <Label htmlFor="status">Status</Label>
              <Select
                value={filters.status || 'all'}
                onValueChange={(value) => handleFilterChange('status', value)}
              >
                <SelectTrigger id="status">
                  <SelectValue placeholder="All Statuses" />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="all">All Statuses</SelectItem>
                  {Object.values(ReviewItemStatus).map((status) => (
                    <SelectItem key={status} value={status}>
                      {status.charAt(0).toUpperCase() + status.slice(1)}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>

            {/* Job ID Filter */}
            <div className="space-y-2">
              <Label htmlFor="job_id">Job ID</Label>
              <Input
                id="job_id"
                placeholder="Filter by job ID"
                value={filters.jobId || ''}
                onChange={(e) =>
                  handleFilterChange('jobId', e.target.value || undefined)
                }
              />
            </div>
          </div>

          {hasFilters && (
            <div className="mt-4 flex justify-end">
              <Button variant="ghost" size="sm" onClick={clearFilters}>
                <X className="mr-2 h-4 w-4" />
                Clear filters
              </Button>
            </div>
          )}
        </CardContent>
      </Card>

      {/* Error State */}
      {error && (
        <Alert variant="destructive">
          <AlertCircle className="h-4 w-4" />
          <AlertTitle>Error loading review items</AlertTitle>
          <AlertDescription>
            {error instanceof Error ? error.message : 'An unexpected error occurred'}
          </AlertDescription>
        </Alert>
      )}

      {/* Items List */}
      {data?.items.length === 0 ? (
        <div className="py-12 text-center">
          <CheckCircle className="mx-auto h-12 w-12 text-muted-foreground" />
          <h3 className="mt-4 text-lg font-medium">No items to review</h3>
          <p className="mt-2 text-sm text-muted-foreground">
            {hasFilters
              ? 'Try adjusting your filters.'
              : 'All your imports and syncs have been fully matched!'}
          </p>
        </div>
      ) : (
        <div className="space-y-4">
          {data?.items.map((item) => (
            <ReviewItemCard
              key={item.id}
              item={item}
              onMatch={handleMatch}
              onSkip={handleSkip}
              onKeep={handleKeep}
              onRemove={handleRemove}
              onView={handleView}
              isProcessing={processingItemId === item.id}
            />
          ))}
        </div>
      )}

      {/* Pagination */}
      {data && data.pages > 1 && (
        <Pagination>
          <PaginationContent>
            <PaginationItem>
              <PaginationPrevious
                onClick={() => setPage((p) => Math.max(1, p - 1))}
                aria-disabled={page <= 1}
                className={page <= 1 ? 'pointer-events-none opacity-50' : 'cursor-pointer'}
              />
            </PaginationItem>
            {Array.from({ length: Math.min(5, data.pages) }, (_, i) => {
              let pageNum: number;
              if (data.pages <= 5) {
                pageNum = i + 1;
              } else if (page <= 3) {
                pageNum = i + 1;
              } else if (page >= data.pages - 2) {
                pageNum = data.pages - 4 + i;
              } else {
                pageNum = page - 2 + i;
              }
              return (
                <PaginationItem key={pageNum}>
                  <PaginationLink
                    onClick={() => setPage(pageNum)}
                    isActive={page === pageNum}
                    className="cursor-pointer"
                  >
                    {pageNum}
                  </PaginationLink>
                </PaginationItem>
              );
            })}
            <PaginationItem>
              <PaginationNext
                onClick={() => setPage((p) => Math.min(data.pages, p + 1))}
                aria-disabled={page >= data.pages}
                className={page >= data.pages ? 'pointer-events-none opacity-50' : 'cursor-pointer'}
              />
            </PaginationItem>
          </PaginationContent>
        </Pagination>
      )}

      {/* IGDB Candidates Modal */}
      <Dialog open={!!selectedItem} onOpenChange={(open) => {
        if (!open) {
          setSelectedItem(null);
          setSearchQuery('');
        }
      }}>
        <DialogContent className="max-w-2xl">
          <DialogHeader>
            <DialogTitle>Match: {selectedItem?.sourceTitle}</DialogTitle>
            <DialogDescription>Select the correct IGDB match for this game</DialogDescription>
          </DialogHeader>

          {selectedItem?.igdbCandidates.length === 0 ? (
            <div className="py-8 text-center">
              <p className="text-muted-foreground">
                No IGDB candidates found. Try searching manually.
              </p>
            </div>
          ) : (
            <div className="max-h-96 space-y-3 overflow-y-auto">
              {selectedItem?.igdbCandidates.map((candidate, index) => (
                <CandidateButton
                  key={candidate.igdbId}
                  candidate={candidate}
                  isBestMatch={index === 0}
                  isProcessing={processingItemId === selectedItem.id}
                  onSelect={() => handleModalMatch(candidate.igdbId)}
                />
              ))}
            </div>
          )}

          {/* IGDB Search Section */}
          <div className="border-t pt-4">
            <p className="mb-2 text-sm text-muted-foreground">
              Can&apos;t find the right match?
            </p>
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
                      <span className="ml-2 text-sm text-muted-foreground">Searching...</span>
                    </div>
                  ) : searchError ? (
                    <div className="p-4 text-center text-sm text-destructive">
                      Search failed. Please try again.
                    </div>
                  ) : searchResults && searchResults.length > 0 ? (
                    <div className="p-1">
                      {searchResults.map((result) => (
                        <SearchResultItem
                          key={result.igdb_id}
                          result={result}
                          isProcessing={processingItemId === selectedItem?.id}
                          onSelect={() => handleSearchResultMatch(result.igdb_id)}
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
              onClick={handleModalSkip}
              disabled={processingItemId === selectedItem?.id}
            >
              Skip
            </Button>
            <Button variant="ghost" onClick={() => setSelectedItem(null)}>
              Cancel
            </Button>
          </div>
        </DialogContent>
      </Dialog>
    </div>
  );
}

interface CandidateButtonProps {
  candidate: IGDBCandidate;
  isBestMatch: boolean;
  isProcessing: boolean;
  onSelect: () => void;
}

function CandidateButton({ candidate, isBestMatch, isProcessing, onSelect }: CandidateButtonProps) {
  return (
    <button
      className="w-full rounded-lg border p-3 text-left transition-colors hover:border-primary hover:bg-muted/50 disabled:opacity-50"
      onClick={onSelect}
      disabled={isProcessing}
    >
      <div className="flex items-start gap-3">
        {candidate.coverUrl ? (
          <img
            src={candidate.coverUrl}
            alt={candidate.name}
            className="h-20 w-16 rounded object-cover"
          />
        ) : (
          <div className="flex h-20 w-16 items-center justify-center rounded bg-muted">
            <ImageOff className="h-8 w-8 text-muted-foreground" />
          </div>
        )}
        <div className="min-w-0 flex-1">
          <p className="font-medium">
            {candidate.name}
            {candidate.firstReleaseDate && (
              <span className="ml-1 text-muted-foreground">
                {formatReleaseYear(candidate.firstReleaseDate)}
              </span>
            )}
          </p>
          {candidate.similarityScore !== null && (
            <p className="mt-0.5 text-xs text-muted-foreground">
              Match confidence: {Math.round(candidate.similarityScore * 100)}%
            </p>
          )}
          {candidate.summary && (
            <p className="mt-1 line-clamp-2 text-sm text-muted-foreground">{candidate.summary}</p>
          )}
          {candidate.platforms && candidate.platforms.length > 0 && (
            <p className="mt-1 text-xs text-muted-foreground">
              Platforms: {candidate.platforms.slice(0, 5).join(', ')}
              {candidate.platforms.length > 5 && ` +${candidate.platforms.length - 5} more`}
            </p>
          )}
        </div>
        {isBestMatch && (
          <span className="rounded bg-green-100 px-2 py-0.5 text-xs font-medium text-green-800 dark:bg-green-900 dark:text-green-300">
            Best Match
          </span>
        )}
      </div>
    </button>
  );
}

interface SearchResultItemProps {
  result: IGDBGameCandidate;
  isProcessing: boolean;
  onSelect: () => void;
}

function SearchResultItem({ result, isProcessing, onSelect }: SearchResultItemProps) {
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
