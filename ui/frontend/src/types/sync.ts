/**
 * Types for sync configuration and status management.
 */

export enum SyncStorefront {
  STEAM = 'steam',
  EPIC_GAMES_STORE = 'epic-games-store',
  GOG = 'gog',
  PLAYSTATION_STORE = 'playstation-store',
  HUMBLE = 'humble-bundle',
}

export const SUPPORTED_SYNC_STOREFRONTS: SyncStorefront[] = [
  SyncStorefront.STEAM,
  SyncStorefront.EPIC_GAMES_STORE,
  SyncStorefront.GOG,
  SyncStorefront.PLAYSTATION_STORE,
  SyncStorefront.HUMBLE,
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
  storefront: SyncStorefront;
  frequency: SyncFrequency;
  lastSyncedAt: string | null;
  createdAt: string;
  updatedAt: string;
  isConfigured: boolean;
}

export interface SyncConfigUpdateData {
  frequency?: SyncFrequency;
}

export interface SyncStatus {
  storefront: SyncStorefront;
  isSyncing: boolean;
  lastSyncedAt: string | null;
  activeJobId: string | null;
  requiresReauth?: boolean;
  authExpired?: boolean;
  externalGameCount: number;
}

export interface ManualSyncResponse {
  message: string;
  jobId: string;
  storefront: string;
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

export interface SteamVerifyResponse {
  valid: boolean;
  steamUsername: string | null;
  error: string | null;
}

// Error message mapping for Steam verification
export const STEAM_VERIFY_ERROR_MESSAGES: Record<string, string> = {
  invalid_api_key: 'Invalid API key. Please check and try again.',
  invalid_steam_id: 'Steam ID not found. Please verify the number.',
  private_profile:
    'Your Steam profile or game details are set to private. Please make them public and try again.',
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
  credentialsError?: boolean;
  displayName?: string;
  /** Machine-readable cause when disabled=true, e.g. "legendary_not_configured". */
  reason?: string;
}

// GOG Auth Types
export interface GOGConnectResponse {
  username: string;
}

export interface GOGConnectionResponse {
  connected: boolean;
  credentialsError?: boolean;
  username?: string;
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
  onlineId: string | null;
  credentialsError: boolean;
}

// Humble Bundle Auth Types
export interface HumbleConnectResponse {
  valid: boolean;
  error: string | null;
}

export interface HumbleStatusResponse {
  configured: boolean;
  credentialsError: boolean;
}

// Steam Connection Types
export interface SteamConnectionData {
  connected: boolean;
  credentialsError: boolean;
  username: string;
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
  has_user_game: boolean;
  user_game_id: string | null;
  igdb_title: string | null;
  user_game_other_platform_count: number;
  sync_status: 'needs_review' | 'failed' | 'matched' | 'skipped' | 'unmatched';
  failed_job_item_id: string | null;
  platforms: string[];
}
