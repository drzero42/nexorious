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
  useMoveToLibrary,
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

export { useJobSourceLabel } from './use-job-source-label';

// Tag hooks
export {
  tagKeys,
  useTags,
  useAllTags,
  useTag,
  useCreateTag,
  useUpdateTag,
  useDeleteTag,
  useReplaceUserGameTags,
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
  useConnectEpicGamesStore,
  useEpicGamesStoreConnection,
  useDisconnectEpicGamesStore,
  useGOGConnection,
  useConnectGOG,
  useDisconnectGOG,
  useConfigurePlaystationStore,
  usePlaystationStoreStatus,
  useDisconnectPlaystationStore,
  useConnectHumble,
  useHumbleStatus,
  useDisconnectHumble,
  useResetSyncData,
} from './use-sync';

// Import/Export hooks
export {
  importExportKeys,
  useImportNexorious,
  useImportSources,
  useImportSource,
  useInspectCsv,
  useImportCsv,
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

// Settings hooks
export { useSettings, useUpdateSettings } from './use-settings';

// Date format hook
export { useDateFormat } from './use-date-format';

// Theme preference hook
export { useThemePreference } from './use-theme-preference';

// Responsive / media-query hook
export { useMediaQuery } from './use-media-query';

// Docs hooks
export { docKeys, useDoc } from './use-docs';

// Changelog hooks
export { useChangelogUnseen, useChangelogContent, changelogKeys } from './use-changelog';

// Pool hooks
export {
  poolKeys,
  usePools,
  usePool,
  usePoolSuggestions,
  useGamePoolMemberships,
  useCreatePool,
  useUpdatePool,
  useDeletePool,
  useReorderPools,
  useAddPoolGame,
  useBulkAddPoolGames,
  useRemovePoolGame,
  useSetQueue,
} from './use-pools';

// Library Health (smells) hooks
export {
  smellKeys,
  useSmellSummary,
  useSmellItems,
  useIgnoredItems,
  useApplySmell,
  useApplyAllSmell,
  useIgnoreSmell,
  useRestoreSmell,
} from './use-library-health';
