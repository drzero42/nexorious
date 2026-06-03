import { PlayStatus } from '@/types';

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
