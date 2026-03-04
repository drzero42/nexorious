import { createFileRoute, useNavigate } from '@tanstack/react-router';
import { useState, useEffect, useRef } from 'react';
import { Upload, X, Loader2 } from 'lucide-react';
import * as authApi from '@/api/auth';
import { Button } from '@/components/ui/button';
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Alert, AlertDescription } from '@/components/ui/alert';

export const Route = createFileRoute('/_public/setup')({
  component: SetupPage,
});

function SetupPage() {
  const navigate = useNavigate();
  const [isCheckingSetup, setIsCheckingSetup] = useState(true);
  const [needsSetup, setNeedsSetup] = useState(false);
  const [username, setUsername] = useState('');
  const [password, setPassword] = useState('');
  const [confirmPassword, setConfirmPassword] = useState('');
  const [error, setError] = useState<string | null>(null);
  const [isSubmitting, setIsSubmitting] = useState(false);
  const usernameInputRef = useRef<HTMLInputElement>(null);
  const [showRestore, setShowRestore] = useState(false);
  const [restoreFile, setRestoreFile] = useState<File | null>(null);
  const [isRestoring, setIsRestoring] = useState(false);
  const fileInputRef = useRef<HTMLInputElement>(null);

  // Check if setup is needed on mount
  useEffect(() => {
    const checkSetup = async () => {
      try {
        const status = await authApi.checkSetupStatus();
        if (!status.needs_setup) {
          navigate({ to: '/login', replace: true });
          return;
        }
        setNeedsSetup(true);
      } catch {
        setError('Failed to check setup status');
      } finally {
        setIsCheckingSetup(false);
      }
    };
    checkSetup();
  }, [navigate]);

  // Focus username input when setup is confirmed needed
  useEffect(() => {
    if (needsSetup && usernameInputRef.current) {
      usernameInputRef.current.focus();
    }
  }, [needsSetup]);

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setError(null);

    // Validate username length
    if (username.length < 3) {
      setError('Username must be at least 3 characters');
      return;
    }

    // Validate password length
    if (password.length < 8) {
      setError('Password must be at least 8 characters');
      return;
    }

    // Validate passwords match
    if (password !== confirmPassword) {
      setError('Passwords do not match');
      return;
    }

    setIsSubmitting(true);

    try {
      await authApi.createInitialAdmin(username, password);
      navigate({ to: '/login' });
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to create admin account');
    } finally {
      setIsSubmitting(false);
    }
  };

  const handleRestore = async () => {
    if (!restoreFile) return;

    setError(null);
    setIsRestoring(true);

    try {
      await authApi.setupRestore(restoreFile);
      navigate({ to: '/login' });
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to restore from backup');
    } finally {
      setIsRestoring(false);
    }
  };

  const handleFileSelect = (e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0];
    if (file) {
      if (!file.name.endsWith('.tar.gz')) {
        setError('Please select a .tar.gz backup file');
        return;
      }
      setRestoreFile(file);
      setError(null);
    }
  };

  const formatFileSize = (bytes: number): string => {
    if (bytes < 1024) return `${bytes} B`;
    if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
    return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
  };

  const cancelRestore = () => {
    setShowRestore(false);
    setRestoreFile(null);
    setError(null);
    if (fileInputRef.current) {
      fileInputRef.current.value = '';
    }
  };

  // Show loading state while checking setup status
  if (isCheckingSetup) {
    return (
      <Card className="w-full max-w-sm">
        <CardContent className="flex items-center justify-center p-6">
          <div className="text-muted-foreground">Checking setup status...</div>
        </CardContent>
      </Card>
    );
  }

  // Don't render the form if setup is not needed (redirect is happening)
  if (!needsSetup) {
    return null;
  }

  return (
    <Card className="w-full max-w-sm">
      <CardHeader className="space-y-1 text-center">
        <CardTitle className="text-2xl font-bold">
          {showRestore ? 'Restore from Backup' : 'Create Admin Account'}
        </CardTitle>
        <CardDescription>
          {showRestore
            ? 'Upload a backup file to restore your data'
            : 'Set up your administrator account to get started'}
        </CardDescription>
      </CardHeader>
      <CardContent>
        {error && (
          <Alert variant="destructive" className="mb-4">
            <AlertDescription>{error}</AlertDescription>
          </Alert>
        )}

        {!showRestore ? (
          <>
            <form onSubmit={handleSubmit} className="space-y-4">
              <div className="space-y-2">
                <Label htmlFor="username">Username</Label>
                <Input
                  ref={usernameInputRef}
                  id="username"
                  type="text"
                  placeholder="Choose a username"
                  value={username}
                  onChange={(e) => setUsername(e.target.value)}
                  required
                  minLength={3}
                  autoComplete="username"
                  disabled={isSubmitting}
                />
                <p className="text-sm text-muted-foreground">
                  Must be at least 3 characters
                </p>
              </div>

              <div className="space-y-2">
                <Label htmlFor="password">Password</Label>
                <Input
                  id="password"
                  type="password"
                  placeholder="Enter a secure password"
                  value={password}
                  onChange={(e) => setPassword(e.target.value)}
                  required
                  minLength={8}
                  autoComplete="new-password"
                  disabled={isSubmitting}
                />
                <p className="text-sm text-muted-foreground">
                  Must be at least 8 characters
                </p>
              </div>

              <div className="space-y-2">
                <Label htmlFor="confirmPassword">Confirm Password</Label>
                <Input
                  id="confirmPassword"
                  type="password"
                  placeholder="Confirm your password"
                  value={confirmPassword}
                  onChange={(e) => setConfirmPassword(e.target.value)}
                  required
                  autoComplete="new-password"
                  disabled={isSubmitting}
                />
              </div>

              <Button
                type="submit"
                className="w-full"
                disabled={isSubmitting || !username || !password || !confirmPassword}
              >
                {isSubmitting ? 'Creating Account...' : 'Create Admin Account'}
              </Button>

              <p className="text-center text-sm text-muted-foreground">
                This account will have full administrative privileges
              </p>
            </form>

            <div className="mt-4 text-center">
              <button
                type="button"
                onClick={() => setShowRestore(true)}
                className="text-sm text-muted-foreground hover:text-foreground hover:underline"
              >
                Restore from backup
              </button>
            </div>
          </>
        ) : (
          <div className="space-y-4">
            <input
              ref={fileInputRef}
              type="file"
              accept=".tar.gz"
              onChange={handleFileSelect}
              className="hidden"
            />

            {!restoreFile ? (
              <button
                type="button"
                onClick={() => fileInputRef.current?.click()}
                className="w-full rounded-lg border-2 border-dashed border-muted-foreground/25 p-8 text-center hover:border-muted-foreground/50 transition-colors"
                disabled={isRestoring}
              >
                <Upload className="mx-auto h-8 w-8 text-muted-foreground mb-2" />
                <p className="text-sm text-muted-foreground">
                  Click to select a backup file
                </p>
                <p className="text-xs text-muted-foreground mt-1">
                  .tar.gz files only
                </p>
              </button>
            ) : (
              <div className="rounded-lg border p-4">
                <div className="flex items-center justify-between">
                  <div className="flex items-center gap-3 min-w-0">
                    <Upload className="h-5 w-5 text-muted-foreground flex-shrink-0" />
                    <div className="min-w-0">
                      <p className="text-sm font-medium truncate">{restoreFile.name}</p>
                      <p className="text-xs text-muted-foreground">
                        {formatFileSize(restoreFile.size)}
                      </p>
                    </div>
                  </div>
                  <button
                    type="button"
                    onClick={() => {
                      setRestoreFile(null);
                      if (fileInputRef.current) fileInputRef.current.value = '';
                    }}
                    className="text-muted-foreground hover:text-foreground"
                    disabled={isRestoring}
                  >
                    <X className="h-4 w-4" />
                  </button>
                </div>
              </div>
            )}

            <Button
              onClick={handleRestore}
              className="w-full"
              disabled={!restoreFile || isRestoring}
            >
              {isRestoring ? (
                <>
                  <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                  Restoring...
                </>
              ) : (
                'Restore'
              )}
            </Button>

            <button
              type="button"
              onClick={cancelRestore}
              className="w-full text-sm text-muted-foreground hover:text-foreground hover:underline"
              disabled={isRestoring}
            >
              Cancel
            </button>
          </div>
        )}
      </CardContent>
    </Card>
  );
}
