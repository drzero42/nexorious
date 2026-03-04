import { useState } from 'react';
import { createFileRoute, Link } from '@tanstack/react-router';
import { toast } from 'sonner';
import { useQueryClient } from '@tanstack/react-query';
import {
  syncKeys,
  useSyncConfig,
  useSyncStatus,
  useUpdateSyncConfig,
  useTriggerSync,
  useJob,
  useCancelJob,
  usePSNStatus,
} from '@/hooks';
import { useCurrentUser, authKeys } from '@/hooks/use-auth';
import { SteamConnectionCard, EpicConnectionCard, PSNConnectionCard, RecentActivity } from '@/components/sync';
import {
  SyncPlatform,
  SyncFrequency,
  SUPPORTED_SYNC_PLATFORMS,
  getPlatformDisplayInfo,
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
import { RefreshCw, Loader2, AlertCircle, Settings, Clock } from 'lucide-react';
import { config as envConfig } from '@/lib/env';

export const Route = createFileRoute('/_authenticated/sync/$platform')({
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
  const { platform: platformParam } = Route.useParams();
  const platform = platformParam as SyncPlatform;
  const queryClient = useQueryClient();

  // Local state for optimistic updates
  const [localFrequency, setLocalFrequency] = useState<SyncFrequency | null>(null);
  const [localAutoAdd, setLocalAutoAdd] = useState<boolean | null>(null);

  const isValidPlatform = SUPPORTED_SYNC_PLATFORMS.includes(platform);

  // Get current user for Steam preferences (must be called before any conditional return)
  const { data: currentUser } = useCurrentUser();

  // Extract Steam credentials from user preferences
  const steamPrefs = currentUser?.preferences?.steam as {
    steam_id?: string;
    username?: string;
  } | undefined;

  // Extract Epic credentials from user preferences
  const epicPrefs = currentUser?.preferences?.epic as
    | {
        display_name?: string;
        account_id?: string;
      }
    | undefined;

  // Extract PSN credentials from user preferences
  const psnPrefs = currentUser?.preferences?.psn as
    | {
        online_id?: string;
        account_id?: string;
        region?: string;
      }
    | undefined;

  // Fetch sync config and status
  const { data: config, isLoading: configLoading, error: configError } = useSyncConfig(platform);
  const { data: status, isLoading: statusLoading } = useSyncStatus(platform);

  // Fetch PSN-specific status
  const { data: psnStatus } = usePSNStatus();

  // Fetch job details if there's an active job
  const { data: activeJob } = useJob(status?.activeJobId ?? undefined, {
    enabled: !!status?.activeJobId,
  });

  // Mutations
  const { mutateAsync: updateConfig, isPending: isUpdating } = useUpdateSyncConfig();
  const { mutateAsync: triggerSync, isPending: isTriggeringSyncPending } = useTriggerSync();
  const { mutateAsync: cancelJob, isPending: isCancelling } = useCancelJob();

  const platformInfo = getPlatformDisplayInfo(platform);
  const isLoading = configLoading || statusLoading;
  const isSyncing = isTriggeringSyncPending || status?.isSyncing;

  // Use local state if set, otherwise use config values
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

  // Validate platform - only allow supported platforms
  if (!isValidPlatform) {
    return (
      <div className="text-center py-12">
        <h3 className="mt-4 text-lg font-medium">Platform not found</h3>
        <p className="mt-2 text-sm text-muted-foreground">
          The sync platform &quot;{platformParam}&quot; is not supported.
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
                variant="outline"
                className={
                  config.isConfigured
                    ? 'bg-green-100 text-green-800 dark:bg-green-900/30 dark:text-green-400'
                    : 'bg-muted text-muted-foreground'
                }
              >
                {config.isConfigured ? 'Configured' : 'Not Configured'}
              </Badge>
            </div>
          </div>
        </CardHeader>
      </Card>

      {/* Steam Connection Card - only show for Steam platform */}
      {platform === SyncPlatform.STEAM && (
        <SteamConnectionCard
          isConfigured={config.isConfigured}
          steamId={steamPrefs?.steam_id}
          steamUsername={steamPrefs?.username}
          onConnectionChange={() => {
            // Invalidate queries to refresh data
            queryClient.invalidateQueries({ queryKey: syncKeys.config(platform) });
            queryClient.invalidateQueries({ queryKey: authKeys.me() });
          }}
        />
      )}

      {/* Epic Connection Card - only show for Epic platform */}
      {platform === SyncPlatform.EPIC && (
        <EpicConnectionCard
          isConfigured={config.isConfigured}
          displayName={epicPrefs?.display_name}
          accountId={epicPrefs?.account_id}
          onConnectionChange={() => {
            queryClient.invalidateQueries({ queryKey: syncKeys.config(platform) });
            queryClient.invalidateQueries({ queryKey: authKeys.me() });
          }}
        />
      )}

      {/* PSN Connection Card - only show for PSN platform */}
      {platform === SyncPlatform.PSN && (
        <PSNConnectionCard
          isConfigured={config.isConfigured}
          tokenExpired={psnStatus?.tokenExpired ?? false}
          onlineId={psnPrefs?.online_id}
          accountId={psnPrefs?.account_id}
          onConnectionChange={() => {
            queryClient.invalidateQueries({ queryKey: syncKeys.config(platform) });
            queryClient.invalidateQueries({ queryKey: syncKeys.psnStatus() });
            queryClient.invalidateQueries({ queryKey: authKeys.me() });
          }}
        />
      )}

      {/* Active Sync Progress */}
      {isSyncing && activeJob && (
        <div className="space-y-4">
          <JobProgressCard job={activeJob} onCancel={handleCancelJob} isCancelling={isCancelling} />

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

      {/* Recent Sync Activity */}
      <RecentActivity platform={platform} />
    </div>
  );
}
