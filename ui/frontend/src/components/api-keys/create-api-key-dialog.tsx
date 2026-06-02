import { useState } from 'react';
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
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Alert, AlertDescription } from '@/components/ui/alert';
import { Copy, Loader2, AlertTriangle } from 'lucide-react';
import { useCreateApiKey } from '@/hooks';
import { EXPIRY_PRESETS, expiryPresetToRFC3339, type ExpiryPreset } from '@/lib/api-key-expiry';
import type { CreatedApiKey } from '@/api/auth';

const schema = z.object({
  name: z.string().min(1, 'Name is required').trim(),
});

type FormValues = z.infer<typeof schema>;

interface CreateApiKeyDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
}

export function CreateApiKeyDialog({ open, onOpenChange }: CreateApiKeyDialogProps) {
  const createApiKey = useCreateApiKey();
  const [created, setCreated] = useState<CreatedApiKey | null>(null);
  const [scopes, setScopes] = useState<'read' | 'write'>('write');
  const [expiry, setExpiry] = useState<ExpiryPreset>('30');

  const {
    register,
    handleSubmit,
    reset,
    formState: { errors },
  } = useForm<FormValues>({
    resolver: zodResolver(schema),
    defaultValues: { name: '' },
  });

  const handleClose = () => {
    // Clear the revealed key and form state, then notify the parent.
    setCreated(null);
    reset({ name: '' });
    setScopes('write');
    setExpiry('30');
    onOpenChange(false);
  };

  const onSubmit = async (values: FormValues) => {
    try {
      const result = await createApiKey.mutateAsync({
        name: values.name,
        scopes,
        expires_at: expiryPresetToRFC3339(expiry),
      });
      setCreated(result);
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Failed to create API key');
    }
  };

  const handleCopy = async () => {
    if (!created) return;
    await navigator.clipboard.writeText(created.key);
    toast.success('API key copied to clipboard');
  };

  return (
    <Dialog open={open} onOpenChange={(o) => (o ? onOpenChange(true) : handleClose())}>
      <DialogContent>
        {created ? (
          <>
            <DialogHeader>
              <DialogTitle>API key created</DialogTitle>
              <DialogDescription>Your new key is ready to use.</DialogDescription>
            </DialogHeader>
            <div className="space-y-3">
              <Alert variant="destructive">
                <AlertTriangle className="h-4 w-4" />
                <AlertDescription>Copy this now — it won&apos;t be shown again.</AlertDescription>
              </Alert>
              <div className="flex items-center gap-2">
                <code className="flex-1 break-all rounded-md border bg-muted/50 p-3 font-mono text-sm">
                  {created.key}
                </code>
                <Button
                  type="button"
                  variant="outline"
                  size="icon"
                  onClick={handleCopy}
                  aria-label="Copy API key"
                >
                  <Copy className="h-4 w-4" />
                </Button>
              </div>
            </div>
            <DialogFooter>
              <Button type="button" onClick={handleClose}>
                Done
              </Button>
            </DialogFooter>
          </>
        ) : (
          <form onSubmit={handleSubmit(onSubmit)}>
            <DialogHeader>
              <DialogTitle>New API key</DialogTitle>
              <DialogDescription>Create a key for programmatic access.</DialogDescription>
            </DialogHeader>
            <div className="space-y-4 py-2">
              <div>
                <Label htmlFor="api-key-name">Name</Label>
                <Input
                  id="api-key-name"
                  className="mt-1"
                  placeholder="e.g. CI token"
                  {...register('name')}
                />
                {errors.name && (
                  <p className="mt-1 text-sm text-destructive">{errors.name.message}</p>
                )}
              </div>
              <div>
                <Label htmlFor="api-key-scopes">Scopes</Label>
                <Select value={scopes} onValueChange={(v) => setScopes(v as 'read' | 'write')}>
                  <SelectTrigger id="api-key-scopes" className="mt-1">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="write">Read &amp; write</SelectItem>
                    <SelectItem value="read">Read only</SelectItem>
                  </SelectContent>
                </Select>
              </div>
              <div>
                <Label htmlFor="api-key-expiry">Expiry</Label>
                <Select value={expiry} onValueChange={(v) => setExpiry(v as ExpiryPreset)}>
                  <SelectTrigger id="api-key-expiry" className="mt-1">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    {EXPIRY_PRESETS.map((p) => (
                      <SelectItem key={p.value} value={p.value}>
                        {p.label}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>
            </div>
            <DialogFooter>
              <Button type="button" variant="outline" onClick={handleClose}>
                Cancel
              </Button>
              <Button type="submit" disabled={createApiKey.isPending}>
                {createApiKey.isPending && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
                Create
              </Button>
            </DialogFooter>
          </form>
        )}
      </DialogContent>
    </Dialog>
  );
}
