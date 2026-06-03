import { Card, CardFooter, CardHeader, CardTitle } from '@/components/ui/card';
import { Button } from '@/components/ui/button';
import { Badge } from '@/components/ui/badge';
import { Loader2, RefreshCw } from 'lucide-react';
import { Link } from '@tanstack/react-router';
import { config as envConfig } from '@/lib/env';
import type { SyncConfig, SyncStatus } from '@/types';
import { getStorefrontDisplayInfo } from '@/types';
import { formatRelativeTime } from '@/types/jobs';

interface SyncServiceCardProps {
  config: SyncConfig;
  status?: SyncStatus;
  pendingReviewCount?: number;
  credentialsError?: boolean;
  onTriggerSync: () => Promise<void>;
  isSyncing?: boolean;
  externalGameCount?: number;
}

export function SyncServiceCard({
  config,
  status,
  pendingReviewCount,
  credentialsError = false,
  onTriggerSync,
  isSyncing = false,
  externalGameCount,
}: SyncServiceCardProps) {
  const platformInfo = getStorefrontDisplayInfo(config.storefront);
  const isCurrentlySyncing = isSyncing || status?.isSyncing;

  return (
    <Card className="overflow-hidden">
      <CardHeader className="pb-4">
        <div className="flex items-center justify-between">
          <div className="flex items-center gap-3">
            <div
              className={`flex h-12 w-12 items-center justify-center rounded-lg ${platformInfo.bgColor}`}
            >
              <img
                src={`${envConfig.staticUrl}${platformInfo.iconUrl}`}
                alt={`${platformInfo.name} icon`}
                width={28}
                height={28}
                className="h-7 w-7"
                loading="lazy"
              />
            </div>
            <div>
              <CardTitle className="text-lg">
                <Link
                  to="/sync/$storefront"
                  params={{ storefront: config.storefront }}
                  className="hover:underline"
                >
                  {platformInfo.name}
                </Link>
              </CardTitle>
              <p className="text-sm text-muted-foreground">
                Last synced: {formatRelativeTime(config.lastSyncedAt, 'Never')}
              </p>
              {externalGameCount !== undefined && externalGameCount > 0 && (
                <p className="text-sm text-muted-foreground">{externalGameCount} games</p>
              )}
            </div>
          </div>
          <div className="flex items-center gap-2">
            {pendingReviewCount !== undefined && pendingReviewCount > 0 && (
              <Link
                to="/sync/$storefront"
                params={{ storefront: config.storefront }}
                hash="needs-review"
              >
                <Badge variant="destructive">{pendingReviewCount}</Badge>
              </Link>
            )}
            {credentialsError ? (
              <Badge variant="destructive">Credentials Error</Badge>
            ) : config.isConfigured ? (
              <Badge
                variant="outline"
                className="bg-green-100 text-green-800 dark:bg-green-900/30 dark:text-green-400"
              >
                Connected
              </Badge>
            ) : (
              <Badge variant="outline" className="bg-muted text-muted-foreground">
                Not Configured
              </Badge>
            )}
          </div>
        </div>
      </CardHeader>

      <CardFooter className="flex items-center justify-end border-t bg-muted/50 px-6 py-4">
        <Button
          onClick={onTriggerSync}
          disabled={isCurrentlySyncing || !config.isConfigured}
          size="sm"
        >
          {isCurrentlySyncing ? (
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
      </CardFooter>
    </Card>
  );
}
