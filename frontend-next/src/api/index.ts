// Re-export auth API functions
export {
  login,
  getMe,
  refreshToken,
  checkSetupStatus,
  createInitialAdmin,
  changeUsername,
  changePassword,
  checkUsernameAvailability,
  updatePreferences,
} from './auth';

// Re-export games API functions
export {
  getUserGames,
  getUserGame,
  createUserGame,
  updateUserGame,
  deleteUserGame,
  searchIGDB,
  importFromIGDB,
  bulkUpdateUserGames,
  bulkDeleteUserGames,
  getCollectionStats,
} from './games';

// Re-export games API types
export type {
  GetUserGamesParams,
  UserGameCreateData,
  UserGameUpdateData,
  BulkUpdateData,
  UserGamesListResponse,
} from './games';

// Re-export platforms API functions
export {
  getPlatforms,
  getAllPlatforms,
  getPlatform,
  getPlatformStorefronts,
  getStorefronts,
  getAllStorefronts,
  getStorefront,
  getPlatformNames,
  getStorefrontNames,
} from './platforms';

// Re-export platforms API types
export type {
  GetPlatformsParams,
  GetStorefrontsParams,
  PlatformsListResponse,
  StorefrontsListResponse,
} from './platforms';

// Re-export tags API functions
export {
  getTags,
  getAllTags,
  getTag,
  createTag,
  createOrGetTag,
  updateTag,
  deleteTag,
  assignTagsToGame,
  removeTagsFromGame,
  bulkAssignTags,
  bulkRemoveTags,
} from './tags';

// Re-export tags API types
export type {
  GetTagsParams,
  TagCreateData,
  TagUpdateData,
  TagsListResponse,
  TagCreateOrGetResponse,
  TagAssignResponse,
  TagRemoveResponse,
} from './tags';

// Re-export sync API functions
export {
  getSyncConfigs,
  getSyncConfig,
  updateSyncConfig,
  triggerSync,
  getSyncStatus,
  getIgnoredGames,
  unignoreGame,
} from './sync';

// Re-export sync API types
export type { SyncConfigsResponse, IgnoredGamesResponse } from './sync';

// Re-export client utilities
export { ApiErrorException, setAuthHandlers } from './client';
