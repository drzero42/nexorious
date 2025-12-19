'use client';

import { useState, useEffect } from 'react';
import Link from 'next/link';
import { useRouter } from 'next/navigation';
import { useAuth } from '@/providers';
import { Card, CardContent, CardHeader, CardTitle, CardDescription } from '@/components/ui/card';
import { Button } from '@/components/ui/button';
import { Badge } from '@/components/ui/badge';
import { Skeleton } from '@/components/ui/skeleton';
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert';
import { toast } from 'sonner';
import {
  Users,
  UserPlus,
  Shield,
  Gamepad2,
  CheckCircle,
  AlertCircle,
  Loader2,
  Package,
  Settings,
} from 'lucide-react';
import * as adminApi from '@/api/admin';
import type { AdminStatistics, SeedDataResult } from '@/types';

function formatDate(dateString: string): string {
  return new Date(dateString).toLocaleDateString('en-US', {
    year: 'numeric',
    month: 'short',
    day: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
  });
}

function AdminDashboardSkeleton() {
  return (
    <div className="space-y-6">
      <div>
        <Skeleton className="mb-2 h-8 w-48" />
        <Skeleton className="h-4 w-64" />
      </div>
      <div className="grid grid-cols-1 gap-6 sm:grid-cols-2 lg:grid-cols-4">
        {[1, 2, 3, 4].map((i) => (
          <Skeleton key={i} className="h-32" />
        ))}
      </div>
      <Skeleton className="h-64" />
      <Skeleton className="h-48" />
    </div>
  );
}

