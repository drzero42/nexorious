import { useForm } from 'react-hook-form';
import { zodResolver } from '@hookform/resolvers/zod';
import { z } from 'zod';
import { toast } from 'sonner';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Alert, AlertDescription } from '@/components/ui/alert';
import { ExternalLink, Info } from 'lucide-react';
import {
  useConnectEpicGamesStore,
  useDisconnectEpicGamesStore,
  useEpicGamesStoreConnection,
} from '@/hooks';
import { EPIC_AUTH_URL } from '@/types';
import {
  CodeHelpAccordion,
  ConnectSubmitButton,
  ConnectedSummary,
  CredentialsErrorBanner,
  DisconnectDialog,
} from './connection';

const epicGamesStoreAuthCodeSchema = z.object({
  authCode: z.string().trim().min(1, 'Authorization code is required'),
});

type EpicGamesStoreAuthCodeForm = z.infer<typeof epicGamesStoreAuthCodeSchema>;

function disabledMessage(reason?: string): string {
  switch (reason) {
    case 'legendary_not_configured':
    default:
      return 'Epic Games Store sync is disabled on this server. Contact your administrator to enable it.';
  }
}

interface EpicGamesStoreConnectionCardProps {
  isConfigured: boolean;
  credentialsError?: boolean;
  onConnectionChange: () => void;
}

export function EpicGamesStoreConnectionCard({
  isConfigured,
  credentialsError = false,
  onConnectionChange,
}: EpicGamesStoreConnectionCardProps) {
  const { data: connection } = useEpicGamesStoreConnection();
  const connectMutation = useConnectEpicGamesStore();
  const disconnectMutation = useDisconnectEpicGamesStore();

  const {
    register,
    handleSubmit,
    formState: { errors },
    setError,
    reset,
  } = useForm<EpicGamesStoreAuthCodeForm>({
    resolver: zodResolver(epicGamesStoreAuthCodeSchema),
  });

  const isConnecting = connectMutation.isPending;
  const isDisconnecting = disconnectMutation.isPending;
  const isDisabled = connection?.disabled === true;
  const displayName = connection?.displayName;
  const resolvedCredentialsError = connection?.credentialsError ?? credentialsError;

  const onSubmit = async (data: EpicGamesStoreAuthCodeForm) => {
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

  const playtimeNote = (
    <Alert>
      <Info className="h-4 w-4" />
      <AlertDescription>
        <strong>Note:</strong> Epic Games Store does not provide playtime data.
      </AlertDescription>
    </Alert>
  );

  return (
    <Card>
      <CardHeader>
        <div>
          <CardTitle>Epic Games Store Connection</CardTitle>
          <CardDescription>
            {isConfigured
              ? 'Your Epic Games Store account is connected'
              : 'Connect your Epic Games Store account to sync your game library'}
          </CardDescription>
        </div>
      </CardHeader>
      <CardContent>
        {isDisabled ? (
          <Alert>
            <Info className="h-4 w-4" />
            <AlertDescription>{disabledMessage(connection?.reason)}</AlertDescription>
          </Alert>
        ) : isConfigured && !resolvedCredentialsError ? (
          <div className="space-y-4">
            <ConnectedSummary name={displayName} />
            {playtimeNote}
            <DisconnectDialog
              serviceLabel="Epic Games Store"
              isDisconnecting={isDisconnecting}
              onDisconnect={handleDisconnect}
            />
          </div>
        ) : (
          <div className="space-y-4">
            {resolvedCredentialsError && (
              <CredentialsErrorBanner
                title="Epic Games credentials are invalid or could not be decrypted"
                description="Please re-authorize with Epic Games to continue syncing your library."
              />
            )}
            <form onSubmit={handleSubmit(onSubmit)} className="space-y-4">
              {playtimeNote}

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

                <CodeHelpAccordion
                  value="epic-code-help"
                  trigger="How do I get an authorization code?"
                >
                  <p className="font-medium text-foreground">
                    Epic Games requires you to log in once to issue a short-lived authorization
                    code.
                  </p>
                  <ol className="list-inside list-decimal space-y-1">
                    <li>
                      <a
                        href={EPIC_AUTH_URL}
                        target="_blank"
                        rel="noopener noreferrer"
                        className="text-primary hover:underline"
                      >
                        Open the Epic Games login page <ExternalLink className="inline h-3 w-3" />
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
                    <strong>Note:</strong> The authorization code is single-use and expires within a
                    few minutes. Paste it as soon as you copy it.
                  </div>
                </CodeHelpAccordion>
              </div>

              <ConnectSubmitButton
                isPending={isConnecting}
                idleLabel={resolvedCredentialsError ? 'Reconfigure' : 'Connect Epic Games Store'}
                pendingLabel={resolvedCredentialsError ? 'Reconfiguring...' : 'Connecting...'}
              />
            </form>
          </div>
        )}
      </CardContent>
    </Card>
  );
}
