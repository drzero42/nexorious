/**
 * Utility functions for configuring generic import components with specific source behavior.
 * 
 * These helpers make it easier to create actions, stats, and other configuration
 * objects for the generic import components.
 */

import type { ImportGame, ImportGameAction, ImportStatCard, ImportStats, ImportService } from '$lib/types/import';

/**
 * Creates standard import game actions for a given service
 */
export function createImportActions(service: ImportService): ImportGameAction[] {
  return [
    {
      type: 'match',
      label: 'Match',
      icon: '🔗',
      enabled: (game: ImportGame) => !game.igdb_id && !game.ignored,
      handler: async () => {
        // This will be handled by the ImportGameTable component for inline matching
        // The actual implementation is in the component's handleMatch function
      },
      buttonClass: 'btn-secondary',
      title: 'Match to IGDB game'
    },
    {
      type: 'auto-match',
      label: 'Auto-match',
      icon: '🤖',
      enabled: (game: ImportGame) => !game.igdb_id && !game.ignored,
      handler: (game: ImportGame) => service.autoMatchSingleGame(game.id),
      buttonClass: 'btn-primary',
      title: 'Automatically match to IGDB using AI'
    },
    {
      type: 'sync',
      label: 'Sync',
      icon: '➕',
      enabled: (game: ImportGame) => Boolean(game.igdb_id && !game.game_id && !game.ignored),
      handler: (game: ImportGame) => service.syncGameToCollection(game.id),
      buttonClass: 'btn-primary',
      title: 'Add to collection'
    },
    {
      type: 'ignore',
      label: 'Ignore',
      icon: '🚫',
      enabled: (game: ImportGame) => !game.ignored,
      handler: (game: ImportGame) => service.toggleGameIgnored(game.id),
      buttonClass: 'btn-secondary text-gray-600 hover:text-red-600',
      title: 'Mark as ignored'
    },
    {
      type: 'unignore',
      label: 'Unignore',
      icon: '↩️',
      enabled: (game: ImportGame) => game.ignored,
      handler: (game: ImportGame) => service.toggleGameIgnored(game.id),
      buttonClass: 'btn-secondary',
      title: 'Remove from ignored'
    },
    {
      type: 'unmatch',
      label: 'Unmatch',
      icon: '🔓',
      enabled: (game: ImportGame) => game.igdb_id !== null && !game.game_id,
      handler: (game: ImportGame) => service.matchGameToIGDB(game.id, null),
      buttonClass: 'btn-secondary text-gray-600 hover:text-orange-600',
      title: 'Remove IGDB match',
      requiresConfirmation: true,
      confirmationMessage: (game: ImportGame) => 
        `Are you sure you want to unmatch "${game.name}" from IGDB? This will remove the IGDB association and move the game back to "Needs Attention".`
    },
    {
      type: 'unsync',
      label: 'Unsync',
      icon: '📤',
      enabled: (game: ImportGame) => game.game_id !== null,
      handler: (game: ImportGame) => service.unsyncGameFromCollection(game.id),
      buttonClass: 'btn-secondary text-gray-600 hover:text-red-600',
      title: 'Remove from collection (keeps IGDB match)',
      requiresConfirmation: true,
      confirmationMessage: (game: ImportGame) => 
        `Are you sure you want to remove "${game.name}" from your collection? The IGDB match will remain intact and you can re-sync it later.`
    }
  ];
}

/**
 * Creates standard import stat cards from ImportStats
 */
export function createImportStatCards(stats: ImportStats): ImportStatCard[] {
  return [
    {
      label: 'Total Games',
      value: stats.total,
      icon: '📚'
    },
    {
      label: 'Unmatched',
      value: stats.unmatched,
      icon: '❓',
      color: 'text-yellow-600'
    },
    {
      label: 'Matched',
      value: stats.matched,
      icon: '✅',
      color: 'text-blue-600'
    },
    {
      label: 'Ignored',
      value: stats.ignored,
      icon: '🚫',
      color: 'text-gray-600'
    },
    {
      label: 'In Collection',
      value: stats.synced,
      icon: '🔥',
      color: 'text-green-600'
    }
  ];
}

/**
 * Calculates ImportStats from a list of games
 */
export function calculateImportStats(games: ImportGame[]): ImportStats {
  const stats: ImportStats = {
    total: games.length,
    unmatched: 0,
    matched: 0,
    ignored: 0,
    synced: 0
  };

  for (const game of games) {
    if (game.ignored) {
      stats.ignored++;
    } else if (game.game_id) {
      stats.synced++;
    } else if (game.igdb_id) {
      stats.matched++;
    } else {
      stats.unmatched++;
    }
  }

  return stats;
}

/**
 * Filters actions based on game context (useful for different table sections)
 */
export function filterActionsForContext(
  actions: ImportGameAction[], 
  context: 'unmatched' | 'matched' | 'ignored' | 'synced'
): ImportGameAction[] {
  const contextMap = {
    unmatched: ['match', 'auto-match', 'ignore'],
    matched: ['sync', 'ignore', 'unmatch'],
    ignored: ['unignore', 'unmatch'],
    synced: ['unsync']
  };

  const allowedTypes = contextMap[context];
  return actions.filter(action => allowedTypes.includes(action.type));
}

/**
 * Configuration field validation functions
 */
export const validators = {
  steamApiKey: (value: string): string | null => {
    if (value && value.length !== 32) {
      return 'Steam Web API key must be exactly 32 characters';
    } else if (value && !/^[a-zA-Z0-9]+$/.test(value)) {
      return 'Steam Web API key must contain only alphanumeric characters';
    }
    return null;
  },

  steamId: (value: string): string | null => {
    if (value && (value.length !== 17 || !/^\d+$/.test(value))) {
      return 'Steam ID must be exactly 17 digits';
    } else if (value && !value.startsWith('7656119')) {
      return 'Invalid Steam ID format';
    }
    return null;
  }
};

/**
 * Creates Steam-specific configuration fields
 */
export function createSteamConfigFields() {
  return [
    {
      id: 'apiKey',
      label: 'Steam Web API Key',
      type: 'password' as const,
      placeholder: 'Enter your 32-character Steam Web API key',
      required: true,
      validation: validators.steamApiKey
    },
    {
      id: 'steamId',
      label: 'Steam ID',
      type: 'text' as const,
      placeholder: '76561198123456789',
      required: false,
      helpText: '17-digit Steam ID for importing your library. Leave empty if you only want to verify the API key.',
      validation: validators.steamId
    }
  ];
}