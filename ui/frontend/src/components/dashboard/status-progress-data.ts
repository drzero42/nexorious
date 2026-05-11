import { PlayStatus } from '@/types';

export const statusColors: Record<PlayStatus, string> = {
  [PlayStatus.NOT_STARTED]: 'bg-gray-400',
  [PlayStatus.IN_PROGRESS]: 'bg-blue-500',
  [PlayStatus.COMPLETED]: 'bg-green-500',
  [PlayStatus.MASTERED]: 'bg-purple-500',
  [PlayStatus.DOMINATED]: 'bg-yellow-500',
  [PlayStatus.SHELVED]: 'bg-orange-500',
  [PlayStatus.DROPPED]: 'bg-red-500',
  [PlayStatus.REPLAY]: 'bg-cyan-500',
};

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

export const statusIcons: Record<PlayStatus, string> = {
  [PlayStatus.NOT_STARTED]: '⏸️',
  [PlayStatus.IN_PROGRESS]: '🎮',
  [PlayStatus.COMPLETED]: '✅',
  [PlayStatus.MASTERED]: '🏆',
  [PlayStatus.DOMINATED]: '👑',
  [PlayStatus.SHELVED]: '📚',
  [PlayStatus.DROPPED]: '❌',
  [PlayStatus.REPLAY]: '🔄',
};

export const statusDescriptions: Record<PlayStatus, string> = {
  [PlayStatus.NOT_STARTED]: 'Games waiting to be played',
  [PlayStatus.IN_PROGRESS]: 'Currently playing',
  [PlayStatus.COMPLETED]: 'Main story finished',
  [PlayStatus.MASTERED]: 'All major content done',
  [PlayStatus.DOMINATED]: '100% completion',
  [PlayStatus.SHELVED]: 'On hold for later',
  [PlayStatus.DROPPED]: 'No longer playing',
  [PlayStatus.REPLAY]: 'Playing again',
};
