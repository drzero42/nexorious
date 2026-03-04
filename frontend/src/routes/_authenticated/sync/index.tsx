import { createFileRoute, Link, useNavigate } from '@tanstack/react-router';
import { useSyncConfigs, useUpdateSyncConfig, useTriggerSync, useSyncStatus, usePendingReviewCount } from '@/hooks';
import { SyncServiceCard } from '@/components/sync';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { Skeleton } from '@/components/ui/skeleton';
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert';
import { AlertCircle, Info, ArrowRight } from 'lucide-react';
import { toast } from 'sonner';
import { SUPPORTED_SYNC_PLATFORMS, SyncPlatform, SyncFrequency } from '@/types';
import type { SyncConfig, SyncConfigUpdateData } from '@/types';

export const Route = createFileRoute('/_authenticated/sync/')({
  component: SyncPage,
});

function SyncPageSkeleton() {
  return (
    <div className="space-y-6">
      <div className="grid grid-cols-1 gap-4 md:grid-cols-2 lg:grid-cols-3">
        <Card>
          <CardHeader>
            <div className="flex items-center gap-3">
              <Skeleton className="h-12 w-12 rounded-lg" />
              <div className="flex-1">
                <Skeleton className="mb-2 h-5 w-24" />
                <Skeleton className="h-4 w-32" />
              </div>
            </div>
          </CardHeader>
          <CardContent className="space-y-4">
            <Skeleton className="h-10 w-full" />
            <Skeleton className="h-10 w-full" />
            <Skeleton className="h-10 w-full" />
          </CardContent>
        </Card>
      </div>
    </div>
  );
}

function SyncServiceCardWithStatus({
  config,
  onUpdate,
  onTriggerSync,
}: {
  config: SyncConfig;
  onUpdate: (platform: SyncPlatform, data: SyncConfigUpdateData) => Promise<void>;
  onTriggerSync: (platform: SyncPlatform) => Promise<void>;
}) {
  const { data: status } = useSyncStatus(config.platform);
  const { data: reviewData } = usePendingReviewCount();
  const { isPending: isUpdating } = useUpdateSyncConfig();
  const { isPending: isSyncing } = useTriggerSync();

  const pendingReviewCount = reviewData?.countsBySource[config.platform] ?? 0;

  const handleUpdate = async (data: SyncConfigUpdateData) => {
    await onUpdate(config.platform, data);
  };

  const handleTriggerSync = async () => {
    await onTriggerSync(config.platform);
  };

  return (
    <SyncServiceCard
      config={config}
      status={status}
      pendingReviewCount={pendingReviewCount}
      onUpdate={handleUpdate}
      onTriggerSync={handleTriggerSync}
      isUpdating={isUpdating}
      isSyncing={isSyncing}
    />
  );
}

function SyncPage() {
  const navigate = useNavigate();
  const { data: configs, isLoading, error } = useSyncConfigs();
  const { mutateAsync: updateConfig } = useUpdateSyncConfig();
  const { mutateAsync: triggerSync } = useTriggerSync();

  // Create map of existing configs by platform
  const configsByPlatform = new Map<SyncPlatform, SyncConfig>();
  configs?.configs.forEach(config => {
    configsByPlatform.set(config.platform, config);
  });

  // Create configs for all supported platforms (will show "not configured" for missing ones)
  const allPlatformConfigs = SUPPORTED_SYNC_PLATFORMS.map(platform => {
    return configsByPlatform.get(platform) || {
      id: `placeholder-${platform}`,
      userId: '',
      platform,
      frequency: SyncFrequency.MANUAL,
      autoAdd: false,
      lastSyncedAt: null,
      createdAt: '',
      updatedAt: '',
      isConfigured: false,
    };
  });

  const handleUpdateConfig = async (platform: SyncPlatform, data: SyncConfigUpdateData) => {
    try {
      await updateConfig({ platform, data });
      toast.success('Sync settings updated successfully');
    } catch (err) {
      const message = err instanceof Error ? err.message : 'Failed to update sync settings';
      toast.error(message);
      throw err;
    }
  };

  const handleTriggerSync = async (platform: SyncPlatform) => {
    try {
      await triggerSync(platform);
      toast.success(`${platform} sync started successfully`);
      navigate({ to: '/sync/$platform', params: { platform } });
    } catch (err) {
      const message = err instanceof Error ? err.message : 'Failed to trigger sync';
      toast.error(message);
      throw err;
    }
  };

  return (
    <div>
      {/* Header */}
      <div className="mb-6">
        <nav className="mb-2 flex items-center text-sm text-muted-foreground">
          <Link to="/dashboard" className="hover:text-foreground">
            Dashboard
          </Link>
          <span className="mx-2">/</span>
          <span className="text-foreground">Sync</span>
        </nav>
        <h1 className="text-2xl font-bold">Sync</h1>
        <p className="text-muted-foreground">
          Sync your Steam, Epic Games, and PlayStation Network libraries with Nexorious.
        </p>
      </div>

      {isLoading && <SyncPageSkeleton />}

      {error && (
        <Alert variant="destructive">
          <AlertCircle className="h-4 w-4" />
          <AlertTitle>Error</AlertTitle>
          <AlertDescription>
            Failed to load sync configurations. Please try again later.
          </AlertDescription>
        </Alert>
      )}

      {!isLoading && !error && (
        <>
          {/* All Platform Services Grid */}
          <div className="mb-6 grid grid-cols-1 gap-4 md:grid-cols-2 lg:grid-cols-3">
            {allPlatformConfigs.map((config: SyncConfig) => (
              <SyncServiceCardWithStatus
                key={config.id}
                config={config}
                onUpdate={handleUpdateConfig}
                onTriggerSync={handleTriggerSync}
              />
            ))}
          </div>

          {/* Info Alert */}
          <Alert className="mb-6">
            <Info className="h-4 w-4" />
            <AlertTitle>About Platform Syncing</AlertTitle>
            <AlertDescription>
              <p className="mb-2">
                Connect your gaming platforms to automatically sync your game libraries. New games
                will appear in your collection, and you can review pending items before they&apos;re
                added.
              </p>
              <p>
                Configure sync frequency and auto-add settings for each platform individually.
                Manual sync is always available regardless of your settings.
              </p>
            </AlertDescription>
          </Alert>

          {/* Quick Links */}
          <Card>
            <CardHeader>
              <CardTitle>Quick Links</CardTitle>
            </CardHeader>
            <CardContent className="space-y-2">
              <Link
                to="/import-export"
                className="flex items-center justify-between rounded-lg border p-4 transition-colors hover:bg-muted"
              >
                <div>
                  <div className="font-medium">Import/Export</div>
                  <div className="text-sm text-muted-foreground">
                    Bulk import or export your collection
                  </div>
                </div>
                <ArrowRight className="h-4 w-4 text-muted-foreground" />
              </Link>
              <Link
                to="/games"
                className="flex items-center justify-between rounded-lg border p-4 transition-colors hover:bg-muted"
              >
                <div>
                  <div className="font-medium">View Collection</div>
                  <div className="text-sm text-muted-foreground">
                    Browse and manage your game library
                  </div>
                </div>
                <ArrowRight className="h-4 w-4 text-muted-foreground" />
              </Link>
            </CardContent>
          </Card>
        </>
      )}
    </div>
  );
}
