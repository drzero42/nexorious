'use client';

import { useCollectionStats } from '@/hooks';
import { ProgressStatistics, CurrentlyPlayingSection } from '@/components/dashboard';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { Skeleton } from '@/components/ui/skeleton';
import { Button } from '@/components/ui/button';
import Link from 'next/link';
import { PlayStatus } from '@/types';

function DashboardSkeleton() {
  return (
    <div className="space-y-6">
      {/* Overview Stats Skeleton */}
      <div className="grid grid-cols-2 gap-4 md:grid-cols-4">
        {Array.from({ length: 4 }).map((_, i) => (
          <Card key={i}>
            <CardContent className="p-4">
              <Skeleton className="mb-2 h-4 w-24" />
              <Skeleton className="h-8 w-16" />
            </CardContent>
          </Card>
        ))}
      </div>
      {/* Progress Breakdown Skeleton */}
      <Card>
        <CardHeader>
          <Skeleton className="h-6 w-40" />
        </CardHeader>
        <CardContent className="space-y-4">
          {Array.from({ length: 8 }).map((_, i) => (
            <div key={i} className="space-y-2">
              <div className="flex justify-between">
                <Skeleton className="h-4 w-32" />
                <Skeleton className="h-4 w-12" />
              </div>
              <Skeleton className="h-2 w-full" />
            </div>
          ))}
        </CardContent>
      </Card>
    </div>
  );
}

function EmptyState() {
  return (
    <Card>
      <CardContent className="flex flex-col items-center justify-center py-12">
        <div className="mb-4 text-6xl">🎮</div>
        <h3 className="mb-2 text-lg font-semibold">No games in your collection</h3>
        <p className="mb-4 text-center text-muted-foreground">
          Add some games to see your statistics and track your gaming progress!
        </p>
        <Button asChild>
          <Link href="/games/add">Add Your First Game</Link>
        </Button>
      </CardContent>
    </Card>
  );
}

function TopGenres({ genreStats }: { genreStats: Record<string, number> }) {
  const sortedGenres = Object.entries(genreStats)
    .sort(([, a], [, b]) => b - a)
    .slice(0, 5);

  if (sortedGenres.length === 0) {
    return null;
  }

  const maxCount = sortedGenres[0]?.[1] || 1;

  return (
    <Card>
      <CardHeader>
        <CardTitle>Top Genres</CardTitle>
      </CardHeader>
      <CardContent className="space-y-3">
        {sortedGenres.map(([genre, count]) => (
          <div key={genre} className="flex items-center justify-between">
            <span className="text-sm font-medium">{genre}</span>
            <div className="flex items-center gap-2">
              <span className="text-sm text-muted-foreground">{count}</span>
              <div className="h-2 w-20 overflow-hidden rounded-full bg-secondary">
                <div
                  className="h-full bg-indigo-500 transition-all"
                  style={{ width: `${(count / maxCount) * 100}%` }}
                />
              </div>
            </div>
          </div>
        ))}
      </CardContent>
    </Card>
  );
}

function PersonalStats({
  stats,
}: {
  stats: {
    averageRating: number | null;
    totalGames: number;
    totalHoursPlayed: number;
    pileOfShame: number;
    completionRate: number;
    completionStats: Record<PlayStatus, number>;
  };
}) {
  // Find the most played game stats
  const completedGames =
    (stats.completionStats[PlayStatus.COMPLETED] || 0) +
    (stats.completionStats[PlayStatus.MASTERED] || 0) +
    (stats.completionStats[PlayStatus.DOMINATED] || 0);

  return (
    <div className="grid grid-cols-1 gap-6 lg:grid-cols-2">
      <Card>
        <CardHeader>
          <CardTitle>Personal Stats</CardTitle>
        </CardHeader>
        <CardContent className="space-y-3">
          <div className="flex items-center justify-between">
            <span className="text-sm font-medium">Average Rating</span>
            <span className="text-sm text-muted-foreground">
              {stats.averageRating != null && stats.averageRating > 0
                ? `${stats.averageRating.toFixed(1)}/5`
                : 'N/A'}
            </span>
          </div>
          <div className="flex items-center justify-between">
            <span className="text-sm font-medium">Average Hours per Game</span>
            <span className="text-sm text-muted-foreground">
              {stats.totalGames > 0
                ? (stats.totalHoursPlayed / stats.totalGames).toFixed(1)
                : 0}
              h
            </span>
          </div>
          <div className="flex items-center justify-between">
            <span className="text-sm font-medium">Games Finished</span>
            <span className="text-sm text-muted-foreground">
              {completedGames}
            </span>
          </div>
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>Game Insights</CardTitle>
        </CardHeader>
        <CardContent className="space-y-3">
          <div className="flex items-center justify-between">
            <span className="text-sm font-medium">Pile of Shame</span>
            <span className="text-sm text-muted-foreground">
              {stats.pileOfShame} games
            </span>
          </div>
          <div className="flex items-center justify-between">
            <span className="text-sm font-medium">Completion Rate</span>
            <span className="text-sm text-muted-foreground">
              {stats.completionRate.toFixed(1)}%
            </span>
          </div>
          <div className="flex items-center justify-between">
            <span className="text-sm font-medium">In Progress</span>
            <span className="text-sm text-muted-foreground">
              {stats.completionStats[PlayStatus.IN_PROGRESS] || 0} games
            </span>
          </div>
        </CardContent>
      </Card>
    </div>
  );
}

export default function DashboardPage() {
  const { data: stats, isLoading, error } = useCollectionStats();

  return (
    <div>
      {/* Header */}
      <div className="mb-6">
        <h1 className="text-2xl font-bold">Dashboard</h1>
        <p className="text-muted-foreground">
          Your gaming statistics and insights
        </p>
      </div>

      {isLoading && <DashboardSkeleton />}

      {error && (
        <Card>
          <CardContent className="py-8 text-center text-destructive">
            Failed to load statistics. Please try again later.
          </CardContent>
        </Card>
      )}

      {stats && stats.totalGames === 0 && <EmptyState />}

      {stats && stats.totalGames > 0 && (
        <>
          <CurrentlyPlayingSection />

          <ProgressStatistics stats={stats} className="mb-6" />
          <TopGenres genreStats={stats.genreStats} />
          <div className="mt-6">
            <PersonalStats stats={stats} />
          </div>
        </>
      )}
    </div>
  );
}
