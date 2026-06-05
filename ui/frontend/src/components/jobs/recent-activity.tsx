import { useState } from 'react';
import type { ReactNode } from 'react';
import { toast } from 'sonner';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from '@/components/ui/collapsible';
import { useRecentJobs, useDownloadExport } from '@/hooks';
import { JobItemsDetails } from './job-items-details';
import type { RecentJobDetail, SyncChangeItem, JobType as JobTypeEnum } from '@/types';
import {
  JobStatus,
  JobType,
  getJobTypeLabel,
  getJobSourceLabel,
  getJobStatusLabel,
  getJobStatusVariant,
  formatRelativeTime,
} from '@/types';
import {
  ChevronDown,
  ChevronRight,
  History,
  Inbox,
  CheckCircle,
  XCircle,
  ArrowRight,
  SkipForward,
  BookMarked,
  RefreshCw,
  Download,
  Loader2,
} from 'lucide-react';

interface RecentActivityProps {
  /** Sync: the storefront to filter by. */
  source?: string;
  /** Import/Export, Maintenance: job types to include. */
  jobTypes?: JobTypeEnum[];
  /** Job IDs to hide (e.g. the currently-displayed job). */
  excludeJobIds?: string[];
  /** Look-back window in days (default 7). */
  daysBack?: number;
  /** Max jobs (default 5). */
  limit?: number;
}

function hasChangeRows(job: RecentJobDetail): boolean {
  return (
    job.addedItems.length > 0 ||
    job.updatedItems.length > 0 ||
    job.removedItems.length > 0 ||
    job.statusChangedItems.length > 0 ||
    job.skippedItems.length > 0 ||
    job.alreadyInLibraryItems.length > 0
  );
}

function ChangeList({
  items,
  label,
  icon,
}: {
  items: SyncChangeItem[];
  label: string;
  icon: ReactNode;
}) {
  const [isOpen, setIsOpen] = useState(false);
  if (items.length === 0) return null;
  return (
    <Collapsible open={isOpen} onOpenChange={setIsOpen}>
      <CollapsibleTrigger asChild>
        <Button variant="ghost" size="sm" className="w-full justify-between h-8 px-2">
          <div className="flex items-center gap-2">
            {isOpen ? <ChevronDown className="h-3 w-3" /> : <ChevronRight className="h-3 w-3" />}
            {icon}
            <span className="text-sm">{label}</span>
          </div>
          <Badge variant="secondary" className="h-5 text-xs">
            {items.length}
          </Badge>
        </Button>
      </CollapsibleTrigger>
      <CollapsibleContent>
        <div className="ml-6 pl-2 border-l space-y-1 py-1">
          {items.map((item, idx) => (
            <div key={idx} className="text-sm py-1 text-muted-foreground">
              {item.title}
              {item.oldStatus && item.newStatus && (
                <span className="ml-2 text-xs">
                  {item.oldStatus} → {item.newStatus}
                </span>
              )}
            </div>
          ))}
        </div>
      </CollapsibleContent>
    </Collapsible>
  );
}

function ChangeBreakdown({ job }: { job: RecentJobDetail }) {
  return (
    <div className="space-y-1">
      <ChangeList
        items={job.addedItems}
        label="Added to library"
        icon={<CheckCircle className="h-4 w-4 text-green-600" />}
      />
      <ChangeList
        items={job.updatedItems}
        label="Updated"
        icon={<RefreshCw className="h-4 w-4 text-blue-500" />}
      />
      <ChangeList
        items={job.removedItems}
        label="Removed from storefront"
        icon={<XCircle className="h-4 w-4 text-muted-foreground" />}
      />
      <ChangeList
        items={job.statusChangedItems}
        label="Status changed"
        icon={<ArrowRight className="h-4 w-4 text-blue-500" />}
      />
      <ChangeList
        items={job.alreadyInLibraryItems}
        label="Already in library"
        icon={<BookMarked className="h-4 w-4 text-muted-foreground" />}
      />
      <ChangeList
        items={job.skippedItems}
        label="Skipped"
        icon={<SkipForward className="h-4 w-4 text-muted-foreground" />}
      />
    </div>
  );
}

function isCompletedExport(job: RecentJobDetail): boolean {
  return job.jobType === JobType.EXPORT && (job.status as JobStatus) === JobStatus.COMPLETED;
}

// ExportDownloadButton re-downloads a completed export's file. The export file
// is retained for 24h after completion; once expired/removed the download
// endpoint returns an error, surfaced here as a toast.
function ExportDownloadButton({ jobId }: { jobId: string }) {
  const { mutate: downloadExport, isPending } = useDownloadExport();
  return (
    <Button
      onClick={() =>
        downloadExport(jobId, {
          onSuccess: () => toast.success('Download started'),
          onError: (error) => toast.error(error.message || 'Failed to download export'),
        })
      }
      disabled={isPending}
      size="sm"
      className="bg-green-600 hover:bg-green-700"
    >
      {isPending ? (
        <>
          <Loader2 className="mr-2 h-4 w-4 animate-spin" />
          Downloading...
        </>
      ) : (
        <>
          <Download className="mr-2 h-4 w-4" />
          Download Export
        </>
      )}
    </Button>
  );
}

