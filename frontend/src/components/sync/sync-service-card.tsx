'use client';

import { useState } from 'react';
import Image from 'next/image';
import { Card, CardContent, CardFooter, CardHeader, CardTitle } from '@/components/ui/card';
import { Button } from '@/components/ui/button';
import { Switch } from '@/components/ui/switch';
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select';
import { Badge } from '@/components/ui/badge';
import { Loader2, RefreshCw, History } from 'lucide-react';
import Link from 'next/link';
import { config as envConfig } from '@/lib/env';
import type { SyncConfig, SyncStatus, SyncConfigUpdateData } from '@/types';
import { SyncFrequency, getSyncFrequencyLabel, getPlatformDisplayInfo } from '@/types';

interface SyncServiceCardProps {
  config: SyncConfig;
  status?: SyncStatus;
  pendingReviewCount?: number;
  onUpdate: (data: SyncConfigUpdateData) => Promise<void>;
  onTriggerSync: () => Promise<void>;
  isUpdating?: boolean;
  isSyncing?: boolean;
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

export function SyncServiceCard({
  config,
  status,
  pendingReviewCount,
  onUpdate,
  onTriggerSync,
  isUpdating = false,
  isSyncing = false,
}: SyncServiceCardProps) {
  const [localFrequency, setLocalFrequency] = useState(config.frequency);
  const [localAutoAdd, setLocalAutoAdd] = useState(config.autoAdd);

  const platformInfo = getPlatformDisplayInfo(config.platform);
  const isCurrentlySyncing = isSyncing || status?.isSyncing;

  const handleFrequencyChange = async (frequency: SyncFrequency) => {
    setLocalFrequency(frequency);
    await onUpdate({ frequency });
  };

  const handleAutoAddChange = async (autoAdd: boolean) => {
    setLocalAutoAdd(autoAdd);
    await onUpdate({ autoAdd });
  };

  return (
    <Card className="overflow-hidden">
      <CardHeader className="pb-4">
        <div className="flex items-center justify-between">
          <div className="flex items-center gap-3">
            <div
              className={`flex h-12 w-12 items-center justify-center rounded-lg ${platformInfo.bgColor}`}
            >
              <Image
                src={`${envConfig.staticUrl}${platformInfo.iconUrl}`}
                alt={`${platformInfo.name} icon`}
                width={28}
                height={28}
                className="h-7 w-7"
              />
            </div>
            <div>
              <CardTitle className="text-lg">{platformInfo.name}</CardTitle>
              <p className="text-sm text-muted-foreground">
                Last synced: {formatLastSync(config.lastSyncedAt)}
              </p>
            </div>
          </div>
          <div className="flex items-center gap-2">
            {pendingReviewCount !== undefined && pendingReviewCount > 0 && (
              <Badge variant="destructive">
                {pendingReviewCount} to review
              </Badge>
            )}
            <Badge
              variant={!config.isConfigured ? 'outline' : 'default'}
              className={
                !config.isConfigured
                  ? 'bg-muted text-muted-foreground'
                  : 'bg-green-100 text-green-800 dark:bg-green-900/30 dark:text-green-400'
              }
            >
              {!config.isConfigured ? 'Not Configured' : 'Connected'}
            </Badge>
          </div>
        </div>
      </CardHeader>

      <CardContent className="space-y-4">
        {/* Frequency Select */}
        <div className="flex items-center justify-between">
          <span className="text-sm font-medium">Sync frequency</span>
          <Select
            value={localFrequency}
            onValueChange={(value) => handleFrequencyChange(value as SyncFrequency)}
            disabled={isUpdating || !config.isConfigured}
          >
            <SelectTrigger className="w-[140px]">
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
          <span className="text-sm font-medium">Auto-add games</span>
          <Switch
            checked={localAutoAdd}
            onCheckedChange={handleAutoAddChange}
            disabled={isUpdating || !config.isConfigured}
          />
        </div>
      </CardContent>

      <CardFooter className="flex items-center justify-between border-t bg-muted/50 px-6 py-4">
        <Link
          href={`/sync/${config.platform}`}
          className="flex items-center gap-1 text-sm text-primary hover:underline"
        >
          <History className="h-4 w-4" />
          View details
        </Link>
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
