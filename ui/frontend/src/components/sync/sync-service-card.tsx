import { useEffect, useRef, useState } from 'react';
import { Card, CardContent, CardFooter, CardHeader, CardTitle } from '@/components/ui/card';
import { Button } from '@/components/ui/button';
import { Switch } from '@/components/ui/switch';
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select';
import { Badge } from '@/components/ui/badge';
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
import { Loader2, RefreshCw, History } from 'lucide-react';
import { Link } from '@tanstack/react-router';
import { config as envConfig } from '@/lib/env';
import type { SyncConfig, SyncStatus, SyncConfigUpdateData } from '@/types';
import { SyncFrequency, getSyncFrequencyLabel, getPlatformDisplayInfo } from '@/types';

interface SyncServiceCardProps {
  config: SyncConfig;
  status?: SyncStatus;
  pendingReviewCount?: number;
  onUpdate: (data: SyncConfigUpdateData) => Promise<void>;
  onTriggerSync: () => Promise<void>;
  onReset?: () => Promise<void>;
  isUpdating?: boolean;
  isSyncing?: boolean;
  isResetting?: boolean;
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
  onReset,
  isUpdating = false,
  isSyncing = false,
  isResetting = false,
}: SyncServiceCardProps) {
  const [localFrequency, setLocalFrequency] = useState(config.frequency);
  const [localAutoAdd, setLocalAutoAdd] = useState(config.autoAdd);
  const [resetDialogOpen, setResetDialogOpen] = useState(false);
  const wasResettingRef = useRef(false);

  // Close the confirmation dialog once the reset mutation has settled.
  useEffect(() => {
    if (wasResettingRef.current && !isResetting) {
      setResetDialogOpen(false);
    }
    wasResettingRef.current = isResetting;
  }, [isResetting]);

  const handleResetClick = (event: React.MouseEvent<HTMLButtonElement>) => {
    // Prevent Radix from auto-closing the dialog — useEffect closes it
    // after the mutation settles instead.
    event.preventDefault();
    void onReset?.();
  };

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
              <CardTitle className="text-lg">{platformInfo.name}</CardTitle>
              <p className="text-sm text-muted-foreground">
                Last synced: {formatLastSync(config.lastSyncedAt)}
              </p>
            </div>
          </div>
          <div className="flex items-center gap-2">
            {pendingReviewCount !== undefined && pendingReviewCount > 0 && (
              <Badge variant="destructive">
                {pendingReviewCount}
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
          to="/sync/$platform" params={{ platform: config.platform }}
          className="flex items-center gap-1 text-sm text-primary hover:underline"
        >
          <History className="h-4 w-4" />
          View details
        </Link>
        <div className="flex items-center gap-2">
          {onReset && config.isConfigured && (
            <AlertDialog
              open={resetDialogOpen}
              onOpenChange={(open) => {
                // Block external close attempts (overlay click, Esc) while resetting.
                if (!open && isResetting) return;
                setResetDialogOpen(open);
              }}
            >
              <AlertDialogTrigger asChild>
                <Button
                  variant="ghost"
                  size="sm"
                  disabled={isResetting}
                >
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
                  <AlertDialogAction
                    onClick={handleResetClick}
                    disabled={isResetting}
                  >
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
        </div>
      </CardFooter>
    </Card>
  );
}
