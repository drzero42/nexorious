import { useForm } from 'react-hook-form';
import { zodResolver } from '@hookform/resolvers/zod';
import { z } from 'zod';
import { toast } from 'sonner';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Alert, AlertDescription } from '@/components/ui/alert';
import { ExternalLink, Info } from 'lucide-react';
import { useConnectGOG, useDisconnectGOG, useGOGConnection } from '@/hooks';
import { GOG_AUTH_URL } from '@/types';
import {
  CodeHelpAccordion,
  ConnectSubmitButton,
  ConnectedSummary,
  CredentialsErrorBanner,
  DisconnectDialog,
} from './connection';

const gogAuthCodeSchema = z.object({
  authCode: z.string().trim().min(1, 'Enter the GOG URL or authorization code'),
});

type GOGAuthCodeForm = z.infer<typeof gogAuthCodeSchema>;

interface GOGConnectionCardProps {
  isConfigured: boolean;
  credentialsError?: boolean;
  onConnectionChange: () => void;
}

export function GOGConnectionCard({
  isConfigured,
  credentialsError = false,
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
  const resolvedCredentialsError = connection?.credentialsError ?? credentialsError;

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

  const playtimeNote = (
    <Alert>
      <Info className="h-4 w-4" />
      <AlertDescription>
        <strong>Note:</strong> GOG does not provide playtime data via its library API.
      </AlertDescription>
    </Alert>
  );

  return (
    <Card>
      <CardHeader>
        <div>
          <CardTitle>GOG Connection</CardTitle>
          <CardDescription>
            {isConfigured
              ? 'Your GOG account is connected'
              : 'Connect your GOG account to sync your game library'}
          </CardDescription>
        </div>
      </CardHeader>
      <CardContent>
        {isConfigured && !resolvedCredentialsError ? (
          <div className="space-y-4">
            <ConnectedSummary name={username} />
            {playtimeNote}
            <DisconnectDialog
              serviceLabel="GOG"
              isDisconnecting={isDisconnecting}
              onDisconnect={handleDisconnect}
            />
          </div>
        ) : (
          <div className="space-y-4">
            {resolvedCredentialsError && (
              <CredentialsErrorBanner
                title="GOG credentials are invalid or could not be decrypted"
                description="Please re-authorize with GOG to continue syncing your library."
              />
            )}
            <form onSubmit={handleSubmit(onSubmit)} className="space-y-4">
              {playtimeNote}

              <div className="space-y-2">
                <Label htmlFor="authCode">GOG URL or Authorization Code</Label>
                <Input
                  id="authCode"
                  type="text"
                  placeholder="Paste the full GOG URL or just the code"
                  autoComplete="off"
                  {...register('authCode')}
                  disabled={isConnecting}
                />
                {errors.authCode && (
                  <p className="text-sm text-destructive">{errors.authCode.message}</p>
                )}

                <CodeHelpAccordion
                  value="gog-code-help"
                  trigger="How do I get an authorization code?"
                >
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
                        Open the GOG login page <ExternalLink className="inline h-3 w-3" />
                      </a>{' '}
                      in a new tab
                    </li>
                    <li>Sign in with your GOG account if prompted</li>
                    <li>
                      After login, you will be redirected to a GOG page — copy the entire URL from
                      your browser&apos;s address bar (it contains <code>?code=…</code>)
                    </li>
                    <li>
                      Paste the URL into the field above (you can also paste just the{' '}
                      <code>code</code> value if you prefer)
                    </li>
                  </ol>
                  <div className="mt-2 rounded border border-yellow-200 bg-yellow-50 p-2 text-yellow-800 dark:border-yellow-800 dark:bg-yellow-900/20 dark:text-yellow-200">
                    <strong>Note:</strong> The authorization code is single-use and expires within a
                    few minutes. Paste it as soon as you copy it.
                  </div>
                </CodeHelpAccordion>
              </div>

              <ConnectSubmitButton
                isPending={isConnecting}
                idleLabel={resolvedCredentialsError ? 'Reconfigure' : 'Connect GOG'}
                pendingLabel={resolvedCredentialsError ? 'Reconfiguring...' : 'Connecting...'}
              />
            </form>
          </div>
        )}
      </CardContent>
    </Card>
  );
}
