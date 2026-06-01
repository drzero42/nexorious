import { useEffect } from 'react';
import { useForm } from 'react-hook-form';
import { zodResolver } from '@hookform/resolvers/zod';
import { z } from 'zod';
import { toast } from 'sonner';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Loader2, Send } from 'lucide-react';
import {
  useCreateChannel,
  useUpdateChannel,
  useTestChannel,
  useTestUrl,
} from '@/hooks/use-notifications';
import type { NotificationChannel } from '@/api/notifications';

const addSchema = z.object({
  name: z.string().min(1, 'Name is required').trim(),
  url: z.string().min(1, 'URL is required').trim(),
});

const editSchema = z.object({
  name: z.string().min(1, 'Name is required').trim(),
  url: z.string().trim(),
});

type AddFormValues = z.infer<typeof addSchema>;
type EditFormValues = z.infer<typeof editSchema>;

interface ChannelDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  channel?: NotificationChannel | null;
}

interface ChannelDialogFormProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  channel?: NotificationChannel | null;
  isEdit: boolean;
}

// Inner component that owns useForm — rendered with a key so it remounts (and
// re-initialises with the correct resolver) whenever the add↔edit mode changes.
function ChannelDialogForm({ open, onOpenChange, channel, isEdit }: ChannelDialogFormProps) {
  const createChannel = useCreateChannel();
  const updateChannel = useUpdateChannel();
  const testChannel = useTestChannel();
  const testUrl = useTestUrl();

  const {
    register,
    handleSubmit,
    reset,
    getValues,
    formState: { errors },
  } = useForm<AddFormValues | EditFormValues>({
    resolver: zodResolver(isEdit ? editSchema : addSchema),
    defaultValues: { name: channel?.name ?? '', url: '' },
  });

  // Reset form when dialog opens or the channel being edited changes
  useEffect(() => {
    if (open) {
      reset({ name: channel?.name ?? '', url: '' });
    }
  }, [open, channel, reset]);

  const onSubmit = async (values: AddFormValues | EditFormValues) => {
    try {
      if (isEdit && channel) {
        const data: { name?: string; url?: string } = { name: values.name };
        if (values.url) {
          data.url = values.url;
        }
        await updateChannel.mutateAsync({ id: channel.id, data });
        toast.success('Channel updated');
      } else {
        await createChannel.mutateAsync({
          name: values.name,
          url: values.url as string,
        });
        toast.success('Channel added');
      }
      onOpenChange(false);
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Failed to save channel');
    }
  };

  const handleTest = async () => {
    const typedUrl = getValues('url').trim();
    try {
      if (typedUrl) {
        // URL field has a value — test the typed URL without saving
        await testUrl.mutateAsync(typedUrl);
      } else if (isEdit && channel) {
        // Edit mode with blank URL field — test the already-saved channel URL
        await testChannel.mutateAsync(channel.id);
      } else {
        // Add mode with blank URL — nothing to test yet
        toast.error('Enter a URL to test first');
        return;
      }
      toast.success('Test notification sent');
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Test failed');
    }
  };

  const isPending = createChannel.isPending || updateChannel.isPending;
  const isTestPending = testChannel.isPending || testUrl.isPending;

  return (
    <form onSubmit={handleSubmit(onSubmit)} className="space-y-4 py-2">
      <div className="space-y-2">
        <Label htmlFor="channel-name">Name</Label>
        <Input
          id="channel-name"
          placeholder="e.g. My Phone"
          {...register('name')}
          disabled={isPending}
        />
        {errors.name && <p className="text-sm text-destructive">{errors.name.message}</p>}
      </div>

      <div className="space-y-2">
        <Label htmlFor="channel-url">Shoutrrr URL{isEdit ? ' (optional)' : ''}</Label>
        <Input
          id="channel-url"
          type="password"
          placeholder={
            isEdit
              ? 'Leave blank to keep current URL'
              : 'ntfy://ntfy.sh/topic or discord://token@id'
          }
          autoComplete="off"
          {...register('url')}
          disabled={isPending}
        />
        {errors.url && <p className="text-sm text-destructive">{errors.url.message}</p>}
        <p className="text-xs text-muted-foreground">
          Shoutrrr URL, e.g. <code className="rounded bg-muted px-1">ntfy://ntfy.sh/topic</code>,{' '}
          <code className="rounded bg-muted px-1">discord://token@id</code>,{' '}
          <code className="rounded bg-muted px-1">smtp://user:pass@host</code>
        </p>
      </div>

      <DialogFooter className="gap-2 sm:gap-0">
        <Button type="button" variant="outline" onClick={handleTest} disabled={isTestPending}>
          {isTestPending ? (
            <Loader2 className="mr-2 h-4 w-4 animate-spin" />
          ) : (
            <Send className="mr-2 h-4 w-4" />
          )}
          Send test
        </Button>
        <Button variant="outline" type="button" onClick={() => onOpenChange(false)}>
          Cancel
        </Button>
        <Button type="submit" disabled={isPending}>
          {isPending && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
          {isEdit ? 'Save' : 'Add Channel'}
        </Button>
      </DialogFooter>
    </form>
  );
}

export function ChannelDialog({ open, onOpenChange, channel }: ChannelDialogProps) {
  const isEdit = Boolean(channel);

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{isEdit ? 'Edit Channel' : 'Add Channel'}</DialogTitle>
          <DialogDescription>
            {isEdit
              ? 'Update the channel name or URL.'
              : 'Add a notification channel using a Shoutrrr URL.'}
          </DialogDescription>
        </DialogHeader>

        {/* key ensures useForm remounts with the correct resolver when mode switches */}
        <ChannelDialogForm
          key={isEdit ? 'edit' : 'add'}
          open={open}
          onOpenChange={onOpenChange}
          channel={channel}
          isEdit={isEdit}
        />
      </DialogContent>
    </Dialog>
  );
}
