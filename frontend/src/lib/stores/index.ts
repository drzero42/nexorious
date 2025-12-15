// Auth store
export { auth } from './auth.svelte';
export type { AuthState, User } from './auth.svelte';

// Games store
export { games } from './games.svelte';
export type { 
  Game, 
  GameSearchFilters, 
  GameListResponse, 
  IGDBGameCandidate, 
  IGDBSearchResponse, 
  GamesState 
} from './games.svelte';

// Platforms store
export { platforms } from './platforms.svelte';
export type { 
  Platform, 
  Storefront, 
  PlatformCreateRequest, 
  PlatformUpdateRequest, 
  StorefrontCreateRequest, 
  StorefrontUpdateRequest, 
  PlatformsState 
} from './platforms.svelte';

// User Games store
export { userGames } from './user-games.svelte';
export type { 
  UserGame, 
  UserGamePlatform, 
  UserGameCreateRequest, 
  UserGameUpdateRequest, 
  ProgressUpdateRequest, 
  UserGamePlatformCreateRequest, 
  UserGameFilters, 
  UserGameListResponse, 
  BulkStatusUpdateRequest, 
  BulkDeleteRequest,
  SuccessResponse,
  CollectionStats, 
  UserGamesState, 
  OwnershipStatus, 
  PlayStatus 
} from './user-games.svelte';

// UI store
export { ui } from './ui.svelte';
export type { 
  NotificationType, 
  Notification, 
  Modal, 
  UIState 
} from './ui.svelte';

// Search store
export { search } from './search.svelte';
export type { 
  SearchQuery, 
  SavedSearch, 
  SearchHistory, 
  SearchState 
} from './search.svelte';

// Admin store
export { admin } from './admin.svelte';
export type { 
  AdminUser, 
  SystemStatistics, 
  AdminState 
} from './admin.svelte';

// Steam store
export { steam } from './steam.svelte';
export type { 
  SteamUserInfo,
  SteamConfig,
  SteamVerificationResult,
  VanityUrlResolveResult,
  SteamState
} from './steam.svelte';

// Tags store
export { tags, tagEventBus, DEFAULT_TAG_COLORS } from './tags.svelte';
export type { 
  Tag,
  TagCreateRequest,
  TagUpdateRequest,
  BulkTagAssignRequest,
  BulkTagRemoveRequest,
  TagsState
} from './tags.svelte';

// Darkadia store
export { darkadia } from './darkadia.svelte';
export type {
  DarkadiaState,
  DarkadiaGameStatusFilter
} from './darkadia.svelte';

// App Status store
export { appStatus } from './app-status.svelte';
export type { AppStatusState } from './app-status.svelte';

// Jobs store
export { jobs } from './jobs.svelte';
export type { Job, JobFilters, JobsState } from './jobs.svelte';
export { JobType, JobSource, JobStatus } from './jobs.svelte';

// Review store
export { review } from './review.svelte';
export type {
  ReviewItem,
  ReviewItemDetail,
  ReviewSummary,
  ReviewFilters,
  ReviewState
} from './review.svelte';
export { ReviewItemStatus } from './review.svelte';

// Re-export IGDBCandidate from types for review pages
export type { IGDBCandidate } from '$lib/types/jobs';

// Sync store
export { sync } from './sync.svelte';
export type { SyncConfig, SyncConfigUpdateRequest, SyncStatusResponse, SyncState } from './sync.svelte';
export { SyncFrequency, SyncPlatform } from './sync.svelte';

// WebSocket store
export { websocket } from './websocket.svelte';
export type {
  WebSocketStatus,
  WebSocketState,
  WebSocketMessage,
  ConnectionMessage,
  JobWebSocketMessage,
  WebSocketEventCallback
} from './websocket.svelte';
export { WebSocketEventType } from './websocket.svelte';
