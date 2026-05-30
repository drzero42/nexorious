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
import { Loader2, Check, ExternalLink, Info, AlertTriangle } from 'lucide-react';
import { useConnectEpic, useDisconnectEpic, useEpicConnection } from '@/hooks';
import { EPIC_AUTH_URL } from '@/types';

const epicAuthCodeSchema = z.object({
  authCode: z.string().trim().min(1, 'Authorization code is required'),
});

type EpicAuthCodeForm = z.infer<typeof epicAuthCodeSchema>;

interface EpicConnectionCardProps {
  isConfigured: boolean;
  credentialsError?: boolean;
  onConnectionChange: () => void;
}

export function EpicConnectionCard({
  isConfigured,
  credentialsError = false,
  onConnectionChange,
}: EpicConnectionCardProps) {
  const { data: connection } = useEpicConnection();
  const connectMutation = useConnectEpic();
  const disconnectMutation = useDisconnectEpic();

  const {
    register,
    handleSubmit,
    formState: { errors },
    setError,
    reset,
  } = useForm<EpicAuthCodeForm>({
    resolver: zodResolver(epicAuthCodeSchema),
  });

  const isConnecting = connectMutation.isPending;
  const isDisconnecting = disconnectMutation.isPending;
  const isDisabled = connection?.disabled === true;
  const displayName = connection?.displayName;
  const accountId = connection?.accountId;
  const resolvedCredentialsError = connection?.credentialsError ?? credentialsError;

  const onSubmit = async (data: EpicAuthCodeForm) => {
    try {
      const result = await connectMutation.mutateAsync(data.authCode);
      toast.success(`Epic Games connected as ${result.displayName}`);
      reset();
      onConnectionChange();
    } catch (err) {
      const message = err instanceof Error ? err.message : 'Failed to connect Epic Games Store';
      setError('authCode', { message });
      toast.error(message);
    }
  };

  const handleDisconnect = async () => {
    try {
      await disconnectMutation.mutateAsync();
      toast.success('Epic Games Store disconnected');
      onConnectionChange();
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Failed to disconnect Epic Games Store');
    }
  };

  const getBadgeState = () => {
    if (isDisabled) {
      return {
        label: 'Disabled',
        className: 'bg-muted text-muted-foreground',
      };
    }
    if (resolvedCredentialsError) {
      return {
        label: 'Credentials Error',
        className: 'bg-yellow-100 text-yellow-800 dark:bg-yellow-900/30 dark:text-yellow-400',
      };
    }
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
            <CardTitle>Epic Games Store Connection</CardTitle>
            <CardDescription>
              {isConfigured
                ? 'Your Epic Games Store account is connected'
                : 'Connect your Epic Games Store account to sync your game library'}
            </CardDescription>
          </div>
          <Badge variant="outline" className={badgeState.className}>
            {badgeState.label}
          </Badge>
        </div>
      </CardHeader>
      <CardContent>
        {isDisabled ? (
          <Alert>
            <Info className="h-4 w-4" />
            <AlertDescription>
              Epic Games Store sync is disabled on this server. The administrator must set the
              <code className="mx-1 rounded bg-muted px-1 py-0.5 text-xs">LEGENDARY_WORK_DIR</code>
              environment variable to enable it.
            </AlertDescription>
          </Alert>
        ) : isConfigured && !resolvedCredentialsError ? (
          <div className="space-y-4">
            <div className="flex items-center gap-3 rounded-lg border bg-muted/50 p-4">
              <Check className="h-5 w-5 text-green-600" />
              <div>
                <p className="font-medium">Connected as {displayName}</p>
                {accountId && (
                  <p className="text-sm text-muted-foreground font-mono">{accountId}</p>
                )}
              </div>
            </div>

            <Alert>
              <Info className="h-4 w-4" />
              <AlertDescription>
                <strong>Note:</strong> Epic Games Store does not provide playtime data.
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
                  <AlertDialogTitle>Disconnect Epic Games Store?</AlertDialogTitle>
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
          <div className="space-y-4">
            {resolvedCredentialsError && (
              <div className="flex items-start gap-3 rounded-lg border border-yellow-200 bg-yellow-50 p-4 dark:border-yellow-800 dark:bg-yellow-900/20">
                <AlertTriangle className="h-5 w-5 text-yellow-600 dark:text-yellow-400" />
                <div>
                  <p className="font-medium text-yellow-800 dark:text-yellow-200">
                    Epic Games credentials are invalid or could not be decrypted
                  </p>
                  <p className="text-sm text-yellow-700 dark:text-yellow-300">
                    Please re-authorize with Epic Games to continue syncing your library.
                  </p>
                </div>
              </div>
            )}
            <form onSubmit={handleSubmit(onSubmit)} className="space-y-4">
              <Alert>
                <Info className="h-4 w-4" />
                <AlertDescription>
                  <strong>Note:</strong> Epic Games Store does not provide playtime data.
                </AlertDescription>
              </Alert>

              <div className="space-y-2">
                <Label htmlFor="authCode">Authorization Code</Label>
                <Input
                  id="authCode"
                  type="text"
                  placeholder="Paste the authorization code from Epic Games"
                  autoComplete="off"
                  {...register('authCode')}
                  disabled={isConnecting}
                />
                {errors.authCode && (
                  <p className="text-sm text-destructive">{errors.authCode.message}</p>
                )}

                <Accordion type="single" collapsible className="w-full">
                  <AccordionItem value="epic-code-help" className="border-none">
                    <AccordionTrigger className="py-2 text-sm text-muted-foreground hover:no-underline">
                      How do I get an authorization code?
                    </AccordionTrigger>
                    <AccordionContent className="text-sm text-muted-foreground">
                      <div className="space-y-2 rounded-lg bg-muted/50 p-3">
                        <p className="font-medium text-foreground">
                          Epic Games requires you to log in once to issue a short-lived
                          authorization code.
                        </p>
                        <ol className="list-inside list-decimal space-y-1">
                          <li>
                            <a
                              href={EPIC_AUTH_URL}
                              target="_blank"
                              rel="noopener noreferrer"
                              className="text-primary hover:underline"
                            >
                              Open the Epic Games login page{' '}
                              <ExternalLink className="inline h-3 w-3" />
                            </a>{' '}
                            in a new tab
                          </li>
                          <li>Sign in with your Epic Games account if prompted</li>
                          <li>
                            The page will display a JSON response containing an{' '}
                            <code>authorizationCode</code> value
                          </li>
                          <li>Copy the code and paste it into the field above</li>
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
                    {resolvedCredentialsError ? 'Reconfiguring...' : 'Connecting...'}
                  </>
                ) : (
                  <>{resolvedCredentialsError ? 'Reconfigure' : 'Connect Epic Games Store'}</>
                )}
              </Button>
            </form>
          </div>
        )}
      </CardContent>
    </Card>
  );
}
