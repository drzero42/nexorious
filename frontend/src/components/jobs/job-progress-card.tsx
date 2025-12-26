'use client';

import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { Button } from '@/components/ui/button';
import { Badge } from '@/components/ui/badge';
import { Progress } from '@/components/ui/progress';
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
import { Loader2, XCircle } from 'lucide-react';
import { useState } from 'react';
import type { Job } from '@/types';
import {
  getJobTypeLabel,
  getJobSourceLabel,
  getJobStatusLabel,
  getJobStatusVariant,
  isJobInProgress,
} from '@/types';

interface JobProgressCardProps {
  job: Job;
  onCancel: () => Promise<void>;
  isCancelling?: boolean;
}

export function JobProgressCard({ job, onCancel, isCancelling }: JobProgressCardProps) {
  const [confirmCancel, setConfirmCancel] = useState(false);
  const showProgress = isJobInProgress(job);

  const handleCancel = async () => {
    await onCancel();
    setConfirmCancel(false);
  };

  return (
    <>
      <Card>
        <CardHeader className="pb-2">
          <div className="flex items-center justify-between">
            <CardTitle className="text-lg">
              {getJobTypeLabel(job.jobType)} - {getJobSourceLabel(job.source)}
            </CardTitle>
            <Badge variant={getJobStatusVariant(job.status)}>
              {isJobInProgress(job) && <Loader2 className="mr-1 h-3 w-3 animate-spin" />}
              {getJobStatusLabel(job.status)}
            </Badge>
          </div>
        </CardHeader>
        <CardContent className="space-y-4">
          {showProgress && job.progress && (
            <div>
              <div className="mb-2 flex justify-between text-sm text-muted-foreground">
                <span>Progress</span>
                <span>
                  {job.progress.completed + job.progress.failed + job.progress.skipped} /{' '}
                  {job.progress.total} ({job.progress.percent}%)
                </span>
              </div>
              <Progress value={job.progress.percent} />
            </div>
          )}

          {job.progress && (
            <div className="grid grid-cols-2 gap-4 text-sm sm:grid-cols-4">
              <div>
                <div className="text-muted-foreground">Completed</div>
                <div className="text-lg font-semibold text-green-600">{job.progress.completed}</div>
              </div>
              <div>
                <div className="text-muted-foreground">Failed</div>
                <div className="text-lg font-semibold text-red-600">{job.progress.failed}</div>
              </div>
              <div>
                <div className="text-muted-foreground">Processing</div>
                <div className="text-lg font-semibold">{job.progress.processing}</div>
              </div>
              <div>
                <div className="text-muted-foreground">Pending</div>
                <div className="text-lg font-semibold">{job.progress.pending}</div>
              </div>
            </div>
          )}

          {job.errorMessage && (
            <div className="rounded-md bg-destructive/10 p-3 text-sm text-destructive">
              {job.errorMessage}
            </div>
          )}

          {!job.isTerminal && (
            <div className="flex justify-end">
              <Button
                variant="outline"
                size="sm"
                onClick={() => setConfirmCancel(true)}
                disabled={isCancelling}
                className="text-amber-600 hover:bg-amber-50 hover:text-amber-700"
              >
                {isCancelling ? (
                  <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                ) : (
                  <XCircle className="mr-2 h-4 w-4" />
                )}
                Cancel
              </Button>
            </div>
          )}
        </CardContent>
      </Card>

      <AlertDialog open={confirmCancel} onOpenChange={setConfirmCancel}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Cancel Job</AlertDialogTitle>
            <AlertDialogDescription>
              Are you sure you want to cancel this job? This will stop processing and remove the
              job.
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>Keep Running</AlertDialogCancel>
            <AlertDialogAction
              onClick={handleCancel}
              disabled={isCancelling}
              className="bg-amber-600 hover:bg-amber-700"
            >
              {isCancelling && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
              Cancel Job
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </>
  );
}
