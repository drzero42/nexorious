/**
 * Types for sync configuration and status management.
 */

export enum SyncPlatform {
  STEAM = 'steam',
  EPIC = 'epic',
  GOG = 'gog',
}

export const SUPPORTED_SYNC_PLATFORMS: SyncPlatform[] = [SyncPlatform.STEAM];

export enum SyncFrequency {
  MANUAL = 'manual',
  HOURLY = 'hourly',
  DAILY = 'daily',
  WEEKLY = 'weekly',
}

export interface SyncConfig {
  id: string;
  userId: string;
  platform: SyncPlatform;
  frequency: SyncFrequency;
  autoAdd: boolean;
  enabled: boolean;
  lastSyncedAt: string | null;
  createdAt: string;
  updatedAt: string;
  isConfigured: boolean;
}

export interface SyncConfigUpdateData {
  frequency?: SyncFrequency;
  autoAdd?: boolean;
  enabled?: boolean;
}

export interface SyncStatus {
  platform: SyncPlatform;
  isSyncing: boolean;
  lastSyncedAt: string | null;
  activeJobId: string | null;
}

export interface ManualSyncResponse {
  message: string;
  jobId: string;
  platform: string;
  status: string;
}

export interface IgnoredGame {
  id: string;
  source: string;
  externalId: string;
  title: string;
  createdAt: string;
}

// Helper to get human-readable frequency label
export function getSyncFrequencyLabel(frequency: SyncFrequency): string {
  const labels: Record<SyncFrequency, string> = {
    [SyncFrequency.MANUAL]: 'Manual',
    [SyncFrequency.HOURLY]: 'Every hour',
    [SyncFrequency.DAILY]: 'Daily',
    [SyncFrequency.WEEKLY]: 'Weekly',
  };
  return labels[frequency];
}

// Helper to get platform display info
export function getPlatformDisplayInfo(platform: SyncPlatform): {
  name: string;
  color: string;
  bgColor: string;
} {
  const info: Record<SyncPlatform, { name: string; color: string; bgColor: string }> = {
    [SyncPlatform.STEAM]: {
      name: 'Steam',
      color: 'text-[#1b2838]',
      bgColor: 'bg-[#1b2838]/10 dark:bg-[#1b2838]/30',
    },
    [SyncPlatform.EPIC]: {
      name: 'Epic Games',
      color: 'text-gray-800 dark:text-gray-200',
      bgColor: 'bg-gray-100 dark:bg-gray-700',
    },
    [SyncPlatform.GOG]: {
      name: 'GOG',
      color: 'text-purple-700 dark:text-purple-400',
      bgColor: 'bg-purple-100 dark:bg-purple-900/30',
    },
  };
  return info[platform];
}

export interface SteamVerifyRequest {
  steamId: string;
  webApiKey: string;
}

export interface SteamVerifyResponse {
  valid: boolean;
  steamUsername: string | null;
  error: string | null;
}

export interface SteamConnectionInfo {
  configured: boolean;
  steamId: string | null;
  steamUsername: string | null;
}

// Error message mapping for Steam verification
export const STEAM_VERIFY_ERROR_MESSAGES: Record<string, string> = {
  invalid_api_key: 'Invalid API key. Please check and try again.',
  invalid_steam_id: 'Steam ID not found. Please verify the number.',
  private_profile: 'Your Steam profile or game details are set to private. Please make them public and try again.',
  rate_limited: 'Steam API rate limit reached. Please try again in a few minutes.',
  network_error: 'Could not connect to Steam. Please try again.',
};
