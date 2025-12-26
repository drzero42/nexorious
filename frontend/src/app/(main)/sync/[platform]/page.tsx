'use client';

import { use, useState, useCallback } from 'react';
import Link from 'next/link';
import { notFound } from 'next/navigation';
import { toast } from 'sonner';
import {
  useSyncConfig,
  useSyncStatus,
  useUpdateSyncConfig,
  useTriggerSync,
  useJob,
  useCancelJob,
  useJobItems,
  useResolveJobItem,
  useSkipJobItem,
  useSearchIGDB,
} from '@/hooks';
import {
  SyncPlatform,
  SyncFrequency,
  SUPPORTED_SYNC_PLATFORMS,
  getPlatformDisplayInfo,
  getSyncFrequencyLabel,
  JobItemStatus,
  formatReleaseYear,
} from '@/types';
import type { SyncConfigUpdateData, JobItem, IGDBCandidate, IGDBGameCandidate } from '@/types';
import { ReviewItemCard } from '@/components/review';
import { JobProgressCard, JobItemsDetails } from '@/components/jobs';
import { Card, CardContent, CardHeader, CardTitle, CardDescription } from '@/components/ui/card';
import { Button } from '@/components/ui/button';
import { Switch } from '@/components/ui/switch';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import { Input } from '@/components/ui/input';
import { Badge } from '@/components/ui/badge';
import { Skeleton } from '@/components/ui/skeleton';
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert';
import { RefreshCw, Loader2, AlertCircle, Settings, Clock, ArrowLeft, ClipboardCheck, Check, ImageOff } from 'lucide-react';

// Platform icons as SVG paths (matching sync-service-card)
const PLATFORM_ICONS: Record<SyncPlatform, string> = {
  [SyncPlatform.STEAM]:
    'M12 2C6.477 2 2 6.477 2 12c0 4.991 3.657 9.128 8.438 9.879V14.89h-2.54V12h2.54V9.797c0-2.506 1.492-3.89 3.777-3.89 1.094 0 2.238.195 2.238.195v2.46h-1.26c-1.243 0-1.63.771-1.63 1.562V12h2.773l-.443 2.89h-2.33v6.989C18.343 21.129 22 16.99 22 12c0-5.523-4.477-10-10-10z',
  [SyncPlatform.EPIC]:
    'M3 3h18v18H3V3zm2 2v14h14V5H5zm3 3h2v8H8V8zm4 0h4v2h-4v2h3v2h-3v2h4v2H12V8z',
  [SyncPlatform.GOG]:
    'M12 2C6.48 2 2 6.48 2 12s4.48 10 10 10 10-4.48 10-10S17.52 2 12 2zm0 18c-4.41 0-8-3.59-8-8s3.59-8 8-8 8 3.59 8 8-3.59 8-8 8zm-2-8c0-1.1.9-2 2-2s2 .9 2 2-.9 2-2 2-2-.9-2-2z',
};

interface SyncDetailPageProps {
  params: Promise<{ platform: string }>;
}

function formatLastSync(dateStr: string | null): string {
  if (!dateStr) return 'Never';
  const date = new Date(dateStr);
  const now = new Date();
  const diffMs = now.getTime() - date.getTime();
  const diffMins = Math.floor(diffMs / 60000);
  const diffHours = Math.floor(diffMins / 60);
  const diffDays = Math.floor(diffHours / 24);

  if (diffMins < 1) return 'Just now';
  if (diffMins < 60) return `${diffMins}m ago`;
  if (diffHours < 24) return `${diffHours}h ago`;
  if (diffDays < 7) return `${diffDays}d ago`;
  return date.toLocaleDateString();
}

function SyncDetailPageSkeleton() {
  return (
    <div className="space-y-6">
      {/* Header Skeleton */}
      <div className="flex items-center justify-between">
        <Skeleton className="h-10 w-32" />
        <Skeleton className="h-10 w-28" />
      </div>

      {/* Platform Header Skeleton */}
      <Card>
        <CardHeader>
          <div className="flex items-center gap-4">
            <Skeleton className="h-16 w-16 rounded-lg" />
            <div className="space-y-2">
              <Skeleton className="h-6 w-32" />
              <Skeleton className="h-4 w-48" />
            </div>
          </div>
        </CardHeader>
      </Card>

      {/* Configuration Skeleton */}
      <Card>
        <CardHeader>
          <Skeleton className="h-5 w-32" />
        </CardHeader>
        <CardContent className="space-y-4">
          <div className="flex items-center justify-between">
            <Skeleton className="h-4 w-24" />
            <Skeleton className="h-6 w-10" />
          </div>
          <div className="flex items-center justify-between">
            <Skeleton className="h-4 w-28" />
            <Skeleton className="h-9 w-36" />
          </div>
          <div className="flex items-center justify-between">
            <Skeleton className="h-4 w-28" />
            <Skeleton className="h-6 w-10" />
          </div>
        </CardContent>
      </Card>
    </div>
  );
}

