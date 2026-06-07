import { useCallback, useEffect, useRef, useState } from 'react';
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
  useHumbleStatus,
  jobsKeys,
  useJobCompletionEffect,
  useStorefront,
} from '@/hooks';
import {
  SteamConnectionCard,
  EpicConnectionCard,
  GOGConnectionCard,
  PSNConnectionCard,
  HumbleConnectionCard,
  ExternalGamesSection,
} from '@/components/sync';
import { RecentActivity } from '@/components/jobs';
import {
  SyncStorefront,
  SyncFrequency,
  SUPPORTED_SYNC_STOREFRONTS,
  getSyncFrequencyLabel,
} from '@/types';
import type { SyncConfigUpdateData } from '@/types';
import { JobProgressCard } from '@/components/jobs';
import { Card, CardContent, CardHeader, CardTitle, CardDescription } from '@/components/ui/card';
import { Button } from '@/components/ui/button';
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
import { RefreshCw, Loader2, AlertCircle, Clock, ChevronDown } from 'lucide-react';
import { config as envConfig } from '@/lib/env';
import { Collapsible, CollapsibleContent } from '@/components/ui/collapsible';
import { formatRelativeTime } from '@/types/jobs';

export const Route = createFileRoute('/_authenticated/sync/$storefront')({
  head: () => ({ meta: [{ title: 'Sync | Nexorious' }] }),
  component: SyncDetailPage,
});

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

  const isValidPlatform = SUPPORTED_SYNC_STOREFRONTS.includes(storefront);

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

  // Fetch Humble Bundle status
  const { data: humbleStatus } = useHumbleStatus();

  // Fetch job details if there's an active job
  const { data: activeJob } = useJob(status?.activeJobId ?? undefined, {
    enabled: !!status?.activeJobId,
  });

  const handleSyncComplete = useCallback(() => {
    queryClient.invalidateQueries({ queryKey: jobsKeys.recents() });
    queryClient.invalidateQueries({ queryKey: syncKeys.externalGames(storefront) });
  }, [queryClient, storefront]);
  useJobCompletionEffect(status?.activeJobId, handleSyncComplete);

  // Mutations
  const { mutateAsync: updateConfig, isPending: isUpdating } = useUpdateSyncConfig();
  const { mutateAsync: triggerSync, isPending: isTriggeringSyncPending } = useTriggerSync();
  const { mutateAsync: resetSync, isPending: isResetting } = useResetSyncData();
  const { mutateAsync: cancelJob, isPending: isCancelling } = useCancelJob();
  const [resetDialogOpen, setResetDialogOpen] = useState(false);
  const wasResettingRef = useRef(false);

  // Close the confirmation dialog once the reset mutation has settled.
  useEffect(() => {
    if (wasResettingRef.current && !isResetting) {
      setResetDialogOpen(false);
    }
    wasResettingRef.current = isResetting;
  }, [isResetting]);

  const { data: storefrontInfo } = useStorefront(storefront);
  const displayName = storefrontInfo?.display_name ?? storefront;
  const isLoading = configLoading || statusLoading;
  const isSyncing = isTriggeringSyncPending || status?.isSyncing;

  // Title and heading share one source (the catalog), so they cannot disagree.
  useEffect(() => {
    document.title = `${displayName} Sync | Nexorious`;
  }, [displayName]);

  // Use local state if set, otherwise use config values
  const effectiveFrequency = localFrequency ?? config?.frequency ?? SyncFrequency.DAILY;

  // Derive credentials error state from storefront-specific connection data
  const credentialsError =
    (storefront === SyncStorefront.STEAM && (steamConnection?.credentialsError ?? false)) ||
    (storefront === SyncStorefront.PLAYSTATION_STORE && (psnStatus?.credentialsError ?? false)) ||
    (storefront === SyncStorefront.EPIC_GAMES_STORE &&
      (epicConnection?.credentialsError ?? false)) ||
    (storefront === SyncStorefront.GOG && (gogConnection?.credentialsError ?? false)) ||
    (storefront === SyncStorefront.HUMBLE && (humbleStatus?.credentialsError ?? false));

  const [connectionSectionOpen, setConnectionSectionOpen] = useState(false);
  const connectionOpenInitialized = useRef(false);

  useEffect(() => {
    if (!connectionOpenInitialized.current && config !== undefined) {
      connectionOpenInitialized.current = true;
      setConnectionSectionOpen(!config.isConfigured || credentialsError);
    }
  }, [config, credentialsError]);

  useEffect(() => {
    if (!connectionOpenInitialized.current) return;
    const shouldBeOpen = !config?.isConfigured || credentialsError;
    setConnectionSectionOpen(shouldBeOpen);
  }, [config?.isConfigured, credentialsError]);

  const handleUpdateConfig = async (data: SyncConfigUpdateData) => {
    try {
      await updateConfig({ storefront, data });
      toast.success('Settings updated');
    } catch (err) {
      const message = err instanceof Error ? err.message : 'Failed to update settings';
      toast.error(message);
      // Reset local state on error
      if (data.frequency !== undefined) setLocalFrequency(null);
    }
  };

  const handleFrequencyChange = async (frequency: SyncFrequency) => {
    setLocalFrequency(frequency);
    await handleUpdateConfig({ frequency });
  };

  const handleTriggerSync = async () => {
    try {
      await triggerSync(storefront);
      toast.success(`${displayName} sync started`);
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
      toast.success(`${displayName} sync data reset`);
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
            <Link to="/sync">Back to Sync</Link>
          </Button>
        </div>

        <Alert variant="destructive">
          <AlertCircle className="h-4 w-4" />
          <AlertTitle>Error</AlertTitle>
          <AlertDescription>
            Failed to load sync configuration for {displayName}. Please try again later.
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
          <span className="text-foreground">{displayName}</span>
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
                    This will remove all imported games and match history for {displayName}. Your
                    game library entries will not be deleted. This cannot be undone.
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
            <div className="flex h-16 w-16 items-center justify-center rounded-lg bg-muted">
              {storefrontInfo?.icon_url && (
                <img
                  src={`${envConfig.staticUrl}${storefrontInfo.icon_url}`}
                  alt={`${displayName} icon`}
                  loading="lazy"
                  className="h-10 w-10"
                />
              )}
            </div>
            <div>
              <CardTitle className="text-2xl">
                {displayName}
                {(status?.externalGameCount ?? 0) > 0 && (
                  <span className="ml-2 text-base font-normal text-muted-foreground">
                    {status!.externalGameCount} games
                  </span>
                )}
              </CardTitle>
              <CardDescription className="flex items-center gap-2 mt-1">
                <Clock className="h-4 w-4" />
                Last synced: {formatRelativeTime(config.lastSyncedAt, 'Never')}
              </CardDescription>
            </div>
            <div className="ml-auto flex flex-col items-end gap-2">
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
              {config.isConfigured && (
                <Select
                  value={effectiveFrequency}
                  onValueChange={(value) => handleFrequencyChange(value as SyncFrequency)}
                  disabled={isUpdating}
                >
                  <SelectTrigger className="w-[140px]" aria-label="Sync frequency">
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
              )}
            </div>
          </div>
        </CardHeader>
      </Card>

      {/* Connection Cards - collapsible, open by default when not configured or credentials error */}
      <Collapsible open={connectionSectionOpen} onOpenChange={setConnectionSectionOpen}>
        <CollapsibleContent className="space-y-4">
          {/* Steam Connection Card - only show for Steam storefront */}
          {storefront === SyncStorefront.STEAM && (
            <SteamConnectionCard
              isConfigured={config.isConfigured}
              credentialsError={steamConnection?.credentialsError ?? false}
              steamUsername={steamConnection?.username}
              onConnectionChange={() => {
                // Invalidate queries to refresh data
                queryClient.invalidateQueries({ queryKey: syncKeys.config(storefront) });
                queryClient.invalidateQueries({ queryKey: syncKeys.steamConnection() });
              }}
            />
          )}

          {/* Epic Connection Card - only show for Epic storefront */}
          {storefront === SyncStorefront.EPIC_GAMES_STORE && (
            <EpicConnectionCard
              isConfigured={config.isConfigured}
              onConnectionChange={() => {
                queryClient.invalidateQueries({ queryKey: syncKeys.config(storefront) });
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
          {storefront === SyncStorefront.PLAYSTATION_STORE && (
            <PSNConnectionCard
              isConfigured={config.isConfigured}
              credentialsError={psnStatus?.credentialsError ?? false}
              onlineId={psnStatus?.onlineId ?? undefined}
              onConnectionChange={() => {
                queryClient.invalidateQueries({ queryKey: syncKeys.config(storefront) });
                queryClient.invalidateQueries({ queryKey: syncKeys.psnStatus() });
              }}
            />
          )}

          {/* Humble Bundle Connection Card - only show for Humble storefront */}
          {storefront === SyncStorefront.HUMBLE && (
            <HumbleConnectionCard
              isConfigured={config.isConfigured}
              credentialsError={humbleStatus?.credentialsError ?? false}
              onConnectionChange={() => {
                queryClient.invalidateQueries({ queryKey: syncKeys.config(storefront) });
                queryClient.invalidateQueries({ queryKey: syncKeys.humbleStatus() });
              }}
            />
          )}
        </CollapsibleContent>
      </Collapsible>

      {/* Active Sync Progress */}
      {isSyncing && activeJob && (
        <JobProgressCard job={activeJob} onCancel={handleCancelJob} isCancelling={isCancelling} />
      )}

      {/* External Games Library */}
      <ExternalGamesSection storefront={storefront} isSyncing={!!isSyncing} />

      {/* Recent Sync Activity */}
      <RecentActivity source={storefront} />
    </div>
  );
}
