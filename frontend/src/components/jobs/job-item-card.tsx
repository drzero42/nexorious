'use client';

import { Card, CardContent, CardFooter, CardHeader, CardTitle } from '@/components/ui/card';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Eye, SkipForward } from 'lucide-react';
import type { JobItem } from '@/types';
import {
  JobItemStatus,
  getJobItemStatusLabel,
  getJobItemStatusVariant,
} from '@/types';

interface JobItemCardProps {
  item: JobItem;
  onMatch?: (item: JobItem, igdbId: number) => void;
  onSkip?: (item: JobItem) => void;
  onView?: (item: JobItem) => void;
  isProcessing?: boolean;
}

export function JobItemCard({
  item,
  onSkip,
  onView,
  isProcessing = false,
}: JobItemCardProps) {
  const isPending = item.status === JobItemStatus.PENDING_REVIEW;

  return (
    <Card>
      <CardHeader className="pb-3">
        <div className="flex items-start justify-between gap-4">
          <div className="min-w-0 flex-1">
            <CardTitle className="truncate text-lg">{item.sourceTitle}</CardTitle>
            {item.resultGameTitle && (
              <p className="mt-1 text-sm text-muted-foreground">
                Matched: {item.resultGameTitle}
              </p>
            )}
          </div>
          <Badge variant={getJobItemStatusVariant(item.status)}>
            {getJobItemStatusLabel(item.status)}
          </Badge>
        </div>
      </CardHeader>

      <CardContent className="space-y-4">
        {/* Resolved info */}
        {!isPending && item.resultIgdbId && (
          <div className="text-sm text-muted-foreground">
            IGDB ID: <span className="font-mono">{item.resultIgdbId}</span>
          </div>
        )}

        {/* Error message if failed */}
        {item.errorMessage && (
          <div className="text-sm text-destructive">
            Error: {item.errorMessage}
          </div>
        )}
      </CardContent>

      <CardFooter className="flex flex-wrap gap-2 border-t pt-4">
        {isPending ? (
          <>
            <Button
              variant="outline"
              size="sm"
              onClick={() => onView?.(item)}
              disabled={isProcessing}
              className="flex-1"
            >
              <Eye className="mr-2 h-4 w-4" />
              Search & Match
            </Button>
            <Button
              variant="ghost"
              size="sm"
              onClick={() => onSkip?.(item)}
              disabled={isProcessing}
            >
              <SkipForward className="mr-2 h-4 w-4" />
              Skip
            </Button>
          </>
        ) : (
          <Button variant="ghost" size="sm" onClick={() => onView?.(item)}>
            <Eye className="mr-2 h-4 w-4" />
            View Details
          </Button>
        )}
      </CardFooter>
    </Card>
  );
}
