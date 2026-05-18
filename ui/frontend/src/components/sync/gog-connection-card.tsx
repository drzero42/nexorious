
import { useForm } from 'react-hook-form';
import { zodResolver } from '@hookform/resolvers/zod';
import { z } from 'zod';
import { toast } from 'sonner';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Badge } from '@/components/ui/badge';
import { Label } from '@/components/ui/label';
import { Alert, AlertDescription } from '@/components/ui/alert';
import {
  Accordion,
  AccordionContent,
  AccordionItem,
  AccordionTrigger,
} from '@/components/ui/accordion';
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
import { Loader2, Check, ExternalLink, Info } from 'lucide-react';
import { useConnectGOG, useDisconnectGOG, useGOGConnection } from '@/hooks';
import { GOG_AUTH_URL } from '@/types';

const gogAuthCodeSchema = z.object({
  authCode: z.string().trim().min(1, 'Authorization code is required'),
});

type GOGAuthCodeForm = z.infer<typeof gogAuthCodeSchema>;

interface GOGConnectionCardProps {
  isConfigured: boolean;
  onConnectionChange: () => void;
}

export function GOGConnectionCard({
  isConfigured,
  onConnectionChange,
}: GOGConnectionCardProps) {
  const { data: connection } = useGOGConnection();
  const connectMutation = useConnectGOG();
  const disconnectMutation = useDisconnectGOG();

  const {
    register,
    handleSubmit,
    formState: { errors },
    setError,
    reset,
  } = useForm<GOGAuthCodeForm>({
    resolver: zodResolver(gogAuthCodeSchema),
  });

  const isConnecting = connectMutation.isPending;
  const isDisconnecting = disconnectMutation.isPending;
  const username = connection?.username;
  const userId = connection?.userId;

  const onSubmit = async (data: GOGAuthCodeForm) => {
    try {
      const result = await connectMutation.mutateAsync(data.authCode);
      toast.success(`GOG connected as ${result.username}`);
      reset();
      onConnectionChange();
    } catch (err) {
      const message = err instanceof Error ? err.message : 'Failed to connect GOG';
      setError('authCode', { message });
      toast.error(message);
    }
  };

  const handleDisconnect = async () => {
    try {
      await disconnectMutation.mutateAsync();
      toast.success('GOG disconnected');
      onConnectionChange();
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Failed to disconnect GOG');
    }
  };

  const getBadgeState = () => {
    if (!isConfigured) {
      return { label: 'Not Configured', className: 'bg-muted text-muted-foreground' };
    }
    return {
      label: 'Connected',
      className: 'bg-green-100 text-green-800 dark:bg-green-900/30 dark:text-green-400',
    };
  };

  const badgeState = getBadgeState();

  return (
    <Card>
      <CardHeader>
        <div className="flex items-center justify-between">
          <div>
            <CardTitle>GOG Connection</CardTitle>
            <CardDescription>
              {isConfigured
                ? 'Your GOG account is connected'
                : 'Connect your GOG account to sync your game library'}
            </CardDescription>
          </div>
          <Badge variant="outline" className={badgeState.className}>
            {badgeState.label}
          </Badge>
        </div>
      </CardHeader>
      <CardContent>
        {isConfigured ? (
          <div className="space-y-4">
            <div className="flex items-center gap-3 rounded-lg border bg-muted/50 p-4">
              <Check className="h-5 w-5 text-green-600" />
              <div>
                <p className="font-medium">Connected as {username}</p>
                {userId && (
                  <p className="text-sm text-muted-foreground font-mono">{userId}</p>
                )}
              </div>
            </div>

            <Alert>
              <Info className="h-4 w-4" />
              <AlertDescription>
                <strong>Note:</strong> GOG does not provide playtime data via its library API.
              </AlertDescription>
            </Alert>

            <AlertDialog>
              <AlertDialogTrigger asChild>
                <Button variant="outline" disabled={isDisconnecting}>
                  {isDisconnecting ? (
                    <>
                      <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                      Disconnecting...
                    </>
                  ) : (
                    'Disconnect'
                  )}
                </Button>
              </AlertDialogTrigger>
              <AlertDialogContent>
                <AlertDialogHeader>
                  <AlertDialogTitle>Disconnect GOG?</AlertDialogTitle>
                  <AlertDialogDescription>
                    Your sync settings will be preserved but syncing will stop until you reconnect.
                  </AlertDialogDescription>
                </AlertDialogHeader>
                <AlertDialogFooter>
                  <AlertDialogCancel>Cancel</AlertDialogCancel>
                  <AlertDialogAction onClick={handleDisconnect}>Disconnect</AlertDialogAction>
                </AlertDialogFooter>
              </AlertDialogContent>
            </AlertDialog>
          </div>
        ) : (
          <form onSubmit={handleSubmit(onSubmit)} className="space-y-4">
            <Alert>
              <Info className="h-4 w-4" />
              <AlertDescription>
                <strong>Note:</strong> GOG does not provide playtime data via its library API.
              </AlertDescription>
            </Alert>

            <div className="space-y-2">
              <Label htmlFor="authCode">Authorization Code</Label>
              <Input
                id="authCode"
                type="text"
                placeholder="Paste the authorization code from GOG"
                autoComplete="off"
                {...register('authCode')}
                disabled={isConnecting}
              />
              {errors.authCode && (
                <p className="text-sm text-destructive">{errors.authCode.message}</p>
              )}

              <Accordion type="single" collapsible className="w-full">
                <AccordionItem value="gog-code-help" className="border-none">
                  <AccordionTrigger className="py-2 text-sm text-muted-foreground hover:no-underline">
                    How do I get an authorization code?
                  </AccordionTrigger>
                  <AccordionContent className="text-sm text-muted-foreground">
                    <div className="space-y-2 rounded-lg bg-muted/50 p-3">
                      <p className="font-medium text-foreground">
                        GOG requires you to log in once to issue a short-lived authorization code.
                      </p>
                      <ol className="list-inside list-decimal space-y-1">
                        <li>
                          <a
                            href={GOG_AUTH_URL}
                            target="_blank"
                            rel="noopener noreferrer"
                            className="text-primary hover:underline"
                          >
                            Open the GOG login page{' '}
                            <ExternalLink className="inline h-3 w-3" />
                          </a>{' '}
                          in a new tab
                        </li>
                        <li>Sign in with your GOG account if prompted</li>
                        <li>
                          After login, you will be redirected to a GOG page — copy the{' '}
                          <code>code</code> value from the URL (it appears as{' '}
                          <code>?code=…</code>)
                        </li>
                        <li>Paste the code into the field above</li>
                      </ol>
                      <div className="mt-2 rounded border border-yellow-200 bg-yellow-50 p-2 text-yellow-800 dark:border-yellow-800 dark:bg-yellow-900/20 dark:text-yellow-200">
                        <strong>Note:</strong> The authorization code is single-use and expires
                        within a few minutes. Paste it as soon as you copy it.
                      </div>
                    </div>
                  </AccordionContent>
                </AccordionItem>
              </Accordion>
            </div>

            <Button type="submit" disabled={isConnecting} className="w-full">
              {isConnecting ? (
                <>
                  <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                  Connecting...
                </>
              ) : (
                'Connect GOG'
              )}
            </Button>
          </form>
        )}
      </CardContent>
    </Card>
  );
}
