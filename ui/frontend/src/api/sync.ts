import { api } from './client';
import type {
  SyncConfig,
  SyncConfigUpdateData,
  SyncStatus,
  ManualSyncResponse,
  SyncStorefront,
  SyncFrequency,
  SteamVerifyResponse,
  SteamConnectionData,
  EpicConnectResponse,
  EpicConnectionResponse,
  GOGConnectResponse,
  GOGConnectionResponse,
  PSNConfigureResponse,
  PSNStatusResponse,
  ExternalGame,
} from '@/types';

// ============================================================================
// API Response Types (snake_case from backend)
// ============================================================================

interface SyncConfigApiResponse {
  id: string;
  user_id: string;
  storefront: string;
  frequency: string;
  last_synced_at: string | null;
  created_at: string;
  updated_at: string;
  is_configured: boolean;
}

interface SyncConfigListApiResponse {
  configs: SyncConfigApiResponse[];
  total: number;
}

interface SyncStatusApiResponse {
  storefront: string;
  is_syncing: boolean;
  last_synced_at: string | null;
  active_job_id: string | null;
  external_game_count: number;
}

interface ManualSyncApiResponse {
  message: string;
  job_id: string;
  storefront: string;
  status: string;
}

// ============================================================================
// Response Types
// ============================================================================

interface SyncConfigsResponse {
  configs: SyncConfig[];
  total: number;
}

// ============================================================================
// Transformation Functions
// ============================================================================

function transformSyncConfig(apiConfig: SyncConfigApiResponse): SyncConfig {
  return {
    id: apiConfig.id,
    userId: apiConfig.user_id,
    storefront: apiConfig.storefront as SyncStorefront,
    frequency: apiConfig.frequency as SyncFrequency,
    lastSyncedAt: apiConfig.last_synced_at,
    createdAt: apiConfig.created_at,
    updatedAt: apiConfig.updated_at,
    isConfigured: apiConfig.is_configured,
  };
}

function transformSyncStatus(apiStatus: SyncStatusApiResponse): SyncStatus {
  return {
    storefront: apiStatus.storefront as SyncStorefront,
    isSyncing: apiStatus.is_syncing,
    lastSyncedAt: apiStatus.last_synced_at,
    activeJobId: apiStatus.active_job_id,
    externalGameCount: apiStatus.external_game_count ?? 0,
  };
}

function transformManualSyncResponse(apiResponse: ManualSyncApiResponse): ManualSyncResponse {
  return {
    message: apiResponse.message,
    jobId: apiResponse.job_id,
    storefront: apiResponse.storefront,
    status: apiResponse.status,
  };
}

// ============================================================================
// API Functions
// ============================================================================

/**
 * Get all sync configurations for the current user.
 */
export async function getSyncConfigs(): Promise<SyncConfigsResponse> {
  const response = await api.get<SyncConfigListApiResponse>('/sync/config');
  return {
    configs: response.configs.map(transformSyncConfig),
    total: response.total,
  };
}

/**
 * Get sync configuration for a specific platform.
 */
export async function getSyncConfig(platform: SyncStorefront): Promise<SyncConfig> {
  const response = await api.get<SyncConfigApiResponse>(`/sync/config/${platform}`);
  return transformSyncConfig(response);
}

/**
 * Update sync configuration for a specific platform.
 */
export async function updateSyncConfig(
  platform: SyncStorefront,
  data: SyncConfigUpdateData,
): Promise<SyncConfig> {
  const requestBody: Record<string, unknown> = {};

  if (data.frequency !== undefined) {
    requestBody.frequency = data.frequency;
  }

  const response = await api.put<SyncConfigApiResponse>(`/sync/config/${platform}`, requestBody);
  return transformSyncConfig(response);
}

/**
 * Trigger a manual sync for a specific platform.
 */
export async function triggerSync(platform: SyncStorefront): Promise<ManualSyncResponse> {
  const response = await api.post<ManualSyncApiResponse>(`/sync/${platform}`);
  return transformManualSyncResponse(response);
}

/**
 * Get the current sync status for a platform.
 */
export async function getSyncStatus(platform: SyncStorefront): Promise<SyncStatus> {
  const response = await api.get<SyncStatusApiResponse>(`/sync/${platform}/status`);
  return transformSyncStatus(response);
}

// ============================================================================
// Steam Verification Types
// ============================================================================

interface SteamVerifyApiRequest {
  steam_id: string;
  web_api_key: string;
}

interface SteamVerifyApiResponse {
  valid: boolean;
  steam_username: string | null;
  error: string | null;
}

interface SteamConnectionApiResponse {
  connected: boolean;
  credentials_error?: boolean;
  username?: string;
}

// ============================================================================
// Epic Auth API Types
// ============================================================================

interface EpicConnectApiRequest {
  auth_code: string;
}

interface EpicConnectApiResponse {
  display_name: string;
  account_id: string;
}

interface EpicConnectionApiResponse {
  connected: boolean;
  disabled: boolean;
  credentials_error?: boolean;
  display_name?: string;
  reason?: string;
}

// ============================================================================
// Steam Verification Functions
// ============================================================================

/**
 * Verify Steam credentials before saving.
 */
export async function verifySteamCredentials(
  steamId: string,
  webApiKey: string,
): Promise<SteamVerifyResponse> {
  const response = await api.post<SteamVerifyApiResponse>('/sync/steam/verify', {
    steam_id: steamId,
    web_api_key: webApiKey,
  } as SteamVerifyApiRequest);

  return {
    valid: response.valid,
    steamUsername: response.steam_username,
    error: response.error,
  };
}

