'use client';

import { useState } from 'react';
import { Card, CardContent } from '@/components/ui/card';
import { Button } from '@/components/ui/button';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import { Label } from '@/components/ui/label';
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
  Pagination,
  PaginationContent,
  PaginationItem,
  PaginationLink,
  PaginationNext,
  PaginationPrevious,
} from '@/components/ui/pagination';
import { AlertCircle, ClipboardList, Loader2, RefreshCw, X } from 'lucide-react';
import { toast } from 'sonner';
import { JobCard } from '@/components/jobs';
import { useJobs, useCancelJob, useDeleteJob } from '@/hooks';
import type { Job, JobFilters } from '@/types';
import {
  JobType,
  JobSource,
  JobStatus,
  getJobTypeLabel,
  getJobSourceLabel,
  getJobStatusLabel,
} from '@/types';

const ITEMS_PER_PAGE = 10;

function JobsPageSkeleton() {
  return (
    <div className="space-y-6">
      <div>
        <Skeleton className="mb-2 h-8 w-48" />
        <Skeleton className="h-4 w-64" />
      </div>
      <Card>
        <CardContent className="grid gap-4 p-4 sm:grid-cols-3">
          <Skeleton className="h-10" />
          <Skeleton className="h-10" />
          <Skeleton className="h-10" />
        </CardContent>
      </Card>
      <div className="space-y-4">
        {[1, 2, 3].map((i) => (
          <Skeleton key={i} className="h-40" />
        ))}
      </div>
    </div>
  );
}

