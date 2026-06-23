import { useState, useEffect, useCallback, useMemo } from 'react';
import { createFileRoute, useNavigate } from '@tanstack/react-router';
import { useQueryClient } from '@tanstack/react-query';
import { useAuth } from '@/providers';
import { useCollectionStats, gameKeys, useSettings, useUpdateSettings } from '@/hooks';
import { Card, CardContent, CardHeader, CardTitle, CardDescription } from '@/components/ui/card';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import { DEAL_REGIONS } from '@/lib/deal-regions';
import { DATE_FORMAT_OPTIONS, type DateFormatPref } from '@/lib/format-date';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Alert, AlertDescription } from '@/components/ui/alert';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import { Progress } from '@/components/ui/progress';
import { Skeleton } from '@/components/ui/skeleton';
import { toast } from 'sonner';
import { Eye, EyeOff, Check, X, Loader2, AlertCircle, User, Trash2 } from 'lucide-react';
import * as authApi from '@/api/auth';
import * as gamesApi from '@/api/games';
import { NotificationsSection } from '@/components/notifications/notifications-section';
import { ApiKeysSection } from '@/components/api-keys/api-keys-section';

export const Route = createFileRoute('/_authenticated/profile')({
  head: () => ({ meta: [{ title: 'Profile | Nexorious' }] }),
  component: ProfilePage,
});

interface PasswordStrength {
  score: number;
  label: string;
  color: string;
}

function calculatePasswordStrength(password: string): PasswordStrength {
  let score = 0;

  if (password.length >= 8) score += 1;
  if (password.length >= 12) score += 1;
  if (/[A-Z]/.test(password)) score += 1;
  if (/[a-z]/.test(password)) score += 1;
  if (/[0-9]/.test(password)) score += 1;
  if (/[^A-Za-z0-9]/.test(password)) score += 1;

  if (score <= 2) return { score, label: 'Weak', color: 'bg-red-500' };
  if (score <= 4) return { score, label: 'Medium', color: 'bg-yellow-500' };
  return { score, label: 'Strong', color: 'bg-green-500' };
}

