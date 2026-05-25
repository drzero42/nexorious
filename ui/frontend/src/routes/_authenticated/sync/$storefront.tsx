import { useEffect, useRef, useState } from 'react';
import { createFileRoute, Link } from '@tanstack/react-router';
import { toast } from 'sonner';
import { useQueryClient } from '@tanstack/react-query';
import {
  syncKeys,
  useSyncConfig,
  useSyncStatus,
  useUpdateSyncConfig,
  useTriggerSync,
  useResetSyncData,
  useJob,
  useCancelJob,
  usePSNStatus,
  useSteamConnection,
  useEpicConnection,
  useGOGConnection,
  jobsKeys,
} from '@/hooks';
import { retryFailedItems } from '@/api/jobs';
import { useCurrentUser, authKeys } from '@/hooks/use-auth';
import { SteamConnectionCard, EpicConnectionCard, GOGConnectionCard, PSNConnectionCard, RecentActivity, ExternalGamesSection } from '@/components/sync';
import {
  SyncStorefront,
  SyncFrequency,
  SUPPORTED_SYNC_STOREFRONTS,
  getStorefrontDisplayInfo,
  getSyncFrequencyLabel,
} from '@/types';
import type { SyncConfigUpdateData } from '@/types';
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
import { Badge } from '@/components/ui/badge';
import { Skeleton } from '@/components/ui/skeleton';
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert';
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
  AlertDialogTrigger,
} from '@/components/ui/alert-dialog';
import { RefreshCw, Loader2, AlertCircle, Settings, Clock, ChevronDown } from 'lucide-react';
import { config as envConfig } from '@/lib/env';
import { Collapsible, CollapsibleContent } from '@/components/ui/collapsible';

export const Route = createFileRoute('/_authenticated/sync/$storefront')({
  component: SyncDetailPage,
});

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

