'use client';

import { useState } from 'react';
import { useJobChildren } from '@/hooks/use-jobs';
import { JobStatus, getJobStatusLabel, getJobStatusVariant } from '@/types';
import { Badge } from '@/components/ui/badge';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import { CheckCircle2, XCircle, Loader2, Clock } from 'lucide-react';

interface JobChildrenListProps {
  jobId: string;
}

export function JobChildrenList({ jobId }: JobChildrenListProps) {
  const [statusFilter, setStatusFilter] = useState<JobStatus | 'all'>('all');

  const { data: children, isLoading, isError } = useJobChildren(
    jobId,
    statusFilter !== 'all' ? { status: statusFilter } : undefined
  );

  if (isLoading) {
    return (
      <div className="flex items-center justify-center py-8">
        <Loader2 className="h-6 w-6 animate-spin text-muted-foreground" />
        <span className="ml-2 text-muted-foreground">Loading children...</span>
      </div>
    );
  }

  if (isError) {
    return (
      <div className="text-destructive py-4">
        Failed to load child jobs
      </div>
    );
  }

  const statusIcon = (status: JobStatus) => {
    switch (status) {
      case JobStatus.COMPLETED:
        return <CheckCircle2 className="h-4 w-4 text-green-500" />;
      case JobStatus.FAILED:
        return <XCircle className="h-4 w-4 text-destructive" />;
      case JobStatus.PROCESSING:
        return <Loader2 className="h-4 w-4 animate-spin text-blue-500" />;
      default:
        return <Clock className="h-4 w-4 text-muted-foreground" />;
    }
  };

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <h3 className="font-medium">Child Jobs</h3>
        <Select
          value={statusFilter}
          onValueChange={(v) => setStatusFilter(v as JobStatus | 'all')}
        >
          <SelectTrigger className="w-[150px]">
            <SelectValue placeholder="Filter by status" />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="all">All</SelectItem>
            <SelectItem value={JobStatus.COMPLETED}>Completed</SelectItem>
            <SelectItem value={JobStatus.FAILED}>Failed</SelectItem>
            <SelectItem value={JobStatus.PROCESSING}>Processing</SelectItem>
            <SelectItem value={JobStatus.PENDING}>Pending</SelectItem>
          </SelectContent>
        </Select>
      </div>

      <div className="space-y-2 max-h-96 overflow-y-auto">
        {children?.map((child) => (
          <div
            key={child.id}
            className="flex items-center justify-between p-3 rounded-lg border"
          >
            <div className="flex items-center gap-3">
              {statusIcon(child.status)}
              <span className="font-medium">
                {(child.resultSummary as { title?: string })?.title || 'Unknown'}
              </span>
            </div>
            <div className="flex items-center gap-2">
              {child.errorMessage && (
                <span className="text-sm text-destructive truncate max-w-[200px]" title={child.errorMessage}>
                  {child.errorMessage}
                </span>
              )}
              <Badge variant={getJobStatusVariant(child.status)}>
                {getJobStatusLabel(child.status)}
              </Badge>
            </div>
          </div>
        ))}

        {children?.length === 0 && (
          <div className="text-center py-4 text-muted-foreground">
            No child jobs found
          </div>
        )}
      </div>
    </div>
  );
}
