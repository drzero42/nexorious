'use client';

import { useState, useEffect, useRef } from 'react';
import { useRouter } from 'next/navigation';
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

export default function SetupPage() {
  const router = useRouter();
  const [isCheckingSetup, setIsCheckingSetup] = useState(true);
  const [needsSetup, setNeedsSetup] = useState(false);
  const [username, setUsername] = useState('');
  const [password, setPassword] = useState('');
  const [confirmPassword, setConfirmPassword] = useState('');
  const [error, setError] = useState<string | null>(null);
  const [isSubmitting, setIsSubmitting] = useState(false);
  const usernameInputRef = useRef<HTMLInputElement>(null);

  // Check if setup is needed on mount
  useEffect(() => {
    const checkSetup = async () => {
      try {
        const status = await authApi.checkSetupStatus();
        if (!status.needs_setup) {
          router.replace('/login');
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
  }, [router]);

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
      router.push('/login');
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to create admin account');
    } finally {
      setIsSubmitting(false);
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
        <CardTitle className="text-2xl font-bold">Create Admin Account</CardTitle>
        <CardDescription>
          Set up your administrator account to get started
        </CardDescription>
      </CardHeader>
      <CardContent>
        <form onSubmit={handleSubmit} className="space-y-4">
          {error && (
            <Alert variant="destructive">
              <AlertDescription>{error}</AlertDescription>
            </Alert>
          )}

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
      </CardContent>
    </Card>
  );
}