export default function AdminDashboardPage() {
  const router = useRouter();
  const { user: currentUser } = useAuth();
  const [statistics, setStatistics] = useState<AdminStatistics | null>(null);
  const [seedDataResult, setSeedDataResult] = useState<SeedDataResult | null>(null);
  const [isLoading, setIsLoading] = useState(true);
  const [isSeedDataLoading, setIsSeedDataLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  // Check admin access
  useEffect(() => {
    if (currentUser && !currentUser.isAdmin) {
      router.replace('/dashboard');
    }
  }, [currentUser, router]);

  // Fetch statistics
  useEffect(() => {
    const fetchStatistics = async () => {
      try {
        setIsLoading(true);
        setError(null);
        const stats = await adminApi.getAdminStatistics();
        setStatistics(stats);
      } catch (err) {
        const errorMessage = err instanceof Error ? err.message : 'Failed to fetch statistics';
        setError(errorMessage);
      } finally {
        setIsLoading(false);
      }
    };

    if (currentUser?.isAdmin) {
      fetchStatistics();
    }
  }, [currentUser]);

  const handleLoadSeedData = async () => {
    const confirmed = window.confirm(
      'This will load official platforms, storefronts, and their default mappings into the database. ' +
        'Existing data will be preserved. Continue?'
    );

    if (!confirmed) return;

    try {
      setIsSeedDataLoading(true);
      const result = await adminApi.loadSeedData();
      setSeedDataResult(result);
      toast.success('Seed data loaded successfully');
    } catch (err) {
      const errorMessage = err instanceof Error ? err.message : 'Failed to load seed data';
      toast.error(errorMessage);
    } finally {
      setIsSeedDataLoading(false);
    }
  };

  // Show nothing while checking auth
  if (!currentUser?.isAdmin) {
    return null;
  }

  if (isLoading) {
    return <AdminDashboardSkeleton />;
  }

  return (
    <div className="space-y-6">
      {/* Header */}
      <div className="border-b pb-5">
        <h1 className="text-3xl font-bold">Admin Dashboard</h1>
        <p className="mt-2 text-muted-foreground">System overview and management tools</p>
      </div>

      {/* Error Alert */}
      {error && (
        <Alert variant="destructive">
          <AlertCircle className="h-4 w-4" />
          <AlertTitle>Error loading dashboard</AlertTitle>
          <AlertDescription className="flex items-center justify-between">
            <span>{error}</span>
            <Button variant="outline" size="sm" onClick={() => setError(null)}>
              Dismiss
            </Button>
          </AlertDescription>
        </Alert>
      )}

      {/* Seed Data Result */}
      {seedDataResult && (
        <Alert className="border-green-200 bg-green-50 text-green-800">
          <CheckCircle className="h-4 w-4 text-green-600" />
          <AlertTitle>Seed Data Loading Complete</AlertTitle>
          <AlertDescription>
            <p>{seedDataResult.message}</p>
            {seedDataResult.totalChanges > 0 && (
              <ul className="mt-2 list-inside list-disc">
                <li>{seedDataResult.platformsAdded} platforms added/updated</li>
                <li>{seedDataResult.storefrontsAdded} storefronts added/updated</li>
                <li>{seedDataResult.mappingsCreated} default mappings created</li>
              </ul>
            )}
            <Button
              variant="ghost"
              size="sm"
              className="mt-2"
              onClick={() => setSeedDataResult(null)}
            >
              Dismiss
            </Button>
          </AlertDescription>
        </Alert>
      )}

      {statistics && (
        <>
          {/* Statistics Cards */}
          <div className="grid grid-cols-1 gap-6 sm:grid-cols-2 lg:grid-cols-4">
            <Card>
              <CardContent className="pt-6">
                <div className="flex items-center gap-4">
                  <div className="rounded-lg bg-blue-100 p-3">
                    <Users className="h-6 w-6 text-blue-600" />
                  </div>
                  <div>
                    <p className="text-sm text-muted-foreground">Total Users</p>
                    <p className="text-3xl font-bold">{statistics.totalUsers}</p>
                  </div>
                </div>
              </CardContent>
            </Card>

            <Card>
              <CardContent className="pt-6">
                <div className="flex items-center gap-4">
                  <div className="rounded-lg bg-purple-100 p-3">
                    <Shield className="h-6 w-6 text-purple-600" />
                  </div>
                  <div>
                    <p className="text-sm text-muted-foreground">Admin Users</p>
                    <p className="text-3xl font-bold">{statistics.totalAdmins}</p>
                  </div>
                </div>
              </CardContent>
            </Card>

            <Card>
              <CardContent className="pt-6">
                <div className="flex items-center gap-4">
                  <div className="rounded-lg bg-green-100 p-3">
                    <Gamepad2 className="h-6 w-6 text-green-600" />
                  </div>
                  <div>
                    <p className="text-sm text-muted-foreground">Total Games</p>
                    <p className="text-3xl font-bold">
                      {statistics.totalGames > 0 ? statistics.totalGames : 'N/A'}
                    </p>
                  </div>
                </div>
              </CardContent>
            </Card>

            <Card>
              <CardContent className="pt-6">
                <div className="flex items-center gap-4">
                  <div className="rounded-lg bg-emerald-100 p-3">
                    <CheckCircle className="h-6 w-6 text-emerald-600" />
                  </div>
                  <div>
                    <p className="text-sm text-muted-foreground">System Status</p>
                    <p className="text-xl font-bold text-green-600">Healthy</p>
                  </div>
                </div>
              </CardContent>
            </Card>
          </div>

          {/* Recent Users */}
          {statistics.recentUsers.length > 0 && (
            <Card>
              <CardHeader>
                <CardTitle>Recent Users</CardTitle>
                <CardDescription>Latest users registered in the system</CardDescription>
              </CardHeader>
              <CardContent>
                <div className="space-y-4">
                  {statistics.recentUsers.map((user) => (
                    <div
                      key={user.id}
                      className="flex items-center justify-between rounded-lg border p-4"
                    >
                      <div className="flex items-center gap-4">
                        <div className="flex h-10 w-10 items-center justify-center rounded-full bg-muted">
                          <span className="text-sm font-medium">
                            {user.username.charAt(0).toUpperCase()}
                          </span>
                        </div>
                        <div>
                          <div className="flex items-center gap-2">
                            <span className="font-medium">{user.username}</span>
                            {user.isAdmin && (
                              <Badge className="bg-purple-100 text-purple-800">Admin</Badge>
                            )}
                            {!user.isActive && <Badge variant="destructive">Inactive</Badge>}
                          </div>
                          <p className="text-sm text-muted-foreground">
                            Created {formatDate(user.createdAt)}
                          </p>
                        </div>
                      </div>
                      <Button variant="outline" size="sm" asChild>
                        <Link href={`/admin/users/${user.id}`}>View</Link>
                      </Button>
                    </div>
                  ))}
                </div>
                <div className="mt-4">
                  <Button variant="outline" className="w-full" asChild>
                    <Link href="/admin/users">View all users</Link>
                  </Button>
                </div>
              </CardContent>
            </Card>
          )}

          {/* Quick Actions */}
          <Card>
            <CardHeader>
              <CardTitle>Quick Actions</CardTitle>
              <CardDescription>Common administrative tasks</CardDescription>
            </CardHeader>
            <CardContent>
              <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-4">
                <Button asChild>
                  <Link href="/admin/users/new">
                    <UserPlus className="mr-2 h-4 w-4" />
                    Create User
                  </Link>
                </Button>
                <Button variant="outline" asChild>
                  <Link href="/admin/users">
                    <Users className="mr-2 h-4 w-4" />
                    Manage Users
                  </Link>
                </Button>
                <Button variant="outline" asChild>
                  <Link href="/admin/platforms">
                    <Settings className="mr-2 h-4 w-4" />
                    Manage Platforms
                  </Link>
                </Button>
                <Button
                  variant="outline"
                  onClick={handleLoadSeedData}
                  disabled={isSeedDataLoading}
                >
                  {isSeedDataLoading ? (
                    <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                  ) : (
                    <Package className="mr-2 h-4 w-4" />
                  )}
                  {isSeedDataLoading ? 'Loading...' : 'Load Seed Data'}
                </Button>
              </div>
            </CardContent>
          </Card>
        </>
      )}
    </div>
  );
}
