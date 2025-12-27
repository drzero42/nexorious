'use client';

import { useState } from 'react';
import { toast } from 'sonner';
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from '@/components/ui/collapsible';
import { Button } from '@/components/ui/button';
import { Badge } from '@/components/ui/badge';
import { Skeleton } from '@/components/ui/skeleton';
import { ChevronDown, ChevronRight, AlertCircle, CheckCircle, Clock, Loader2, RotateCcw } from 'lucide-react';
import { useJobItems, useRetryFailedItems, useRetryJobItem } from '@/hooks';
import { JobItemStatus, getJobItemStatusLabel, getJobItemStatusVariant } from '@/types';

interface JobItemsDetailsProps {
  jobId: string;
  progress: {
    pending: number;
    processing: number;
    completed: number;
    pendingReview: number;
    skipped: number;
    failed: number;
  };
  isTerminal: boolean;
}

interface StatusSectionProps {
  jobId: string;
  status: JobItemStatus;
  count: number;
  defaultOpen?: boolean;
  isTerminal: boolean;
}

function StatusSection({ jobId, status, count, defaultOpen = false, isTerminal }: StatusSectionProps) {
  const [isOpen, setIsOpen] = useState(defaultOpen);
  const [page, setPage] = useState(1);
  const { data, isLoading, refetch } = useJobItems(jobId, status, page, 20, { enabled: isOpen && count > 0 });

  // Retry mutations
  const retryAllMutation = useRetryFailedItems();
  const retryItemMutation = useRetryJobItem();

  // Determine if retry buttons should be shown
  const isFailedSection = status === JobItemStatus.FAILED;
  const canRetry = isFailedSection && isTerminal;

  const handleRetryAll = async () => {
    try {
      const result = await retryAllMutation.mutateAsync(jobId);
      toast.success(result.message);
    } catch (error) {
      toast.error(error instanceof Error ? error.message : 'Failed to retry items');
    }
  };

  const handleRetryItem = async (itemId: string) => {
    try {
      await retryItemMutation.mutateAsync(itemId);
      toast.success('Item queued for retry');
      refetch();
    } catch (error) {
      toast.error(error instanceof Error ? error.message : 'Failed to retry item');
    }
  };

  if (count === 0) return null;

  const iconMap: Record<JobItemStatus, React.ReactNode> = {
    [JobItemStatus.PENDING]: <Clock className="h-4 w-4 text-muted-foreground" />,
    [JobItemStatus.PROCESSING]: <Loader2 className="h-4 w-4 animate-spin text-blue-500" />,
    [JobItemStatus.COMPLETED]: <CheckCircle className="h-4 w-4 text-green-600" />,
    [JobItemStatus.PENDING_REVIEW]: <AlertCircle className="h-4 w-4 text-yellow-600" />,
    [JobItemStatus.SKIPPED]: <Clock className="h-4 w-4 text-muted-foreground" />,
    [JobItemStatus.FAILED]: <AlertCircle className="h-4 w-4 text-red-600" />,
  };

  return (
    <Collapsible open={isOpen} onOpenChange={setIsOpen}>
      <CollapsibleTrigger asChild>
        <Button variant="ghost" className="w-full justify-between px-4 py-2 h-auto">
          <div className="flex items-center gap-2">
            {isOpen ? <ChevronDown className="h-4 w-4" /> : <ChevronRight className="h-4 w-4" />}
            {iconMap[status]}
            <span>{getJobItemStatusLabel(status)}</span>
          </div>
          <div className="flex items-center gap-2">
            {canRetry && (
              <Button
                variant="outline"
                size="sm"
                onClick={(e) => {
                  e.stopPropagation();
                  handleRetryAll();
                }}
                disabled={retryAllMutation.isPending}
                className="h-7"
              >
                {retryAllMutation.isPending ? (
                  <Loader2 className="h-3 w-3 animate-spin mr-1" />
                ) : (
                  <RotateCcw className="h-3 w-3 mr-1" />
                )}
                Retry All
              </Button>
            )}
            <Badge variant={getJobItemStatusVariant(status)}>{count}</Badge>
          </div>
        </Button>
      </CollapsibleTrigger>
      <CollapsibleContent>
        <div className="border-l-2 border-muted ml-6 pl-4 py-2 space-y-2">
          {isLoading ? (
            <div className="space-y-2">
              <Skeleton className="h-10 w-full" />
              <Skeleton className="h-10 w-full" />
              <Skeleton className="h-10 w-full" />
            </div>
          ) : (
            <>
              {data?.items.map((item) => (
                <div
                  key={item.id}
                  className="flex items-start justify-between rounded-md border p-3 text-sm"
                >
                  <div className="min-w-0 flex-1">
                    <div className="font-medium truncate">{item.sourceTitle}</div>
                    {item.resultGameTitle && (
                      <div className="text-muted-foreground truncate">
                        &rarr; {item.resultGameTitle}
                      </div>
                    )}
                    {item.errorMessage && (
                      <div className="text-red-600 text-xs mt-1">{item.errorMessage}</div>
                    )}
                  </div>
                  {canRetry && (
                    <Button
                      variant="ghost"
                      size="sm"
                      onClick={() => handleRetryItem(item.id)}
                      disabled={retryItemMutation.isPending}
                      className="ml-2 h-8"
                    >
                      {retryItemMutation.isPending ? (
                        <Loader2 className="h-3 w-3 animate-spin" />
                      ) : (
                        <RotateCcw className="h-3 w-3" />
                      )}
                    </Button>
                  )}
                </div>
              ))}
              {data && data.pages > 1 && (
                <div className="flex items-center justify-center gap-2 pt-2">
                  <Button
                    variant="outline"
                    size="sm"
                    onClick={() => setPage((p) => Math.max(1, p - 1))}
                    disabled={page <= 1}
                  >
                    Previous
                  </Button>
                  <span className="text-sm text-muted-foreground">
                    Page {page} of {data.pages}
                  </span>
                  <Button
                    variant="outline"
                    size="sm"
                    onClick={() => setPage((p) => Math.min(data.pages, p + 1))}
                    disabled={page >= data.pages}
                  >
                    Next
                  </Button>
                </div>
              )}
            </>
          )}
        </div>
      </CollapsibleContent>
    </Collapsible>
  );
}

export function JobItemsDetails({ jobId, progress, isTerminal }: JobItemsDetailsProps) {
  const sections = [
    { status: JobItemStatus.FAILED, count: progress.failed, defaultOpen: progress.failed > 0 },
    { status: JobItemStatus.PROCESSING, count: progress.processing, defaultOpen: false },
    { status: JobItemStatus.PENDING, count: progress.pending, defaultOpen: false },
    { status: JobItemStatus.COMPLETED, count: progress.completed, defaultOpen: false },
    { status: JobItemStatus.PENDING_REVIEW, count: progress.pendingReview, defaultOpen: false },
    { status: JobItemStatus.SKIPPED, count: progress.skipped, defaultOpen: false },
  ];

  const hasItems = sections.some(s => s.count > 0);

  if (!hasItems) {
    return null;
  }

  return (
    <div className="rounded-lg border">
      <div className="border-b p-3">
        <h3 className="font-medium">Item Details</h3>
      </div>
      <div className="divide-y">
        {sections.map(({ status, count, defaultOpen }) => (
          <StatusSection
            key={status}
            jobId={jobId}
            status={status}
            count={count}
            defaultOpen={defaultOpen}
            isTerminal={isTerminal}
          />
        ))}
      </div>
    </div>
  );
}
