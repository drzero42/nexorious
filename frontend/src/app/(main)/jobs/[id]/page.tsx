'use client';

import { use } from 'react';
import Link from 'next/link';
import { useRouter } from 'next/navigation';
import { Card, CardContent, CardFooter, CardHeader, CardTitle } from '@/components/ui/card';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Progress } from '@/components/ui/progress';
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert';
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
import { Skeleton } from '@/components/ui/skeleton';
import {
  AlertCircle,
  AlertTriangle,
  ArrowLeft,
  ClipboardList,
  Download,
  ExternalLink,
  Loader2,
  RefreshCw,
  Trash2,
  XCircle,
} from 'lucide-react';
import { toast } from 'sonner';
import { useState } from 'react';
import {
  useJob,
  useCancelJob,
  useDeleteJob,
  useDownloadExport,
} from '@/hooks';
import {
  JobStatus,
  JobType,
  getJobTypeLabel,
  getJobSourceLabel,
  getJobStatusLabel,
  getJobStatusVariant,
  formatDuration,
  canCancelJob,
  canDeleteJob,
  isJobInProgress,
} from '@/types';

interface JobDetailPageProps {
  params: Promise<{ id: string }>;
}

function JobDetailSkeleton() {
  return (
    <div className="space-y-6">
      <Skeleton className="h-6 w-32" />
      <Card>
        <CardHeader>
          <div className="flex items-start justify-between">
            <div className="space-y-2">
              <Skeleton className="h-7 w-64" />
              <Skeleton className="h-4 w-48" />
            </div>
            <Skeleton className="h-6 w-24" />
          </div>
        </CardHeader>
        <CardContent className="space-y-6">
          <Skeleton className="h-4 w-full" />
          <div className="grid grid-cols-2 gap-4 sm:grid-cols-3">
            {[1, 2, 3, 4, 5, 6].map((i) => (
              <div key={i}>
                <Skeleton className="mb-1 h-4 w-20" />
                <Skeleton className="h-5 w-32" />
              </div>
            ))}
          </div>
        </CardContent>
        <CardFooter>
          <Skeleton className="h-10 w-full" />
        </CardFooter>
      </Card>
    </div>
  );
}

function formatDate(dateStr: string | null): string {
  if (!dateStr) return '-';
  const date = new Date(dateStr);
  return date.toLocaleString();
}

