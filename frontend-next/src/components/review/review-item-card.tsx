'use client';

import { Card, CardContent, CardFooter, CardHeader, CardTitle } from '@/components/ui/card';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Alert, AlertDescription } from '@/components/ui/alert';
import { AlertTriangle, Check, Eye, Loader2, SkipForward, X, ImageOff } from 'lucide-react';
import type { ReviewItem, IGDBCandidate } from '@/types';
import {
  ReviewItemStatus,
  getReviewStatusLabel,
  getReviewStatusVariant,
  isRemovalItem,
  formatReleaseYear,
} from '@/types';

interface ReviewItemCardProps {
  item: ReviewItem;
  onMatch?: (item: ReviewItem, igdbId: number) => void;
  onSkip?: (item: ReviewItem) => void;
  onKeep?: (item: ReviewItem) => void;
  onRemove?: (item: ReviewItem) => void;
  onView?: (item: ReviewItem) => void;
  isProcessing?: boolean;
}

export function ReviewItemCard({
  item,
  onMatch,
  onSkip,
  onKeep,
  onRemove,
  onView,
  isProcessing = false,
}: ReviewItemCardProps) {
  const isPending = item.status === ReviewItemStatus.PENDING;
  const isRemoval = isRemovalItem(item);
  const topCandidate = item.igdbCandidates[0] || null;

  return (
    <Card>
      <CardHeader className="pb-3">
        <div className="flex items-start justify-between gap-4">
          <div className="min-w-0 flex-1">
            <CardTitle className="truncate text-lg">{item.sourceTitle}</CardTitle>
            {item.jobType && item.jobSource && (
              <p className="mt-1 text-sm text-muted-foreground">
                From: {item.jobType} ({item.jobSource})
              </p>
            )}
          </div>
          <Badge variant={getReviewStatusVariant(item.status)}>
            {getReviewStatusLabel(item.status)}
          </Badge>
        </div>
      </CardHeader>

      <CardContent className="space-y-4">
        {/* Removal warning */}
        {isRemoval && (
          <Alert variant="default" className="border-orange-200 bg-orange-50 text-orange-800 dark:border-orange-800 dark:bg-orange-900/20 dark:text-orange-300">
            <AlertTriangle className="h-4 w-4" />
            <AlertDescription>
              This game was detected as removed from your library during sync.
            </AlertDescription>
          </Alert>
        )}

        {/* Best match suggestion */}
        {isPending && topCandidate && (
          <CandidatePreview candidate={topCandidate} />
        )}

        {/* Resolved info */}
        {!isPending && item.resolvedIgdbId && (
          <div className="text-sm text-muted-foreground">
            Matched to IGDB ID: <span className="font-mono">{item.resolvedIgdbId}</span>
          </div>
        )}
      </CardContent>

      <CardFooter className="flex flex-wrap gap-2 border-t pt-4">
        {isPending ? (
          isRemoval ? (
            <>
              <Button
                variant="default"
                size="sm"
                onClick={() => onKeep?.(item)}
                disabled={isProcessing}
                className="flex-1 bg-green-600 hover:bg-green-700"
              >
                {isProcessing ? (
                  <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                ) : (
                  <Check className="mr-2 h-4 w-4" />
                )}
                Keep in Collection
              </Button>
              <Button
                variant="destructive"
                size="sm"
                onClick={() => onRemove?.(item)}
                disabled={isProcessing}
                className="flex-1"
              >
                {isProcessing ? (
                  <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                ) : (
                  <X className="mr-2 h-4 w-4" />
                )}
                Remove
              </Button>
            </>
          ) : (
            <>
              {topCandidate && (
                <Button
                  variant="default"
                  size="sm"
                  onClick={() => onMatch?.(item, topCandidate.igdbId)}
                  disabled={isProcessing}
                  className="flex-1"
                >
                  {isProcessing ? (
                    <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                  ) : (
                    <Check className="mr-2 h-4 w-4" />
                  )}
                  Match
                </Button>
              )}
              <Button
                variant="outline"
                size="sm"
                onClick={() => onView?.(item)}
                disabled={isProcessing}
                className="flex-1"
              >
                <Eye className="mr-2 h-4 w-4" />
                {item.igdbCandidates.length > 1
                  ? `View ${item.igdbCandidates.length} Options`
                  : 'Search IGDB'}
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
          )
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

interface CandidatePreviewProps {
  candidate: IGDBCandidate;
}

function CandidatePreview({ candidate }: CandidatePreviewProps) {
  return (
    <div className="flex items-start gap-3 rounded-lg border bg-muted/50 p-3">
      {candidate.coverUrl ? (
        <img
          src={candidate.coverUrl}
          alt={candidate.name}
          className="h-16 w-12 flex-shrink-0 rounded object-cover"
        />
      ) : (
        <div className="flex h-16 w-12 flex-shrink-0 items-center justify-center rounded bg-muted">
          <ImageOff className="h-6 w-6 text-muted-foreground" />
        </div>
      )}
      <div className="min-w-0 flex-1">
        <p className="text-sm font-medium">
          {candidate.name}
          {candidate.firstReleaseDate && (
            <span className="ml-1 text-muted-foreground">
              {formatReleaseYear(candidate.firstReleaseDate)}
            </span>
          )}
        </p>
        {candidate.similarityScore !== null && (
          <p className="mt-0.5 text-xs text-muted-foreground">
            Match confidence: {Math.round(candidate.similarityScore * 100)}%
          </p>
        )}
        {candidate.summary && (
          <p className="mt-1 line-clamp-2 text-xs text-muted-foreground" title={candidate.summary}>
            {candidate.summary}
          </p>
        )}
      </div>
    </div>
  );
}
