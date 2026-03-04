import { useState, useEffect } from 'react';
import { createFileRoute, Link, useNavigate } from '@tanstack/react-router';
import { useAuth } from '@/providers';
import { useActiveJob, useCancelJob } from '@/hooks';
import { JobType } from '@/types';
import { Card, CardContent, CardHeader, CardTitle, CardDescription } from '@/components/ui/card';
import { Button } from '@/components/ui/button';
import { Skeleton } from '@/components/ui/skeleton';
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert';
import { JobProgressCard, JobItemsDetails, RecentActivity } from '@/components/jobs';
import { toast } from 'sonner';
import {
  Package,
  Trash2,
  RefreshCw,
  Loader2,
  CheckCircle,
  RotateCcw,
} from 'lucide-react';
import * as adminApi from '@/api/admin';
import type { SeedDataResult } from '@/types';

export const Route = createFileRoute('/_authenticated/admin/maintenance')({
  component: MaintenancePage,
});

function MaintenancePageSkeleton() {
  return (
    <div className="space-y-6">
      <div>
        <Skeleton className="mb-2 h-8 w-48" />
        <Skeleton className="h-4 w-64" />
      </div>
      <div className="grid grid-cols-1 gap-6 lg:grid-cols-2">
        <Skeleton className="h-64" />
        <Skeleton className="h-64" />
      </div>
      <Skeleton className="h-64" />
    </div>
  );
}