function PreferencesSection() {
  const { data: settings } = useSettings();
  const updateSettings = useUpdateSettings();

  return (
    <Card>
      <CardHeader>
        <CardTitle>Preferences</CardTitle>
        <CardDescription>Personalise how Nexorious works for you</CardDescription>
      </CardHeader>
      <CardContent className="space-y-4">
        <div>
          <Label htmlFor="dealRegion">Deal region</Label>
          <p className="mb-2 text-sm text-muted-foreground">
            Used for PSprices deal links on your wishlist.
          </p>
          <Select
            value={settings?.dealRegion ?? 'us'}
            onValueChange={(code) => updateSettings.mutate({ dealRegion: code })}
          >
            <SelectTrigger id="dealRegion" className="w-64">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {DEAL_REGIONS.map((r) => (
                <SelectItem key={r.code} value={r.code}>
                  {r.label}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>
        <div>
          <Label htmlFor="dateFormat">Date format</Label>
          <p className="mb-2 text-sm text-muted-foreground">
            How dates are displayed throughout the app. Auto follows your browser locale.
          </p>
          <Select
            value={settings?.dateFormat ?? 'auto'}
            onValueChange={(value) =>
              updateSettings.mutate({ dateFormat: value as DateFormatPref })
            }
          >
            <SelectTrigger id="dateFormat" className="w-64">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {DATE_FORMAT_OPTIONS.map((o) => (
                <SelectItem key={o.value} value={o.value}>
                  {o.label}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>
      </CardContent>
    </Card>
  );
}

function ProfilePageSkeleton() {
  return (
    <div className="space-y-6">
      <div>
        <Skeleton className="mb-2 h-8 w-48" />
        <Skeleton className="h-4 w-64" />
      </div>
      <Card className="max-w-2xl">
        <CardHeader>
          <Skeleton className="h-6 w-40" />
        </CardHeader>
        <CardContent className="space-y-4">
          <Skeleton className="h-10 w-full" />
          <Skeleton className="h-10 w-full" />
          <Skeleton className="h-10 w-32" />
        </CardContent>
      </Card>
    </div>
  );
}

function ProfilePage() {
  const { user, logout } = useAuth();

  // Username state — initialized directly from user since the route guard
  // ensures user is non-null before this component renders.
  const [newUsername, setNewUsername] = useState(user?.username ?? '');
  const [isCheckingUsername, setIsCheckingUsername] = useState(false);
  const [usernameAvailable, setUsernameAvailable] = useState<boolean | null>(null);
  const [usernameError, setUsernameError] = useState('');
  const [isSubmittingUsername, setIsSubmittingUsername] = useState(false);

  // Password state
  const [currentPassword, setCurrentPassword] = useState('');
  const [newPassword, setNewPassword] = useState('');
  const [confirmPassword, setConfirmPassword] = useState('');
  const [showCurrentPassword, setShowCurrentPassword] = useState(false);
  const [showNewPassword, setShowNewPassword] = useState(false);
  const [showConfirmPassword, setShowConfirmPassword] = useState(false);
  const [passwordError, setPasswordError] = useState('');
  const [isSubmittingPassword, setIsSubmittingPassword] = useState(false);

  // Derived values
  const passwordStrength = useMemo(() => calculatePasswordStrength(newPassword), [newPassword]);
  const passwordsMatch = newPassword && confirmPassword && newPassword === confirmPassword;

  const { data: stats } = useCollectionStats();
  const navigate = useNavigate();
  const queryClient = useQueryClient();

  // Clear library state
  const [isClearDialogOpen, setIsClearDialogOpen] = useState(false);
  const [clearConfirmText, setClearConfirmText] = useState('');
  const [isClearing, setIsClearing] = useState(false);

  const handleClearLibrary = async () => {
    setIsClearing(true);
    try {
      const result = await gamesApi.clearLibrary();
      // Remove all user-games cache so /games renders empty immediately, not from stale data.
      queryClient.removeQueries({ queryKey: gameKeys.all });
      toast.success(
        `Cleared ${result.deleted} game${result.deleted === 1 ? '' : 's'} from your library`,
      );
      setIsClearDialogOpen(false);
      setClearConfirmText('');
      void navigate({ to: '/games' });
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Failed to clear library');
    } finally {
      setIsClearing(false);
    }
  };

  // Debounced username availability check
  const checkUsername = useCallback(
    async (username: string) => {
      if (!username || !user || username === user.username) {
        setUsernameAvailable(null);
        setUsernameError('');
        return;
      }

      if (username.length < 3) {
        setUsernameAvailable(false);
        setUsernameError('Username must be at least 3 characters');
        return;
      }

      setIsCheckingUsername(true);
      try {
        const result = await authApi.checkUsernameAvailability(username);
        setUsernameAvailable(result.available);
        setUsernameError(result.available ? '' : 'Username is already taken');
      } catch {
        setUsernameAvailable(false);
        setUsernameError('Error checking username availability');
      } finally {
        setIsCheckingUsername(false);
      }
    },
    [user],
  );

  // Debounce username check
  useEffect(() => {
    if (!user) return;

    const timeout = setTimeout(() => {
      checkUsername(newUsername);
    }, 500);

    return () => clearTimeout(timeout);
  }, [newUsername, checkUsername, user]);

  const handleUsernameSubmit = async () => {
    if (!newUsername || newUsername === user?.username || !usernameAvailable) {
      return;
    }

    setIsSubmittingUsername(true);
    try {
      await authApi.changeUsername(newUsername);
      toast.success('Username updated successfully! Please log in again.');
      // After username change, we need to re-login since the session might be invalidated
      setTimeout(() => {
        void logout();
      }, 1500);
    } catch (err) {
      const errorMessage = err instanceof Error ? err.message : 'Failed to update username';
      toast.error(errorMessage);
    } finally {
      setIsSubmittingUsername(false);
    }
  };

  const handlePasswordSubmit = async () => {
    setPasswordError('');

    // Validation
    if (!currentPassword || !newPassword || !confirmPassword) {
      setPasswordError('All password fields are required');
      return;
    }

    if (newPassword !== confirmPassword) {
      setPasswordError('New passwords do not match');
      return;
    }

    if (newPassword.length < 8) {
      setPasswordError('New password must be at least 8 characters');
      return;
    }

    if (currentPassword === newPassword) {
      setPasswordError('New password must be different from current password');
      return;
    }

    setIsSubmittingPassword(true);
    try {
      await authApi.changePassword(currentPassword, newPassword);
      toast.success('Password changed successfully! Please log in again.');
      setTimeout(() => {
        void logout();
      }, 1500);
    } catch (err) {
      const errorMessage = err instanceof Error ? err.message : 'Failed to change password';
      setPasswordError(errorMessage);
    } finally {
      setIsSubmittingPassword(false);
    }
  };

  const resetPasswordForm = () => {
    setCurrentPassword('');
    setNewPassword('');
    setConfirmPassword('');
    setPasswordError('');
  };

  if (!user) {
    return <ProfilePageSkeleton />;
  }

  const isUsernameChanged = newUsername !== user.username;
  const canSubmitUsername =
    isUsernameChanged && usernameAvailable === true && !isSubmittingUsername;
  const canSubmitPassword =
    currentPassword &&
    newPassword &&
    confirmPassword &&
    passwordsMatch &&
    newPassword.length >= 8 &&
    currentPassword !== newPassword &&
    !isSubmittingPassword;

  return (
    <div className="space-y-6">
      {/* Header */}
      <div>
        <h1 className="text-2xl font-bold">Profile Settings</h1>
        <p className="text-muted-foreground">
          Manage your account information and security settings
        </p>
      </div>

      <div className="grid gap-6 lg:grid-cols-3">
        {/* Main Content */}
        <div className="space-y-6 lg:col-span-2">
          {/* Account Information Section */}
          <Card>
            <CardHeader>
              <CardTitle className="flex items-center gap-2">
                <User className="h-5 w-5" />
                Account Information
              </CardTitle>
              <CardDescription>Update your username</CardDescription>
            </CardHeader>
            <CardContent className="space-y-4">
              {/* Current Username Display */}
              <div>
                <Label>Current Username</Label>
                <div className="mt-1 rounded-md border bg-muted/50 p-3 font-medium">
                  {user.username}
                </div>
              </div>

              {/* New Username Input */}
              <div>
                <Label htmlFor="newUsername">New Username</Label>
                <div className="relative mt-1">
                  <Input
                    id="newUsername"
                    type="text"
                    value={newUsername}
                    onChange={(e) => setNewUsername(e.target.value)}
                    placeholder="Enter new username"
                    className={
                      usernameAvailable === true
                        ? 'border-green-500 pr-10'
                        : usernameAvailable === false
                          ? 'border-red-500 pr-10'
                          : 'pr-10'
                    }
                  />
                  <div className="absolute inset-y-0 right-0 flex items-center pr-3">
                    {isCheckingUsername && (
                      <Loader2 className="h-5 w-5 animate-spin text-muted-foreground" />
                    )}
                    {!isCheckingUsername && usernameAvailable === true && (
                      <Check className="h-5 w-5 text-green-500" />
                    )}
                    {!isCheckingUsername && usernameAvailable === false && (
                      <X className="h-5 w-5 text-red-500" />
                    )}
                  </div>
                </div>
                {usernameError && <p className="mt-2 text-sm text-red-600">{usernameError}</p>}
                {usernameAvailable === true && (
                  <p className="mt-2 text-sm text-green-600">Username is available</p>
                )}
              </div>

              {/* Update Username Button */}
              <Button onClick={handleUsernameSubmit} disabled={!canSubmitUsername}>
                {isSubmittingUsername && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
                Update Username
              </Button>
            </CardContent>
          </Card>

          {/* Password & Security Section */}
          <Card>
            <CardHeader>
              <CardTitle>Password & Security</CardTitle>
              <CardDescription>Change your password</CardDescription>
            </CardHeader>
            <CardContent className="space-y-4">
              {/* Current Password */}
              <div>
                <Label htmlFor="currentPassword">Current Password</Label>
                <div className="relative mt-1">
                  <Input
                    id="currentPassword"
                    type={showCurrentPassword ? 'text' : 'password'}
                    value={currentPassword}
                    onChange={(e) => setCurrentPassword(e.target.value)}
                    placeholder="Enter current password"
                    className="pr-10"
                  />
                  <button
                    type="button"
                    onClick={() => setShowCurrentPassword(!showCurrentPassword)}
                    className="absolute inset-y-0 right-0 flex items-center pr-3"
                    aria-label={
                      showCurrentPassword ? 'Hide current password' : 'Show current password'
                    }
                  >
                    {showCurrentPassword ? (
                      <EyeOff className="h-5 w-5 text-muted-foreground" />
                    ) : (
                      <Eye className="h-5 w-5 text-muted-foreground" />
                    )}
                  </button>
                </div>
              </div>

              {/* New Password */}
              <div>
                <Label htmlFor="newPassword">New Password</Label>
                <div className="relative mt-1">
                  <Input
                    id="newPassword"
                    type={showNewPassword ? 'text' : 'password'}
                    value={newPassword}
                    onChange={(e) => setNewPassword(e.target.value)}
                    placeholder="Enter new password"
                    className="pr-10"
                  />
                  <button
                    type="button"
                    onClick={() => setShowNewPassword(!showNewPassword)}
                    className="absolute inset-y-0 right-0 flex items-center pr-3"
                    aria-label={showNewPassword ? 'Hide new password' : 'Show new password'}
                  >
                    {showNewPassword ? (
                      <EyeOff className="h-5 w-5 text-muted-foreground" />
                    ) : (
                      <Eye className="h-5 w-5 text-muted-foreground" />
                    )}
                  </button>
                </div>

                {/* Password Strength Meter */}
                {newPassword && (
                  <div className="mt-2 space-y-1">
                    <div className="flex items-center gap-2">
                      <Progress value={(passwordStrength.score / 6) * 100} className="h-2 flex-1" />
                      <span className="text-sm text-muted-foreground">
                        {passwordStrength.label}
                      </span>
                    </div>
                  </div>
                )}
              </div>

              {/* Confirm New Password */}
              <div>
                <Label htmlFor="confirmPassword">Confirm New Password</Label>
                <div className="relative mt-1">
                  <Input
                    id="confirmPassword"
                    type={showConfirmPassword ? 'text' : 'password'}
                    value={confirmPassword}
                    onChange={(e) => setConfirmPassword(e.target.value)}
                    placeholder="Confirm new password"
                    className={
                      confirmPassword
                        ? passwordsMatch
                          ? 'border-green-500 pr-10'
                          : 'border-red-500 pr-10'
                        : 'pr-10'
                    }
                  />
                  <button
                    type="button"
                    onClick={() => setShowConfirmPassword(!showConfirmPassword)}
                    className="absolute inset-y-0 right-0 flex items-center pr-3"
                    aria-label={
                      showConfirmPassword ? 'Hide confirm password' : 'Show confirm password'
                    }
                  >
                    {showConfirmPassword ? (
                      <EyeOff className="h-5 w-5 text-muted-foreground" />
                    ) : (
                      <Eye className="h-5 w-5 text-muted-foreground" />
                    )}
                  </button>
                </div>
                {confirmPassword && !passwordsMatch && (
                  <p className="mt-2 text-sm text-red-600">Passwords do not match</p>
                )}
              </div>

              {/* Error Message */}
              {passwordError && (
                <Alert variant="destructive">
                  <AlertCircle className="h-4 w-4" />
                  <AlertDescription>{passwordError}</AlertDescription>
                </Alert>
              )}

              {/* Buttons */}
              <div className="flex gap-3">
                <Button
                  onClick={handlePasswordSubmit}
                  disabled={!canSubmitPassword}
                  variant="destructive"
                >
                  {isSubmittingPassword && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
                  Change Password
                </Button>
                <Button onClick={resetPasswordForm} variant="outline" type="button">
                  Cancel
                </Button>
              </div>
            </CardContent>
          </Card>

          {/* Preferences Section */}
          <PreferencesSection />

          {/* Notifications Section */}
          <NotificationsSection />

          {/* API Keys Section */}
          <ApiKeysSection />
        </div>

        {/* Requirements Info Box (Desktop) */}
        <div className="hidden lg:block">
          <Card>
            <CardHeader>
              <CardTitle className="text-sm">Username Requirements</CardTitle>
            </CardHeader>
            <CardContent className="space-y-4">
              <ul className="space-y-1 text-xs text-muted-foreground">
                <li>3-100 characters long</li>
                <li>Letters, numbers, underscore only</li>
                <li>No spaces or special characters</li>
                <li>Must be unique</li>
              </ul>

              <div>
                <h4 className="mb-2 text-sm font-semibold">Password Requirements</h4>
                <ul className="space-y-1 text-xs text-muted-foreground">
                  <li>Minimum 8 characters</li>
                  <li>At least one uppercase letter</li>
                  <li>At least one number</li>
                  <li>Special character recommended</li>
                </ul>
              </div>
            </CardContent>
          </Card>
        </div>
      </div>

      {/* Danger Zone */}
      <Card className="border-red-200 dark:border-red-800">
        <CardHeader>
          <CardTitle className="flex items-center gap-2 text-red-600 dark:text-red-400">
            <Trash2 className="h-5 w-5" />
            Danger Zone
          </CardTitle>
          <CardDescription>These actions are permanent and cannot be undone.</CardDescription>
        </CardHeader>
        <CardContent>
          <div className="flex items-center justify-between">
            <div>
              <p className="font-medium">Clear Library</p>
              <p className="text-sm text-muted-foreground">Remove all games from your library.</p>
            </div>
            <Button variant="destructive" onClick={() => setIsClearDialogOpen(true)}>
              Clear Library
            </Button>
          </div>
        </CardContent>
      </Card>

      <Dialog
        open={isClearDialogOpen}
        onOpenChange={(open) => {
          if (!open) {
            setIsClearDialogOpen(false);
            setClearConfirmText('');
          }
        }}
      >
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Clear Library</DialogTitle>
            <DialogDescription>
              This will permanently remove all{' '}
              <strong>
                {stats?.totalGames ?? '?'} game{stats?.totalGames === 1 ? '' : 's'}
              </strong>{' '}
              from your library. This cannot be undone.
            </DialogDescription>
          </DialogHeader>
          <div className="space-y-2">
            <p className="text-sm text-muted-foreground">
              Type <strong>DELETE</strong> to confirm:
            </p>
            <Input
              value={clearConfirmText}
              onChange={(e) => setClearConfirmText(e.target.value)}
              placeholder="Type DELETE to confirm"
              autoComplete="off"
            />
          </div>
          <DialogFooter>
            <Button
              variant="outline"
              onClick={() => {
                setIsClearDialogOpen(false);
                setClearConfirmText('');
              }}
            >
              Cancel
            </Button>
            <Button
              variant="destructive"
              disabled={clearConfirmText !== 'DELETE' || isClearing}
              onClick={handleClearLibrary}
            >
              {isClearing ? (
                <>
                  <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                  Clearing...
                </>
              ) : (
                'Clear Library'
              )}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}