/**
 * Disconnect Steam integration.
 */
export async function disconnectSteam(): Promise<void> {
  await api.delete('/sync/steam/connection');
}

// ============================================================================
// Epic Auth Functions
// ============================================================================

/**
 * Connect Epic Games Store by exchanging the legendary auth code for an
 * access/refresh token. Backend runs `legendary auth --code <code>` and
 * persists the resulting state snapshot.
 */
export async function connectEpic(authCode: string): Promise<EpicConnectResponse> {
  const response = await api.post<EpicConnectApiResponse>('/sync/epic/connect', {
    auth_code: authCode,
  } as EpicConnectApiRequest);
  return {
    displayName: response.display_name,
    accountId: response.account_id,
  };
}

/**
 * Get current Epic Games Store connection status.
 */
export async function getEpicConnection(): Promise<EpicConnectionResponse> {
  const response = await api.get<EpicConnectionApiResponse>('/sync/epic/connection');
  return {
    connected: response.connected,
    disabled: response.disabled,
    credentialsError: response.credentials_error ?? false,
    displayName: response.display_name,
    reason: response.reason,
  };
}

/**
 * Disconnect Epic Games Store. Clears legendary state and per-user working dir.
 */
export async function disconnectEpic(): Promise<void> {
  await api.delete('/sync/epic/connection');
}

// ============================================================================
// GOG Auth API Types
// ============================================================================

interface GOGConnectApiRequest {
  auth_code: string;
}

interface GOGConnectApiResponse {
  username: string;
  user_id: string;
}

interface GOGConnectionApiResponse {
  connected: boolean;
  credentials_error?: boolean;
  username?: string;
  auth_url?: string;
}

// ============================================================================
// GOG Auth Functions
// ============================================================================

export async function connectGOG(authCode: string): Promise<GOGConnectResponse> {
  const response = await api.post<GOGConnectApiResponse>('/sync/gog/connect', {
    auth_code: authCode,
  } as GOGConnectApiRequest);
  return {
    username: response.username,
    userId: response.user_id,
  };
}

export async function getGOGConnection(): Promise<GOGConnectionResponse> {
  const response = await api.get<GOGConnectionApiResponse>('/sync/gog/connection');
  return {
    connected: response.connected,
    credentialsError: response.credentials_error ?? false,
    username: response.username,
    authUrl: response.auth_url,
  };
}

export async function disconnectGOG(): Promise<void> {
  await api.delete('/sync/gog/connection');
}

// ============================================================================
// PSN API Types
// ============================================================================

interface PSNConfigureApiRequest {
  npsso_token: string;
}

interface PSNConfigureApiResponse {
  success: boolean;
  online_id: string | null;
  account_id: string | null;
  region: string | null;
  message: string;
}

interface PSNStatusApiResponse {
  is_configured: boolean;
  credentials_error?: boolean;
  online_id: string | null;
}

// ============================================================================
// PSN Functions
// ============================================================================

/**
 * Configure PSN with NPSSO token.
 */
export async function configurePSN(npssoToken: string): Promise<PSNConfigureResponse> {
  const response = await api.post<PSNConfigureApiResponse>('/sync/psn/configure', {
    npsso_token: npssoToken,
  } as PSNConfigureApiRequest);

  return {
    valid: response.success,
    accountId: response.account_id,
    onlineId: response.online_id,
    error: response.success ? null : response.message,
  };
}

/**
 * Get PSN connection status.
 */
export async function getPSNStatus(): Promise<PSNStatusResponse> {
  const response = await api.get<PSNStatusApiResponse>('/sync/psn/connection');

  return {
    configured: response.is_configured,
    onlineId: response.online_id,
    credentialsError: response.credentials_error ?? false,
  };
}

/**
 * Get Steam connection status.
 */
export async function getSteamConnection(): Promise<SteamConnectionData> {
  const response = await api.get<SteamConnectionApiResponse>('/sync/steam/connection');
  return {
    connected: response.connected,
    credentialsError: response.credentials_error ?? false,
    username: response.username ?? '',
  };
}

/**
 * Disconnect PSN integration.
 */
export async function disconnectPSN(): Promise<void> {
  await api.delete('/sync/psn/connection');
}

// ============================================================================
// External Games
// ============================================================================

export async function getExternalGames(platform: SyncStorefront): Promise<ExternalGame[]> {
  const response = await api.get<ExternalGame[]>(`/sync/${platform}/external-games`);
  return response;
}

export async function skipExternalGame(id: string): Promise<void> {
  await api.post(`/sync/ignored/${id}`);
}

export async function unskipExternalGame(id: string): Promise<void> {
  await api.delete(`/sync/ignored/${id}`);
}

export async function rematchExternalGame(
  id: string,
  igdbId: number,
  orphanAction?: 'keep' | 'remove',
): Promise<void> {
  await api.post(`/sync/external-games/${id}/rematch`, {
    igdb_id: igdbId,
    orphan_action: orphanAction ?? '',
  });
}

export async function resetSyncData(platform: SyncStorefront): Promise<void> {
  await api.delete(`/sync/${platform}/data`);
}

export async function retryFailedExternalGames(storefront: SyncStorefront): Promise<void> {
  await api.post(`/sync/${storefront}/external-games/retry-failed`);
}
