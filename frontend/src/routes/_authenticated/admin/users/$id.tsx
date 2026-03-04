import { useState, useEffect, useMemo } from 'react';
import { createFileRoute, Link, useNavigate } from '@tanstack/react-router';
import { useAuth } from '@/providers';
import { Card, CardContent, CardHeader, CardTitle, CardDescription } from '@/components/ui/card';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Badge } from '@/components/ui/badge';
import { Checkbox } from '@/components/ui/checkbox';
import { Skeleton } from '@/components/ui/skeleton';
import { Alert, AlertDescription } from '@/components/ui/alert';
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from '@/components/ui/alert-dialog';
import { toast } from 'sonner';
import { ArrowLeft, AlertCircle, Loader2, Check, Eye, EyeOff, AlertTriangle, Trash2 } from 'lucide-react';
import * as adminApi from '@/api/admin';
import type { AdminUser, UserDeletionImpact } from '@/types';

export const Route = createFileRoute('/_authenticated/admin/users/$id')({
  component: EditUserPage,
});

function formatDate(dateString: string): string {
  return new Date(dateString).toLocaleDateString('en-US', {
    year: 'numeric',
    month: 'short',
    day: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
  });
}

function UserStatusBadges({ user }: { user: AdminUser }) {
  return (
    <div className="flex flex-wrap gap-1">
      {user.isAdmin && (
        <Badge className="bg-purple-100 text-purple-800 hover:bg-purple-200">Admin</Badge>
      )}
      {!user.isActive ? (
        <Badge variant="destructive">Inactive</Badge>
      ) : !user.isAdmin ? (
        <Badge className="bg-green-100 text-green-800 hover:bg-green-200">User</Badge>
      ) : null}
    </div>
  );
}

function EditUserSkeleton() {
  return (
    <div className="space-y-6">
      <div className="flex items-center gap-4">
        <Skeleton className="h-6 w-6" />
        <div>
          <Skeleton className="h-8 w-64 mb-2" />
          <Skeleton className="h-4 w-96" />
        </div>
      </div>
      <Card className="max-w-2xl">
        <CardHeader>
          <Skeleton className="h-6 w-40" />
        </CardHeader>
        <CardContent className="space-y-6">
          <Skeleton className="h-16 w-full" />
          <Skeleton className="h-10 w-full" />
          <Skeleton className="h-10 w-full" />
          <Skeleton className="h-10 w-32" />
        </CardContent>
      </Card>
    </div>
  );
}

