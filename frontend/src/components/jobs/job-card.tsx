'use client';

import Link from 'next/link';
import { Card, CardContent, CardFooter, CardHeader, CardTitle } from '@/components/ui/card';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Progress } from '@/components/ui/progress';
import {
  AlertTriangle,
  Clock,
  ExternalLink,
  FileText,
  Loader2,
  Trash2,
  XCircle,
} from 'lucide-react';
import type { Job } from '@/types';
import {
  JobStatus,
  getJobTypeLabel,
  getJobSourceLabel,
  getJobStatusLabel,
  getJobStatusVariant,
  formatDuration,
  formatRelativeTime,
  canCancelJob,
  canDeleteJob,
  isJobInProgress,
} from '@/types';

interface JobCardProps {
  job: Job;
  compact?: boolean;
  onView?: (job: Job) => void;
  onCancel?: (job: Job) => void;
  onDelete?: (job: Job) => void;
  isCancelling?: boolean;
  isDeleting?: boolean;
}

export function JobCard({
  job,
  compact = false,
  onView,
  onCancel,
  onDelete,
  isCancelling,
  isDeleting,
}: JobCardProps) {
  const showProgress = job.status === JobStatus.PROCESSING;

  if (compact) {
    return (
      <Link href={`/jobs/${job.id}`} className="block">
        <Card className="transition-colors hover:border-primary/50">
          <CardContent className="flex items-center justify-between p-4">
            <div className="flex min-w-0 items-center gap-4">
              <Badge variant={getJobStatusVariant(job.status)}>
                {getJobStatusLabel(job.status)}
              </Badge>
              <div className="min-w-0">
                <div className="truncate text-sm font-medium">
                  {getJobTypeLabel(job.jobType)} - {getJobSourceLabel(job.source)}
                </div>
                <div className="text-xs text-muted-foreground">
                  {formatRelativeTime(job.createdAt)}
                </div>
              </div>
            </div>
            {showProgress && (
              <div className="w-24 flex-shrink-0">
                <Progress value={job.progress.percent} className="h-2" />
              </div>
            )}
          </CardContent>
        </Card>
      </Link>
    );
  }

  return (
    <Card>
      <CardHeader className="pb-3">
        <div className="flex items-start justify-between">
          <div>
            <CardTitle className="text-lg">
              {getJobTypeLabel(job.jobType)} - {getJobSourceLabel(job.source)}
            </CardTitle>
            <p className="mt-1 text-sm text-muted-foreground">
              Started {formatRelativeTime(job.startedAt || job.createdAt)}
            </p>
          </div>
          <Badge variant={getJobStatusVariant(job.status)}>
            {isJobInProgress(job) && <Loader2 className="mr-1 h-3 w-3 animate-spin" />}
            {getJobStatusLabel(job.status)}
          </Badge>
        </div>
      </CardHeader>

      <CardContent className="space-y-4">
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

        {/* Stats */}
        <div className="grid grid-cols-2 gap-4 text-sm">
          <div className="flex items-center gap-2">
            <Clock className="h-4 w-4 text-muted-foreground" />
            <span className="text-muted-foreground">Duration:</span>
            <span className="font-medium">{formatDuration(job.durationSeconds)}</span>
          </div>
          {job.progress && job.progress.pendingReview > 0 && (
            <div className="flex items-center gap-2">
              <FileText className="h-4 w-4 text-muted-foreground" />
              <span className="text-muted-foreground">Pending Review:</span>
              <span className="font-medium">{job.progress.pendingReview}</span>
            </div>
          )}
        </div>

        {/* Error message */}
        {job.errorMessage && (
          <div className="flex items-start gap-2 rounded-md bg-destructive/10 p-3 text-sm text-destructive">
            <AlertTriangle className="mt-0.5 h-4 w-4 flex-shrink-0" />
            <span>{job.errorMessage}</span>
          </div>
        )}
      </CardContent>

      <CardFooter className="gap-2 border-t pt-4">
        <Button
          variant="outline"
          size="sm"
          asChild
          className="ml-auto"
          onClick={() => onView?.(job)}
        >
          <Link href={`/jobs/${job.id}`}>
            <ExternalLink className="mr-2 h-4 w-4" />
            View Details
          </Link>
        </Button>
        {canCancelJob(job) && onCancel && (
          <Button
            variant="outline"
            size="sm"
            onClick={() => onCancel(job)}
            disabled={isCancelling}
            className="text-amber-600 hover:bg-amber-50 hover:text-amber-700 dark:text-amber-400 dark:hover:bg-amber-950 dark:hover:text-amber-300"
          >
            {isCancelling ? (
              <Loader2 className="mr-2 h-4 w-4 animate-spin" />
            ) : (
              <XCircle className="mr-2 h-4 w-4" />
            )}
            Cancel
          </Button>
        )}
        {canDeleteJob(job) && onDelete && (
          <Button
            variant="outline"
            size="sm"
            onClick={() => onDelete(job)}
            disabled={isDeleting}
            className="text-destructive hover:bg-destructive/10"
          >
            {isDeleting ? (
              <Loader2 className="mr-2 h-4 w-4 animate-spin" />
            ) : (
              <Trash2 className="mr-2 h-4 w-4" />
            )}
            Delete
          </Button>
        )}
      </CardFooter>
    </Card>
  );
}
