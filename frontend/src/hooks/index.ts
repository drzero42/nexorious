// Game hooks
export {
  gameKeys,
  useUserGames,
  useUserGame,
  useUserGameIds,
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
  useIgnoredGames,
  useUpdateSyncConfig,
  useTriggerSync,
  useUnignoreGame,
} from './use-sync';

// Import/Export hooks
export {
  importExportKeys,
  useImportNexorious,
  useImportDarkadia,
  useExportCollection,
  useExportWishlist,
  useDownloadExport,
} from './use-import-export';

// Jobs hooks
export {
  jobsKeys,
  useJobs,
  useJob,
  useCancelJob,
  useDeleteJob,
  useConfirmJob,
} from './use-jobs';

// Review hooks
export {
  reviewKeys,
  useReviewItems,
  useReviewItem,
  useReviewSummary,
  useReviewCountsByType,
  usePlatformSummary,
  useMatchReviewItem,
  useSkipReviewItem,
  useKeepReviewItem,
  useRemoveReviewItem,
  useFinalizeImport,
} from './use-review';
