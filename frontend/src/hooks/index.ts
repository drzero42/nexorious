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

// Setup hooks
export { useSetupStatus } from './use-setup-status';

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
  useStartEpicAuth,
  useCompleteEpicAuth,
  useCheckEpicAuth,
  useDisconnectEpic,
  useConfigurePSN,
  usePSNStatus,
  useDisconnectPSN,
} from './use-sync';

// Import/Export hooks
export {
  importExportKeys,
  useImportNexorious,
  useExportCollection,
  useDownloadExport,
} from './use-import-export';

// Jobs hooks
export {
  jobsKeys,
  useJobs,
  useJob,
  useJobsSummary,
  useJobItems,
  useActiveJob,
  useCancelJob,
  useDeleteJob,
  usePendingReviewCount,
  useRecentJobs,
  useResolveJobItem,
  useSkipJobItem,
  useRetryFailedItems,
  useRetryJobItem,
} from './use-jobs';

// Import Mapping hooks
export {
  importMappingKeys,
  useImportMappings,
  useImportMapping,
  useLookupImportMapping,
  useCreateImportMapping,
  useUpdateImportMapping,
  useDeleteImportMapping,
  useBatchImportMappings,
} from './use-import-mappings';