function SyncDetailPage() {
  const { storefront: storefrontParam } = Route.useParams();
  const storefront = storefrontParam as SyncStorefront;
  const queryClient = useQueryClient();

  // Local state for optimistic updates
  const [localFrequency, setLocalFrequency] = useState<SyncFrequency | null>(null);
  const [localAutoAdd, setLocalAutoAdd] = useState<boolean | null>(null);

  const isValidPlatform = SUPPORTED_SYNC_STOREFRONTS.includes(storefront);

  // Get current user for PSN/GOG preferences (must be called before any conditional return)
  const { data: currentUser } = useCurrentUser();

  // Extract PSN credentials from user preferences
  const psnPrefs = currentUser?.preferences?.psn as
    | {
        online_id?: string;
        account_id?: string;
        region?: string;
      }
    | undefined;

  // Fetch sync config and status
  const { data: config, isLoading: configLoading, error: configError } = useSyncConfig(storefront);
  const { data: status, isLoading: statusLoading } = useSyncStatus(storefront);

  // Fetch PSN-specific status
  const { data: psnStatus } = usePSNStatus();

  // Fetch Steam connection status
  const { data: steamConnection } = useSteamConnection();

  // Fetch Epic and GOG connection status
  const { data: epicConnection } = useEpicConnection();
  const { data: gogConnection } = useGOGConnection();

  // Fetch job details if there's an active job
  const { data: activeJob } = useJob(status?.activeJobId ?? undefined, {
    enabled: !!status?.activeJobId,
  });

  const invalidatedJobRef = useRef<string | undefined>(undefined);
  useEffect(() => {
    if (activeJob?.isTerminal && activeJob.id !== invalidatedJobRef.current) {
      invalidatedJobRef.current = activeJob.id;
      queryClient.invalidateQueries({ queryKey: jobsKeys.recent(storefront) });
    }
  }, [activeJob?.isTerminal, activeJob?.id, storefront, queryClient]);

  // Mutations
  const { mutateAsync: updateConfig, isPending: isUpdating } = useUpdateSyncConfig();
  const { mutateAsync: triggerSync, isPending: isTriggeringSyncPending } = useTriggerSync();
  const { mutateAsync: resetSync, isPending: isResetting } = useResetSyncData();
  const { mutateAsync: cancelJob, isPending: isCancelling } = useCancelJob();
  const [isRetrying, setIsRetrying] = useState(false);
  const [resetDialogOpen, setResetDialogOpen] = useState(false);
  const wasResettingRef = useRef(false);

  // Close the confirmation dialog once the reset mutation has settled.
  useEffect(() => {
    if (wasResettingRef.current && !isResetting) {
      setResetDialogOpen(false);
    }
    wasResettingRef.current = isResetting;
  }, [isResetting]);

  const platformInfo = getStorefrontDisplayInfo(storefront);
  const isLoading = configLoading || statusLoading;
  const isSyncing = isTriggeringSyncPending || status?.isSyncing;

  // Use local state if set, otherwise use config values
  const effectiveFrequency = localFrequency ?? config?.frequency ?? SyncFrequency.DAILY;
  const effectiveAutoAdd = localAutoAdd ?? config?.autoAdd ?? false;

  // Derive credentials error state from storefront-specific connection data
  const credentialsError =
    (storefront === SyncStorefront.STEAM && (steamConnection?.credentialsError ?? false)) ||
    (storefront === SyncStorefront.PSN && (psnStatus?.credentialsError ?? false)) ||
    (storefront === SyncStorefront.EPIC && (epicConnection?.credentialsError ?? false)) ||
    (storefront === SyncStorefront.GOG && (gogConnection?.credentialsError ?? false));

  // Connection section is open by default when not configured or when there's a credentials error
  const [connectionSectionOpen, setConnectionSectionOpen] = useState(
    () => !config?.isConfigured || credentialsError
  );

  const handleUpdateConfig = async (data: SyncConfigUpdateData) => {
    try {
      await updateConfig({ storefront, data });
      toast.success('Settings updated');
    } catch (err) {
      const message = err instanceof Error ? err.message : 'Failed to update settings';
      toast.error(message);
      // Reset local state on error
      if (data.frequency !== undefined) setLocalFrequency(null);
      if (data.autoAdd !== undefined) setLocalAutoAdd(null);
    }
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
      await triggerSync(storefront);
      toast.success(`${platformInfo.name} sync started`);
    } catch (err) {
      const message = err instanceof Error ? err.message : 'Failed to start sync';
      toast.error(message);
    }
  };

  const handleReset = async (event: React.MouseEvent<HTMLButtonElement>) => {
    // Prevent Radix from auto-closing the dialog — the useEffect above
    // closes it after the mutation settles.
    event.preventDefault();
    try {
      await resetSync(storefront);
      toast.success(`${platformInfo.name} sync data reset`);
    } catch (err) {
      const message = err instanceof Error ? err.message : 'Failed to reset sync data';
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

  const handleRetryIGDBErrors = async () => {
    if (!activeJob) return;
    setIsRetrying(true);
    try {
      await retryFailedItems(activeJob.id);
      await queryClient.invalidateQueries({ queryKey: jobsKeys.all });
      toast.success('IGDB errors re-queued for retry');
    } catch (err) {
      const message = err instanceof Error ? err.message : 'Failed to retry IGDB errors';
      toast.error(message);
    } finally {
      setIsRetrying(false);
    }
  };

  // Validate storefront - only allow supported storefronts
  if (!isValidPlatform) {
    return (
      <div className="text-center py-12">
        <h3 className="mt-4 text-lg font-medium">Storefront not found</h3>
        <p className="mt-2 text-sm text-muted-foreground">
          The sync storefront &quot;{storefrontParam}&quot; is not supported.
        </p>
        <div className="mt-6">
          <Link to="/sync">
            <Button>Back to Sync</Button>
          </Link>
        </div>
      </div>
    );
  }

  if (isLoading) {
    return <SyncDetailPageSkeleton />;
  }

  if (configError || !config) {
    return (
      <div className="space-y-6">
        {/* Header */}
        <div className="flex items-center gap-4">
          <Button variant="outline" asChild>
            <Link to="/sync">
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
          <Link to="/dashboard" className="hover:text-foreground">
            Dashboard
          </Link>
          <span className="mx-2">/</span>
          <Link to="/sync" className="hover:text-foreground">
            Sync
          </Link>
          <span className="mx-2">/</span>
          <span className="text-foreground">{platformInfo.name}</span>
        </nav>

        <div className="flex items-center gap-2">
          {config.isConfigured && (
            <AlertDialog
              open={resetDialogOpen}
              onOpenChange={(open) => {
                if (!open && isResetting) return;
                setResetDialogOpen(open);
              }}
            >
              <AlertDialogTrigger asChild>
                <Button variant="ghost" disabled={isResetting}>
                  Reset
                </Button>
              </AlertDialogTrigger>
              <AlertDialogContent>
                <AlertDialogHeader>
                  <AlertDialogTitle>Reset sync data?</AlertDialogTitle>
                  <AlertDialogDescription>
                    This will remove all imported games and match history for {platformInfo.name}.
                    Your game library entries will not be deleted. This cannot be undone.
                  </AlertDialogDescription>
                </AlertDialogHeader>
                <AlertDialogFooter>
                  <AlertDialogCancel disabled={isResetting}>Cancel</AlertDialogCancel>
                  <AlertDialogAction onClick={handleReset} disabled={isResetting}>
                    {isResetting ? (
                      <>
                        <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                        Resetting...
                      </>
                    ) : (
                      'Reset'
                    )}
                  </AlertDialogAction>
                </AlertDialogFooter>
              </AlertDialogContent>
            </AlertDialog>
          )}
          <Button onClick={handleTriggerSync} disabled={isSyncing || !config.isConfigured}>
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
      </div>

      {/* Platform Header */}
      <Card>
        <CardHeader>
          <div className="flex items-center gap-4">
            <div
              className={`flex h-16 w-16 items-center justify-center rounded-lg ${platformInfo.bgColor}`}
            >
              <img
                src={`${envConfig.staticUrl}${platformInfo.iconUrl}`}
                alt={`${platformInfo.name} icon`}
                loading="lazy"
                className="h-10 w-10"
              />
            </div>
            <div>
              <CardTitle className="text-2xl">{platformInfo.name}</CardTitle>
              <CardDescription className="flex items-center gap-2 mt-1">
                <Clock className="h-4 w-4" />
                Last synced: {formatLastSync(config.lastSyncedAt)}
              </CardDescription>
            </div>
            <div className="ml-auto">
              <Badge
                variant={credentialsError ? 'destructive' : 'outline'}
                className={
                  credentialsError
                    ? 'cursor-pointer'
                    : config.isConfigured
                      ? 'cursor-pointer bg-green-100 text-green-800 dark:bg-green-900/30 dark:text-green-400'
                      : 'cursor-pointer bg-muted text-muted-foreground'
                }
                onClick={() => setConnectionSectionOpen((o) => !o)}
              >
                {credentialsError ? (
                  <>
                    Credentials Error
                    <ChevronDown
                      className={`ml-1 h-3 w-3 transition-transform ${connectionSectionOpen ? 'rotate-180' : ''}`}
                    />
                  </>
                ) : config.isConfigured ? (
                  <>
                    Connected
                    <ChevronDown
                      className={`ml-1 h-3 w-3 transition-transform ${connectionSectionOpen ? 'rotate-180' : ''}`}
                    />
                  </>
                ) : (
                  <>
                    Not Configured
                    <ChevronDown
                      className={`ml-1 h-3 w-3 transition-transform ${connectionSectionOpen ? 'rotate-180' : ''}`}
                    />
                  </>
                )}
              </Badge>
            </div>
          </div>
        </CardHeader>
      </Card>

      {/* Connection Cards - collapsible, open by default when not configured or credentials error */}
      <Collapsible open={connectionSectionOpen} onOpenChange={setConnectionSectionOpen}>
        <CollapsibleContent>
          {/* Steam Connection Card - only show for Steam storefront */}
          {storefront === SyncStorefront.STEAM && (
            <SteamConnectionCard
              isConfigured={config.isConfigured}
              credentialsError={steamConnection?.credentialsError ?? false}
              steamId={steamConnection?.steamId}
              steamUsername={steamConnection?.username}
              onConnectionChange={() => {
                // Invalidate queries to refresh data
                queryClient.invalidateQueries({ queryKey: syncKeys.config(storefront) });
                queryClient.invalidateQueries({ queryKey: syncKeys.steamConnection() });
                queryClient.invalidateQueries({ queryKey: authKeys.me() });
              }}
            />
          )}

          {/* Epic Connection Card - only show for Epic storefront */}
          {storefront === SyncStorefront.EPIC && (
            <EpicConnectionCard
              isConfigured={config.isConfigured}
              onConnectionChange={() => {
                queryClient.invalidateQueries({ queryKey: syncKeys.config(storefront) });
                queryClient.invalidateQueries({ queryKey: authKeys.me() });
              }}
            />
          )}

          {/* GOG Connection Card - only show for GOG storefront */}
          {storefront === SyncStorefront.GOG && (
            <GOGConnectionCard
              isConfigured={!!config?.isConfigured}
              onConnectionChange={() => {
                queryClient.invalidateQueries({ queryKey: syncKeys.gogConnection() });
              }}
            />
          )}

          {/* PSN Connection Card - only show for PSN storefront */}
          {storefront === SyncStorefront.PSN && (
            <PSNConnectionCard
              isConfigured={config.isConfigured}
              credentialsError={psnStatus?.credentialsError ?? false}
              onlineId={psnPrefs?.online_id}
              accountId={psnPrefs?.account_id}
              onConnectionChange={() => {
                queryClient.invalidateQueries({ queryKey: syncKeys.config(storefront) });
                queryClient.invalidateQueries({ queryKey: syncKeys.psnStatus() });
                queryClient.invalidateQueries({ queryKey: authKeys.me() });
              }}
            />
          )}
        </CollapsibleContent>
      </Collapsible>

      {/* Active Sync Progress */}
      {isSyncing && activeJob && (
        <div className="space-y-4">
          <JobProgressCard
            job={activeJob}
            onCancel={handleCancelJob}
            isCancelling={isCancelling}
            onRetry={handleRetryIGDBErrors}
            isRetrying={isRetrying}
          />

          {activeJob.progress && (
            <JobItemsDetails jobId={activeJob.id} progress={activeJob.progress} isTerminal={activeJob.isTerminal} />
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
              disabled={isUpdating || !config.isConfigured}
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
              disabled={isUpdating || !config.isConfigured}
            />
          </div>
        </CardContent>
      </Card>

      {/* External Games Library */}
      <ExternalGamesSection storefront={storefront} />

      {/* Recent Sync Activity */}
      <RecentActivity platform={storefront} />
    </div>
  );
}
