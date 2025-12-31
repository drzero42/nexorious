'use client';

import { useState } from 'react';
import { useForm } from 'react-hook-form';
import { zodResolver } from '@hookform/resolvers/zod';
import { z } from 'zod';
import { toast } from 'sonner';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Loader2, ExternalLink, Copy, Check } from 'lucide-react';
import { useStartEpicAuth, useCompleteEpicAuth } from '@/hooks';
import { EPIC_AUTH_ERROR_MESSAGES } from '@/types';

const authCodeSchema = z.object({
  code: z.string().min(1, 'Authorization code is required'),
});

type AuthCodeForm = z.infer<typeof authCodeSchema>;

interface EpicAuthDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onSuccess: () => void;
}

export function EpicAuthDialog({ open, onOpenChange, onSuccess }: EpicAuthDialogProps) {
  const [step, setStep] = useState<'start' | 'code'>('start');
  const [authUrl, setAuthUrl] = useState<string>('');
  const [urlCopied, setUrlCopied] = useState(false);

  const startMutation = useStartEpicAuth();
  const completeMutation = useCompleteEpicAuth();

  const {
    register,
    handleSubmit,
    formState: { errors },
    reset,
  } = useForm<AuthCodeForm>({
    resolver: zodResolver(authCodeSchema),
  });

  const handleStart = async () => {
    try {
      const result = await startMutation.mutateAsync();
      setAuthUrl(result.authUrl);
      setStep('code');
    } catch (err) {
      toast.error('Failed to start Epic authentication');
    }
  };

  const handleCopyUrl = async () => {
    await navigator.clipboard.writeText(authUrl);
    setUrlCopied(true);
    setTimeout(() => setUrlCopied(false), 2000);
    toast.success('URL copied to clipboard');
  };

  const onSubmit = async (data: AuthCodeForm) => {
    try {
      const result = await completeMutation.mutateAsync(data.code);

      if (!result.valid) {
        const errorMessage = result.error
          ? EPIC_AUTH_ERROR_MESSAGES[result.error] || 'Authentication failed'
          : 'Authentication failed';
        toast.error(errorMessage);
        return;
      }

      toast.success(`Epic Games connected as ${result.displayName}`);
      handleClose();
      onSuccess();
    } catch (err) {
      toast.error('Failed to complete Epic authentication');
    }
  };

  const handleClose = () => {
    setStep('start');
    setAuthUrl('');
    setUrlCopied(false);
    reset();
    onOpenChange(false);
  };

  const isLoading = startMutation.isPending || completeMutation.isPending;

  return (
    <Dialog open={open} onOpenChange={handleClose}>
      <DialogContent className="sm:max-w-[500px]">
        {step === 'start' ? (
          <>
            <DialogHeader>
              <DialogTitle>Connect Epic Games Store</DialogTitle>
              <DialogDescription>
                Authenticate with Epic Games to sync your library
              </DialogDescription>
            </DialogHeader>
            <div className="space-y-4 py-4">
              <p className="text-sm text-muted-foreground">
                You&apos;ll be redirected to Epic Games to authorize Nexorious. After logging in,
                you&apos;ll receive an authorization code to complete the connection.
              </p>
              <Button onClick={handleStart} disabled={isLoading} className="w-full">
                {isLoading ? (
                  <>
                    <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                    Starting...
                  </>
                ) : (
                  'Start Authentication'
                )}
              </Button>
            </div>
          </>
        ) : (
          <>
            <DialogHeader>
              <DialogTitle>Enter Authorization Code</DialogTitle>
              <DialogDescription>
                Complete authentication by entering the code from Epic Games
              </DialogDescription>
            </DialogHeader>
            <div className="space-y-4 py-4">
              <div className="rounded-lg border bg-muted/50 p-4 space-y-3">
                <p className="text-sm font-medium">Step 1: Visit Epic Games</p>
                <div className="flex gap-2">
                  <Button
                    variant="outline"
                    size="sm"
                    onClick={() => window.open(authUrl, '_blank')}
                    className="flex-1"
                  >
                    <ExternalLink className="mr-2 h-4 w-4" />
                    Open Epic Login
                  </Button>
                  <Button variant="outline" size="sm" onClick={handleCopyUrl}>
                    {urlCopied ? <Check className="h-4 w-4" /> : <Copy className="h-4 w-4" />}
                  </Button>
                </div>
                <p className="text-xs text-muted-foreground">
                  Log in to your Epic account and authorize Nexorious
                </p>
              </div>

              <form onSubmit={handleSubmit(onSubmit)} className="space-y-4">
                <div className="space-y-2">
                  <Label htmlFor="code">Step 2: Enter Authorization Code</Label>
                  <Input
                    id="code"
                    placeholder="Paste the code from Epic Games"
                    {...register('code')}
                    disabled={isLoading}
                    autoComplete="off"
                  />
                  {errors.code && (
                    <p className="text-sm text-destructive">{errors.code.message}</p>
                  )}
                </div>

                <div className="flex gap-2">
                  <Button
                    type="button"
                    variant="outline"
                    onClick={handleClose}
                    disabled={isLoading}
                    className="flex-1"
                  >
                    Cancel
                  </Button>
                  <Button type="submit" disabled={isLoading} className="flex-1">
                    {isLoading ? (
                      <>
                        <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                        Verifying...
                      </>
                    ) : (
                      'Connect'
                    )}
                  </Button>
                </div>
              </form>
            </div>
          </>
        )}
      </DialogContent>
    </Dialog>
  );
}