function EditUserPage() {
  const navigate = useNavigate();
  const { id: userId } = Route.useParams();
  const { user: currentUser } = useAuth();

  const [user, setUser] = useState<AdminUser | null>(null);
  const [isLoading, setIsLoading] = useState(true);
  const [isSaving, setIsSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [successMessage, setSuccessMessage] = useState<string | null>(null);

  // Form state
  const [formData, setFormData] = useState({
    username: '',
    isActive: true,
    isAdmin: false,
  });
  const [originalData, setOriginalData] = useState({
    username: '',
    isActive: true,
    isAdmin: false,
  });

  // Password reset state
  const [showPasswordReset, setShowPasswordReset] = useState(false);
  const [newPassword, setNewPassword] = useState('');
  const [showNewPassword, setShowNewPassword] = useState(false);

  // Delete state
  const [showDeleteDialog, setShowDeleteDialog] = useState(false);
  const [deletionStep, setDeletionStep] = useState(1);
  const [deletionImpact, setDeletionImpact] = useState<UserDeletionImpact | null>(null);
  const [isDeletionImpactLoading, setIsDeletionImpactLoading] = useState(false);
  const [usernameConfirmation, setUsernameConfirmation] = useState('');

  // Derived values
  const isEditingSelf = useMemo(() => currentUser?.id === userId, [currentUser, userId]);
  const hasChanges = useMemo(
    () =>
      formData.username !== originalData.username ||
      formData.isActive !== originalData.isActive ||
      formData.isAdmin !== originalData.isAdmin,
    [formData, originalData]
  );

  // Check admin access
  useEffect(() => {
    if (currentUser && !currentUser.isAdmin) {
      navigate({ to: '/dashboard', replace: true });
    }
  }, [currentUser, navigate]);

  // Fetch user
  useEffect(() => {
    const fetchUser = async () => {
      try {
        setIsLoading(true);
        setError(null);
        const fetchedUser = await adminApi.getUserById(userId);
        setUser(fetchedUser);
        const data = {
          username: fetchedUser.username,
          isActive: fetchedUser.isActive,
          isAdmin: fetchedUser.isAdmin,
        };
        setFormData(data);
        setOriginalData(data);
      } catch (err) {
        const errorMessage = err instanceof Error ? err.message : 'Failed to fetch user';
        setError(errorMessage);
      } finally {
        setIsLoading(false);
      }
    };

    if (currentUser?.isAdmin && userId) {
      fetchUser();
    }
  }, [currentUser, userId]);

  // Handle save
  const handleSave = async () => {
    if (!user || !hasChanges) return;

    // Validate form
    if (!formData.username.trim()) {
      setError('Username is required');
      return;
    }

    // Check for self-modification restrictions
    if (isEditingSelf) {
      if (!formData.isActive) {
        setError('You cannot deactivate your own account');
        return;
      }
      if (!formData.isAdmin) {
        setError('You cannot remove your own admin privileges');
        return;
      }
    }

    try {
      setIsSaving(true);
      setError(null);
      setSuccessMessage(null);

      const updatedUser = await adminApi.updateUser(user.id, {
        username: formData.username.trim(),
        is_active: formData.isActive,
        is_admin: formData.isAdmin,
      });

      setUser(updatedUser);
      const data = {
        username: updatedUser.username,
        isActive: updatedUser.isActive,
        isAdmin: updatedUser.isAdmin,
      };
      setFormData(data);
      setOriginalData(data);
      setSuccessMessage('User updated successfully');
      toast.success('User updated successfully');

      setTimeout(() => setSuccessMessage(null), 3000);
    } catch (err) {
      const errorMessage = err instanceof Error ? err.message : 'Failed to update user';
      setError(errorMessage);
    } finally {
      setIsSaving(false);
    }
  };

  // Handle password reset
  const handlePasswordReset = async () => {
    if (!user || !newPassword.trim()) return;

    try {
      setIsSaving(true);
      setError(null);

      await adminApi.resetUserPassword(user.id, newPassword.trim());

      setSuccessMessage('Password reset successfully. User will need to log in again.');
      toast.success('Password reset successfully');
      setNewPassword('');
      setShowPasswordReset(false);

      setTimeout(() => setSuccessMessage(null), 5000);
    } catch (err) {
      const errorMessage = err instanceof Error ? err.message : 'Failed to reset password';
      setError(errorMessage);
    } finally {
      setIsSaving(false);
    }
  };

  // Handle delete
  const startDeletion = async () => {
    if (!user) return;

    setShowDeleteDialog(true);
    setDeletionStep(1);
    setUsernameConfirmation('');

    try {
      setIsDeletionImpactLoading(true);
      const impact = await adminApi.getUserDeletionImpact(user.id);
      setDeletionImpact(impact);
    } catch (err) {
      const errorMessage = err instanceof Error ? err.message : 'Failed to get deletion impact';
      setError(errorMessage);
      setShowDeleteDialog(false);
    } finally {
      setIsDeletionImpactLoading(false);
    }
  };

  const confirmDeletion = async () => {
    if (!user || !deletionImpact || usernameConfirmation !== user.username) return;

    try {
      setIsSaving(true);
      await adminApi.deleteUser(user.id);
      toast.success('User deleted successfully');
      navigate({ to: '/admin/users' });
    } catch (err) {
      const errorMessage = err instanceof Error ? err.message : 'Failed to delete user';
      setError(errorMessage);
      setIsSaving(false);
    }
  };

  const cancelDeletion = () => {
    setShowDeleteDialog(false);
    setDeletionStep(1);
    setDeletionImpact(null);
    setUsernameConfirmation('');
  };

  const resetForm = () => {
    setFormData({ ...originalData });
    setError(null);
    setSuccessMessage(null);
  };

  // Show loading state
  if (!currentUser?.isAdmin) {
    return null;
  }

  if (isLoading) {
    return <EditUserSkeleton />;
  }

  return (
    <div className="space-y-6">
      {/* Header */}
      <div className="flex items-center gap-4">
        <Button variant="ghost" size="icon" asChild>
          <Link to="/admin/users" aria-label="Back to users">
            <ArrowLeft className="h-4 w-4" />
          </Link>
        </Button>
        <div>
          <h1 className="text-2xl font-bold">
            {user ? `Edit User: ${user.username}` : 'User Not Found'}
          </h1>
          {user && (
            <p className="text-muted-foreground">
              User ID: {user.id} - Created {formatDate(user.createdAt)}
              {isEditingSelf && (
                <span className="ml-2 text-primary font-medium">(This is you)</span>
              )}
            </p>
          )}
        </div>
      </div>

      {/* Error Alert */}
      {error && (
        <Alert variant="destructive">
          <AlertCircle className="h-4 w-4" />
          <AlertDescription className="flex items-center justify-between">
            <span>{error}</span>
            <Button variant="outline" size="sm" onClick={() => setError(null)}>
              Dismiss
            </Button>
          </AlertDescription>
        </Alert>
      )}

      {/* Success Alert */}
      {successMessage && (
        <Alert className="border-green-500 bg-green-50 text-green-800">
          <Check className="h-4 w-4" />
          <AlertDescription>{successMessage}</AlertDescription>
        </Alert>
      )}

      {user ? (
        <div className="grid gap-6 lg:grid-cols-2">
          {/* User Information Card */}
          <Card>
            <CardHeader>
              <CardTitle>User Information</CardTitle>
              <CardDescription>Edit user details and permissions</CardDescription>
            </CardHeader>
            <CardContent className="space-y-6">
              {/* Current Status Display */}
              <div className="p-4 bg-muted/50 rounded-lg">
                <div className="flex items-center gap-4">
                  <div className="h-12 w-12 rounded-full bg-muted flex items-center justify-center">
                    <span className="text-lg font-medium">
                      {user.username.charAt(0).toUpperCase()}
                    </span>
                  </div>
                  <div>
                    <p className="font-medium text-lg">{user.username}</p>
                    <UserStatusBadges user={user} />
                    <p className="text-sm text-muted-foreground mt-1">
                      Last updated {formatDate(user.updatedAt)}
                    </p>
                  </div>
                </div>
              </div>

              {/* Username Input */}
              <div>
                <Label htmlFor="username">Username</Label>
                <Input
                  id="username"
                  value={formData.username}
                  onChange={(e) => setFormData((prev) => ({ ...prev, username: e.target.value }))}
                  className="mt-1"
                />
              </div>

              {/* Account Status */}
              <div className="space-y-4">
                <Label>Account Status</Label>

                <div className="flex items-center space-x-3">
                  <Checkbox
                    id="isActive"
                    checked={formData.isActive}
                    onCheckedChange={(checked) =>
                      setFormData((prev) => ({ ...prev, isActive: checked === true }))
                    }
                    disabled={isEditingSelf}
                  />
                  <label
                    htmlFor="isActive"
                    className={`text-sm ${isEditingSelf ? 'text-muted-foreground' : ''}`}
                  >
                    Account is active
                    {isEditingSelf && (
                      <span className="text-xs ml-2">(Cannot modify your own account status)</span>
                    )}
                  </label>
                </div>

                <div className="flex items-center space-x-3">
                  <Checkbox
                    id="isAdmin"
                    checked={formData.isAdmin}
                    onCheckedChange={(checked) =>
                      setFormData((prev) => ({ ...prev, isAdmin: checked === true }))
                    }
                    disabled={isEditingSelf}
                  />
                  <label
                    htmlFor="isAdmin"
                    className={`text-sm ${isEditingSelf ? 'text-muted-foreground' : ''}`}
                  >
                    Administrator privileges
                    {isEditingSelf && (
                      <span className="text-xs ml-2">(Cannot modify your own admin privileges)</span>
                    )}
                  </label>
                </div>
              </div>

              {/* Form Actions */}
              <div className="flex gap-3 pt-4 border-t">
                <Button onClick={handleSave} disabled={!hasChanges || isSaving}>
                  {isSaving && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
                  Save Changes
                </Button>
                <Button variant="outline" onClick={resetForm} disabled={!hasChanges || isSaving}>
                  Reset
                </Button>
              </div>
            </CardContent>
          </Card>

          {/* Account Actions Card */}
          <Card>
            <CardHeader>
              <CardTitle>Account Actions</CardTitle>
              <CardDescription>Password reset and account deletion</CardDescription>
            </CardHeader>
            <CardContent className="space-y-6">
              {/* Password Reset */}
              <div className="border rounded-lg p-4">
                <h4 className="font-medium mb-2">Reset Password</h4>
                <p className="text-sm text-muted-foreground mb-4">
                  Generate a new password for this user. They will need to log in again with the new
                  password.
                </p>

                {showPasswordReset ? (
                  <div className="space-y-3">
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
                          aria-label={showNewPassword ? 'Hide password' : 'Show password'}
                        >
                          {showNewPassword ? (
                            <EyeOff className="h-4 w-4 text-muted-foreground" />
                          ) : (
                            <Eye className="h-4 w-4 text-muted-foreground" />
                          )}
                        </button>
                      </div>
                    </div>
                    <div className="flex gap-3">
                      <Button
                        variant="secondary"
                        onClick={handlePasswordReset}
                        disabled={!newPassword.trim() || isSaving}
                      >
                        {isSaving && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
                        Reset Password
                      </Button>
                      <Button
                        variant="outline"
                        onClick={() => {
                          setShowPasswordReset(false);
                          setNewPassword('');
                        }}
                        disabled={isSaving}
                      >
                        Cancel
                      </Button>
                    </div>
                  </div>
                ) : (
                  <Button variant="secondary" onClick={() => setShowPasswordReset(true)}>
                    Reset Password
                  </Button>
                )}
              </div>

              {/* Delete User */}
              {!isEditingSelf && (
                <div className="border border-destructive/50 rounded-lg p-4">
                  <h4 className="font-medium text-destructive mb-2">Delete User</h4>
                  <p className="text-sm text-destructive/80 mb-4">
                    Permanently delete this user account and all associated data. This action cannot
                    be undone.
                  </p>
                  <Button variant="destructive" onClick={startDeletion} disabled={isSaving}>
                    <Trash2 className="mr-2 h-4 w-4" />
                    Delete User
                  </Button>
                </div>
              )}
            </CardContent>
          </Card>
        </div>
      ) : (
        /* User Not Found */
        <Card className="max-w-md">
          <CardContent className="pt-6">
            <div className="text-center py-12">
              <h3 className="text-lg font-medium mb-2">User Not Found</h3>
              <p className="text-muted-foreground mb-4">
                The user with ID &quot;{userId}&quot; could not be found.
              </p>
              <Button asChild>
                <Link to="/admin/users">Back to Users</Link>
              </Button>
            </div>
          </CardContent>
        </Card>
      )}

      {/* Delete Confirmation Dialog */}
      <AlertDialog open={showDeleteDialog} onOpenChange={setShowDeleteDialog}>
        <AlertDialogContent>
          {deletionStep === 1 ? (
            <>
              <AlertDialogHeader>
                <AlertDialogTitle className="flex items-center gap-2">
                  <AlertTriangle className="h-5 w-5 text-destructive" />
                  Delete User: {user?.username}
                </AlertDialogTitle>
                <AlertDialogDescription asChild>
                  <div className="space-y-4">
                    {isDeletionImpactLoading ? (
                      <div className="flex justify-center py-4">
                        <Loader2 className="h-8 w-8 animate-spin text-destructive" />
                      </div>
                    ) : deletionImpact ? (
                      <>
                        <p>This action will permanently delete the following data:</p>
                        <div className="bg-destructive/10 border border-destructive/20 rounded-lg p-4 space-y-2 text-sm">
                          <div className="flex justify-between">
                            <span>Games in collection:</span>
                            <span className="font-medium text-destructive">
                              {deletionImpact.total_games}
                            </span>
                          </div>
                          <div className="flex justify-between">
                            <span>User-created tags:</span>
                            <span className="font-medium text-destructive">
                              {deletionImpact.total_tags}
                            </span>
                          </div>
                          <div className="flex justify-between">
                            <span>Wishlist items:</span>
                            <span className="font-medium text-destructive">
                              {deletionImpact.total_wishlist_items}
                            </span>
                          </div>
                          <div className="flex justify-between">
                            <span>Import jobs:</span>
                            <span className="font-medium text-destructive">
                              {deletionImpact.total_import_jobs}
                            </span>
                          </div>
                          <div className="flex justify-between">
                            <span>Active sessions:</span>
                            <span className="font-medium text-destructive">
                              {deletionImpact.total_sessions}
                            </span>
                          </div>
                        </div>
                        <Alert>
                          <AlertTriangle className="h-4 w-4" />
                          <AlertDescription>{deletionImpact.warning}</AlertDescription>
                        </Alert>
                      </>
                    ) : null}
                  </div>
                </AlertDialogDescription>
              </AlertDialogHeader>
              <AlertDialogFooter>
                <AlertDialogCancel onClick={cancelDeletion}>Cancel</AlertDialogCancel>
                <Button
                  variant="destructive"
                  onClick={() => setDeletionStep(2)}
                  disabled={isDeletionImpactLoading || !deletionImpact}
                >
                  Continue
                </Button>
              </AlertDialogFooter>
            </>
          ) : (
            <>
              <AlertDialogHeader>
                <AlertDialogTitle>Final Confirmation</AlertDialogTitle>
                <AlertDialogDescription asChild>
                  <div className="space-y-4">
                    <p>
                      To confirm deletion, please type the username{' '}
                      <strong>{user?.username}</strong> in the field below:
                    </p>
                    <Input
                      value={usernameConfirmation}
                      onChange={(e) => setUsernameConfirmation(e.target.value)}
                      placeholder="Enter username to confirm"
                    />
                    <Alert variant="destructive">
                      <AlertTriangle className="h-4 w-4" />
                      <AlertDescription>
                        This will permanently delete all user data and cannot be undone.
                      </AlertDescription>
                    </Alert>
                  </div>
                </AlertDialogDescription>
              </AlertDialogHeader>
              <AlertDialogFooter>
                <AlertDialogCancel onClick={() => setDeletionStep(1)}>Back</AlertDialogCancel>
                <AlertDialogAction
                  onClick={confirmDeletion}
                  disabled={isSaving || usernameConfirmation !== user?.username}
                  className="bg-destructive text-destructive-foreground hover:bg-destructive/90"
                >
                  {isSaving && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
                  Delete User
                </AlertDialogAction>
              </AlertDialogFooter>
            </>
          )}
        </AlertDialogContent>
      </AlertDialog>
    </div>
  );
}
