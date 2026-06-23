import { useState } from 'react';
import { toast } from 'sonner';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { Button } from '@/components/ui/button';
import { Switch } from '@/components/ui/switch';
import { Label } from '@/components/ui/label';
import { Skeleton } from '@/components/ui/skeleton';
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
import { Bell, Plus, Pencil, Trash2, Loader2, RotateCcw } from 'lucide-react';
import {
  useChannels,
  useEventTypes,
  useSubscriptions,
  useDeleteChannel,
  usePutSubscriptions,
  useResetSubscriptions,
} from '@/hooks/use-notifications';
import { useDateFormat } from '@/hooks';
import { ChannelDialog } from './channel-dialog';
import type { NotificationChannel } from '@/api/notifications';

export function NotificationsSection() {
  const { formatDate } = useDateFormat();
  const { data: channels, isLoading: channelsLoading } = useChannels();
  const { data: eventTypes, isLoading: eventTypesLoading } = useEventTypes();
  const { data: subscriptions, isLoading: subscriptionsLoading } = useSubscriptions();

  const deleteChannel = useDeleteChannel();
  const putSubscriptions = usePutSubscriptions();
  const resetSubscriptions = useResetSubscriptions();

  // Channel dialog state
  const [channelDialogOpen, setChannelDialogOpen] = useState(false);
  const [editingChannel, setEditingChannel] = useState<NotificationChannel | null>(null);

  // Delete confirm state
  const [deleteTarget, setDeleteTarget] = useState<NotificationChannel | null>(null);

  // Reset confirm state
  const [resetDialogOpen, setResetDialogOpen] = useState(false);

  const handleOpenAdd = () => {
    setEditingChannel(null);
    setChannelDialogOpen(true);
  };

  const handleOpenEdit = (channel: NotificationChannel) => {
    setEditingChannel(channel);
    setChannelDialogOpen(true);
  };

  const handleDelete = async () => {
    if (!deleteTarget) return;
    try {
      await deleteChannel.mutateAsync(deleteTarget.id);
      toast.success('Channel deleted');
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Failed to delete channel');
    } finally {
      setDeleteTarget(null);
    }
  };

  const handleToggleEvent = async (type: string, currentlyOn: boolean) => {
    if (!subscriptions) return;
    const current = subscriptions.event_types;
    const next = currentlyOn ? current.filter((t) => t !== type) : [...current, type];
    try {
      await putSubscriptions.mutateAsync(next);
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Failed to update subscriptions');
    }
  };

  const handleReset = async () => {
    try {
      await resetSubscriptions.mutateAsync();
      toast.success('Subscriptions reset to defaults');
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Failed to reset subscriptions');
    } finally {
      setResetDialogOpen(false);
    }
  };

  // Group event types by category
  const categorised = eventTypes
    ? eventTypes.reduce<Record<string, typeof eventTypes>>((acc, et) => {
        const cat = et.category;
        if (!acc[cat]) acc[cat] = [];
        acc[cat].push(et);
        return acc;
      }, {})
    : {};

  const isEventsLoading = eventTypesLoading || subscriptionsLoading;

  return (
    <>
      <Card>
        <CardHeader>
          <div className="flex items-center justify-between">
            <CardTitle className="flex items-center gap-2">
              <Bell className="h-5 w-5" />
              Notifications
            </CardTitle>
          </div>
        </CardHeader>

        <CardContent className="space-y-8">
          {/* ── Channels ── */}
          <section aria-label="Notification channels">
            <div className="mb-4 flex items-center justify-between">
              <h3 className="text-sm font-semibold">Channels</h3>
              <Button size="sm" variant="outline" onClick={handleOpenAdd}>
                <Plus className="mr-1 h-4 w-4" />
                Add channel
              </Button>
            </div>

            {channelsLoading ? (
              <div className="space-y-2">
                <Skeleton className="h-10 w-full" />
                <Skeleton className="h-10 w-full" />
              </div>
            ) : !channels || channels.length === 0 ? (
              <p className="text-sm text-muted-foreground">
                No channels yet. Add one to start receiving notifications.
              </p>
            ) : (
              <div className="divide-y rounded-md border">
                {channels.map((ch) => (
                  <div key={ch.id} className="flex items-center justify-between px-4 py-3">
                    <div>
                      <p className="text-sm font-medium">{ch.name}</p>
                      <p className="text-xs text-muted-foreground">
                        Added {formatDate(ch.created_at)}
                      </p>
                    </div>
                    <div className="flex items-center gap-1">
                      <Button
                        variant="ghost"
                        size="sm"
                        onClick={() => handleOpenEdit(ch)}
                        aria-label={`Edit ${ch.name}`}
                      >
                        <Pencil className="h-4 w-4" />
                      </Button>
                      <Button
                        variant="ghost"
                        size="sm"
                        onClick={() => setDeleteTarget(ch)}
                        aria-label={`Delete ${ch.name}`}
                      >
                        <Trash2 className="h-4 w-4 text-destructive" />
                      </Button>
                    </div>
                  </div>
                ))}
              </div>
            )}
          </section>

          {/* ── Event toggles ── */}
          <section aria-label="Event subscriptions">
            <div className="mb-4 flex items-center justify-between">
              <h3 className="text-sm font-semibold">Events</h3>
              <Button
                size="sm"
                variant="outline"
                onClick={() => setResetDialogOpen(true)}
                disabled={resetSubscriptions.isPending}
              >
                {resetSubscriptions.isPending ? (
                  <Loader2 className="mr-1 h-4 w-4 animate-spin" />
                ) : (
                  <RotateCcw className="mr-1 h-4 w-4" />
                )}
                Reset to defaults
              </Button>
            </div>

            {isEventsLoading ? (
              <div className="space-y-3">
                <Skeleton className="h-6 w-32" />
                <Skeleton className="h-8 w-full" />
                <Skeleton className="h-8 w-full" />
              </div>
            ) : (
              <div className="space-y-6">
                {Object.entries(categorised).map(([category, types]) => (
                  <div key={category}>
                    <h4 className="mb-3 text-xs font-semibold uppercase tracking-wide text-muted-foreground">
                      {category}
                    </h4>
                    <div className="space-y-3">
                      {types.map((et) => {
                        const isOn = subscriptions?.event_types.includes(et.type) ?? false;
                        return (
                          <div key={et.type} className="flex items-center justify-between">
                            <Label
                              htmlFor={`event-${et.type}`}
                              className="cursor-pointer text-sm font-normal"
                            >
                              {et.label}
                            </Label>
                            <Switch
                              id={`event-${et.type}`}
                              checked={isOn}
                              onCheckedChange={() => handleToggleEvent(et.type, isOn)}
                              disabled={putSubscriptions.isPending}
                              aria-label={et.label}
                            />
                          </div>
                        );
                      })}
                    </div>
                  </div>
                ))}

                {Object.keys(categorised).length === 0 && (
                  <p className="text-sm text-muted-foreground">No event types available.</p>
                )}
              </div>
            )}
          </section>
        </CardContent>
      </Card>

      {/* Channel add/edit dialog */}
      <ChannelDialog
        open={channelDialogOpen}
        onOpenChange={setChannelDialogOpen}
        channel={editingChannel}
      />

      {/* Delete channel confirm */}
      <AlertDialog open={Boolean(deleteTarget)} onOpenChange={(v) => !v && setDeleteTarget(null)}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Delete channel</AlertDialogTitle>
            <AlertDialogDescription>
              Delete &ldquo;{deleteTarget?.name}&rdquo;? This cannot be undone.
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>Cancel</AlertDialogCancel>
            <AlertDialogAction
              onClick={handleDelete}
              className="bg-destructive text-destructive-foreground hover:bg-destructive/90"
            >
              {deleteChannel.isPending && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
              Delete
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>

      {/* Reset subscriptions confirm */}
      <AlertDialog open={resetDialogOpen} onOpenChange={setResetDialogOpen}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Reset to defaults?</AlertDialogTitle>
            <AlertDialogDescription>
              This will discard your customizations and restore the default event subscriptions.
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>Cancel</AlertDialogCancel>
            <AlertDialogAction onClick={handleReset}>Reset</AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </>
  );
}
