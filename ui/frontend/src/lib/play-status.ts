import { PlayStatus } from '@/types';

// Canonical badge background colour per play status. Single source of truth for
// the game list, the game card, and the dashboard status breakdown.
export const statusColors: Record<PlayStatus, string> = {
  [PlayStatus.NOT_STARTED]: 'bg-gray-500',
  [PlayStatus.IN_PROGRESS]: 'bg-blue-500',
  [PlayStatus.COMPLETED]: 'bg-green-500',
  [PlayStatus.MASTERED]: 'bg-purple-500',
  [PlayStatus.DOMINATED]: 'bg-yellow-500',
  [PlayStatus.SHELVED]: 'bg-orange-500',
  [PlayStatus.DROPPED]: 'bg-red-500',
  [PlayStatus.REPLAY]: 'bg-cyan-500',
};

// Canonical human-readable label per play status.
export const statusLabels: Record<PlayStatus, string> = {
  [PlayStatus.NOT_STARTED]: 'Not Started',
  [PlayStatus.IN_PROGRESS]: 'In Progress',
  [PlayStatus.COMPLETED]: 'Completed',
  [PlayStatus.MASTERED]: 'Mastered',
  [PlayStatus.DOMINATED]: 'Dominated',
  [PlayStatus.SHELVED]: 'Shelved',
  [PlayStatus.DROPPED]: 'Dropped',
  [PlayStatus.REPLAY]: 'Replay',
};
