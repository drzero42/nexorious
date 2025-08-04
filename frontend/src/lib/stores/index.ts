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