export default function SyncDetailPage({ params }: SyncDetailPageProps) {
  const { platform: platformParam } = use(params);
  const platform = platformParam as SyncPlatform;

  // Local state for optimistic updates
  const [localEnabled, setLocalEnabled] = useState<boolean | null>(null);
  const [localFrequency, setLocalFrequency] = useState<SyncFrequency | null>(null);
  const [localAutoAdd, setLocalAutoAdd] = useState<boolean | null>(null);

  // State for review functionality
  const [selectedItem, setSelectedItem] = useState<JobItem | null>(null);
  const [processingItemId, setProcessingItemId] = useState<string | null>(null);
  const [searchQuery, setSearchQuery] = useState('');

  // Validate platform - only allow supported platforms
  if (!SUPPORTED_SYNC_PLATFORMS.includes(platform)) {
    notFound();
  }

  // Fetch sync config and status
  const { data: config, isLoading: configLoading, error: configError } = useSyncConfig(platform);
  const { data: status, isLoading: statusLoading } = useSyncStatus(platform);

  // Fetch job details if there's an active job
  const { data: activeJob } = useJob(status?.activeJobId ?? undefined, {
    enabled: !!status?.activeJobId,
  });

  // Fetch items needing review for the active job
  const { data: reviewData, isLoading: reviewLoading } = useJobItems(
    activeJob?.id ?? '',
    JobItemStatus.PENDING_REVIEW,
    1,
    20,
    { enabled: !!activeJob?.id }
  );
  const pendingReviewCount = reviewData?.total ?? 0;

  // Mutations
  const { mutateAsync: updateConfig, isPending: isUpdating } = useUpdateSyncConfig();
  const { mutateAsync: triggerSync, isPending: isTriggeringSyncPending } = useTriggerSync();
  const { mutateAsync: cancelJob, isPending: isCancelling } = useCancelJob();

  // Job item mutations
  const resolveMutation = useResolveJobItem();
  const skipMutation = useSkipJobItem();
  const { data: searchResults, isLoading: isSearching, error: searchError } = useSearchIGDB(searchQuery);

  const platformInfo = getPlatformDisplayInfo(platform);
  const isLoading = configLoading || statusLoading;
  const isSyncing = isTriggeringSyncPending || status?.isSyncing;

  // Use local state if set, otherwise use config values
  const effectiveEnabled = localEnabled ?? config?.enabled ?? false;
  const effectiveFrequency = localFrequency ?? config?.frequency ?? SyncFrequency.DAILY;
  const effectiveAutoAdd = localAutoAdd ?? config?.autoAdd ?? false;

  const handleUpdateConfig = async (data: SyncConfigUpdateData) => {
    try {
      await updateConfig({ platform, data });
      toast.success('Settings updated');
    } catch (err) {
      const message = err instanceof Error ? err.message : 'Failed to update settings';
      toast.error(message);
      // Reset local state on error
      if (data.enabled !== undefined) setLocalEnabled(null);
      if (data.frequency !== undefined) setLocalFrequency(null);
      if (data.autoAdd !== undefined) setLocalAutoAdd(null);
    }
  };

  const handleEnabledChange = async (enabled: boolean) => {
    setLocalEnabled(enabled);
    await handleUpdateConfig({ enabled });
  };

  const handleFrequencyChange = async (frequency: SyncFrequency) => {
    setLocalFrequency(frequency);
    await handleUpdateConfig({ frequency });
  };

  const handleAutoAddChange = async (autoAdd: boolean) => {
    setLocalAutoAdd(autoAdd);
    await handleUpdateConfig({ autoAdd });
  };

  const handleTriggerSync = async () => {
    try {
      await triggerSync(platform);
      toast.success(`${platformInfo.name} sync started`);
    } catch (err) {
      const message = err instanceof Error ? err.message : 'Failed to start sync';
      toast.error(message);
    }
  };

  const handleCancelJob = async () => {
    if (!activeJob) return;
    try {
      await cancelJob(activeJob.id);
      toast.success('Sync cancelled');
    } catch (err) {
      const message = err instanceof Error ? err.message : 'Failed to cancel sync';
      toast.error(message);
    }
  };

  // Review handlers
  const handleMatch = useCallback(
    async (item: JobItem, igdbId: number) => {
      setProcessingItemId(item.id);
      try {
        await resolveMutation.mutateAsync({ itemId: item.id, igdbId });
        toast.success(`Matched "${item.sourceTitle}" to IGDB`);
      } catch (err) {
        toast.error(err instanceof Error ? err.message : 'Failed to match item');
      } finally {
        setProcessingItemId(null);
      }
    },
    [resolveMutation]
  );

  const handleSkip = useCallback(
    async (item: JobItem) => {
      setProcessingItemId(item.id);
      try {
        await skipMutation.mutateAsync({ itemId: item.id });
        toast.success(`Skipped "${item.sourceTitle}"`);
      } catch (err) {
        toast.error(err instanceof Error ? err.message : 'Failed to skip item');
      } finally {
        setProcessingItemId(null);
      }
    },
    [skipMutation]
  );

  const handleView = useCallback((item: JobItem) => {
    setSelectedItem(item);
  }, []);

  const handleModalMatch = useCallback(
    async (igdbId: number) => {
      if (!selectedItem) return;
      setProcessingItemId(selectedItem.id);
      try {
        await resolveMutation.mutateAsync({ itemId: selectedItem.id, igdbId });
        toast.success(`Matched "${selectedItem.sourceTitle}" to IGDB`);
        setSelectedItem(null);
      } catch (err) {
        toast.error(err instanceof Error ? err.message : 'Failed to match item');
      } finally {
        setProcessingItemId(null);
      }
    },
    [selectedItem, resolveMutation]
  );

  const handleModalSkip = useCallback(async () => {
    if (!selectedItem) return;
    setProcessingItemId(selectedItem.id);
    try {
      await skipMutation.mutateAsync({ itemId: selectedItem.id });
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
        await resolveMutation.mutateAsync({ itemId: selectedItem.id, igdbId });
        toast.success(`Matched "${selectedItem.sourceTitle}" to IGDB`);
        setSelectedItem(null);
        setSearchQuery('');
      } catch (err) {
        toast.error(err instanceof Error ? err.message : 'Failed to match item');
      } finally {
        setProcessingItemId(null);
      }
    },
    [selectedItem, resolveMutation]
  );

  if (isLoading) {
    return <SyncDetailPageSkeleton />;
  }

  if (configError || !config) {
    return (
      <div className="space-y-6">
        {/* Header */}
        <div className="flex items-center gap-4">
          <Button variant="outline" asChild>
            <Link href="/sync">
              <ArrowLeft className="mr-2 h-4 w-4" />
              Back to Sync
            </Link>
          </Button>
        </div>

        <Alert variant="destructive">
          <AlertCircle className="h-4 w-4" />
          <AlertTitle>Error</AlertTitle>
          <AlertDescription>
            Failed to load sync configuration for {platformInfo.name}. Please try again later.
          </AlertDescription>
        </Alert>
      </div>
    );
  }

  return (
    <div className="space-y-6">
      {/* Navigation Header */}
      <div className="flex items-center justify-between">
        <nav className="flex items-center text-sm text-muted-foreground">
          <Link href="/dashboard" className="hover:text-foreground">
            Dashboard
          </Link>
          <span className="mx-2">/</span>
          <Link href="/sync" className="hover:text-foreground">
            Sync
          </Link>
          <span className="mx-2">/</span>
          <span className="text-foreground">{platformInfo.name}</span>
        </nav>

        <Button onClick={handleTriggerSync} disabled={!effectiveEnabled || isSyncing}>
          {isSyncing ? (
            <>
              <Loader2 className="mr-2 h-4 w-4 animate-spin" />
              Syncing...
            </>
          ) : (
            <>
              <RefreshCw className="mr-2 h-4 w-4" />
              Sync Now
            </>
          )}
        </Button>
      </div>

      {/* Platform Header */}
      <Card>
        <CardHeader>
          <div className="flex items-center gap-4">
            <div
              className={`flex h-16 w-16 items-center justify-center rounded-lg ${platformInfo.bgColor}`}
            >
              <svg className={`h-10 w-10 ${platformInfo.color}`} viewBox="0 0 24 24" fill="currentColor">
                <path d={PLATFORM_ICONS[platform]} />
              </svg>
            </div>
            <div>
              <CardTitle className="text-2xl">{platformInfo.name}</CardTitle>
              <CardDescription className="flex items-center gap-2 mt-1">
                <Clock className="h-4 w-4" />
                Last synced: {formatLastSync(config.lastSyncedAt)}
              </CardDescription>
            </div>
            <div className="ml-auto">
              <Badge variant={effectiveEnabled ? 'default' : 'secondary'} className="text-sm">
                {effectiveEnabled ? 'Enabled' : 'Disabled'}
              </Badge>
            </div>
          </div>
        </CardHeader>
      </Card>

      {/* Active Sync Progress */}
      {isSyncing && activeJob && (
        <div className="space-y-4">
          <JobProgressCard job={activeJob} onCancel={handleCancelJob} isCancelling={isCancelling} />

          {activeJob.progress && (
            <JobItemsDetails jobId={activeJob.id} progress={activeJob.progress} />
          )}
        </div>
      )}

      {/* Configuration Section */}
      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2">
            <Settings className="h-5 w-5" />
            Configuration
          </CardTitle>
          <CardDescription>
            Configure how {platformInfo.name} syncs with your collection
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-6">
          {/* Enable Toggle */}
          <div className="flex items-center justify-between">
            <div>
              <div className="font-medium">Enable Sync</div>
              <div className="text-sm text-muted-foreground">
                Allow automatic syncing of your {platformInfo.name} library
              </div>
            </div>
            <Switch
              checked={effectiveEnabled}
              onCheckedChange={handleEnabledChange}
              disabled={isUpdating}
            />
          </div>

          {/* Frequency Select */}
          <div className="flex items-center justify-between">
            <div>
              <div className="font-medium">Sync Frequency</div>
              <div className="text-sm text-muted-foreground">
                How often to automatically sync
              </div>
            </div>
            <Select
              value={effectiveFrequency}
              onValueChange={(value) => handleFrequencyChange(value as SyncFrequency)}
              disabled={!effectiveEnabled || isUpdating}
            >
              <SelectTrigger className="w-[160px]">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {Object.values(SyncFrequency).map((freq) => (
                  <SelectItem key={freq} value={freq}>
                    {getSyncFrequencyLabel(freq)}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>

          {/* Auto-add Toggle */}
          <div className="flex items-center justify-between">
            <div>
              <div className="font-medium">Auto-add Games</div>
              <div className="text-sm text-muted-foreground">
                Automatically add matched games to your collection without review
              </div>
            </div>
            <Switch
              checked={effectiveAutoAdd}
              onCheckedChange={handleAutoAddChange}
              disabled={!effectiveEnabled || isUpdating}
            />
          </div>
        </CardContent>
      </Card>

      {/* Review Items Section */}
      <Card>
        <CardHeader>
          <div className="flex items-center justify-between">
            <div>
              <CardTitle className="flex items-center gap-2">
                <ClipboardCheck className="h-5 w-5" />
                Items Needing Review
              </CardTitle>
              <CardDescription>
                Games from this platform that need your review before being added
              </CardDescription>
            </div>
            {pendingReviewCount > 0 && (
              <Badge variant="secondary">{pendingReviewCount} pending</Badge>
            )}
          </div>
        </CardHeader>
        <CardContent>
          {reviewLoading ? (
            <div className="space-y-4">
              <Skeleton className="h-32" />
              <Skeleton className="h-32" />
            </div>
          ) : pendingReviewCount === 0 ? (
            <div className="flex flex-col items-center justify-center py-8 text-center text-muted-foreground">
              <Check className="h-12 w-12 mb-4 text-green-500" />
              <p>All caught up!</p>
              <p className="text-sm">No items need review right now.</p>
            </div>
          ) : (
            <div className="space-y-4">
              {reviewData?.items.map((item) => (
                <ReviewItemCard
                  key={item.id}
                  item={item}
                  onMatch={handleMatch}
                  onSkip={handleSkip}
                  onView={handleView}
                  isProcessing={processingItemId === item.id}
                />
              ))}
            </div>
          )}
        </CardContent>
      </Card>

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

      {/* Recent Sync Activity */}
      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2">
            <Clock className="h-5 w-5" />
            Recent Activity
          </CardTitle>
        </CardHeader>
        <CardContent>
          {config.lastSyncedAt ? (
            <div className="space-y-2">
              <div className="flex items-center justify-between rounded-lg border p-4">
                <div>
                  <div className="font-medium">Last Sync</div>
                  <div className="text-sm text-muted-foreground">
                    {new Date(config.lastSyncedAt).toLocaleString()}
                  </div>
                </div>
                <Badge variant="outline">{formatLastSync(config.lastSyncedAt)}</Badge>
              </div>
            </div>
          ) : (
            <div className="flex flex-col items-center justify-center py-8 text-center text-muted-foreground">
              <Clock className="h-12 w-12 mb-4 opacity-50" />
              <p>No sync history yet</p>
              <p className="text-sm mt-1">
                Start your first sync to see activity here
              </p>
            </div>
          )}
        </CardContent>
      </Card>
    </div>
  );
}

// Helper components for IGDB candidate selection
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
