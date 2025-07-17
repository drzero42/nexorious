// Auth store
export { auth } from './auth.svelte.js';
export type { AuthState, User } from './auth.svelte.js';

// Games store
export { games } from './games.svelte.js';
export type { 
  Game, 
  GameSearchFilters, 
  GameListResponse, 
  IGDBGameCandidate, 
  IGDBSearchResponse, 
  GamesState 
} from './games.svelte.js';

// Platforms store
export { platforms } from './platforms.svelte.js';
export type { 
  Platform, 
  Storefront, 
  PlatformCreateRequest, 
  PlatformUpdateRequest, 
  StorefrontCreateRequest, 
  StorefrontUpdateRequest, 
  PlatformsState 
} from './platforms.svelte.js';

// User Games store
export { userGames } from './user-games.svelte.js';
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
  CollectionStats, 
  UserGamesState, 
  OwnershipStatus, 
  PlayStatus 
} from './user-games.svelte.js';

// UI store
export { ui } from './ui.svelte.js';
export type { 
  NotificationType, 
  Notification, 
  Modal, 
  UIState 
} from './ui.svelte.js';

// Search store
export { search } from './search.svelte.js';
export type { 
  SearchQuery, 
  SavedSearch, 
  SearchHistory, 
  SearchState 
} from './search.svelte.js';

// Wishlist store
export { wishlist } from './wishlist.svelte.js';
export type { 
  WishlistItem, 
  WishlistState 
} from './wishlist.svelte.js';