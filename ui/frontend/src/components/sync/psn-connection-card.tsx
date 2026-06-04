import { useForm } from 'react-hook-form';
import { zodResolver } from '@hookform/resolvers/zod';
import { z } from 'zod';
import { toast } from 'sonner';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { ExternalLink } from 'lucide-react';
import { useConfigurePSN, useDisconnectPSN } from '@/hooks';
import {
  CodeHelpAccordion,
  ConnectSubmitButton,
  ConnectedSummary,
  CredentialsErrorBanner,
  DisconnectDialog,
} from './connection';

const psnCredentialsSchema = z.object({
  npssoToken: z
    .string()
    .length(64, 'NPSSO token must be exactly 64 characters')
    .regex(/^[A-Za-z0-9]+$/, 'NPSSO token must contain only alphanumeric characters'),
});

type PSNCredentialsForm = z.infer<typeof psnCredentialsSchema>;

interface PSNConnectionCardProps {
  isConfigured: boolean;
  credentialsError: boolean;
  onlineId?: string;
  onConnectionChange: () => void;
}

export function PSNConnectionCard({
  isConfigured,
  credentialsError,
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

  // Show form if not configured OR credentials error (e.g. token expired)
  const showForm = !isConfigured || credentialsError;

  return (
    <Card>
      <CardHeader>
        <div>
          <CardTitle>PlayStation Network Connection</CardTitle>
          <CardDescription>
            {isConfigured
              ? 'Your PlayStation Network account is connected'
              : 'Connect your PlayStation Network account to sync your game library'}
          </CardDescription>
        </div>
      </CardHeader>
      <CardContent>
        {showForm ? (
          <div className="space-y-4">
            {credentialsError && (
              <CredentialsErrorBanner
                title="Your NPSSO token has expired"
                description="Please enter a new NPSSO token to continue syncing your PlayStation games."
              />
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

                <CodeHelpAccordion value="npsso-help" trigger="How do I get my NPSSO token?">
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
                        Sony SSO Cookie Page <ExternalLink className="inline h-3 w-3" />
                      </a>
                    </li>
                    <li>Sign in with your PlayStation Network account if prompted</li>
                    <li>
                      Copy the entire 64-character code that appears on the page (it will look like
                      a long string of letters and numbers)
                    </li>
                    <li>Paste it into the field above</li>
                  </ol>
                  <div className="mt-2 space-y-2">
                    <div className="rounded border border-yellow-200 bg-yellow-50 p-2 text-yellow-800 dark:border-yellow-800 dark:bg-yellow-900/20 dark:text-yellow-200">
                      <strong>Security Note:</strong> Your NPSSO token expires approximately every 2
                      months. You&apos;ll need to get a new token when it expires. The token is
                      stored securely and only used to sync your library.
                    </div>
                    <div className="rounded border border-blue-200 bg-blue-50 p-2 text-blue-800 dark:border-blue-800 dark:bg-blue-900/20 dark:text-blue-200">
                      <strong>Note:</strong> PS3 games cannot be synced due to PlayStation Network
                      API limitations. Only PS4, PS5, and PS Vita games are supported.
                    </div>
                  </div>
                </CodeHelpAccordion>
              </div>

              <ConnectSubmitButton
                isPending={isConfiguring}
                idleLabel={credentialsError ? 'Reconfigure' : 'Configure PSN'}
                pendingLabel={credentialsError ? 'Reconfiguring...' : 'Configuring...'}
              />
            </form>
          </div>
        ) : (
          <div className="space-y-4">
            <ConnectedSummary name={onlineId} />
            <DisconnectDialog
              serviceLabel="PlayStation Network"
              isDisconnecting={isDisconnecting}
              onDisconnect={handleDisconnect}
            />
          </div>
        )}
      </CardContent>
    </Card>
  );
}
