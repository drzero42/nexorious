'use client';

import { useState } from 'react';
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
  SkipForward,
  AlertCircle,
} from 'lucide-react';
import { useRecentJobs } from '@/hooks';
import type { RecentJobDetail, JobItemSummary } from '@/types';

interface RecentActivityProps {
  platform: string;
}

function formatDate(dateStr: string): string {
  return new Date(dateStr).toLocaleString();
}

function ItemsList({
  items,
  type,
}: {
  items: JobItemSummary[];
  type: 'completed' | 'skipped' | 'failed';
}) {
  const [isOpen, setIsOpen] = useState(false);

  if (items.length === 0) return null;

  const iconMap = {
    completed: <CheckCircle className="h-4 w-4 text-green-600" />,
    skipped: <SkipForward className="h-4 w-4 text-muted-foreground" />,
    failed: <AlertCircle className="h-4 w-4 text-red-600" />,
  };

  const labelMap = {
    completed: 'Completed',
    skipped: 'Skipped',
    failed: 'Failed',
  };

  return (
    <Collapsible open={isOpen} onOpenChange={setIsOpen}>
      <CollapsibleTrigger asChild>
        <Button variant="ghost" size="sm" className="w-full justify-between h-8 px-2">
          <div className="flex items-center gap-2">
            {isOpen ? <ChevronDown className="h-3 w-3" /> : <ChevronRight className="h-3 w-3" />}
            {iconMap[type]}
            <span className="text-sm">{labelMap[type]}</span>
          </div>
          <Badge variant="secondary" className="h-5 text-xs">
            {items.length}
          </Badge>
        </Button>
      </CollapsibleTrigger>
      <CollapsibleContent>
        <div className="ml-6 pl-2 border-l space-y-1 py-1">
          {items.map((item, idx) => (
            <div key={idx} className="text-sm py-1">
              {type === 'completed' && (
                <div>
                  <span className="text-muted-foreground">{item.sourceTitle}</span>
                  {item.resultGameTitle && (
                    <>
                      <span className="mx-1">&rarr;</span>
                      <span className="font-medium">{item.resultGameTitle}</span>
                      {item.resultIgdbId && (
                        <span className="text-muted-foreground ml-1">
                          (IGDB: {item.resultIgdbId})
                        </span>
                      )}
                      <span className="ml-2 text-xs">
                        {item.isNewAddition ? (
                          <Badge variant="outline" className="h-4 text-[10px]">Added</Badge>
                        ) : (
                          <Badge variant="secondary" className="h-4 text-[10px]">Already in library</Badge>
                        )}
                      </span>
                    </>
                  )}
                </div>
              )}
              {type === 'skipped' && (
                <span className="text-muted-foreground">{item.sourceTitle}</span>
              )}
              {type === 'failed' && (
                <div>
                  <span>{item.sourceTitle}</span>
                  {item.errorMessage && (
                    <span className="text-red-600 text-xs ml-2">- {item.errorMessage}</span>
                  )}
                </div>
              )}
            </div>
          ))}
        </div>
      </CollapsibleContent>
    </Collapsible>
  );
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
          <Badge variant="outline">{job.totalItems} games processed</Badge>
        </Button>
      </CollapsibleTrigger>
      <CollapsibleContent>
        <div className="px-4 pb-4 space-y-1">
          <ItemsList items={job.completedItems} type="completed" />
          <ItemsList items={job.skippedItems} type="skipped" />
          <ItemsList items={job.failedItems} type="failed" />
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