function JobActivityItem({
  job,
  isExpanded,
  onToggle,
}: {
  job: RecentJobDetail;
  isExpanded: boolean;
  onToggle: () => void;
}) {
  const showBreakdown = hasChangeRows(job);
  return (
    <Collapsible open={isExpanded} onOpenChange={onToggle}>
      <div className="rounded-lg border">
        <CollapsibleTrigger asChild>
          <button
            className="w-full p-3 flex items-center justify-between hover:bg-muted/50 transition-colors text-left"
            type="button"
          >
            <div className="flex items-center gap-3 min-w-0 flex-1">
              <Badge variant={getJobStatusVariant(job.status as JobStatus)} className="shrink-0">
                {getJobStatusLabel(job.status as JobStatus)}
              </Badge>
              <span className="font-medium truncate">
                {getJobTypeLabel(job.jobType)} - {getJobSourceLabel(job.source)}
              </span>
            </div>
            <div className="flex items-center gap-4 text-sm text-muted-foreground shrink-0 ml-4">
              <div className="hidden sm:flex items-center gap-2">
                {job.jobType === JobType.EXPORT ? (
                  // Export jobs create no job_items, so a "completed" count is
                  // always 0; show the number of games exported instead.
                  <span className="text-green-600">{job.totalItems} games</span>
                ) : (
                  <>
                    <span className="text-green-600">{job.completedCount} completed</span>
                    {job.failedCount > 0 && (
                      <span className="text-red-600">{job.failedCount} failed</span>
                    )}
                  </>
                )}
              </div>
              <span className="text-xs">
                {formatRelativeTime(job.completedAt || job.createdAt)}
              </span>
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
            {job.errorMessage && (
              <div className="rounded-md bg-destructive/10 p-3 text-sm text-destructive mb-3">
                {job.errorMessage}
              </div>
            )}
            {isCompletedExport(job) ? (
              <div className="flex items-center justify-between gap-3">
                <span className="text-sm text-muted-foreground">
                  Export file ready ({job.totalItems} games). Available for 24 hours.
                </span>
                <ExportDownloadButton jobId={job.id} />
              </div>
            ) : showBreakdown ? (
              <ChangeBreakdown job={job} />
            ) : (
              <JobItemsDetails jobId={job.id} progress={job.progress} isTerminal />
            )}
          </div>
        </CollapsibleContent>
      </div>
    </Collapsible>
  );
}

function EmptyState({ daysBack }: { daysBack: number }) {
  return (
    <div className="flex flex-col items-center justify-center py-8 text-center">
      <div className="rounded-full bg-muted p-3 mb-4">
        <Inbox className="h-6 w-6 text-muted-foreground" />
      </div>
      <h3 className="font-medium text-muted-foreground mb-1">No recent activity</h3>
      <p className="text-sm text-muted-foreground">
        Completed activity from the last {daysBack} days will appear here.
      </p>
    </div>
  );
}

/**
 * Recent Activity over completed/failed jobs. Renders a rich per-outcome
 * breakdown when the job has change rows (sync, import); otherwise falls back to
 * aggregate counts + per-item details (export, metadata-refresh).
 */
export function RecentActivity({
  source,
  jobTypes,
  excludeJobIds = [],
  daysBack = 7,
  limit = 5,
}: RecentActivityProps) {
  const [isOpen, setIsOpen] = useState(true);
  const [expandedJobId, setExpandedJobId] = useState<string | null>(null);
  const { data, isLoading } = useRecentJobs({ source, jobTypes, daysBack, limit });

  if (isLoading) return null;

  const jobs = (data?.jobs ?? []).filter((j) => !excludeJobIds.includes(j.id));

  return (
    <Collapsible open={isOpen} onOpenChange={setIsOpen}>
      <Card>
        <CollapsibleTrigger asChild>
          <CardHeader className="cursor-pointer hover:bg-muted/50 transition-colors">
            <div className="flex items-center justify-between">
              <div className="flex items-center gap-2">
                <History className="h-5 w-5 text-muted-foreground" />
                <CardTitle className="text-lg">Recent Activity</CardTitle>
                {jobs.length > 0 && (
                  <Badge variant="secondary" className="ml-2">
                    {jobs.length}
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
            {jobs.length === 0 ? (
              <EmptyState daysBack={daysBack} />
            ) : (
              <div className="space-y-2">
                {jobs.map((job) => (
                  <JobActivityItem
                    key={job.id}
                    job={job}
                    isExpanded={expandedJobId === job.id}
                    onToggle={() =>
                      setExpandedJobId((current) => (current === job.id ? null : job.id))
                    }
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
