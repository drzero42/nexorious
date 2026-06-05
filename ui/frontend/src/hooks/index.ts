// Game hooks
export {
  gameKeys,
  useUserGames,
  useUserGame,
  useUserGameIds,
  useUserGameGenres,
  useFilterOptions,
  useSearchIGDB,
  useCollectionStats,
  useCreateUserGame,
  useUpdateUserGame,
  useDeleteUserGame,
  useImportFromIGDB,
  useBulkUpdateUserGames,
  useBulkDeleteUserGames,
  useAddPlatformToUserGame,
  useUpdatePlatformAssociation,
  useRemovePlatformFromUserGame,
} from './use-games';

// Platform hooks
export {
  platformKeys,
  storefrontKeys,
  usePlatforms,
  useAllPlatforms,
  usePlatform,
  usePlatformStorefronts,
  usePlatformNames,
  useStorefronts,
  useAllStorefronts,
  useStorefront,
  useStorefrontNames,
} from './use-platforms';

// Tag hooks
export {
  tagKeys,
  useTags,
  useAllTags,
  useTag,
  useCreateTag,
  useCreateOrGetTag,
  useUpdateTag,
  useDeleteTag,
  useAssignTagsToGame,
  useRemoveTagsFromGame,
} from './use-tags';

// Sync hooks
export {
  syncKeys,
  useSyncConfigs,
  useSyncConfig,
  useSyncStatus,
  useSyncStatuses,
  useUpdateSyncConfig,
  useTriggerSync,
  useVerifySteamCredentials,
  useDisconnectSteam,
  useSteamConnection,
  useConnectEpic,
  useEpicConnection,
  useDisconnectEpic,
  useGOGConnection,
  useConnectGOG,
  useDisconnectGOG,
  useConfigurePSN,
  usePSNStatus,
  useDisconnectPSN,
  useConnectHumble,
  useHumbleStatus,
  useDisconnectHumble,
  useResetSyncData,
} from './use-sync';

// Import/Export hooks
export {
  importExportKeys,
  useImportNexorious,
  useImportDarkadia,
  useExportCollection,
  useDownloadExport,
} from './use-import-export';

// Health hooks
export { useHealthStatus } from './use-health-status';
export type { HealthStatus } from './use-health-status';

// Jobs hooks
export {
  jobsKeys,
  useJobs,
  useJob,
  useJobsSummary,
  useJobItems,
  useJobTypeStatus,
  useCancelJob,
  useDeleteJob,
  usePendingReviewCount,
  useRecentJobs,
  useRetryFailedItems,
  useRetryJobItem,
  useResolveJobItem,
  useSkipJobItem,
} from './use-jobs';

// Job completion effect hook
export { useJobCompletionEffect } from './use-job-completion-effect';

// Version hooks
export { useVersion } from './use-version';
export type { VersionInfo } from './use-version';

// API key hooks
export { apiKeysKeys, useApiKeys, useCreateApiKey, useRevokeApiKey } from './use-api-keys';
