'use client';

import { useState, useEffect, useMemo } from 'react';
import Link from 'next/link';
import { useRouter } from 'next/navigation';
import { useAuth } from '@/providers';
import { Card, CardContent, CardHeader, CardTitle, CardDescription } from '@/components/ui/card';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Checkbox } from '@/components/ui/checkbox';
import { Alert, AlertDescription } from '@/components/ui/alert';
import { Progress } from '@/components/ui/progress';
import { toast } from 'sonner';
import { ArrowLeft, AlertCircle, Loader2, Eye, EyeOff, UserPlus } from 'lucide-react';
import * as adminApi from '@/api/admin';

interface PasswordStrength {
  score: number;
  label: string;
}

function calculatePasswordStrength(password: string): PasswordStrength {
  let score = 0;

  if (password.length >= 6) score += 1;
  if (password.length >= 8) score += 1;
  if (password.length >= 12) score += 1;
  if (/[A-Z]/.test(password)) score += 1;
  if (/[a-z]/.test(password)) score += 1;
  if (/[0-9]/.test(password)) score += 1;
  if (/[^A-Za-z0-9]/.test(password)) score += 1;

  if (score <= 2) return { score, label: 'Weak' };
  if (score <= 4) return { score, label: 'Medium' };
  return { score, label: 'Strong' };
}

