
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { PlayStatus } from '@/types';
import { StatusProgress } from './status-progress';
import { statusIcons, statusLabels } from './status-progress-data';

interface CollectionStats {
  totalGames: number;
  completionStats: Record<PlayStatus, number>;
  ownershipStats: Record<string, number>;
  platformStats: Record<string, number>;
  genreStats: Record<string, number>;
  pileOfShame: number;
  completionRate: number;
  averageRating: number | null;
  totalHoursPlayed: number;
}

interface ProgressStatisticsProps {
  stats: CollectionStats;
  className?: string;
}

// Order of play statuses in the progress journey
const statusOrder: PlayStatus[] = [
  PlayStatus.NOT_STARTED,
  PlayStatus.IN_PROGRESS,
  PlayStatus.COMPLETED,
  PlayStatus.MASTERED,
  PlayStatus.DOMINATED,
  PlayStatus.SHELVED,
  PlayStatus.DROPPED,
  PlayStatus.REPLAY,
];

// Background colors for journey icons
const statusBgColors: Record<PlayStatus, string> = {
  [PlayStatus.NOT_STARTED]: 'bg-gray-100',
  [PlayStatus.IN_PROGRESS]: 'bg-blue-100',
  [PlayStatus.COMPLETED]: 'bg-green-100',
  [PlayStatus.MASTERED]: 'bg-purple-100',
  [PlayStatus.DOMINATED]: 'bg-yellow-100',
  [PlayStatus.SHELVED]: 'bg-orange-100',
  [PlayStatus.DROPPED]: 'bg-red-100',
  [PlayStatus.REPLAY]: 'bg-cyan-100',
};

// Journey statuses (main progression)
const journeyStatuses: PlayStatus[] = [
  PlayStatus.NOT_STARTED,
  PlayStatus.IN_PROGRESS,
  PlayStatus.COMPLETED,
  PlayStatus.MASTERED,
  PlayStatus.DOMINATED,
];

const journeyDescriptions: Record<PlayStatus, string> = {
  [PlayStatus.NOT_STARTED]: 'games waiting',
  [PlayStatus.IN_PROGRESS]: 'games active',
  [PlayStatus.COMPLETED]: 'main stories finished',
  [PlayStatus.MASTERED]: 'games fully explored',
  [PlayStatus.DOMINATED]: 'games at 100%',
  [PlayStatus.SHELVED]: 'games on hold',
  [PlayStatus.DROPPED]: 'games stopped',
  [PlayStatus.REPLAY]: 'games replaying',
};

export function ProgressStatistics({
  stats,
  className,
}: ProgressStatisticsProps) {
  const {
    totalGames,
    completionStats,
    completionRate,
    totalHoursPlayed,
  } = stats;

  // Calculate active games (in progress + replay)
  const activeGames =
    (completionStats[PlayStatus.IN_PROGRESS] || 0) +
    (completionStats[PlayStatus.REPLAY] || 0);

  // Calculate time metrics
  const averageHoursPerGame = totalGames > 0 ? totalHoursPlayed / totalGames : 0;

  // Calculate completed games with hours for average completion time
  const completedCount =
    (completionStats[PlayStatus.COMPLETED] || 0) +
    (completionStats[PlayStatus.MASTERED] || 0) +
    (completionStats[PlayStatus.DOMINATED] || 0);

  // Estimate average completion time (rough approximation)
  const averageCompletionTime =
    completedCount > 0 && totalHoursPlayed > 0
      ? totalHoursPlayed / completedCount
      : 0;

  return (
    <div className={className}>
      {/* Overview Stats */}
      <div className="mb-6 grid grid-cols-2 gap-4 md:grid-cols-4">
        <Card>
          <CardContent className="p-4">
            <div className="text-sm font-medium text-muted-foreground">
              Total Games
            </div>
            <div className="mt-1 text-2xl font-bold">{totalGames}</div>
          </CardContent>
        </Card>
        <Card>
          <CardContent className="p-4">
            <div className="text-sm font-medium text-muted-foreground">
              Completion Rate
            </div>
            <div className="mt-1 text-2xl font-bold text-green-600">
              {completionRate.toFixed(1)}%
            </div>
          </CardContent>
        </Card>
        <Card>
          <CardContent className="p-4">
            <div className="text-sm font-medium text-muted-foreground">
              Total Hours
            </div>
            <div className="mt-1 text-2xl font-bold text-blue-600">
              {totalHoursPlayed.toLocaleString()}
            </div>
          </CardContent>
        </Card>
        <Card>
          <CardContent className="p-4">
            <div className="text-sm font-medium text-muted-foreground">
              Active Games
            </div>
            <div className="mt-1 text-2xl font-bold text-purple-600">
              {activeGames}
            </div>
          </CardContent>
        </Card>
      </div>

      {/* Progress Breakdown */}
      <Card className="mb-6">
        <CardHeader>
          <CardTitle>Progress Breakdown</CardTitle>
        </CardHeader>
        <CardContent className="space-y-4">
          {statusOrder.map((status) => (
            <StatusProgress
              key={status}
              status={status}
              count={completionStats[status] || 0}
              total={totalGames}
              showDescription
            />
          ))}
        </CardContent>
      </Card>

      {/* Completion Journey */}
      <Card className="mb-6">
        <CardHeader>
          <CardTitle>Completion Journey</CardTitle>
        </CardHeader>
        <CardContent>
          <div className="relative">
            <div className="absolute bottom-0 left-8 top-8 w-0.5 bg-border"></div>
            <div className="space-y-6">
              {journeyStatuses.map((status) => (
                <div key={status} className="flex items-center gap-4">
                  <div
                    className={`relative z-10 flex h-16 w-16 items-center justify-center rounded-full text-2xl ${statusBgColors[status]}`}
                    role="img"
                    aria-label={statusLabels[status]}
                  >
                    {statusIcons[status]}
                  </div>
                  <div className="flex-1">
                    <h4 className="font-medium">{statusLabels[status]}</h4>
                    <p className="text-sm text-muted-foreground">
                      {completionStats[status] || 0} {journeyDescriptions[status]}
                    </p>
                  </div>
                </div>
              ))}
            </div>
          </div>
        </CardContent>
      </Card>

      {/* Time Investment */}
      {totalHoursPlayed > 0 && (
        <Card>
          <CardHeader>
            <CardTitle>Time Investment</CardTitle>
          </CardHeader>
          <CardContent>
            <div className="grid grid-cols-1 gap-4 md:grid-cols-3">
              <div className="text-center">
                <div className="text-3xl font-bold text-blue-600">
                  {totalHoursPlayed.toLocaleString()}
                </div>
                <div className="mt-1 text-sm text-muted-foreground">
                  Total Hours Played
                </div>
              </div>
              <div className="text-center">
                <div className="text-3xl font-bold text-green-600">
                  {averageHoursPerGame.toFixed(1)}
                </div>
                <div className="mt-1 text-sm text-muted-foreground">
                  Average Hours per Game
                </div>
              </div>
              <div className="text-center">
                <div className="text-3xl font-bold text-purple-600">
                  {averageCompletionTime.toFixed(1)}
                </div>
                <div className="mt-1 text-sm text-muted-foreground">
                  Average Completion Time
                </div>
              </div>
            </div>
          </CardContent>
        </Card>
      )}
    </div>
  );
}
