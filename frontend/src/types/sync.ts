/**
 * Types for sync configuration and status management.
 */

export enum SyncPlatform {
  STEAM = 'steam',
  EPIC = 'epic',
  GOG = 'gog',
  PSN = 'psn',
}

export const SUPPORTED_SYNC_PLATFORMS: SyncPlatform[] = [
  SyncPlatform.STEAM,
  SyncPlatform.EPIC,
  SyncPlatform.PSN,
];

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
  lastSyncedAt: string | null;
  createdAt: string;
  updatedAt: string;
  isConfigured: boolean;
}

export interface SyncConfigUpdateData {
  frequency?: SyncFrequency;
  autoAdd?: boolean;
}

export interface SyncStatus {
  platform: SyncPlatform;
  isSyncing: boolean;
  lastSyncedAt: string | null;
  activeJobId: string | null;
  requiresReauth?: boolean;
  authExpired?: boolean;
}

export interface ManualSyncResponse {
  message: string;
  jobId: string;
  platform: string;
  status: string;
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
  iconUrl: string;
} {
  const info: Record<SyncPlatform, { name: string; color: string; bgColor: string; iconUrl: string }> = {
    [SyncPlatform.STEAM]: {
      name: 'Steam',
      color: 'text-[#1b2838]',
      bgColor: 'bg-[#1b2838]/10 dark:bg-[#1b2838]/30',
      iconUrl: '/static/logos/storefronts/steam/steam-icon-light.svg',
    },
    [SyncPlatform.EPIC]: {
      name: 'Epic Games',
      color: 'text-gray-800 dark:text-gray-200',
      bgColor: 'bg-gray-100 dark:bg-gray-700',
      iconUrl: '/static/logos/storefronts/epic-games-store/epic-games-store-icon-light.svg',
    },
    [SyncPlatform.GOG]: {
      name: 'GOG',
      color: 'text-purple-700 dark:text-purple-400',
      bgColor: 'bg-purple-100 dark:bg-purple-900/30',
      iconUrl: '/static/logos/storefronts/gog/gog-icon-light.svg',
    },
    [SyncPlatform.PSN]: {
      name: 'PlayStation Network',
      color: 'text-[#003087]',
      bgColor: 'bg-[#003087]/10 dark:bg-[#003087]/30',
      iconUrl: '/static/logos/storefronts/playstation-store/playstation-store-icon-light.svg',
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

// Epic Auth Types
export interface EpicAuthStartResponse {
  authUrl: string;
  instructions: string;
}

export interface EpicAuthCompleteRequest {
  code: string;
}

export interface EpicAuthCompleteResponse {
  valid: boolean;
  displayName: string | null;
  error: string | null;
}

export interface EpicAuthCheckResponse {
  isAuthenticated: boolean;
  displayName: string | null;
}

export interface EpicConnectionInfo {
  configured: boolean;
  displayName: string | null;
  accountId: string | null;
}

// Error message mapping for Epic auth
export const EPIC_AUTH_ERROR_MESSAGES: Record<string, string> = {
  invalid_code: 'Invalid authorization code. Please try again.',
  network_error: 'Could not connect to Epic Games. Please try again.',
  expired_code: 'Authorization code expired. Please request a new one.',
};

// PSN Auth Types
export interface PSNConfigureRequest {
  npssoToken: string;
}

export interface PSNConfigureResponse {
  valid: boolean;
  accountId: string | null;
  onlineId: string | null;
  error: string | null;
}

export interface PSNStatusResponse {
  configured: boolean;
  accountId: string | null;
  onlineId: string | null;
  tokenExpired: boolean;
}

export interface PSNConnectionInfo {
  configured: boolean;
  accountId: string | null;
  onlineId: string | null;
  tokenExpired: boolean;
}

// Error message mapping for PSN configuration
export const PSN_CONFIG_ERROR_MESSAGES: Record<string, string> = {
  invalid_token: 'Invalid NPSSO token. Please check and try again.',
  expired_token: 'NPSSO token has expired. Please obtain a new one.',
  network_error: 'Could not connect to PlayStation Network. Please try again.',
  rate_limited: 'PSN API rate limit reached. Please try again in a few minutes.',
};