export default function CreateUserPage() {
  const router = useRouter();
  const { user: currentUser } = useAuth();

  const [username, setUsername] = useState('');
  const [password, setPassword] = useState('');
  const [confirmPassword, setConfirmPassword] = useState('');
  const [isAdmin, setIsAdmin] = useState(false);
  const [showPassword, setShowPassword] = useState(false);
  const [showConfirmPassword, setShowConfirmPassword] = useState(false);
  const [isLoading, setIsLoading] = useState(false);
  const [error, setError] = useState('');

  // Derived values
  const passwordStrength = useMemo(() => calculatePasswordStrength(password), [password]);
  const passwordsMatch = password && confirmPassword && password === confirmPassword;

  // Check admin access
  useEffect(() => {
    if (currentUser && !currentUser.isAdmin) {
      router.replace('/dashboard');
    }
  }, [currentUser, router]);

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setError('');

    // Validation
    if (!username.trim()) {
      setError('Username is required');
      return;
    }

    if (username.trim().length < 3) {
      setError('Username must be at least 3 characters');
      return;
    }

    if (!password) {
      setError('Password is required');
      return;
    }

    if (password !== confirmPassword) {
      setError('Passwords do not match');
      return;
    }

    if (password.length < 6) {
      setError('Password must be at least 6 characters long');
      return;
    }

    setIsLoading(true);

    try {
      await adminApi.createUser({
        username: username.trim(),
        password,
        is_admin: isAdmin,
      });
      toast.success('User created successfully');
      router.push('/admin/users');
    } catch (err) {
      const errorMessage = err instanceof Error ? err.message : 'Failed to create user';
      setError(errorMessage);
    } finally {
      setIsLoading(false);
    }
  };

  // Show loading state
  if (!currentUser?.isAdmin) {
    return null;
  }

  const canSubmit =
    username.trim().length >= 3 &&
    password.length >= 6 &&
    passwordsMatch &&
    !isLoading;

  return (
    <div className="space-y-6">
      {/* Header */}
      <div className="flex items-center gap-4">
        <Button variant="ghost" size="icon" asChild>
          <Link href="/admin/users" aria-label="Back to users">
            <ArrowLeft className="h-4 w-4" />
          </Link>
        </Button>
        <div>
          <nav className="flex items-center gap-2 text-sm text-muted-foreground mb-1">
            <Link href="/admin/users" className="hover:text-foreground">
              User Management
            </Link>
            <span>/</span>
            <span>Create User</span>
          </nav>
          <h1 className="text-2xl font-bold flex items-center gap-2">
            <UserPlus className="h-6 w-6" />
            Create New User
          </h1>
          <p className="text-muted-foreground">
            Create a new user account with username and password. You can also assign admin privileges.
          </p>
        </div>
      </div>

      <div className="grid gap-6 lg:grid-cols-3">
        {/* Main Form */}
        <div className="lg:col-span-2">
          <Card>
            <CardHeader>
              <CardTitle>User Details</CardTitle>
              <CardDescription>Enter the new user&apos;s account information</CardDescription>
            </CardHeader>
            <CardContent>
              {error && (
                <Alert variant="destructive" className="mb-6">
                  <AlertCircle className="h-4 w-4" />
                  <AlertDescription>{error}</AlertDescription>
                </Alert>
              )}

              <form onSubmit={handleSubmit} className="space-y-6">
                {/* Username */}
                <div>
                  <Label htmlFor="username">
                    Username <span className="text-destructive">*</span>
                  </Label>
                  <Input
                    id="username"
                    type="text"
                    value={username}
                    onChange={(e) => setUsername(e.target.value)}
                    placeholder="Enter username"
                    className="mt-1"
                    disabled={isLoading}
                    autoComplete="username"
                  />
                  <p className="mt-1 text-xs text-muted-foreground">
                    Username will be used for login and display purposes
                  </p>
                </div>

                {/* Password */}
                <div className="grid gap-4 sm:grid-cols-2">
                  <div>
                    <Label htmlFor="password">
                      Password <span className="text-destructive">*</span>
                    </Label>
                    <div className="relative mt-1">
                      <Input
                        id="password"
                        type={showPassword ? 'text' : 'password'}
                        value={password}
                        onChange={(e) => setPassword(e.target.value)}
                        placeholder="Enter password"
                        className="pr-10"
                        disabled={isLoading}
                        autoComplete="new-password"
                      />
                      <button
                        type="button"
                        onClick={() => setShowPassword(!showPassword)}
                        className="absolute inset-y-0 right-0 flex items-center pr-3"
                        aria-label={showPassword ? 'Hide password' : 'Show password'}
                      >
                        {showPassword ? (
                          <EyeOff className="h-4 w-4 text-muted-foreground" />
                        ) : (
                          <Eye className="h-4 w-4 text-muted-foreground" />
                        )}
                      </button>
                    </div>

                    {/* Password Strength Meter */}
                    {password && (
                      <div className="mt-2 space-y-1">
                        <div className="flex items-center gap-2">
                          <Progress value={(passwordStrength.score / 7) * 100} className="h-2 flex-1" />
                          <span className="text-xs text-muted-foreground">
                            {passwordStrength.label}
                          </span>
                        </div>
                      </div>
                    )}
                  </div>

                  <div>
                    <Label htmlFor="confirmPassword">
                      Confirm Password <span className="text-destructive">*</span>
                    </Label>
                    <div className="relative mt-1">
                      <Input
                        id="confirmPassword"
                        type={showConfirmPassword ? 'text' : 'password'}
                        value={confirmPassword}
                        onChange={(e) => setConfirmPassword(e.target.value)}
                        placeholder="Confirm password"
                        className={`pr-10 ${
                          confirmPassword
                            ? passwordsMatch
                              ? 'border-green-500'
                              : 'border-destructive'
                            : ''
                        }`}
                        disabled={isLoading}
                        autoComplete="new-password"
                      />
                      <button
                        type="button"
                        onClick={() => setShowConfirmPassword(!showConfirmPassword)}
                        className="absolute inset-y-0 right-0 flex items-center pr-3"
                        aria-label={showConfirmPassword ? 'Hide password' : 'Show password'}
                      >
                        {showConfirmPassword ? (
                          <EyeOff className="h-4 w-4 text-muted-foreground" />
                        ) : (
                          <Eye className="h-4 w-4 text-muted-foreground" />
                        )}
                      </button>
                    </div>
                    {confirmPassword && !passwordsMatch && (
                      <p className="mt-1 text-xs text-destructive">Passwords do not match</p>
                    )}
                  </div>
                </div>

                {/* Admin Role */}
                <div className="flex items-start space-x-3 p-4 bg-muted/50 rounded-lg">
                  <Checkbox
                    id="isAdmin"
                    checked={isAdmin}
                    onCheckedChange={(checked) => setIsAdmin(checked === true)}
                    disabled={isLoading}
                  />
                  <div>
                    <label htmlFor="isAdmin" className="font-medium text-sm cursor-pointer">
                      Admin User
                    </label>
                    <p className="text-xs text-muted-foreground">
                      Grant administrative privileges to this user. Admin users can manage other
                      users and system settings.
                    </p>
                  </div>
                </div>

                {/* Form Actions */}
                <div className="flex justify-end gap-3 pt-4 border-t">
                  <Button
                    type="button"
                    variant="outline"
                    onClick={() => router.push('/admin/users')}
                    disabled={isLoading}
                  >
                    Cancel
                  </Button>
                  <Button type="submit" disabled={!canSubmit}>
                    {isLoading && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
                    Create User
                  </Button>
                </div>
              </form>
            </CardContent>
          </Card>
        </div>

        {/* Requirements Info Box */}
        <div className="hidden lg:block">
          <Card>
            <CardHeader>
              <CardTitle className="text-sm">Requirements</CardTitle>
            </CardHeader>
            <CardContent className="space-y-4">
              <div>
                <h4 className="text-xs font-semibold uppercase text-muted-foreground mb-2">
                  Username
                </h4>
                <ul className="space-y-1 text-xs text-muted-foreground">
                  <li>3-100 characters long</li>
                  <li>Letters, numbers, underscore only</li>
                  <li>No spaces or special characters</li>
                  <li>Must be unique</li>
                </ul>
              </div>

              <div>
                <h4 className="text-xs font-semibold uppercase text-muted-foreground mb-2">
                  Password
                </h4>
                <ul className="space-y-1 text-xs text-muted-foreground">
                  <li>Minimum 6 characters</li>
                  <li>At least one uppercase letter (recommended)</li>
                  <li>At least one number (recommended)</li>
                  <li>Special character (recommended)</li>
                </ul>
              </div>

              <div>
                <h4 className="text-xs font-semibold uppercase text-muted-foreground mb-2">
                  Admin Privileges
                </h4>
                <ul className="space-y-1 text-xs text-muted-foreground">
                  <li>Access to user management</li>
                  <li>System settings configuration</li>
                  <li>Platform administration</li>
                </ul>
              </div>
            </CardContent>
          </Card>
        </div>
      </div>
    </div>
  );
}
