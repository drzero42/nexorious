import { useState, useEffect } from 'react';
import { createFileRoute, Link, useNavigate } from '@tanstack/react-router';
import { useAuth } from '@/providers';
import { useActiveJob, useCancelJob } from '@/hooks';
import { JobType } from '@/types';
import { Card, CardContent, CardHeader, CardTitle, CardDescription } from '@/components/ui/card';
import { Button } from '@/components/ui/button';
import { Skeleton } from '@/components/ui/skeleton';
import { JobProgressCard, JobItemsDetails, RecentActivity } from '@/components/jobs';
import { toast } from 'sonner';
import {
  RefreshCw,
  Loader2,
  RotateCcw,
} from 'lucide-react';
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from '@/components/ui/tooltip';
import { useHealthStatus } from '@/hooks/use-health-status';
import * as adminApi from '@/api/admin';

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
      <Skeleton className="h-64" />
      <Skeleton className="h-64" />
    </div>
  );
}

function MaintenancePage() {
  const navigate = useNavigate();
  const { user: currentUser } = useAuth();
  const [isRefreshLoading, setIsRefreshLoading] = useState(false);
  const [dismissedJobId, setDismissedJobId] = useState<string | null>(null);

  const { data: health } = useHealthStatus();
  const igdbUnavailable = health?.igdb_status !== undefined && health.igdb_status !== 'ok';

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
    }
  }, [currentUser, navigate]);

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

  if (!currentUser) {
    return <MaintenancePageSkeleton />;
  }

  if (!currentUser.isAdmin) {
    return null;
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
            {igdbUnavailable ? (
              <TooltipProvider>
                <Tooltip>
                  <TooltipTrigger asChild>
                    <div className="w-full">
                      <Button disabled className="w-full">
                        <RefreshCw className="mr-2 h-4 w-4" />
                        Refresh All Game Metadata
                      </Button>
                    </div>
                  </TooltipTrigger>
                  <TooltipContent>IGDB not configured</TooltipContent>
                </Tooltip>
              </TooltipProvider>
            ) : (
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
            )}
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
