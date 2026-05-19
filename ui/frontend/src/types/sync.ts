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
  SyncPlatform.GOG,
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
      iconUrl: '/logos/storefronts/steam/steam-icon-light.svg',
    },
    [SyncPlatform.EPIC]: {
      name: 'Epic Games',
      color: 'text-gray-800 dark:text-gray-200',
      bgColor: 'bg-gray-100 dark:bg-gray-700',
      iconUrl: '/logos/storefronts/epic-games-store/epic-games-store-icon-light.svg',
    },
    [SyncPlatform.GOG]: {
      name: 'GOG',
      color: 'text-purple-700 dark:text-purple-400',
      bgColor: 'bg-purple-100 dark:bg-purple-900/30',
      iconUrl: '/logos/storefronts/gog/gog-icon-light.svg',
    },
    [SyncPlatform.PSN]: {
      name: 'PlayStation Network',
      color: 'text-[#003087]',
      bgColor: 'bg-[#003087]/10 dark:bg-[#003087]/30',
      iconUrl: '/logos/storefronts/playstation-store/playstation-store-icon-light.svg',
    },
  };
  return info[platform];
}

export interface SteamVerifyResponse {
  valid: boolean;
  steamUsername: string | null;
  error: string | null;
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

/**
 * Legendary CLI's hardcoded OAuth URL. Users visit this in a browser, log in
 * to Epic, and receive a short-lived authorization code which the backend
 * exchanges via `legendary auth --code <code>`.
 */
export const EPIC_AUTH_URL =
  'https://www.epicgames.com/id/api/redirect?clientId=34a02cf8f4414e29b15921876da36f9a&responseType=code';

export const GOG_AUTH_URL =
  'https://login.gog.com/auth?client_id=46899977096215655&redirect_uri=https%3A%2F%2Fembed.gog.com%2Fon_login_success%3Forigin%3Dclient&response_type=code&layout=client2';

export interface EpicConnectResponse {
  displayName: string;
  accountId: string;
}

export interface EpicConnectionResponse {
  connected: boolean;
  disabled: boolean;
  displayName?: string;
  accountId?: string;
  /** "legendary_not_configured" when disabled=true (LEGENDARY_WORK_DIR unset). */
  reason?: string;
}

// GOG Auth Types
export interface GOGConnectResponse {
  username: string;
  userId: string;
}

export interface GOGConnectionResponse {
  connected: boolean;
  username?: string;
  userId?: string;
  authUrl?: string;
}

// PSN Auth Types
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

export interface ExternalGame {
  id: string;
  storefront: string;
  external_id: string;
  title: string;
  resolved_igdb_id: number | null;
  is_skipped: boolean;
  is_available: boolean;
  is_subscription: boolean;
  playtime_hours: number;
  has_user_game: boolean;
  user_game_id: string | null;
  igdb_title: string | null;
  user_game_other_platform_count: number;
}