export default function JobDetailPage({ params }: JobDetailPageProps) {
  const router = useRouter();
  const { id: jobId } = use(params);

  const [confirmCancel, setConfirmCancel] = useState(false);
  const [confirmDelete, setConfirmDelete] = useState(false);

  const { data: job, isLoading, error, refetch, isFetching } = useJob(jobId);

  const cancelJobMutation = useCancelJob();
  const deleteJobMutation = useDeleteJob();
  const downloadExportMutation = useDownloadExport();

  const showProgress = job?.status === JobStatus.PROCESSING;

  // Helper to check if job is a completed export
  const isCompletedExport = job?.jobType === JobType.EXPORT && job?.status === JobStatus.COMPLETED;

  // Check if import job has items pending review
  const hasPendingReview = job?.jobType === JobType.IMPORT && (job?.progress?.pendingReview ?? 0) > 0;

  const handleCancel = async () => {
    try {
      await cancelJobMutation.mutateAsync(jobId);
      toast.success('Job cancelled successfully');
      setConfirmCancel(false);
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Failed to cancel job');
    }
  };

  const handleDelete = async () => {
    try {
      await deleteJobMutation.mutateAsync(jobId);
      toast.success('Job deleted successfully');
      setConfirmDelete(false);
      router.push('/jobs');
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Failed to delete job');
    }
  };

  const handleDownload = async () => {
    if (!job) return;
    try {
      await downloadExportMutation.mutateAsync(job.id);
      toast.success('Download started');
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Failed to download export');
    }
  };

  if (isLoading) {
    return <JobDetailSkeleton />;
  }

  if (error) {
    return (
      <div className="space-y-6">
        <Link
          href="/jobs"
          className="inline-flex items-center text-sm text-muted-foreground hover:text-foreground"
        >
          <ArrowLeft className="mr-2 h-4 w-4" />
          Back to Jobs
        </Link>
        <Alert variant="destructive">
          <AlertCircle className="h-4 w-4" />
          <AlertTitle>Error loading job</AlertTitle>
          <AlertDescription>
            {error instanceof Error ? error.message : 'An unexpected error occurred'}
          </AlertDescription>
        </Alert>
      </div>
    );
  }

  if (!job) {
    return (
      <div className="space-y-6">
        <Link
          href="/jobs"
          className="inline-flex items-center text-sm text-muted-foreground hover:text-foreground"
        >
          <ArrowLeft className="mr-2 h-4 w-4" />
          Back to Jobs
        </Link>
        <div className="py-12 text-center">
          <ClipboardList className="mx-auto h-12 w-12 text-muted-foreground" />
          <h3 className="mt-4 text-lg font-medium">Job not found</h3>
          <p className="mt-2 text-sm text-muted-foreground">
            The job you&apos;re looking for doesn&apos;t exist or has been deleted.
          </p>
          <Button asChild className="mt-6">
            <Link href="/jobs">Back to Jobs</Link>
          </Button>
        </div>
      </div>
    );
  }

  return (
    <div className="space-y-6">
      {/* Back link */}
      <Link
        href="/jobs"
        className="inline-flex items-center text-sm text-muted-foreground hover:text-foreground"
      >
        <ArrowLeft className="mr-2 h-4 w-4" />
        Back to Jobs
      </Link>

      {/* Job Card */}
      <Card>
        <CardHeader>
          <div className="flex items-start justify-between">
            <div>
              <CardTitle className="text-2xl">
                {getJobTypeLabel(job.jobType)} - {getJobSourceLabel(job.source)}
              </CardTitle>
              <p className="mt-1 font-mono text-sm text-muted-foreground">Job ID: {job.id}</p>
            </div>
            <div className="flex items-center gap-2">
              <Badge variant={getJobStatusVariant(job.status)}>
                {isJobInProgress(job) && <Loader2 className="mr-1 h-3 w-3 animate-spin" />}
                {getJobStatusLabel(job.status)}
              </Badge>
              <Button
                variant="ghost"
                size="icon"
                onClick={() => refetch()}
                disabled={isFetching}
              >
                {isFetching ? (
                  <Loader2 className="h-4 w-4 animate-spin" />
                ) : (
                  <RefreshCw className="h-4 w-4" />
                )}
              </Button>
            </div>
          </div>
        </CardHeader>

        <CardContent className="space-y-6">
          {/* Progress */}
          {showProgress && job.progress && (
            <div>
              <div className="mb-2 flex justify-between text-sm text-muted-foreground">
                <span>Progress</span>
                <span>
                  {job.progress.completed + job.progress.pendingReview + job.progress.skipped + job.progress.failed} / {job.progress.total} ({job.progress.percent}%)
                </span>
              </div>
              <Progress value={job.progress.percent} />
            </div>
          )}

          {/* Error message */}
          {job.errorMessage && (
            <Alert variant="destructive">
              <AlertTriangle className="h-4 w-4" />
              <AlertTitle>Error</AlertTitle>
              <AlertDescription>{job.errorMessage}</AlertDescription>
            </Alert>
          )}

          {/* Job Details */}
          <div className="grid grid-cols-2 gap-4 text-sm sm:grid-cols-3">
            <div>
              <dt className="font-medium text-muted-foreground">Created</dt>
              <dd className="mt-1">{formatDate(job.createdAt)}</dd>
            </div>
            <div>
              <dt className="font-medium text-muted-foreground">Started</dt>
              <dd className="mt-1">{formatDate(job.startedAt)}</dd>
            </div>
            <div>
              <dt className="font-medium text-muted-foreground">Completed</dt>
              <dd className="mt-1">{formatDate(job.completedAt)}</dd>
            </div>
            <div>
              <dt className="font-medium text-muted-foreground">Duration</dt>
              <dd className="mt-1">{formatDuration(job.durationSeconds)}</dd>
            </div>
            <div>
              <dt className="font-medium text-muted-foreground">Priority</dt>
              <dd className="mt-1 capitalize">{job.priority}</dd>
            </div>
            <div>
              <dt className="font-medium text-muted-foreground">Total Items</dt>
              <dd className="mt-1">{job.totalItems}</dd>
            </div>
          </div>

          {/* Review Items Section */}
          {hasPendingReview && (
            <div className="rounded-lg border p-4">
              <div className="flex items-center justify-between">
                <div>
                  <h3 className="font-medium">Review Items</h3>
                  <p className="mt-1 text-sm text-muted-foreground">
                    {job.progress.pendingReview} pending out of {job.progress.total} total
                  </p>
                </div>
                <Button variant="outline" asChild>
                  <Link href={`/review?job_id=${job.id}`}>
                    <ExternalLink className="mr-2 h-4 w-4" />
                    View Review Queue
                  </Link>
                </Button>
              </div>
            </div>
          )}

          {/* Progress Summary */}
          {job.progress && job.progress.total > 0 && (
            <div>
              <h3 className="mb-3 font-medium">Progress Summary</h3>
              <div className="grid grid-cols-2 gap-4 text-sm sm:grid-cols-3">
                <div>
                  <dt className="text-xs font-medium text-muted-foreground">Completed</dt>
                  <dd className="mt-1">{job.progress.completed}</dd>
                </div>
                <div>
                  <dt className="text-xs font-medium text-muted-foreground">Pending Review</dt>
                  <dd className="mt-1">{job.progress.pendingReview}</dd>
                </div>
                <div>
                  <dt className="text-xs font-medium text-muted-foreground">Skipped</dt>
                  <dd className="mt-1">{job.progress.skipped}</dd>
                </div>
                <div>
                  <dt className="text-xs font-medium text-muted-foreground">Failed</dt>
                  <dd className="mt-1">{job.progress.failed}</dd>
                </div>
                <div>
                  <dt className="text-xs font-medium text-muted-foreground">Processing</dt>
                  <dd className="mt-1">{job.progress.processing}</dd>
                </div>
                <div>
                  <dt className="text-xs font-medium text-muted-foreground">Pending</dt>
                  <dd className="mt-1">{job.progress.pending}</dd>
                </div>
              </div>
            </div>
          )}
        </CardContent>

        {/* Actions */}
        <CardFooter className="gap-3 border-t pt-6">
          <div className="ml-auto flex gap-3">
            {isCompletedExport && (
              <Button
                onClick={handleDownload}
                disabled={downloadExportMutation.isPending}
                className="bg-green-600 hover:bg-green-700"
              >
                {downloadExportMutation.isPending ? (
                  <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                ) : (
                  <Download className="mr-2 h-4 w-4" />
                )}
                Download Export
              </Button>
            )}
            {canCancelJob(job) && (
              <Button
                variant="outline"
                onClick={() => setConfirmCancel(true)}
                className="text-amber-600 hover:bg-amber-50 hover:text-amber-700 dark:text-amber-400 dark:hover:bg-amber-950 dark:hover:text-amber-300"
              >
                <XCircle className="mr-2 h-4 w-4" />
                Cancel Job
              </Button>
            )}
            {canDeleteJob(job) && (
              <Button
                variant="outline"
                onClick={() => setConfirmDelete(true)}
                className="text-destructive hover:bg-destructive/10"
              >
                <Trash2 className="mr-2 h-4 w-4" />
                Delete Job
              </Button>
            )}
          </div>
        </CardFooter>
      </Card>

      {/* Cancel Confirmation Dialog */}
      <AlertDialog open={confirmCancel} onOpenChange={setConfirmCancel}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Cancel Job</AlertDialogTitle>
            <AlertDialogDescription>
              Are you sure you want to cancel this job? This action cannot be undone.
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>Close</AlertDialogCancel>
            <AlertDialogAction
              onClick={handleCancel}
              disabled={cancelJobMutation.isPending}
              className="bg-amber-600 hover:bg-amber-700"
            >
              {cancelJobMutation.isPending && (
                <Loader2 className="mr-2 h-4 w-4 animate-spin" />
              )}
              Cancel Job
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>

      {/* Delete Confirmation Dialog */}
      <AlertDialog open={confirmDelete} onOpenChange={setConfirmDelete}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Delete Job</AlertDialogTitle>
            <AlertDialogDescription>
              Are you sure you want to delete this job? This will also delete all associated
              job items. This action cannot be undone.
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>Cancel</AlertDialogCancel>
            <AlertDialogAction
              onClick={handleDelete}
              disabled={deleteJobMutation.isPending}
              className="bg-destructive hover:bg-destructive/90"
            >
              {deleteJobMutation.isPending && (
                <Loader2 className="mr-2 h-4 w-4 animate-spin" />
              )}
              Delete
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  );
}
