'use client';

import { useForm } from 'react-hook-form';
import { zodResolver } from '@hookform/resolvers/zod';
import { z } from 'zod';
import { toast } from 'sonner';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Badge } from '@/components/ui/badge';
import { Label } from '@/components/ui/label';
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
import { Loader2, Check, ExternalLink, AlertTriangle } from 'lucide-react';
import { useConfigurePSN, useDisconnectPSN } from '@/hooks';

const psnCredentialsSchema = z.object({
  npssoToken: z
    .string()
    .length(64, 'NPSSO token must be exactly 64 characters')
    .regex(/^[A-Za-z0-9]+$/, 'NPSSO token must contain only alphanumeric characters'),
});

type PSNCredentialsForm = z.infer<typeof psnCredentialsSchema>;

interface PSNConnectionCardProps {
  isConfigured: boolean;
  tokenExpired: boolean;
  accountId?: string;
  onlineId?: string;
  onConnectionChange: () => void;
}

export function PSNConnectionCard({
  isConfigured,
  tokenExpired,
  accountId,
  onlineId,
  onConnectionChange,
}: PSNConnectionCardProps) {
  const {
    register,
    handleSubmit,
    formState: { errors },
    setError,
  } = useForm<PSNCredentialsForm>({
    resolver: zodResolver(psnCredentialsSchema),
  });

  const configureMutation = useConfigurePSN();
  const disconnectMutation = useDisconnectPSN();

  const isConfiguring = configureMutation.isPending;
  const isDisconnecting = disconnectMutation.isPending;

  const onSubmit = async (data: PSNCredentialsForm) => {
    try {
      const result = await configureMutation.mutateAsync(data.npssoToken);

      if (!result.valid) {
        const errorMessage = result.error || 'Configuration failed';
        setError('npssoToken', { message: errorMessage });
        toast.error(errorMessage);
        return;
      }

      toast.success('PlayStation Network connected successfully');
      onConnectionChange();
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Failed to connect PlayStation Network');
    }
  };

  const handleDisconnect = async () => {
    try {
      await disconnectMutation.mutateAsync();
      toast.success('PlayStation Network disconnected');
      onConnectionChange();
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Failed to disconnect PlayStation Network');
    }
  };

  const getBadgeState = () => {
    if (tokenExpired) {
      return {
        label: 'Token Expired',
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

  // Show form if not configured OR token expired
  const showForm = !isConfigured || tokenExpired;

  return (
    <Card>
      <CardHeader>
        <div className="flex items-center justify-between">
          <div>
            <CardTitle>PlayStation Network Connection</CardTitle>
            <CardDescription>
              {isConfigured
                ? 'Your PlayStation Network account is connected'
                : 'Connect your PlayStation Network account to sync your game library'}
            </CardDescription>
          </div>
          <Badge variant="outline" className={badgeState.className}>
            {badgeState.label}
          </Badge>
        </div>
      </CardHeader>
      <CardContent>
        {showForm ? (
          <div className="space-y-4">
            {tokenExpired && (
              <div className="flex items-start gap-3 rounded-lg border border-yellow-200 bg-yellow-50 p-4 dark:border-yellow-800 dark:bg-yellow-900/20">
                <AlertTriangle className="h-5 w-5 text-yellow-600 dark:text-yellow-400" />
                <div>
                  <p className="font-medium text-yellow-800 dark:text-yellow-200">
                    Your NPSSO token has expired
                  </p>
                  <p className="text-sm text-yellow-700 dark:text-yellow-300">
                    Please enter a new NPSSO token to continue syncing your PlayStation games.
                  </p>
                </div>
              </div>
            )}

            <form onSubmit={handleSubmit(onSubmit)} className="space-y-4">
              <div className="space-y-2">
                <Label htmlFor="npssoToken">NPSSO Token</Label>
                <Input
                  id="npssoToken"
                  type="password"
                  placeholder="64-character alphanumeric token"
                  {...register('npssoToken')}
                  disabled={isConfiguring}
                />
                {errors.npssoToken && (
                  <p className="text-sm text-destructive">{errors.npssoToken.message}</p>
                )}

                <Accordion type="single" collapsible className="w-full">
                  <AccordionItem value="npsso-help" className="border-none">
                    <AccordionTrigger className="py-2 text-sm text-muted-foreground hover:no-underline">
                      How do I get my NPSSO token?
                    </AccordionTrigger>
                    <AccordionContent className="text-sm text-muted-foreground">
                      <div className="space-y-2 rounded-lg bg-muted/50 p-3">
                        <p className="font-medium text-foreground">
                          Your NPSSO token is a session cookie that allows Nexorious to access your
                          PlayStation game library.
                        </p>
                        <ol className="list-inside list-decimal space-y-1">
                          <li>
                            Go to{' '}
                            <a
                              href="https://ca.account.sony.com/api/v1/ssocookie"
                              target="_blank"
                              rel="noopener noreferrer"
                              className="text-primary hover:underline"
                            >
                              Sony SSO Cookie Page{' '}
                              <ExternalLink className="inline h-3 w-3" />
                            </a>
                          </li>
                          <li>Sign in with your PlayStation Network account if prompted</li>
                          <li>
                            Copy the entire 64-character code that appears on the page (it will look
                            like a long string of letters and numbers)
                          </li>
                          <li>Paste it into the field above</li>
                        </ol>
                        <div className="mt-2 space-y-2">
                          <div className="rounded border border-yellow-200 bg-yellow-50 p-2 text-yellow-800 dark:border-yellow-800 dark:bg-yellow-900/20 dark:text-yellow-200">
                            <strong>Security Note:</strong> Your NPSSO token expires approximately
                            every 2 months. You&apos;ll need to get a new token when it expires.
                            The token is stored securely and only used to sync your library.
                          </div>
                          <div className="rounded border border-blue-200 bg-blue-50 p-2 text-blue-800 dark:border-blue-800 dark:bg-blue-900/20 dark:text-blue-200">
                            <strong>Note:</strong> PS3 games cannot be synced due to PlayStation
                            Network API limitations. Only PS4, PS5, and PS Vita games are
                            supported.
                          </div>
                        </div>
                      </div>
                    </AccordionContent>
                  </AccordionItem>
                </Accordion>
              </div>

              <Button type="submit" disabled={isConfiguring} className="w-full">
                {isConfiguring ? (
                  <>
                    <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                    {tokenExpired ? 'Reconfiguring...' : 'Configuring...'}
                  </>
                ) : (
                  <>{tokenExpired ? 'Reconfigure' : 'Configure PSN'}</>
                )}
              </Button>
            </form>
          </div>
        ) : (
          <div className="space-y-4">
            <div className="flex items-center gap-3 rounded-lg border bg-muted/50 p-4">
              <Check className="h-5 w-5 text-green-600" />
              <div>
                <p className="font-medium">Connected as {onlineId}</p>
                {accountId && <p className="text-sm text-muted-foreground">{accountId}</p>}
              </div>
            </div>

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
                  <AlertDialogTitle>Disconnect PlayStation Network?</AlertDialogTitle>
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
        )}
      </CardContent>
    </Card>
  );
}
