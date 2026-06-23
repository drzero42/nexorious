import { useState } from 'react';
import { toast } from 'sonner';
import { Card, CardContent, CardHeader, CardTitle, CardDescription } from '@/components/ui/card';
import { Button } from '@/components/ui/button';
import { Badge } from '@/components/ui/badge';
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
import { KeyRound, Plus, Trash2, Loader2 } from 'lucide-react';
import { useApiKeys, useRevokeApiKey, useDateFormat } from '@/hooks';
import type { ApiKey } from '@/api/auth';
import { CreateApiKeyDialog } from './create-api-key-dialog';

function isExpired(key: ApiKey): boolean {
  return key.expires_at !== null && new Date(key.expires_at).getTime() < Date.now();
}

export function ApiKeysSection() {
  const { formatRelativeTime, formatDate } = useDateFormat();
  const { data: keys, isLoading } = useApiKeys();
  const revokeApiKey = useRevokeApiKey();
  const [createOpen, setCreateOpen] = useState(false);
  const [toRevoke, setToRevoke] = useState<ApiKey | null>(null);

  const handleRevoke = async () => {
    if (!toRevoke) return;
    try {
      await revokeApiKey.mutateAsync(toRevoke.id);
      toast.success(`Revoked "${toRevoke.name}"`);
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Failed to revoke API key');
    } finally {
      setToRevoke(null);
    }
  };

  return (
    <Card>
      <CardHeader className="flex flex-row items-start justify-between gap-4">
        <div>
          <CardTitle className="flex items-center gap-2">
            <KeyRound className="h-5 w-5" />
            API Keys
          </CardTitle>
          <CardDescription>Manage keys for programmatic access to your account.</CardDescription>
        </div>
        <Button size="sm" onClick={() => setCreateOpen(true)}>
          <Plus className="mr-2 h-4 w-4" />
          New API key
        </Button>
      </CardHeader>
      <CardContent>
        {isLoading ? (
          <div className="space-y-3">
            <Skeleton className="h-14 w-full" />
            <Skeleton className="h-14 w-full" />
          </div>
        ) : !keys || keys.length === 0 ? (
          <p className="py-6 text-center text-sm text-muted-foreground">No API keys yet.</p>
        ) : (
          <ul className="divide-y">
            {keys.map((key) => (
              <li key={key.id} className="flex items-center justify-between gap-4 py-3">
                <div className="min-w-0">
                  <div className="flex items-center gap-2">
                    <span className="truncate font-medium">{key.name}</span>
                    <Badge variant="secondary">{key.scopes}</Badge>
                    {isExpired(key) && <Badge variant="destructive">Expired</Badge>}
                  </div>
                  <p className="mt-1 text-xs text-muted-foreground">
                    {key.last_used_at
                      ? `Last used ${formatRelativeTime(key.last_used_at)}`
                      : 'Never used'}
                    {' · '}Created {formatDate(key.created_at)}
                    {key.expires_at
                      ? ` · Expires ${formatDate(key.expires_at)}`
                      : ' · Never expires'}
                  </p>
                </div>
                <Button
                  variant="ghost"
                  size="icon"
                  aria-label={`Revoke ${key.name}`}
                  onClick={() => setToRevoke(key)}
                >
                  <Trash2 className="h-4 w-4 text-destructive" />
                </Button>
              </li>
            ))}
          </ul>
        )}
      </CardContent>

      <CreateApiKeyDialog open={createOpen} onOpenChange={setCreateOpen} />

      <AlertDialog open={toRevoke !== null} onOpenChange={(o) => !o && setToRevoke(null)}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Revoke API key?</AlertDialogTitle>
            <AlertDialogDescription>
              This permanently revokes <strong>{toRevoke?.name}</strong>. Any client using it will
              stop working immediately. This cannot be undone.
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>Cancel</AlertDialogCancel>
            <AlertDialogAction
              onClick={(e) => {
                e.preventDefault();
                void handleRevoke();
              }}
              disabled={revokeApiKey.isPending}
              className="bg-destructive text-destructive-foreground hover:bg-destructive/90"
            >
              {revokeApiKey.isPending && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
              Revoke
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </Card>
  );
}
