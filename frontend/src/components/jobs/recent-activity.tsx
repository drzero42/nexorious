'use client';

import { useState, useMemo } from 'react';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from '@/components/ui/collapsible';
import { useJobs } from '@/hooks';
import { JobItemsDetails } from './job-items-details';
import type { Job, JobType as JobTypeEnum } from '@/types';
import {
  JobType,
  getJobTypeLabel,
  getJobSourceLabel,
  getJobStatusLabel,
  getJobStatusVariant,
  formatRelativeTime,
  formatDuration,
} from '@/types';
import {
  ChevronDown,
  ChevronRight,
  Clock,
  History,
  Inbox,
} from 'lucide-react';

interface RecentActivityProps {
  /** Job types to show (defaults to import and export) */
  jobTypes?: JobTypeEnum[];
  /** Job IDs to exclude (e.g., currently displayed job) */
  excludeJobIds?: string[];
  /** Number of days to look back (defaults to 7) */
  daysBack?: number;
}

/**
 * Recent Activity section showing completed jobs from the last N days.
 * Jobs are shown in a collapsible list with expandable details.
 */
export function RecentActivity({
  jobTypes = [JobType.IMPORT, JobType.EXPORT],
  excludeJobIds = [],
  daysBack = 7,
}: RecentActivityProps) {
  const [isOpen, setIsOpen] = useState(true);
  const [expandedJobId, setExpandedJobId] = useState<string | null>(null);

  // Fetch recent jobs - we'll filter by date on the client side
  // since the backend doesn't support date filtering
  const { data: jobsData, isLoading } = useJobs(undefined, 1, 50);

  // Filter to jobs within the date range and matching types
  const jobs = jobsData?.jobs;
  const recentJobs = useMemo(() => {
    if (!jobs) return [];

    const cutoffDate = new Date();
    cutoffDate.setDate(cutoffDate.getDate() - daysBack);

    return jobs.filter((job) => {
      // Must be one of the requested types
      if (!jobTypes.includes(job.jobType)) return false;

      // Must not be in the exclude list
      if (excludeJobIds.includes(job.id)) return false;

      // Must be within the date range
      const jobDate = new Date(job.createdAt);
      if (jobDate < cutoffDate) return false;

      // Must be terminal (completed, failed, cancelled)
      if (!job.isTerminal) return false;

      return true;
    });
  }, [jobs, jobTypes, excludeJobIds, daysBack]);

  const toggleJobExpanded = (jobId: string) => {
    setExpandedJobId((current) => (current === jobId ? null : jobId));
  };

  // Don't render anything while loading
  if (isLoading) {
    return null;
  }

  return (
    <Collapsible open={isOpen} onOpenChange={setIsOpen}>
      <Card>
        <CollapsibleTrigger asChild>
          <CardHeader className="cursor-pointer hover:bg-muted/50 transition-colors">
            <div className="flex items-center justify-between">
              <div className="flex items-center gap-2">
                <History className="h-5 w-5 text-muted-foreground" />
                <CardTitle className="text-lg">Recent Activity</CardTitle>
                {recentJobs.length > 0 && (
                  <Badge variant="secondary" className="ml-2">
                    {recentJobs.length}
                  </Badge>
                )}
              </div>
              <Button variant="ghost" size="sm" className="h-8 w-8 p-0">
                {isOpen ? (
                  <ChevronDown className="h-4 w-4" />
                ) : (
                  <ChevronRight className="h-4 w-4" />
                )}
              </Button>
            </div>
          </CardHeader>
        </CollapsibleTrigger>

        <CollapsibleContent>
          <CardContent className="pt-0">
            {recentJobs.length === 0 ? (
              <EmptyState daysBack={daysBack} />
            ) : (
              <div className="space-y-2">
                {recentJobs.map((job) => (
                  <JobActivityItem
                    key={job.id}
                    job={job}
                    isExpanded={expandedJobId === job.id}
                    onToggle={() => toggleJobExpanded(job.id)}
                  />
                ))}
              </div>
            )}
          </CardContent>
        </CollapsibleContent>
      </Card>
    </Collapsible>
  );
}

interface EmptyStateProps {
  daysBack: number;
}

function EmptyState({ daysBack }: EmptyStateProps) {
  return (
    <div className="flex flex-col items-center justify-center py-8 text-center">
      <div className="rounded-full bg-muted p-3 mb-4">
        <Inbox className="h-6 w-6 text-muted-foreground" />
      </div>
      <h3 className="font-medium text-muted-foreground mb-1">No recent activity</h3>
      <p className="text-sm text-muted-foreground">
        Completed imports and exports from the last {daysBack} days will appear here.
      </p>
    </div>
  );
}

interface JobActivityItemProps {
  job: Job;
  isExpanded: boolean;
  onToggle: () => void;
}

function JobActivityItem({ job, isExpanded, onToggle }: JobActivityItemProps) {
  return (
    <Collapsible open={isExpanded} onOpenChange={onToggle}>
      <div className="rounded-lg border">
        <CollapsibleTrigger asChild>
          <button
            className="w-full p-3 flex items-center justify-between hover:bg-muted/50 transition-colors text-left"
            type="button"
          >
            <div className="flex items-center gap-3 min-w-0 flex-1">
              <Badge variant={getJobStatusVariant(job.status)} className="shrink-0">
                {getJobStatusLabel(job.status)}
              </Badge>
              <span className="font-medium truncate">
                {getJobTypeLabel(job.jobType)} - {getJobSourceLabel(job.source)}
              </span>
            </div>

            <div className="flex items-center gap-4 text-sm text-muted-foreground shrink-0 ml-4">
              {/* Summary counts */}
              {job.progress && (
                <div className="hidden sm:flex items-center gap-2">
                  <span className="text-green-600">{job.progress.completed} completed</span>
                  {job.progress.failed > 0 && (
                    <span className="text-red-600">{job.progress.failed} failed</span>
                  )}
                </div>
              )}

              {/* Duration */}
              {job.durationSeconds !== null && (
                <div className="hidden md:flex items-center gap-1">
                  <Clock className="h-3 w-3" />
                  <span>{formatDuration(job.durationSeconds)}</span>
                </div>
              )}

              {/* Time ago */}
              <span className="text-xs">{formatRelativeTime(job.completedAt || job.createdAt)}</span>

              {/* Expand icon */}
              {isExpanded ? (
                <ChevronDown className="h-4 w-4" />
              ) : (
                <ChevronRight className="h-4 w-4" />
              )}
            </div>
          </button>
        </CollapsibleTrigger>

        <CollapsibleContent>
          <div className="border-t p-3 bg-muted/30">
            {/* Summary stats for mobile */}
            {job.progress && (
              <div className="sm:hidden mb-3 flex items-center gap-3 text-sm">
                <span className="text-green-600">{job.progress.completed} completed</span>
                {job.progress.failed > 0 && (
                  <span className="text-red-600">{job.progress.failed} failed</span>
                )}
                {job.durationSeconds !== null && (
                  <div className="flex items-center gap-1 text-muted-foreground">
                    <Clock className="h-3 w-3" />
                    <span>{formatDuration(job.durationSeconds)}</span>
                  </div>
                )}
              </div>
            )}

            {/* Error message if present */}
            {job.errorMessage && (
              <div className="rounded-md bg-destructive/10 p-3 text-sm text-destructive mb-3">
                {job.errorMessage}
              </div>
            )}

            {/* Job items details */}
            {job.progress && (
              <JobItemsDetails jobId={job.id} progress={job.progress} isTerminal={job.isTerminal} />
            )}
          </div>
        </CollapsibleContent>
      </div>
    </Collapsible>
  );
}