export default function JobsPage() {
  const [page, setPage] = useState(1);
  const [filters, setFilters] = useState<JobFilters>({});
  const [confirmCancelJob, setConfirmCancelJob] = useState<Job | null>(null);
  const [confirmDeleteJob, setConfirmDeleteJob] = useState<Job | null>(null);

  const { data, isLoading, error, refetch, isFetching } = useJobs(
    filters,
    page,
    ITEMS_PER_PAGE
  );

  const cancelJobMutation = useCancelJob();
  const deleteJobMutation = useDeleteJob();

  const hasFilters =
    filters.jobType !== undefined ||
    filters.source !== undefined ||
    filters.status !== undefined;

  const handleFilterChange = (key: keyof JobFilters, value: string | undefined) => {
    setFilters((prev) => ({
      ...prev,
      [key]: value === 'all' ? undefined : value,
    }));
    setPage(1);
  };

  const clearFilters = () => {
    setFilters({});
    setPage(1);
  };

  const handleCancel = async () => {
    if (!confirmCancelJob) return;
    try {
      await cancelJobMutation.mutateAsync(confirmCancelJob.id);
      toast.success('Job cancelled successfully');
      setConfirmCancelJob(null);
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Failed to cancel job');
    }
  };

  const handleDelete = async () => {
    if (!confirmDeleteJob) return;
    try {
      await deleteJobMutation.mutateAsync(confirmDeleteJob.id);
      toast.success('Job deleted successfully');
      setConfirmDeleteJob(null);
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Failed to delete job');
    }
  };

  if (isLoading) {
    return <JobsPageSkeleton />;
  }

  return (
    <div className="space-y-6">
      {/* Header */}
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold">Background Jobs</h1>
          <p className="text-muted-foreground">
            View and manage your sync, import, and export tasks
          </p>
        </div>
        <Button variant="outline" size="sm" onClick={() => refetch()} disabled={isFetching}>
          {isFetching ? (
            <Loader2 className="mr-2 h-4 w-4 animate-spin" />
          ) : (
            <RefreshCw className="mr-2 h-4 w-4" />
          )}
          Refresh
        </Button>
      </div>

      {/* Filters */}
      <Card>
        <CardContent className="p-4">
          <div className="grid gap-4 sm:grid-cols-3">
            {/* Job Type Filter */}
            <div className="space-y-2">
              <Label htmlFor="job-type">Type</Label>
              <Select
                value={filters.jobType || 'all'}
                onValueChange={(value) => handleFilterChange('jobType', value)}
              >
                <SelectTrigger id="job-type">
                  <SelectValue placeholder="All Types" />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="all">All Types</SelectItem>
                  {Object.values(JobType).map((type) => (
                    <SelectItem key={type} value={type}>
                      {getJobTypeLabel(type)}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>

            {/* Source Filter */}
            <div className="space-y-2">
              <Label htmlFor="source">Source</Label>
              <Select
                value={filters.source || 'all'}
                onValueChange={(value) => handleFilterChange('source', value)}
              >
                <SelectTrigger id="source">
                  <SelectValue placeholder="All Sources" />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="all">All Sources</SelectItem>
                  {Object.values(JobSource).map((source) => (
                    <SelectItem key={source} value={source}>
                      {getJobSourceLabel(source)}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>

            {/* Status Filter */}
            <div className="space-y-2">
              <Label htmlFor="status">Status</Label>
              <Select
                value={filters.status || 'all'}
                onValueChange={(value) => handleFilterChange('status', value)}
              >
                <SelectTrigger id="status">
                  <SelectValue placeholder="All Statuses" />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="all">All Statuses</SelectItem>
                  {Object.values(JobStatus).map((status) => (
                    <SelectItem key={status} value={status}>
                      {getJobStatusLabel(status)}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
          </div>

          {hasFilters && (
            <div className="mt-4 flex justify-end">
              <Button variant="ghost" size="sm" onClick={clearFilters}>
                <X className="mr-2 h-4 w-4" />
                Clear filters
              </Button>
            </div>
          )}
        </CardContent>
      </Card>

      {/* Error State */}
      {error && (
        <Alert variant="destructive">
          <AlertCircle className="h-4 w-4" />
          <AlertTitle>Error loading jobs</AlertTitle>
          <AlertDescription>
            {error instanceof Error ? error.message : 'An unexpected error occurred'}
          </AlertDescription>
        </Alert>
      )}

      {/* Jobs List */}
      {data?.jobs.length === 0 ? (
        <div className="py-12 text-center">
          <ClipboardList className="mx-auto h-12 w-12 text-muted-foreground" />
          <h3 className="mt-4 text-lg font-medium">No jobs found</h3>
          <p className="mt-2 text-sm text-muted-foreground">
            {hasFilters
              ? 'Try adjusting your filters.'
              : 'Jobs will appear here when you sync, import, or export data.'}
          </p>
        </div>
      ) : (
        <div className="space-y-4">
          {data?.jobs.map((job) => (
            <JobCard
              key={job.id}
              job={job}
              onCancel={(j) => setConfirmCancelJob(j)}
              onDelete={(j) => setConfirmDeleteJob(j)}
              isCancelling={cancelJobMutation.isPending && confirmCancelJob?.id === job.id}
              isDeleting={deleteJobMutation.isPending && confirmDeleteJob?.id === job.id}
            />
          ))}
        </div>
      )}

      {/* Pagination */}
      {data && data.pages > 1 && (
        <Pagination>
          <PaginationContent>
            <PaginationItem>
              <PaginationPrevious
                onClick={() => setPage((p) => Math.max(1, p - 1))}
                aria-disabled={page <= 1}
                className={page <= 1 ? 'pointer-events-none opacity-50' : 'cursor-pointer'}
              />
            </PaginationItem>
            {Array.from({ length: Math.min(5, data.pages) }, (_, i) => {
              // Show pages around current page
              let pageNum: number;
              if (data.pages <= 5) {
                pageNum = i + 1;
              } else if (page <= 3) {
                pageNum = i + 1;
              } else if (page >= data.pages - 2) {
                pageNum = data.pages - 4 + i;
              } else {
                pageNum = page - 2 + i;
              }
              return (
                <PaginationItem key={pageNum}>
                  <PaginationLink
                    onClick={() => setPage(pageNum)}
                    isActive={page === pageNum}
                    className="cursor-pointer"
                  >
                    {pageNum}
                  </PaginationLink>
                </PaginationItem>
              );
            })}
            <PaginationItem>
              <PaginationNext
                onClick={() => setPage((p) => Math.min(data.pages, p + 1))}
                aria-disabled={page >= data.pages}
                className={
                  page >= data.pages ? 'pointer-events-none opacity-50' : 'cursor-pointer'
                }
              />
            </PaginationItem>
          </PaginationContent>
        </Pagination>
      )}

      {/* Cancel Confirmation Dialog */}
      <AlertDialog
        open={!!confirmCancelJob}
        onOpenChange={(open) => !open && setConfirmCancelJob(null)}
      >
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
      <AlertDialog
        open={!!confirmDeleteJob}
        onOpenChange={(open) => !open && setConfirmDeleteJob(null)}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Delete Job</AlertDialogTitle>
            <AlertDialogDescription>
              Are you sure you want to delete this job? This will also delete all associated
              review items. This action cannot be undone.
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
