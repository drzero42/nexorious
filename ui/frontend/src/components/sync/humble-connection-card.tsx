import { useForm } from 'react-hook-form';
import { zodResolver } from '@hookform/resolvers/zod';
import { z } from 'zod';
import { toast } from 'sonner';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';
import { Label } from '@/components/ui/label';
import { Textarea } from '@/components/ui/textarea';
import { useConnectHumble, useDisconnectHumble } from '@/hooks';
import {
  CodeHelpAccordion,
  ConnectSubmitButton,
  ConnectedSummary,
  CredentialsErrorBanner,
  DisconnectDialog,
} from './connection';

const humbleCredentialsSchema = z.object({
  sessionCookie: z.string().trim().min(1, 'Session cookie is required'),
});

type HumbleCredentialsForm = z.infer<typeof humbleCredentialsSchema>;

interface HumbleConnectionCardProps {
  isConfigured: boolean;
  credentialsError: boolean;
  onConnectionChange: () => void;
}

export function HumbleConnectionCard({
  isConfigured,
  credentialsError,
  onConnectionChange,
}: HumbleConnectionCardProps) {
  const {
    register,
    handleSubmit,
    formState: { errors },
    setError,
  } = useForm<HumbleCredentialsForm>({
    resolver: zodResolver(humbleCredentialsSchema),
  });

  const connectMutation = useConnectHumble();
  const disconnectMutation = useDisconnectHumble();

  const isConnecting = connectMutation.isPending;
  const isDisconnecting = disconnectMutation.isPending;

  const onSubmit = async (data: HumbleCredentialsForm) => {
    try {
      const result = await connectMutation.mutateAsync(data.sessionCookie);
      if (!result.valid) {
        const errorMessage = result.error || 'Connection failed';
        setError('sessionCookie', { message: errorMessage });
        toast.error(errorMessage);
        return;
      }
      toast.success('Humble Bundle connected successfully');
      onConnectionChange();
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Failed to connect Humble Bundle');
    }
  };

  const handleDisconnect = async () => {
    try {
      await disconnectMutation.mutateAsync();
      toast.success('Humble Bundle disconnected');
      onConnectionChange();
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Failed to disconnect Humble Bundle');
    }
  };

  const showForm = !isConfigured || credentialsError;

  return (
    <Card>
      <CardHeader>
        <div>
          <CardTitle>Humble Bundle Connection</CardTitle>
          <CardDescription>
            {isConfigured
              ? 'Your Humble Bundle account is connected'
              : 'Connect your Humble Bundle account to sync your DRM-free game downloads'}
          </CardDescription>
        </div>
      </CardHeader>
      <CardContent>
        {showForm ? (
          <div className="space-y-4">
            {credentialsError && (
              <CredentialsErrorBanner
                title="Your Humble Bundle session has expired"
                description="Please paste a fresh _simpleauth_sess cookie to continue syncing your library."
              />
            )}

            <form onSubmit={handleSubmit(onSubmit)} className="space-y-4">
              <div className="space-y-2">
                <Label htmlFor="sessionCookie">Session cookie (_simpleauth_sess)</Label>
                <Textarea
                  id="sessionCookie"
                  rows={3}
                  placeholder="Paste the value of your _simpleauth_sess cookie"
                  {...register('sessionCookie')}
                  disabled={isConnecting}
                />
                {errors.sessionCookie && (
                  <p className="text-sm text-destructive">{errors.sessionCookie.message}</p>
                )}

                <CodeHelpAccordion
                  value="humble-help"
                  trigger="How do I get my _simpleauth_sess cookie?"
                >
                  <p className="font-medium text-foreground">
                    The _simpleauth_sess cookie is a session token that lets Nexorious read your
                    Humble Bundle library. Only DRM-free game downloads are imported — never ebooks,
                    audio, video, or Steam-key-only titles.
                  </p>
                  <ol className="list-inside list-decimal space-y-1">
                    <li>Sign in at humblebundle.com in your browser.</li>
                    <li>
                      Open your browser&apos;s developer tools (F12) and go to the{' '}
                      <strong>Application</strong> tab (Chrome/Edge) or <strong>Storage</strong> tab
                      (Firefox).
                    </li>
                    <li>
                      Under <strong>Cookies → https://www.humblebundle.com</strong>, find the cookie
                      named <code>_simpleauth_sess</code>.
                    </li>
                    <li>Copy its entire value and paste it into the field above.</li>
                  </ol>
                  <div className="mt-2 rounded border border-yellow-200 bg-yellow-50 p-2 text-yellow-800 dark:border-yellow-800 dark:bg-yellow-900/20 dark:text-yellow-200">
                    <strong>Security Note:</strong> This cookie grants access to your Humble library
                    and expires periodically. It is stored encrypted and only used to sync your
                    games. You&apos;ll re-paste it when it expires.
                  </div>
                </CodeHelpAccordion>
              </div>

              <ConnectSubmitButton
                isPending={isConnecting}
                idleLabel={credentialsError ? 'Reconnect' : 'Connect Humble Bundle'}
                pendingLabel={credentialsError ? 'Reconnecting...' : 'Connecting...'}
              />
            </form>
          </div>
        ) : (
          <div className="space-y-4">
            <ConnectedSummary />
            <DisconnectDialog
              serviceLabel="Humble Bundle"
              isDisconnecting={isDisconnecting}
              onDisconnect={handleDisconnect}
            />
          </div>
        )}
      </CardContent>
    </Card>
  );
}
