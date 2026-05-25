import { useState } from 'react';
import type { ReactNode } from 'react';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from '@/components/ui/collapsible';
import { Button } from '@/components/ui/button';
import { Badge } from '@/components/ui/badge';
import { Skeleton } from '@/components/ui/skeleton';
import {
  Clock,
  ChevronDown,
  ChevronRight,
  CheckCircle,
  XCircle,
  ArrowRight,
} from 'lucide-react';
import { useRecentJobs } from '@/hooks';
import type { RecentJobDetail, SyncChangeItem } from '@/types';

interface RecentActivityProps {
  platform: string;
}

function formatDate(dateStr: string): string {
  return new Date(dateStr).toLocaleString();
}

function SyncChangeList({
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
          <Badge variant="secondary" className="h-5 text-xs">{items.length}</Badge>
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

function formatSummary(job: RecentJobDetail): string {
  const parts: string[] = [];
  if (job.completedCount > 0) parts.push(`${job.completedCount} matched`);
  if (job.skippedCount > 0) parts.push(`${job.skippedCount} skipped`);
  if (job.failedCount > 0) parts.push(`${job.failedCount} failed`);
  return parts.join(' · ');
}

function JobCard({ job }: { job: RecentJobDetail }) {
  const [isOpen, setIsOpen] = useState(false);

  return (
    <Collapsible open={isOpen} onOpenChange={setIsOpen}>
      <CollapsibleTrigger asChild>
        <Button variant="ghost" className="w-full justify-between px-4 py-3 h-auto">
          <div className="flex items-center gap-2">
            {isOpen ? <ChevronDown className="h-4 w-4" /> : <ChevronRight className="h-4 w-4" />}
            <span>{job.completedAt ? formatDate(job.completedAt) : 'In progress'}</span>
          </div>
          <div className="flex items-center gap-2">
            <span className="text-sm text-muted-foreground">{formatSummary(job)}</span>
            <Badge
              variant={job.status === 'completed' ? 'outline' : 'destructive'}
              className={
                job.status === 'completed'
                  ? 'h-5 text-xs bg-green-100 text-green-800 dark:bg-green-900/30 dark:text-green-400'
                  : 'h-5 text-xs'
              }
            >
              {job.status === 'completed' ? 'Completed' : 'Failed'}
            </Badge>
          </div>
        </Button>
      </CollapsibleTrigger>
      <CollapsibleContent>
        <div className="px-4 pb-4 space-y-1">
          <SyncChangeList
            items={job.addedItems}
            label="Added to library"
            icon={<CheckCircle className="h-4 w-4 text-green-600" />}
          />
          <SyncChangeList
            items={job.removedItems}
            label="Removed from storefront"
            icon={<XCircle className="h-4 w-4 text-muted-foreground" />}
          />
          <SyncChangeList
            items={job.statusChangedItems}
            label="Status changed"
            icon={<ArrowRight className="h-4 w-4 text-blue-500" />}
          />
        </div>
      </CollapsibleContent>
    </Collapsible>
  );
}

export function RecentActivity({ platform }: RecentActivityProps) {
  const { data, isLoading, error } = useRecentJobs(platform, 5);

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <Clock className="h-5 w-5" />
          Recent Activity
        </CardTitle>
      </CardHeader>
      <CardContent>
        {isLoading ? (
          <div className="space-y-2">
            <Skeleton className="h-12" />
            <Skeleton className="h-12" />
          </div>
        ) : error ? (
          <div className="text-center py-8 text-muted-foreground">
            Failed to load recent activity
          </div>
        ) : !data || data.jobs.length === 0 ? (
          <div className="flex flex-col items-center justify-center py-8 text-center text-muted-foreground">
            <Clock className="h-12 w-12 mb-4 opacity-50" />
            <p>No sync history yet</p>
            <p className="text-sm mt-1">Start your first sync to see activity here</p>
          </div>
        ) : (
          <div className="divide-y">
            {data.jobs.map((job) => (
              <JobCard key={job.id} job={job} />
            ))}
          </div>
        )}
      </CardContent>
    </Card>
  );
}
