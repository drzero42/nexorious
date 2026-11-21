import { useState } from 'react';
import { useForm } from 'react-hook-form';
import { zodResolver } from '@hookform/resolvers/zod';
import { z } from 'zod';
import { toast } from 'sonner';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Alert, AlertDescription } from '@/components/ui/alert';
import { ExternalLink, Info, Loader2 } from 'lucide-react';
import {
  useAmazonGamesConnection,
  useConnectAmazonGames,
  useDisconnectAmazonGames,
  useStartAmazonGamesLogin,
} from '@/hooks';
import type { AmazonGamesLoginResponse } from '@/types';
import {
  CodeHelpAccordion,
  ConnectSubmitButton,
  ConnectedSummary,
  CredentialsErrorBanner,
  DisconnectDialog,
} from './connection';

const amazonRedirectSchema = z.object({
  redirectUrl: z.string().trim().min(1, 'Paste the address Amazon took you to'),
});

type AmazonRedirectForm = z.infer<typeof amazonRedirectSchema>;

function disabledMessage(reason?: string): string {
  switch (reason) {
    case 'nile_not_installed':
    default:
      return 'Amazon Games sync is disabled on this server. Contact your administrator to enable it.';
  }
}

interface AmazonGamesConnectionCardProps {
  isConfigured: boolean;
  credentialsError?: boolean;
  onConnectionChange: () => void;
}

export function AmazonGamesConnectionCard({
  isConfigured,
  credentialsError = false,
  onConnectionChange,
}: AmazonGamesConnectionCardProps) {
  const { data: connection } = useAmazonGamesConnection();
  const startMutation = useStartAmazonGamesLogin();
  const connectMutation = useConnectAmazonGames();
  const disconnectMutation = useDisconnectAmazonGames();
  const [login, setLogin] = useState<AmazonGamesLoginResponse | null>(null);

  const {
    register,
    handleSubmit,
    formState: { errors },
    setError,
    reset,
  } = useForm<AmazonRedirectForm>({
    resolver: zodResolver(amazonRedirectSchema),
  });

  const isConnecting = connectMutation.isPending;
  const isDisabled = connection?.disabled === true;
  const resolvedCredentialsError = connection?.credentialsError ?? credentialsError;

  const handleStart = async () => {
    try {
      setLogin(await startMutation.mutateAsync());
      reset();
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Could not start an Amazon sign-in');
    }
  };

  const onSubmit = async (data: AmazonRedirectForm) => {
    if (!login) return;
    try {
      const result = await connectMutation.mutateAsync({
        attemptId: login.attemptId,
        redirectUrl: data.redirectUrl,
      });
      toast.success(
        result.username ? `Amazon Games connected as ${result.username}` : 'Amazon Games connected',
      );
      setLogin(null);
      reset();
      onConnectionChange();
    } catch (err) {
      const message = err instanceof Error ? err.message : 'Failed to connect Amazon Games';
      setError('redirectUrl', { message });
      toast.error(message);
    }
  };

  const handleDisconnect = async () => {
    try {
      await disconnectMutation.mutateAsync();
      toast.success('Amazon Games disconnected');
      onConnectionChange();
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Failed to disconnect Amazon Games');
    }
  };

  const playtimeNote = (
    <Alert>
      <Info className="h-4 w-4" />
      <AlertDescription>
        <strong>Note:</strong> Amazon Games does not provide playtime data.
      </AlertDescription>
    </Alert>
  );

  return (
    <Card>
      <CardHeader>
        <div>
          <CardTitle>Amazon Games Connection</CardTitle>
          <CardDescription>
            {isConfigured
              ? 'Your Amazon account is connected'
              : 'Connect your Amazon account to sync your Amazon Games and Prime Gaming library'}
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
            <ConnectedSummary name={connection?.username} />
            {playtimeNote}
            <DisconnectDialog
              serviceLabel="Amazon Games"
              isDisconnecting={disconnectMutation.isPending}
              onDisconnect={handleDisconnect}
            />
          </div>
        ) : (
          <div className="space-y-4">
            {resolvedCredentialsError && (
              <CredentialsErrorBanner
                title="Amazon sign-in is no longer valid"
                description="Please sign in to Amazon again to continue syncing your library."
              />
            )}
            {playtimeNote}

            {!login ? (
              <Button
                type="button"
                className="w-full"
                onClick={handleStart}
                disabled={startMutation.isPending}
              >
                {startMutation.isPending && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
                {resolvedCredentialsError ? 'Sign in to Amazon again' : 'Sign in with Amazon'}
              </Button>
            ) : (
              <form onSubmit={handleSubmit(onSubmit)} className="space-y-4">
                <ol className="list-outside list-decimal pl-5 space-y-1 text-sm">
                  <li>
                    <a
                      href={login.loginUrl}
                      target="_blank"
                      rel="noopener noreferrer"
                      className="text-primary underline underline-offset-4"
                    >
                      Open the Amazon sign-in page <ExternalLink className="inline h-3 w-3" />
                    </a>{' '}
                    and sign in
                  </li>
                  <li>Copy the full address of the amazon.com page you land on</li>
                  <li>Paste it below</li>
                </ol>

                <div className="space-y-2">
                  <Label htmlFor="amazonRedirectUrl">Address after signing in</Label>
                  <Input
                    id="amazonRedirectUrl"
                    type="text"
                    placeholder="amazon.com/?…authorization_code=…"
                    autoComplete="off"
                    {...register('redirectUrl')}
                    disabled={isConnecting}
                    aria-invalid={errors.redirectUrl ? true : undefined}
                    aria-describedby={errors.redirectUrl ? 'amazonRedirectUrlError' : undefined}
                    className={errors.redirectUrl ? 'border-destructive' : undefined}
                  />
                  {errors.redirectUrl && (
                    <p id="amazonRedirectUrlError" className="text-sm text-destructive">
                      {errors.redirectUrl.message}
                    </p>
                  )}

                  <CodeHelpAccordion value="amazon-code-help" trigger="What address do I paste?">
                    <p>
                      After you sign in, Amazon sends you to an amazon.com page whose address
                      contains <code>openid.oa2.authorization_code</code>. Copy that whole address
                      from the browser&apos;s address bar. The page itself may look empty or like
                      the Amazon home page.
                    </p>
                    <div className="mt-2 rounded border border-yellow-200 bg-yellow-50 p-2 text-yellow-800 dark:border-yellow-800 dark:bg-yellow-900/20 dark:text-yellow-200">
                      <strong>Note:</strong> The sign-in expires after 15 minutes. If it does,
                      choose &ldquo;Start a new sign-in&rdquo;.
                    </div>
                  </CodeHelpAccordion>
                </div>

                <ConnectSubmitButton
                  isPending={isConnecting}
                  idleLabel={resolvedCredentialsError ? 'Reconfigure' : 'Connect Amazon Games'}
                  pendingLabel={resolvedCredentialsError ? 'Reconfiguring...' : 'Connecting...'}
                />
                <Button
                  type="button"
                  variant="ghost"
                  className="w-full"
                  onClick={handleStart}
                  disabled={startMutation.isPending || isConnecting}
                >
                  Start a new sign-in
                </Button>
              </form>
            )}
          </div>
        )}
      </CardContent>
    </Card>
  );
}
