import { createFileRoute, Link } from '@tanstack/react-router';
import {
  useSyncConfigs,
  useTriggerSync,
  useSyncStatus,
  usePendingReviewCount,
  useSteamConnection,
  usePSNStatus,
  useEpicConnection,
  useGOGConnection,
} from '@/hooks';
import { SyncServiceCard } from '@/components/sync';
import { Card, CardHeader } from '@/components/ui/card';
import { Skeleton } from '@/components/ui/skeleton';
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert';
import { AlertCircle, Info } from 'lucide-react';
import { toast } from 'sonner';
import { SUPPORTED_SYNC_STOREFRONTS, SyncStorefront, SyncFrequency } from '@/types';
import type { SyncConfig } from '@/types';

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
        </Card>
      </div>
    </div>
  );
}

function SyncServiceCardWithStatus({
  config,
  onTriggerSync,
}: {
  config: SyncConfig;
  onTriggerSync: (storefront: SyncStorefront) => Promise<void>;
}) {
  const { data: status } = useSyncStatus(config.storefront);
  const { data: reviewData } = usePendingReviewCount();
  const { isPending: isSyncing } = useTriggerSync();

  // Fetch storefront-specific connection data for credentials error state
  const { data: steamConnection } = useSteamConnection();
  const { data: psnStatus } = usePSNStatus();
  const { data: epicConnection } = useEpicConnection();
  const { data: gogConnection } = useGOGConnection();

  const pendingReviewCount = reviewData?.countsBySource?.[config.storefront] ?? 0;

  const credentialsError =
    (config.storefront === SyncStorefront.STEAM && (steamConnection?.credentialsError ?? false)) ||
    (config.storefront === SyncStorefront.PSN && (psnStatus?.credentialsError ?? false)) ||
    (config.storefront === SyncStorefront.EPIC && (epicConnection?.credentialsError ?? false)) ||
    (config.storefront === SyncStorefront.GOG && (gogConnection?.credentialsError ?? false));

  const handleTriggerSync = async () => {
    await onTriggerSync(config.storefront);
  };

  return (
    <SyncServiceCard
      config={config}
      status={status}
      pendingReviewCount={pendingReviewCount}
      credentialsError={credentialsError}
      onTriggerSync={handleTriggerSync}
      isSyncing={isSyncing}
      externalGameCount={status?.externalGameCount}
    />
  );
}

function SyncPage() {
  const { data: configs, isLoading, error } = useSyncConfigs();
  const { mutateAsync: triggerSync } = useTriggerSync();

  // Create map of existing configs by storefront
  const configsByStorefront = new Map<SyncStorefront, SyncConfig>();
  configs?.configs.forEach((config) => {
    configsByStorefront.set(config.storefront, config);
  });

  // Create configs for all supported storefronts (will show "not configured" for missing ones)
  const allStorefrontConfigs = SUPPORTED_SYNC_STOREFRONTS.map((storefront) => {
    return (
      configsByStorefront.get(storefront) || {
        id: `placeholder-${storefront}`,
        userId: '',
        storefront,
        frequency: SyncFrequency.MANUAL,
        lastSyncedAt: null,
        createdAt: '',
        updatedAt: '',
        isConfigured: false,
      }
    );
  });

  const handleTriggerSync = async (storefront: SyncStorefront) => {
    try {
      await triggerSync(storefront);
      toast.success(`${storefront} sync started successfully`);
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
          {/* All Storefront Services Grid */}
          <div className="mb-6 grid grid-cols-1 gap-4 md:grid-cols-2 lg:grid-cols-3">
            {allStorefrontConfigs.map((config: SyncConfig) => (
              <SyncServiceCardWithStatus
                key={config.id}
                config={config}
                onTriggerSync={handleTriggerSync}
              />
            ))}
          </div>

          {/* Info Alert */}
          <Alert className="mb-6">
            <Info className="h-4 w-4" />
            <AlertTitle>About Storefront Syncing</AlertTitle>
            <AlertDescription>
              <p className="mb-2">
                Connect your gaming storefronts to automatically sync your game libraries. New games
                will appear in your collection, and you can review pending items before they&apos;re
                added.
              </p>
              <p>
                Configure sync frequency for each storefront individually from the storefront
                details page. Manual sync is always available regardless of your settings.
              </p>
            </AlertDescription>
          </Alert>
        </>
      )}
    </div>
  );
}
