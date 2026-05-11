
import { useState } from 'react';
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
import { Loader2, Check, ExternalLink } from 'lucide-react';
import { useVerifySteamCredentials, useDisconnectSteam } from '@/hooks';
import { useUpdateProfile } from '@/hooks/use-auth';
import { STEAM_VERIFY_ERROR_MESSAGES } from '@/types';

const steamCredentialsSchema = z.object({
  steamId: z
    .string()
    .min(17, 'Steam ID must be 17 digits')
    .max(17, 'Steam ID must be 17 digits')
    .regex(/^7656119\d{10}$/, 'Invalid Steam ID format'),
  webApiKey: z
    .string()
    .length(32, 'API key must be 32 characters')
    .regex(/^[A-Fa-f0-9]{32}$/, 'Invalid API key format'),
});

type SteamCredentialsForm = z.infer<typeof steamCredentialsSchema>;

interface SteamConnectionCardProps {
  isConfigured: boolean;
  steamId?: string;
  steamUsername?: string;
  onConnectionChange: () => void;
}

export function SteamConnectionCard({
  isConfigured,
  steamId,
  steamUsername,
  onConnectionChange,
}: SteamConnectionCardProps) {
  const [verifiedUsername, setVerifiedUsername] = useState<string | null>(null);

  const {
    register,
    handleSubmit,
    formState: { errors },
    setError,
  } = useForm<SteamCredentialsForm>({
    resolver: zodResolver(steamCredentialsSchema),
  });

  const verifyMutation = useVerifySteamCredentials();
  const disconnectMutation = useDisconnectSteam();
  const updateProfileMutation = useUpdateProfile();

  const isVerifying = verifyMutation.isPending || updateProfileMutation.isPending;
  const isDisconnecting = disconnectMutation.isPending;

  const onSubmit = async (data: SteamCredentialsForm) => {
    try {
      // First verify credentials with Steam API
      const result = await verifyMutation.mutateAsync({
        steamId: data.steamId,
        webApiKey: data.webApiKey,
      });

      if (!result.valid) {
        const errorMessage = result.error
          ? STEAM_VERIFY_ERROR_MESSAGES[result.error] || 'Verification failed'
          : 'Verification failed';

        if (result.error === 'invalid_steam_id') {
          setError('steamId', { message: errorMessage });
        } else if (result.error === 'invalid_api_key') {
          setError('webApiKey', { message: errorMessage });
        } else {
          toast.error(errorMessage);
        }
        return;
      }

      setVerifiedUsername(result.steamUsername);

      // Save credentials to user preferences
      await updateProfileMutation.mutateAsync({
        preferences: {
          steam: {
            steam_id: data.steamId,
            web_api_key: data.webApiKey,
            is_verified: true,
            username: result.steamUsername,
          },
        },
      });

      toast.success('Steam connected successfully');
      onConnectionChange();
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Failed to connect Steam');
    }
  };

  const handleDisconnect = async () => {
    try {
      await disconnectMutation.mutateAsync();
      toast.success('Steam disconnected');
      onConnectionChange();
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Failed to disconnect Steam');
    }
  };

  const getBadgeState = () => {
    if (!isConfigured) return { label: 'Not Configured', className: 'bg-muted text-muted-foreground' };
    return { label: 'Connected', className: 'bg-green-100 text-green-800 dark:bg-green-900/30 dark:text-green-400' };
  };

  const badgeState = getBadgeState();

  return (
    <Card>
      <CardHeader>
        <div className="flex items-center justify-between">
          <div>
            <CardTitle>Steam Connection</CardTitle>
            <CardDescription>
              {isConfigured
                ? 'Your Steam account is connected'
                : 'Connect your Steam account to sync your game library'}
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
                <p className="font-medium">
                  Connected as {steamUsername || verifiedUsername}
                </p>
                {steamId && (
                  <p className="text-sm text-muted-foreground">{steamId}</p>
                )}
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
                  <AlertDialogTitle>Disconnect Steam?</AlertDialogTitle>
                  <AlertDialogDescription>
                    Your sync settings will be preserved but syncing will stop until you reconnect.
                  </AlertDialogDescription>
                </AlertDialogHeader>
                <AlertDialogFooter>
                  <AlertDialogCancel>Cancel</AlertDialogCancel>
                  <AlertDialogAction onClick={handleDisconnect}>
                    Disconnect
                  </AlertDialogAction>
                </AlertDialogFooter>
              </AlertDialogContent>
            </AlertDialog>
          </div>
        ) : (
          <form onSubmit={handleSubmit(onSubmit)} className="space-y-4">
            <div className="space-y-2">
              <Label htmlFor="steamId">Steam ID</Label>
              <Input
                id="steamId"
                placeholder="76561198012345678"
                {...register('steamId')}
                disabled={isVerifying}
              />
              {errors.steamId && (
                <p className="text-sm text-destructive">{errors.steamId.message}</p>
              )}

              <Accordion type="single" collapsible className="w-full">
                <AccordionItem value="steam-id-help" className="border-none">
                  <AccordionTrigger className="py-2 text-sm text-muted-foreground hover:no-underline">
                    How do I find my Steam ID?
                  </AccordionTrigger>
                  <AccordionContent className="text-sm text-muted-foreground">
                    <div className="space-y-2 rounded-lg bg-muted/50 p-3">
                      <p className="font-medium text-foreground">
                        Your Steam ID is a 17-digit number that uniquely identifies your account.
                      </p>
                      <ol className="list-inside list-decimal space-y-1">
                        <li>Open Steam and go to your <strong>Profile</strong></li>
                        <li>Look at the URL:
                          <ul className="ml-4 list-inside list-disc">
                            <li>If it shows <code>steamcommunity.com/profiles/76561198...</code>, that number is your Steam ID</li>
                            <li>If it shows <code>steamcommunity.com/id/customname/</code>, you have a custom URL</li>
                          </ul>
                        </li>
                        <li>
                          <strong>If you have a custom URL:</strong> Go to{' '}
                          <a
                            href="https://steamid.io"
                            target="_blank"
                            rel="noopener noreferrer"
                            className="text-primary hover:underline"
                          >
                            steamid.io <ExternalLink className="inline h-3 w-3" />
                          </a>
                          , paste your profile URL, and copy the <strong>steamID64</strong> value
                        </li>
                      </ol>
                      <div className="mt-2 rounded border border-yellow-200 bg-yellow-50 p-2 text-yellow-800 dark:border-yellow-800 dark:bg-yellow-900/20 dark:text-yellow-200">
                        <strong>Important:</strong> Your Steam profile must be set to <strong>Public</strong> for sync to work.
                        <ol className="ml-4 mt-1 list-inside list-decimal">
                          <li>Go to Steam → Settings → Privacy Settings</li>
                          <li>Set &quot;My profile&quot; to Public</li>
                          <li>Set &quot;Game details&quot; to Public</li>
                        </ol>
                      </div>
                    </div>
                  </AccordionContent>
                </AccordionItem>
              </Accordion>
            </div>

            <div className="space-y-2">
              <Label htmlFor="webApiKey">Steam Web API Key</Label>
              <Input
                id="webApiKey"
                type="password"
                placeholder="********************************"
                {...register('webApiKey')}
                disabled={isVerifying}
              />
              {errors.webApiKey && (
                <p className="text-sm text-destructive">{errors.webApiKey.message}</p>
              )}

              <Accordion type="single" collapsible className="w-full">
                <AccordionItem value="api-key-help" className="border-none">
                  <AccordionTrigger className="py-2 text-sm text-muted-foreground hover:no-underline">
                    How do I get an API key?
                  </AccordionTrigger>
                  <AccordionContent className="text-sm text-muted-foreground">
                    <div className="space-y-2 rounded-lg bg-muted/50 p-3">
                      <p className="font-medium text-foreground">
                        A Steam Web API key allows Nexorious to read your game library.
                      </p>
                      <ol className="list-inside list-decimal space-y-1">
                        <li>
                          Go to{' '}
                          <a
                            href="https://steamcommunity.com/dev/apikey"
                            target="_blank"
                            rel="noopener noreferrer"
                            className="text-primary hover:underline"
                          >
                            Steam Web API Key Registration <ExternalLink className="inline h-3 w-3" />
                          </a>
                        </li>
                        <li>Sign in with your Steam account if prompted</li>
                        <li>Enter a domain name (you can use <code>localhost</code> or any domain)</li>
                        <li>Click <strong>Register</strong> and copy the 32-character key</li>
                      </ol>
                      <p className="mt-2 text-xs">
                        <strong>Note:</strong> Keep your API key private. It&apos;s stored securely and only used to sync your library.
                      </p>
                    </div>
                  </AccordionContent>
                </AccordionItem>
              </Accordion>
            </div>

            <Button type="submit" disabled={isVerifying} className="w-full">
              {isVerifying ? (
                <>
                  <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                  Verifying...
                </>
              ) : (
                'Verify & Connect'
              )}
            </Button>
          </form>
        )}
      </CardContent>
    </Card>
  );
}