function MaintenancePage() {
  const navigate = useNavigate();
  const { user: currentUser } = useAuth();
  const [isLoading, setIsLoading] = useState(true);
  const [isSeedLoading, setIsSeedLoading] = useState(false);
  const [seedResult, setSeedResult] = useState<SeedDataResult | null>(null);
  const [isRefreshLoading, setIsRefreshLoading] = useState(false);
  const [dismissedJobId, setDismissedJobId] = useState<string | null>(null);

  // Track active maintenance job
  const { data: activeMaintenanceJob, refetch: refetchMaintenanceJob } = useActiveJob(JobType.MAINTENANCE);
  const { mutate: cancelJob, isPending: isCancelling } = useCancelJob();

  // Determine which job to display (not dismissed)
  const activeJob = activeMaintenanceJob && activeMaintenanceJob.id !== dismissedJobId
    ? activeMaintenanceJob
    : null;

  const showJobProgress = activeJob != null;
  const hasActiveJob = activeJob != null && !activeJob.isTerminal;

  // Check admin access
  useEffect(() => {
    if (currentUser && !currentUser.isAdmin) {
      navigate({ to: '/dashboard', replace: true });
    } else if (currentUser?.isAdmin) {
      setIsLoading(false);
    }
  }, [currentUser, navigate]);

  const handleLoadSeedData = async () => {
    try {
      setIsSeedLoading(true);
      const result = await adminApi.loadSeedData();
      setSeedResult(result);
      toast.success('Seed data loaded successfully');
    } catch (err) {
      const message = err instanceof Error ? err.message : 'Failed to load seed data';
      toast.error(message);
    } finally {
      setIsSeedLoading(false);
    }
  };

  const handleStartMetadataRefresh = async () => {
    try {
      setIsRefreshLoading(true);
      setDismissedJobId(null);
      await adminApi.startMetadataRefreshJob();
      toast.success('Metadata refresh job started');
      refetchMaintenanceJob();
    } catch (err) {
      const message = err instanceof Error ? err.message : 'Failed to start metadata refresh';
      toast.error(message);
    } finally {
      setIsRefreshLoading(false);
    }
  };

  const handleCancelJob = async () => {
    if (!activeJob) return;

    cancelJob(activeJob.id, {
      onSuccess: () => {
        toast.success('Job cancelled');
        refetchMaintenanceJob();
      },
      onError: (error) => {
        toast.error(error.message || 'Failed to cancel job');
      },
    });
  };

  const handleDismissJob = () => {
    if (activeJob) {
      setDismissedJobId(activeJob.id);
    }
  };

  // Show nothing while checking auth
  if (!currentUser?.isAdmin) {
    return null;
  }

  if (isLoading) {
    return <MaintenancePageSkeleton />;
  }

  return (
    <div className="space-y-6">
      {/* Header */}
      <div className="border-b pb-5">
        <nav className="mb-2 flex items-center text-sm text-muted-foreground">
          <Link to="/dashboard" className="hover:text-foreground">
            Dashboard
          </Link>
          <span className="mx-2">/</span>
          <Link to="/admin" className="hover:text-foreground">
            Admin
          </Link>
          <span className="mx-2">/</span>
          <span className="text-foreground">Maintenance</span>
        </nav>
        <h1 className="text-3xl font-bold">System Maintenance</h1>
        <p className="mt-2 text-muted-foreground">
          Administrative tools for database maintenance and data management
        </p>
      </div>

      {/* Seed Data and Cleanup in 2-column grid */}
      <div className="grid grid-cols-1 gap-6 lg:grid-cols-2">
        {/* Seed Data Section */}
        <Card>
          <CardHeader>
            <CardTitle className="flex items-center gap-2">
              <Package className="h-5 w-5" />
              Seed Data
            </CardTitle>
            <CardDescription>
              Load official platforms, storefronts, and default mappings
            </CardDescription>
          </CardHeader>
          <CardContent className="space-y-4">
            {seedResult && (
              <Alert className="border-green-200 bg-green-50 text-green-800 dark:border-green-800 dark:bg-green-950 dark:text-green-200">
                <CheckCircle className="h-4 w-4 text-green-600 dark:text-green-400" />
                <AlertTitle>Success</AlertTitle>
                <AlertDescription>
                  {seedResult.message}
                  {seedResult.totalChanges > 0 && (
                    <ul className="mt-2 list-inside list-disc text-sm">
                      <li>{seedResult.platformsAdded} platforms</li>
                      <li>{seedResult.storefrontsAdded} storefronts</li>
                      <li>{seedResult.mappingsCreated} mappings</li>
                    </ul>
                  )}
                </AlertDescription>
              </Alert>
            )}
            <p className="text-sm text-muted-foreground">
              This operation is idempotent and safe to run multiple times. Existing data will be
              preserved.
            </p>
            <Button onClick={handleLoadSeedData} disabled={isSeedLoading} className="w-full">
              {isSeedLoading ? (
                <>
                  <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                  Loading...
                </>
              ) : (
                <>
                  <Package className="mr-2 h-4 w-4" />
                  Load Seed Data
                </>
              )}
            </Button>
          </CardContent>
        </Card>

        {/* Database Cleanup Section */}
        <Card>
          <CardHeader>
            <CardTitle className="flex items-center gap-2">
              <Trash2 className="h-5 w-5" />
              Database Cleanup
            </CardTitle>
            <CardDescription>Remove orphaned data and expired records</CardDescription>
          </CardHeader>
          <CardContent className="space-y-4">
            <div className="rounded-lg border p-4">
              <div className="flex items-center justify-between">
                <div>
                  <p className="font-medium">Orphaned Files</p>
                  <p className="text-sm text-muted-foreground">
                    Remove cover art not linked to any game
                  </p>
                </div>
                <Button variant="outline" size="sm" disabled>
                  Coming Soon
                </Button>
              </div>
            </div>
            <div className="rounded-lg border p-4">
              <div className="flex items-center justify-between">
                <div>
                  <p className="font-medium">Expired Jobs</p>
                  <p className="text-sm text-muted-foreground">
                    Clean up job data older than 7 days
                  </p>
                </div>
                <Button variant="outline" size="sm" disabled>
                  Coming Soon
                </Button>
              </div>
            </div>
          </CardContent>
        </Card>
      </div>

      {/* Active Job Progress View */}
      {showJobProgress && activeJob && (
        <section className="space-y-4">
          <JobProgressCard
            job={activeJob}
            onCancel={handleCancelJob}
            isCancelling={isCancelling}
          />

          {activeJob.progress && (
            <JobItemsDetails
              jobId={activeJob.id}
              progress={activeJob.progress}
              isTerminal={activeJob.isTerminal}
            />
          )}

          {/* Actions for completed jobs */}
          {activeJob.isTerminal && (
            <div className="flex gap-3">
              <Button
                variant="outline"
                onClick={handleDismissJob}
              >
                <RotateCcw className="mr-2 h-4 w-4" />
                Start New
              </Button>
            </div>
          )}
        </section>
      )}

      {/* IGDB Refresh Section - Full width, hidden when job is in progress */}
      {!hasActiveJob && (
        <Card>
          <CardHeader>
            <CardTitle className="flex items-center gap-2">
              <RefreshCw className="h-5 w-5" />
              IGDB Data Refresh
            </CardTitle>
            <CardDescription>Update game metadata from IGDB across your collection</CardDescription>
          </CardHeader>
          <CardContent className="space-y-4">
            <p className="text-sm text-muted-foreground">
              Start a background job to refresh game modes, themes, player perspectives, and other
              metadata from IGDB for all games in your collection. This operation runs asynchronously
              and respects IGDB rate limits.
            </p>
            <Button onClick={handleStartMetadataRefresh} disabled={isRefreshLoading} className="w-full">
              {isRefreshLoading ? (
                <>
                  <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                  Starting...
                </>
              ) : (
                <>
                  <RefreshCw className="mr-2 h-4 w-4" />
                  Refresh All Game Metadata
                </>
              )}
            </Button>
          </CardContent>
        </Card>
      )}

      {/* Recent Maintenance Jobs - shows completed jobs from last 7 days */}
      {!hasActiveJob && (
        <RecentActivity
          jobTypes={[JobType.MAINTENANCE]}
          excludeJobIds={activeJob ? [activeJob.id] : []}
        />
      )}
    </div>
  );
}
