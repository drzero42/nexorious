'use client';

import { useState } from 'react';
import { Card, CardContent, CardFooter, CardHeader, CardTitle } from '@/components/ui/card';
import { Button } from '@/components/ui/button';
import { Switch } from '@/components/ui/switch';
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select';
import { Badge } from '@/components/ui/badge';
import { Loader2, RefreshCw, History } from 'lucide-react';
import Link from 'next/link';
import type { SyncConfig, SyncStatus, SyncConfigUpdateData } from '@/types';
import { SyncFrequency, SyncPlatform, getSyncFrequencyLabel, getPlatformDisplayInfo } from '@/types';

// Platform icons as SVG paths
const PLATFORM_ICONS: Record<SyncPlatform, string> = {
  [SyncPlatform.STEAM]:
    'M12 2C6.477 2 2 6.477 2 12c0 4.991 3.657 9.128 8.438 9.879V14.89h-2.54V12h2.54V9.797c0-2.506 1.492-3.89 3.777-3.89 1.094 0 2.238.195 2.238.195v2.46h-1.26c-1.243 0-1.63.771-1.63 1.562V12h2.773l-.443 2.89h-2.33v6.989C18.343 21.129 22 16.99 22 12c0-5.523-4.477-10-10-10z',
  [SyncPlatform.EPIC]:
    'M3 3h18v18H3V3zm2 2v14h14V5H5zm3 3h2v8H8V8zm4 0h4v2h-4v2h3v2h-3v2h4v2H12V8z',
  [SyncPlatform.GOG]:
    'M12 2C6.48 2 2 6.48 2 12s4.48 10 10 10 10-4.48 10-10S17.52 2 12 2zm0 18c-4.41 0-8-3.59-8-8s3.59-8 8-8 8 3.59 8 8-3.59 8-8 8zm-2-8c0-1.1.9-2 2-2s2 .9 2 2-.9 2-2 2-2-.9-2-2z',
};

interface SyncServiceCardProps {
  config: SyncConfig;
  status?: SyncStatus;
  onUpdate: (data: SyncConfigUpdateData) => Promise<void>;
  onTriggerSync: () => Promise<void>;
  isUpdating?: boolean;
  isSyncing?: boolean;
  pendingReviewCount?: number;
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
  onUpdate,
  onTriggerSync,
  isUpdating = false,
  isSyncing = false,
  pendingReviewCount,
}: SyncServiceCardProps) {
  const [localEnabled, setLocalEnabled] = useState(config.enabled);
  const [localFrequency, setLocalFrequency] = useState(config.frequency);
  const [localAutoAdd, setLocalAutoAdd] = useState(config.autoAdd);

  const platformInfo = getPlatformDisplayInfo(config.platform);
  const isCurrentlySyncing = isSyncing || status?.isSyncing;

  const handleEnabledChange = async (enabled: boolean) => {
    setLocalEnabled(enabled);
    await onUpdate({ enabled });
  };

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
              <svg
                className={`h-7 w-7 ${platformInfo.color}`}
                viewBox="0 0 24 24"
                fill="currentColor"
              >
                <path d={PLATFORM_ICONS[config.platform]} />
              </svg>
            </div>
            <div>
              <CardTitle className="text-lg">{platformInfo.name}</CardTitle>
              <p className="text-sm text-muted-foreground">
                Last synced: {formatLastSync(config.lastSyncedAt)}
              </p>
            </div>
          </div>
          <Badge variant={localEnabled ? 'default' : 'secondary'}>
            {localEnabled ? 'Connected' : 'Disconnected'}
          </Badge>
        </div>
      </CardHeader>

      <CardContent className="space-y-4">
        {/* Enable Toggle */}
        <div className="flex items-center justify-between">
          <span className="text-sm font-medium">Enable sync</span>
          <Switch
            checked={localEnabled}
            onCheckedChange={handleEnabledChange}
            disabled={isUpdating}
          />
        </div>

        {/* Frequency Select */}
        <div className="flex items-center justify-between">
          <span className="text-sm font-medium">Sync frequency</span>
          <Select
            value={localFrequency}
            onValueChange={(value) => handleFrequencyChange(value as SyncFrequency)}
            disabled={!localEnabled || isUpdating}
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
            disabled={!localEnabled || isUpdating}
          />
        </div>
      </CardContent>

      <CardFooter className="flex items-center justify-between border-t bg-muted/50 px-6 py-4">
        <div className="flex items-center gap-3">
          <Link
            href={`/sync/${config.platform}`}
            className="flex items-center gap-1 text-sm text-primary hover:underline"
          >
            <History className="h-4 w-4" />
            View details
          </Link>
          {pendingReviewCount !== undefined && pendingReviewCount > 0 && (
            <Badge variant="secondary" className="text-xs">
              {pendingReviewCount} pending
            </Badge>
          )}
        </div>
        <Button
          onClick={onTriggerSync}
          disabled={!localEnabled || isCurrentlySyncing}
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
