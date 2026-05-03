
import { useState, useEffect, useRef } from 'react';
import { toast } from 'sonner';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';
import { Button } from '@/components/ui/button';
import { Badge } from '@/components/ui/badge';
import { Alert, AlertDescription } from '@/components/ui/alert';
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from '@/components/ui/alert-dialog';
import { EpicAuthDialog } from './epic-auth-dialog';
import { useSyncStatus, useDisconnectEpic } from '@/hooks';
import { SyncPlatform } from '@/types';
import { Info, AlertCircle, Unplug } from 'lucide-react';

interface EpicConnectionCardProps {
  isConfigured: boolean;
  displayName?: string;
  accountId?: string;
  onConnectionChange: () => void;
}

export function EpicConnectionCard({
  isConfigured,
  displayName,
  accountId,
  onConnectionChange,
}: EpicConnectionCardProps) {
  const [showAuthDialog, setShowAuthDialog] = useState(false);
  const [showDisconnectDialog, setShowDisconnectDialog] = useState(false);
  const authExpiredToastShownRef = useRef(false);
  const disconnectMutation = useDisconnectEpic();

  // Poll for sync status to detect auth expiration
  const { data: syncStatus } = useSyncStatus(SyncPlatform.EPIC);

  const authExpired =
    isConfigured &&
    syncStatus?.authExpired === true;

  // Show toast notification when auth expires
  useEffect(() => {
    if (authExpired && !authExpiredToastShownRef.current) {
      toast.error('Epic Games Store Authentication Expired', {
        description: 'Please reconnect to continue syncing.',
        action: {
          label: 'Reconnect',
          onClick: () => setShowAuthDialog(true),
        },
      });
      authExpiredToastShownRef.current = true;
    }

    // Reset the flag when auth is restored
    if (!authExpired && authExpiredToastShownRef.current) {
      authExpiredToastShownRef.current = false;
    }
  }, [authExpired]);

  const handleDisconnect = async () => {
    try {
      await disconnectMutation.mutateAsync();
      toast.success('Epic Games Store account has been disconnected.');
      onConnectionChange();
      setShowDisconnectDialog(false);
    } catch (error) {
      toast.error(error instanceof Error ? error.message : 'Failed to disconnect');
    }
  };

  const getStatusBadge = () => {
    if (!isConfigured) {
      return <Badge variant="secondary">Not Configured</Badge>;
    }
    if (authExpired) {
      return <Badge variant="destructive">Auth Expired</Badge>;
    }
    return <Badge variant="default">Connected</Badge>;
  };

  return (
    <>
      <Card>
        <CardHeader>
          <div className="flex items-center justify-between">
            <div>
              <CardTitle>Epic Games Store</CardTitle>
              <CardDescription>
                Sync your Epic Games Store library
              </CardDescription>
            </div>
            {getStatusBadge()}
          </div>
        </CardHeader>
        <CardContent className="space-y-4">
          {/* Connection Status */}
          {!isConfigured && (
            <div className="space-y-4">
              <p className="text-sm text-muted-foreground">
                Connect your Epic Games Store account to automatically sync your game library.
              </p>
              <Alert>
                <Info className="h-4 w-4" />
                <AlertDescription>
                  <strong>Note:</strong> Epic Games Store does not provide playtime data.
                </AlertDescription>
              </Alert>
              <Button onClick={() => setShowAuthDialog(true)}>
                Connect Epic Games Store
              </Button>
            </div>
          )}

          {isConfigured && !authExpired && (
            <div className="space-y-4">
              <div className="space-y-2">
                <div className="flex items-center justify-between">
                  <span className="text-sm font-medium">Account Name</span>
                  <span className="text-sm text-muted-foreground">{displayName}</span>
                </div>
                <div className="flex items-center justify-between">
                  <span className="text-sm font-medium">Account ID</span>
                  <span className="text-sm text-muted-foreground font-mono text-xs">
                    {accountId}
                  </span>
                </div>
              </div>
              <Alert>
                <Info className="h-4 w-4" />
                <AlertDescription>
                  <strong>Note:</strong> Epic Games Store does not provide playtime data.
                </AlertDescription>
              </Alert>
              <Button
                variant="destructive"
                onClick={() => setShowDisconnectDialog(true)}
              >
                <Unplug className="mr-2 h-4 w-4" />
                Disconnect
              </Button>
            </div>
          )}

          {isConfigured && authExpired && (
            <div className="space-y-4">
              <Alert variant="destructive">
                <AlertCircle className="h-4 w-4" />
                <AlertDescription>
                  Your Epic Games Store authentication has expired. Please reconnect to
                  continue syncing.
                </AlertDescription>
              </Alert>
              <div className="space-y-2">
                <div className="flex items-center justify-between">
                  <span className="text-sm font-medium">Account Name</span>
                  <span className="text-sm text-muted-foreground">{displayName}</span>
                </div>
                <div className="flex items-center justify-between">
                  <span className="text-sm font-medium">Account ID</span>
                  <span className="text-sm text-muted-foreground font-mono text-xs">
                    {accountId}
                  </span>
                </div>
              </div>
              <div className="flex gap-2">
                <Button onClick={() => setShowAuthDialog(true)}>
                  Reconnect
                </Button>
                <Button
                  variant="outline"
                  onClick={() => setShowDisconnectDialog(true)}
                >
                  <Unplug className="mr-2 h-4 w-4" />
                  Disconnect
                </Button>
              </div>
            </div>
          )}
        </CardContent>
      </Card>

      {/* Auth Dialog */}
      <EpicAuthDialog
        open={showAuthDialog}
        onOpenChange={setShowAuthDialog}
        onSuccess={() => {
          setShowAuthDialog(false);
          onConnectionChange();
        }}
      />

      {/* Disconnect Confirmation Dialog */}
      <AlertDialog open={showDisconnectDialog} onOpenChange={setShowDisconnectDialog}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Disconnect Epic Games Store?</AlertDialogTitle>
            <AlertDialogDescription>
              This will remove your Epic Games Store connection and stop syncing your
              library. Games already imported will remain in your collection.
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>Cancel</AlertDialogCancel>
            <AlertDialogAction
              onClick={handleDisconnect}
              disabled={disconnectMutation.isPending}
            >
              {disconnectMutation.isPending ? 'Disconnecting...' : 'Disconnect'}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </>
  );
}
