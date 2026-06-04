import { useState } from 'react';
import { useForm } from 'react-hook-form';
import { zodResolver } from '@hookform/resolvers/zod';
import { z } from 'zod';
import { toast } from 'sonner';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { ExternalLink } from 'lucide-react';
import { useVerifySteamCredentials, useDisconnectSteam } from '@/hooks';
import { STEAM_VERIFY_ERROR_MESSAGES } from '@/types';
import {
  CodeHelpAccordion,
  ConnectSubmitButton,
  ConnectedSummary,
  CredentialsErrorBanner,
  DisconnectDialog,
} from './connection';

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
  credentialsError?: boolean;
  steamId?: string;
  steamUsername?: string;
  onConnectionChange: () => void;
}

export function SteamConnectionCard({
  isConfigured,
  credentialsError = false,
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

  const isVerifying = verifyMutation.isPending;
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

      // Credentials are persisted server-side by the verify step (into the
      // encrypted user_sync_configs blob); no client-side profile write needed.
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

  return (
    <Card>
      <CardHeader>
        <div>
          <CardTitle>Steam Connection</CardTitle>
          <CardDescription>
            {isConfigured
              ? 'Your Steam account is connected'
              : 'Connect your Steam account to sync your game library'}
          </CardDescription>
        </div>
      </CardHeader>
      <CardContent>
        {isConfigured && !credentialsError ? (
          <div className="space-y-4">
            <ConnectedSummary
              name={steamUsername || verifiedUsername || undefined}
              secondary={steamId}
            />
            <DisconnectDialog
              serviceLabel="Steam"
              isDisconnecting={isDisconnecting}
              onDisconnect={handleDisconnect}
            />
          </div>
        ) : (
          <div className="space-y-4">
            {credentialsError && (
              <CredentialsErrorBanner
                title="Steam credentials are invalid or could not be decrypted"
                description="Please re-enter your Steam credentials to continue syncing."
              />
            )}
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

                <CodeHelpAccordion value="steam-id-help" trigger="How do I find my Steam ID?">
                  <p className="font-medium text-foreground">
                    Your Steam ID is a 17-digit number that uniquely identifies your account.
                  </p>
                  <ol className="list-inside list-decimal space-y-1">
                    <li>
                      Open Steam and go to your <strong>Profile</strong>
                    </li>
                    <li>
                      Look at the URL:
                      <ul className="ml-4 list-inside list-disc">
                        <li>
                          If it shows <code>steamcommunity.com/profiles/76561198...</code>, that
                          number is your Steam ID
                        </li>
                        <li>
                          If it shows <code>steamcommunity.com/id/customname/</code>, you have a
                          custom URL
                        </li>
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
                    <strong>Important:</strong> Your Steam profile must be set to{' '}
                    <strong>Public</strong> for sync to work.
                    <ol className="ml-4 mt-1 list-inside list-decimal">
                      <li>Go to Steam → Settings → Privacy Settings</li>
                      <li>Set &quot;My profile&quot; to Public</li>
                      <li>Set &quot;Game details&quot; to Public</li>
                    </ol>
                  </div>
                </CodeHelpAccordion>
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

                <CodeHelpAccordion value="api-key-help" trigger="How do I get an API key?">
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
                    <li>
                      Enter a domain name (you can use <code>localhost</code> or any domain)
                    </li>
                    <li>
                      Click <strong>Register</strong> and copy the 32-character key
                    </li>
                  </ol>
                  <p className="mt-2 text-xs">
                    <strong>Note:</strong> Keep your API key private. It&apos;s stored securely and
                    only used to sync your library.
                  </p>
                </CodeHelpAccordion>
              </div>

              <ConnectSubmitButton
                isPending={isVerifying}
                idleLabel={credentialsError ? 'Reconfigure' : 'Verify & Connect'}
                pendingLabel={credentialsError ? 'Reconfiguring...' : 'Verifying...'}
              />
            </form>
          </div>
        )}
      </CardContent>
    </Card>
  );
}
