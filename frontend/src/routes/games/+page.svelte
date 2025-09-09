<script lang="ts">
 import { userGames, platforms, ui, tags } from '$lib/stores';
 import { onMount, onDestroy } from 'svelte';
 import { goto } from '$app/navigation';
import { page } from '$app/stores';
 import { RouteGuard, Pagination, PlatformBadges, PlatformSelector } from '$lib/components';
import { TagFilter } from '$lib/components/tags';
 import PlatformRemovalSelector from '$lib/components/PlatformRemovalSelector.svelte';
 import { resolveImageUrl } from '$lib/utils/image-url';
 import type { UserGameFilters } from '$lib/stores';
 import { PlayStatus, OwnershipStatus, type BulkStatusUpdateRequest, type BulkDeleteRequest, type BulkAddPlatformRequest, type UserGamePlatformCreateRequest } from '$lib/stores/user-games.svelte';

 let viewMode = $state<'grid' | 'list'>('grid');
 let searchQuery = $state('');
 let selectedPlatform = $state('');
 let selectedStorefront = $state('');
 let selectedStatus = $state<PlayStatus | ''>('');
 let selectedOwnershipStatus = $state<OwnershipStatus | ''>('');
 let lovedOnly = $state(false);
 let hasNotesOnly = $state(false);
 let ratingMin = $state('');
 let ratingMax = $state('');
 let sortBy = $state('title');
 let sortOrder = $state<'asc' | 'desc'>('asc');

 // Bulk selection state
 let selectedGameIds = $state<Set<string>>(new Set());
 let isSelectingAll = $state(false);
 // let showBulkActions = false; // Not yet implemented
 let showBulkModal = $state(false);

 // Computed state for bulk selection mode
 const isBulkSelectionMode = $derived(selectedGameIds.size > 0);

// Real-time update states
const hasOptimisticUpdates = $derived(userGames.entityState?.optimisticUpdates?.isPending ?? false);
const isBulkProcessing = $derived(userGames.entityState?.bulkOperations?.isProcessing ?? false);
let recentlyUpdatedGameIds = $state<Set<string>>(new Set());
let updateTimeout: NodeJS.Timeout | undefined;

 // Bulk operations modal state
 let bulkStatus = $state('');
 let bulkRating = $state('');
 let bulkIsLoved = $state(false);
 let bulkOwnershipStatus = $state('');
 let isBulkUpdating = $state(false);
let showDeleteConfirmation = $state(false);
let isDeletingBulk = $state(false);

// Bulk platform operations state
let showBulkPlatformAddModal = $state(false);
let showBulkPlatformRemoveModal = $state(false);
let isProcessingBulkPlatforms = $state(false);

// Platform selection for bulk operations
let bulkSelectedPlatforms = $state<Set<string>>(new Set());
let bulkPlatformStorefronts = $state<Map<string, Set<string>>>(new Map());
let bulkPlatformStoreUrls = $state<Map<string, string>>(new Map());

// Platform removal selection
let availablePlatformAssociations = $state<Array<{
  platformId: string;
  platformName: string;
  storefrontId?: string;
  storefrontName?: string;
  associationIds: string[];
  platformIconUrl?: string;
}>>([]);
let selectedAssociationIds = $state<Set<string>>(new Set());

 // Local state for debounced search
 let searchTimeout: ReturnType<typeof setTimeout>;

 let eventCleanupFunctions: Array<() => void> = [];

 // State tracking for effects to prevent infinite loops
 let prevFilters = $state({
   selectedStatus: '',
   selectedPlatform: '',
   selectedStorefront: '',
   selectedOwnershipStatus: '',
   lovedOnly: false,
   hasNotesOnly: false,
   ratingMin: '',
   ratingMax: '',
   sortBy: 'title',
   sortOrder: 'asc'
 });
 let prevSearchQuery = $state('');

 // Derived stores for better type safety
 const platformsData = $derived(platforms.value || { platforms: [], storefronts: [], isLoading: false, error: null });
 
 onMount(() => {
  // Load user games and platforms - authentication is handled by RouteGuard
  loadGames();
  platforms.fetchAll();
  
  // Load tags for filtering
  if (tags.value.tags.length === 0) {
    tags.fetchTags();
  }
  
  // Set up event listeners for cross-view real-time updates
  setupRealTimeUpdates();
 });
 
 onDestroy(() => {
  // Clean up event listeners
  eventCleanupFunctions.forEach(cleanup => cleanup());
 });
 
 function setupRealTimeUpdates() {
  // Listen for individual game updates
  const handleGameUpdated = (data: any) => {
    console.log('Real-time update: Game updated', data);
    highlightUpdatedGame(data.id);
  };
  
  const handlePlatformAdded = (data: any) => {
    console.log('Real-time update: Platform added', data);
    highlightUpdatedGame(data.gameId);
  };
  
  const handlePlatformRemoved = (data: any) => {
    console.log('Real-time update: Platform removed', data);
    highlightUpdatedGame(data.gameId);
  };
  
  const handleBulkUpdated = (data: any) => {
    console.log('Real-time update: Bulk updated', data);
    data.gameIds?.forEach((gameId: string) => highlightUpdatedGame(gameId));
  };
  
  // Register event listeners
  userGames.on('user-game-updated', handleGameUpdated);
  userGames.on('platform-added', handlePlatformAdded);
  userGames.on('platform-removed', handlePlatformRemoved);
  userGames.on('bulk-updated', handleBulkUpdated);
  userGames.on('bulk-platforms-added', handleBulkUpdated);
  userGames.on('bulk-platforms-removed', handleBulkUpdated);
  
  // Store cleanup functions
  eventCleanupFunctions.push(
    () => userGames.off('user-game-updated', handleGameUpdated),
    () => userGames.off('platform-added', handlePlatformAdded),
    () => userGames.off('platform-removed', handlePlatformRemoved),
    () => userGames.off('bulk-updated', handleBulkUpdated),
    () => userGames.off('bulk-platforms-added', handleBulkUpdated),
    () => userGames.off('bulk-platforms-removed', handleBulkUpdated)
  );
 }
 
 function highlightUpdatedGame(gameId: string) {
  recentlyUpdatedGameIds.add(gameId);
  recentlyUpdatedGameIds = new Set(recentlyUpdatedGameIds); // Trigger reactivity
  
  // Clear the highlight after a short delay
  clearTimeout(updateTimeout);
  updateTimeout = setTimeout(() => {
    recentlyUpdatedGameIds.delete(gameId);
    recentlyUpdatedGameIds = new Set(recentlyUpdatedGameIds);
  }, 3000);
 }

 // Parse tag IDs from URL parameters
 const selectedTagIds = $derived(() => {
   const tagParam = $page.url.searchParams.get('tag');
   if (!tagParam) return [];
   return tagParam.split(',').filter(Boolean);
 });

 // Build filters based on current selections
 const filters = $derived(() => {
  const baseFilters: UserGameFilters = {
    sort_by: sortBy,
    sort_order: sortOrder
  };
  
  if (searchQuery) baseFilters.q = searchQuery;
  if (selectedStatus) baseFilters.play_status = selectedStatus as PlayStatus;
  if (selectedOwnershipStatus) baseFilters.ownership_status = selectedOwnershipStatus as OwnershipStatus;
  if (selectedPlatform) baseFilters.platform_id = selectedPlatform;
  if (selectedStorefront) baseFilters.storefront_id = selectedStorefront;
  if (lovedOnly) baseFilters.is_loved = true;
  if (hasNotesOnly) baseFilters.has_notes = true;
  if (ratingMin) baseFilters.rating_min = parseInt(ratingMin);
  if (ratingMax) baseFilters.rating_max = parseInt(ratingMax);
  if (selectedTagIds().length > 0) baseFilters.tag_ids = selectedTagIds();
  
  return baseFilters;
 });

 // Load games with current filters and pagination
 async function loadGames() {
  try {
   await userGames.loadUserGames(
    filters(),
    userGames.value.pagination.page,
    userGames.value.pagination.per_page
   );
  } catch (error) {
   console.error('Failed to load games:', error);
  }
 }

 // Handle search with debouncing
 function handleSearch() {
  clearTimeout(searchTimeout);
  searchTimeout = setTimeout(() => {
   loadGames();
  }, 300);
 }

 // Handle filter changes
 function handleFilterChange() {
  loadGamesWithReset();
 } 
 
 // Load games with page reset
 async function loadGamesWithReset() {
  try {
   await userGames.loadUserGames(filters(), 1, userGames.value.pagination.per_page);
  } catch (error) {
   console.error('Failed to load games:', error);
  }
 }

 // Watch for filter changes - track previous values to prevent unnecessary calls
 $effect(() => {
  const currentFilters = {
    selectedStatus,
    selectedPlatform,
    selectedStorefront,
    selectedOwnershipStatus,
    lovedOnly,
    hasNotesOnly,
    ratingMin,
    ratingMax,
    sortBy,
    sortOrder
  };
  
  // Only trigger if filters actually changed
  const hasChanged = Object.keys(currentFilters).some(
    key => currentFilters[key as keyof typeof currentFilters] !== prevFilters[key as keyof typeof prevFilters]
  );
  
  if (hasChanged && (selectedStatus || selectedPlatform || selectedStorefront || selectedOwnershipStatus || lovedOnly || hasNotesOnly || ratingMin || ratingMax || sortBy || sortOrder)) {
    prevFilters = { ...currentFilters };
    handleFilterChange();
  }
 });

 // Watch for search query changes - track previous value to prevent unnecessary calls
 $effect(() => {
  // Only trigger if search query actually changed
  if (searchQuery !== prevSearchQuery && searchQuery !== undefined) {
    prevSearchQuery = searchQuery;
    handleSearch();
  }
 });

 function handleAddGame() {
  goto('/games/add');
 }

 function handleGameClick(gameId: string) {
  if (isBulkSelectionMode) {
   // In bulk selection mode, toggle selection instead of navigating
   toggleGameSelection(gameId);
  } else {
   // Normal mode, navigate to game detail
   // Find the user game to get the actual game ID (IGDB ID)
   const userGame = userGames.value.userGames?.find(ug => ug.id === gameId);
   if (userGame) {
    goto(`/games/${userGame.game.id}`);
   }
  }
 }

 // Pagination handlers
 async function handlePageChange(page: number) {
  try {
   await userGames.loadUserGames(filters(), page, userGames.value.pagination.per_page);
  } catch (error) {
   console.error('Failed to load games:', error);
  }
 }

 async function handleItemsPerPageChange(perPage: number) {
  try {
   await userGames.loadUserGames(filters(), 1, perPage);
  } catch (error) {
   console.error('Failed to load games:', error);
  }
 }

 function getStatusLabel(status: string) {
  const labels: Record<string, string> = {
   'not_started': 'Not Started',
   'in_progress': 'In Progress',
   'completed': 'Completed',
   'mastered': 'Mastered',
   'dominated': 'Dominated',
   'shelved': 'Shelved',
   'dropped': 'Dropped',
   'replay': 'Replay'
  };
  return labels[status] || status;
 }

 function clearAllFilters() {
  searchQuery = '';
  selectedPlatform = '';
  selectedStorefront = '';
  selectedStatus = '';
  selectedOwnershipStatus = '';
  lovedOnly = false;
  hasNotesOnly = false;
  ratingMin = '';
  ratingMax = '';
  sortBy = 'title';
  sortOrder = 'asc';
 }

 // Check if any filters are active
 const hasActiveFilters = $derived(searchQuery || selectedPlatform || selectedStorefront || selectedStatus || selectedOwnershipStatus || lovedOnly || hasNotesOnly || ratingMin || ratingMax || selectedTagIds().length > 0);

 // Bulk Selection Functions
 function toggleGameSelection(gameId: string) {
  selectedGameIds = new Set(selectedGameIds);
  if (selectedGameIds.has(gameId)) {
   selectedGameIds.delete(gameId);
  } else {
   selectedGameIds.add(gameId);
  }
  updateBulkActionsVisibility();
 }

   
 function clearSelection() {
  selectedGameIds = new Set();
  isSelectingAll = false;
  updateBulkActionsVisibility();
 }

 function updateBulkActionsVisibility() {
  // showBulkActions = selectedGameIds.size > 0; // Not yet implemented
 }

 // Reset selection when games change (e.g., after filtering)
 $effect(() => {
  if (userGames.value?.userGames && Array.isArray(userGames.value.userGames)) {
    // Remove any selected IDs that are no longer in current results
    const currentGameIds = new Set(userGames.value.userGames.map(game => game.id));
    const filteredSelectedIds = [...selectedGameIds].filter(id => currentGameIds.has(id as any));
    
    // Only update if the selection actually changed to prevent infinite loops
    if (filteredSelectedIds.length !== selectedGameIds.size || 
        !filteredSelectedIds.every(id => selectedGameIds.has(id))) {
      selectedGameIds = new Set(filteredSelectedIds);
    }
    
    // Update "select all" state based on the current (possibly updated) selection
    const newIsSelectingAll = selectedGameIds.size > 0 && selectedGameIds.size === userGames.value.userGames.length;
    if (newIsSelectingAll !== isSelectingAll) {
      isSelectingAll = newIsSelectingAll;
    }
    
    updateBulkActionsVisibility();
  }
 });

 // Bulk Operations Functions
 function resetBulkModal() {
  bulkStatus = '';
  bulkRating = '';
  bulkIsLoved = false;
  bulkOwnershipStatus = '';
 }

 function closeBulkModal() {
  showBulkModal = false;
  resetBulkModal();
 }

 async function applyBulkOperations() {
  if (selectedGameIds.size === 0) return;

  isBulkUpdating = true;
  
  try {
   const updateData: BulkStatusUpdateRequest = {
    user_game_ids: Array.from(selectedGameIds)
   };

   // Add fields only if they have values
   if (bulkStatus) updateData.play_status = bulkStatus as PlayStatus;
   if (bulkRating) updateData.personal_rating = parseFloat(bulkRating);
   if (bulkIsLoved) updateData.is_loved = true;
   if (bulkOwnershipStatus) updateData.ownership_status = bulkOwnershipStatus as OwnershipStatus;

   await userGames.bulkUpdateStatus(updateData);
   
   // Show success notification
   ui.showSuccess(
    'Bulk Update Successful', 
    `Updated ${selectedGameIds.size} game${selectedGameIds.size !== 1 ? 's' : ''} successfully.`
   );
   
   // Clear selection and close modal immediately after success
   clearSelection();
   closeBulkModal();
  } catch (error) {
   console.error('Failed to apply bulk operations:', error);
   const errorMessage = error instanceof Error ? error.message : 'Unknown error occurred';
   ui.showError(
    'Bulk Update Failed', 
    `Failed to update games: ${errorMessage}`
   );
   // Still close modal and clear selection on error
   clearSelection();
   closeBulkModal();
  } finally {
   isBulkUpdating = false;
  }
 }

function showBulkDeleteConfirmation() {
 showDeleteConfirmation = true;
}

function cancelBulkDelete() {
 showDeleteConfirmation = false;
}

async function confirmBulkDelete() {
 if (selectedGameIds.size === 0) return;

 isDeletingBulk = true;
 
 try {
  const deleteData: BulkDeleteRequest = {
   user_game_ids: Array.from(selectedGameIds)
  };

  await userGames.bulkDelete(deleteData);
  
  // Show success notification
  ui.showSuccess(
   'Bulk Delete Successful', 
   `Deleted ${selectedGameIds.size} game${selectedGameIds.size !== 1 ? 's' : ''} successfully.`
  );
  
  // Close modals and clear selection immediately after success
  clearSelection();
  showDeleteConfirmation = false;
  closeBulkModal();
 } catch (error) {
  console.error('Failed to delete bulk games:', error);
  const errorMessage = error instanceof Error ? error.message : 'Unknown error occurred';
  ui.showError(
   'Bulk Delete Failed', 
   `Failed to delete games: ${errorMessage}`
  );
  // Still close modals and clear selection on error
  clearSelection();
  showDeleteConfirmation = false;
  closeBulkModal();
 } finally {
  isDeletingBulk = false;
 }
}

// Bulk Platform Operations Functions
function resetBulkPlatformState() {
  bulkSelectedPlatforms = new Set<string>();
  bulkPlatformStorefronts = new Map<string, Set<string>>();
  bulkPlatformStoreUrls = new Map<string, string>();
  // Reset platform removal state
  availablePlatformAssociations = [];
  selectedAssociationIds = new Set();
}

function closeBulkPlatformAddModal() {
  showBulkPlatformAddModal = false;
  resetBulkPlatformState();
}

function closeBulkPlatformRemoveModal() {
  showBulkPlatformRemoveModal = false;
  resetBulkPlatformState();
}

function openBulkPlatformRemoveModal() {
  // Get available platform associations for the selected games
  availablePlatformAssociations = userGames.getAvailablePlatformAssociationsForGames(Array.from(selectedGameIds));
  selectedAssociationIds = new Set();
  showBulkPlatformRemoveModal = true;
}

// Platform selection functions for bulk operations (similar to game editing)
function toggleBulkPlatform(platformId: string) {
  if (bulkSelectedPlatforms.has(platformId)) {
    bulkSelectedPlatforms.delete(platformId);
    bulkPlatformStorefronts.delete(platformId);
    bulkPlatformStoreUrls.delete(platformId);
  } else {
    bulkSelectedPlatforms.add(platformId);
    
    const storefronts = new Set<string>();
    const platform = platformsData.platforms.find((p: any) => p.id === platformId);
    
    if (platform?.default_storefront_id) {
      storefronts.add(platform.default_storefront_id);
    }
    
    bulkPlatformStorefronts.set(platformId, storefronts);
  }
  
  bulkSelectedPlatforms = new Set(bulkSelectedPlatforms);
  bulkPlatformStorefronts = new Map(bulkPlatformStorefronts);
  bulkPlatformStoreUrls = new Map(bulkPlatformStoreUrls);
}

function toggleBulkStorefrontForPlatform(platformId: string, storefrontId: string) {
  const storefronts = bulkPlatformStorefronts.get(platformId) || new Set<string>();
  if (storefronts.has(storefrontId)) {
    storefronts.delete(storefrontId);
  } else {
    storefronts.add(storefrontId);
  }
  
  bulkPlatformStorefronts.set(platformId, storefronts);
  bulkPlatformStorefronts = new Map(bulkPlatformStorefronts);
}

function setBulkStoreUrlForPlatform(platformId: string, url: string) {
  if (url.trim()) {
    bulkPlatformStoreUrls.set(platformId, url);
  } else {
    bulkPlatformStoreUrls.delete(platformId);
  }
  bulkPlatformStoreUrls = new Map(bulkPlatformStoreUrls);
}

async function applyBulkAddPlatforms() {
  if (selectedGameIds.size === 0 || bulkSelectedPlatforms.size === 0) return;

  isProcessingBulkPlatforms = true;
  
  try {
    // Build platform associations array
    const platformAssociations: UserGamePlatformCreateRequest[] = [];
    
    for (const platformId of bulkSelectedPlatforms) {
      const storefronts = bulkPlatformStorefronts.get(platformId) || new Set();
      const storeUrl = bulkPlatformStoreUrls.get(platformId);
      
      if (storefronts.size === 0) {
        // Add platform without storefront
        platformAssociations.push({
          platform_id: platformId,
          is_available: true,
          ...(storeUrl && { store_url: storeUrl })
        });
      } else {
        // Add platform with each selected storefront
        for (const storefrontId of storefronts) {
          platformAssociations.push({
            platform_id: platformId,
            storefront_id: storefrontId,
            is_available: true,
            ...(storeUrl && { store_url: storeUrl })
          });
        }
      }
    }
    
    const request: BulkAddPlatformRequest = {
      user_game_ids: Array.from(selectedGameIds),
      platform_associations: platformAssociations
    };

    await userGames.bulkAddPlatforms(request);
    
    ui.showSuccess(
      'Bulk Platform Add Successful', 
      `Added platforms to ${selectedGameIds.size} game${selectedGameIds.size !== 1 ? 's' : ''} successfully.`
    );
    
    clearSelection();
    closeBulkPlatformAddModal();
  } catch (error) {
    console.error('Failed to bulk add platforms:', error);
    const errorMessage = error instanceof Error ? error.message : 'Unknown error occurred';
    ui.showError(
      'Bulk Platform Add Failed', 
      `Failed to add platforms: ${errorMessage}`
    );
    closeBulkPlatformAddModal();
  } finally {
    isProcessingBulkPlatforms = false;
  }
}

// Bulk platform removal function
async function handleBulkRemovePlatforms() {
  if (selectedAssociationIds.size === 0) {
    ui.showError('No Selection', 'Please select at least one platform association to remove.');
    return;
  }

  isProcessingBulkPlatforms = true;
  
  try {
    const request = {
      user_game_ids: Array.from(selectedGameIds),
      platform_association_ids: Array.from(selectedAssociationIds)
    };

    await userGames.bulkRemovePlatforms(request);
    
    ui.showSuccess(
      'Bulk Platform Remove Successful', 
      `Removed ${selectedAssociationIds.size} platform association${selectedAssociationIds.size !== 1 ? 's' : ''} from ${selectedGameIds.size} game${selectedGameIds.size !== 1 ? 's' : ''} successfully.`
    );
    
    clearSelection();
    closeBulkPlatformRemoveModal();
  } catch (error) {
    console.error('Failed to bulk remove platforms:', error);
    const errorMessage = error instanceof Error ? error.message : 'Unknown error occurred';
    ui.showError(
      'Bulk Platform Remove Failed', 
      `Failed to remove platforms: ${errorMessage}`
    );
    closeBulkPlatformRemoveModal();
  } finally {
    isProcessingBulkPlatforms = false;
  }
}

// Handle platform selection changes from PlatformRemovalSelector
function handleSelectionChange(event: CustomEvent<{ selectedAssociationIds: Set<string> }>) {
  selectedAssociationIds = event.detail.selectedAssociationIds;
}
</script>

<svelte:head>
 <title>My Games - Nexorious</title>
</svelte:head>

<RouteGuard requireAuth={true}>
<div class="space-y-6" data-testid="games-page-content">
 <!-- Real-time Update Indicator -->
 {#if hasOptimisticUpdates || isBulkProcessing}
  <div class="bg-blue-50 border border-blue-200 rounded-lg p-4 mb-6">
    <div class="flex items-center justify-between">
      <div class="flex items-center space-x-3">
        <svg class="animate-spin h-5 w-5 text-blue-600" fill="none" viewBox="0 0 24 24" stroke="currentColor">
          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15" />
        </svg>
        <div>
          <p class="text-sm font-medium text-blue-800">
            {#if isBulkProcessing}
              Processing bulk operation...
            {:else if hasOptimisticUpdates}
              Updating games...
            {/if}
          </p>
          <p class="text-xs text-blue-600">Changes will be reflected automatically</p>
        </div>
      </div>
      {#if recentlyUpdatedGameIds.size > 0}
        <span class="inline-flex items-center px-2 py-1 rounded-full text-xs font-medium bg-green-100 text-green-800">
          {recentlyUpdatedGameIds.size} recently updated
        </span>
      {/if}
    </div>
  </div>
 {/if}

 <!-- Header -->
 <div class="sm:flex sm:items-center sm:justify-between">
  <div>
   <h1 class="text-2xl font-bold leading-7 text-gray-900 sm:truncate sm:text-3xl sm:tracking-tight">My Games</h1>
   <p class="mt-1 text-sm text-gray-500">
    {userGames.value.pagination.total} unique games across all platforms in your collection
   </p>
  </div>
  <div class="mt-4 sm:ml-16 sm:mt-0 sm:flex-none">
   <button
    onclick={handleAddGame}
    class="btn-primary inline-flex items-center gap-x-2"
   >
    <svg class="-ml-0.5 h-5 w-5" viewBox="0 0 20 20" fill="currentColor">
     <path d="M10.75 4.75a.75.75 0 00-1.5 0v4.5h-4.5a.75.75 0 000 1.5h4.5v4.5a.75.75 0 001.5 0v-4.5h4.5a.75.75 0 000-1.5h-4.5v-4.5z" />
    </svg>
    Add Game
   </button>
  </div>
 </div>

 <!-- Advanced Filters and Search -->
 <div class="border-b border-gray-200 pb-5">
  <!-- Filter Header with Clear Button -->
  <div class="flex items-center justify-between mb-4">
   <h3 class="text-lg font-medium text-gray-900">Filters</h3>
   <div class="flex items-center space-x-4">
    {#if hasActiveFilters}
     <button
      onclick={clearAllFilters}
      class="text-sm text-primary-600 hover:text-primary-700 focus:outline-none focus:underline"
     >
      Clear all filters
     </button>
    {/if}
    
    <!-- Bulk Selection Controls -->
    {#if userGames.value.userGames?.length > 0}
     <div class="flex items-center space-x-2">
      {#if selectedGameIds.size < (userGames.value.userGames?.length ?? 0)}
       <button
        onclick={() => {
         selectedGameIds = new Set(userGames.value.userGames?.map(game => game.id) ?? []);
         isSelectingAll = true;
         updateBulkActionsVisibility();
        }}
        class="text-sm text-gray-600 hover:text-gray-700 focus:outline-none focus:underline"
       >
        Select All
       </button>
      {/if}
      {#if selectedGameIds.size > 0}
       <button
        onclick={clearSelection}
        class="text-sm text-gray-600 hover:text-gray-700 focus:outline-none focus:underline"
       >
        Deselect All
       </button>
      {/if}
      {#if selectedGameIds.size > 0}
       <span class="text-sm text-gray-500">|</span>
       <span class="text-sm text-gray-600">{selectedGameIds.size} selected</span>
       <span class="text-sm text-gray-500">|</span>
       <span class="text-xs text-primary-600">Click games to select/deselect</span>
       <button
        onclick={() => showBulkModal = true}
        class="btn-secondary text-sm px-3 py-1"
       >
        Bulk Edit
       </button>
       <button
        onclick={() => showBulkPlatformAddModal = true}
        class="btn-secondary text-sm px-3 py-1 ml-2"
       >
        Add Platforms
       </button>
       <button
        onclick={openBulkPlatformRemoveModal}
        class="btn-secondary text-sm px-3 py-1 ml-2"
       >
        Remove Platforms
       </button>
      {/if}
     </div>
    {/if}
    
    <!-- View Mode Toggle -->
    <div class="inline-flex rounded-md shadow-sm" role="group">
     <button
      onclick={() => viewMode = 'grid'}
      class="{viewMode === 'grid' ? 'bg-primary-50 border-primary-500 text-primary-700' : 'bg-white border-gray-300 text-gray-700 hover:bg-gray-50'} relative inline-flex items-center rounded-l-md border px-3 py-2 text-sm font-medium focus:z-10 focus:border-primary-500 focus:outline-none focus:ring-1 focus:ring-primary-500"
     >
      <svg class="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke-width="1.5" stroke="currentColor">
       <path stroke-linecap="round" stroke-linejoin="round" d="M3.375 19.5h17.25m-17.25 0a1.125 1.125 0 01-1.125-1.125M3.375 19.5h17.25m-17.25 0a1.125 1.125 0 001.125 1.125m0 0V4.875c0-.621.504-1.125 1.125-1.125M3.375 19.5V4.875c0-.621.504-1.125 1.125-1.125m0 0h17.25m-17.25 0a1.125 1.125 0 011.125-1.125h15.75m0 0a1.125 1.125 0 011.125-1.125M18.375 2.25h-7.5A1.125 1.125 0 009.75 3.375v1.875m7.5-1.875A1.125 1.125 0 0018.375 3.375v1.875m-7.5 0V18.375m7.5-13.125V18.375m-7.5 0h7.5" />
      </svg>
      <span class="sr-only">Grid view</span>
     </button>
     <button
      onclick={() => viewMode = 'list'}
      class="{viewMode === 'list' ? 'bg-primary-50 border-primary-500 text-primary-700' : 'bg-white border-gray-300 text-gray-700 hover:bg-gray-50'} relative -ml-px inline-flex items-center rounded-r-md border px-3 py-2 text-sm font-medium focus:z-10 focus:border-primary-500 focus:outline-none focus:ring-1 focus:ring-primary-500"
     >
      <svg class="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke-width="1.5" stroke="currentColor">
       <path stroke-linecap="round" stroke-linejoin="round" d="M8.25 6.75h12M8.25 12h12m-12 5.25h12M3.75 6.75h.007v.008H3.75V6.75zm.375 0a.375.375 0 11-.75 0 .375.375 0 01.75 0zM3.75 12h.007v.008H3.75V12zm.375 0a.375.375 0 11-.75 0 .375.375 0 01.75 0zM3.75 17.25h.007v.008H3.75v-.008zm.375 0a.375.375 0 11-.75 0 .375.375 0 01.75 0z" />
      </svg>
      <span class="sr-only">List view</span>
     </button>
    </div>
   </div>
  </div>

  <!-- Search and Sort Row -->
  <div class="grid grid-cols-1 gap-4 lg:grid-cols-6 lg:gap-6 mb-4">
   <!-- Search -->
   <div class="lg:col-span-3">
    <label for="search" class="form-label">
     Search Games
    </label>
    <div class="relative">
     {#if !searchQuery}
       <div class="pointer-events-none absolute inset-y-0 left-0 flex items-center pl-3">
        <svg class="h-5 w-5 text-gray-400" viewBox="0 0 20 20" fill="currentColor">
         <path fill-rule="evenodd" d="M9 3.5a5.5 5.5 0 100 11 5.5 5.5 0 000-11zM2 9a7 7 0 1112.452 4.391l3.328 3.329a.75.75 0 11-1.06 1.06l-3.329-3.328A7 7 0 012 9z" clip-rule="evenodd" />
        </svg>
       </div>
     {/if}
     <input
      id="search"
      type="text"
      bind:value={searchQuery}
      placeholder="Search by title, genre, developer..."
      class="form-input pl-10"
     />
    </div>
   </div>

   <!-- Sort By -->
   <div class="lg:col-span-2">
    <label for="sortBy" class="form-label">
     Sort By
    </label>
    <select
     id="sortBy"
     bind:value={sortBy}
     class="form-input"
    >
     <option value="title">Title</option>
     <option value="personal_rating">Rating</option>
     <option value="play_status">Status</option>
     <option value="genre">Genre</option>
     <option value="release_date">Release Date</option>
     <option value="hours_played">Hours Played</option>
     <option value="acquired_date">Date Acquired</option>
    </select>
   </div>

   <!-- Sort Order -->
   <div>
    <label for="sortOrder" class="form-label">
     Order
    </label>
    <select
     id="sortOrder"
     bind:value={sortOrder}
     class="form-input"
    >
     <option value="asc">Ascending</option>
     <option value="desc">Descending</option>
    </select>
   </div>
  </div>

  <!-- Tag Filter -->
  <div class="mb-6">
   <TagFilter />
  </div>

  <!-- Filter Controls -->
  <div class="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-4 xl:grid-cols-6">
   <!-- Play Status Filter -->
   <div>
    <label for="status" class="form-label">
     Play Status
    </label>
    <select
     id="status"
     bind:value={selectedStatus}
     class="form-input"
    >
     <option value="">All Statuses</option>
     <option value="not_started">Not Started</option>
     <option value="in_progress">In Progress</option>
     <option value="completed">Completed</option>
     <option value="mastered">Mastered</option>
     <option value="dominated">Dominated</option>
     <option value="shelved">Shelved</option>
     <option value="dropped">Dropped</option>
     <option value="replay">Replay</option>
    </select>
   </div>

   <!-- Ownership Status Filter -->
   <div>
    <label for="ownershipStatus" class="form-label">
     Ownership
    </label>
    <select
     id="ownershipStatus"
     bind:value={selectedOwnershipStatus}
     class="form-input"
    >
     <option value="">All Types</option>
     <option value="owned">Owned</option>
     <option value="borrowed">Borrowed</option>
     <option value="rented">Rented</option>
     <option value="subscription">Subscription</option>
     <option value="no_longer_owned">No Longer Owned</option>
    </select>
   </div>

   <!-- Platform Filter -->
   <div>
    <label for="platform" class="form-label">
     Platform
    </label>
    <select
     id="platform"
     bind:value={selectedPlatform}
     class="form-input"
    >
     <option value="">All Platforms</option>
     {#each platformsData.platforms as platform (platform.id)}
      <option value={platform.id}>{platform.display_name}</option>
     {/each}
    </select>
   </div>

   <!-- Storefront Filter -->
   <div>
    <label for="storefront" class="form-label">
     Storefront
    </label>
    <select
     id="storefront"
     bind:value={selectedStorefront}
     class="form-input"
    >
     <option value="">All Storefronts</option>
     {#each platformsData.storefronts as storefront (storefront.id)}
      <option value={storefront.id}>{storefront.display_name}</option>
     {/each}
    </select>
   </div>

   <!-- Rating Range -->
   <div>
    <label for="ratingMin" class="form-label">
     Min Rating
    </label>
    <select
     id="ratingMin"
     bind:value={ratingMin}
     class="form-input"
    >
     <option value="">Any</option>
     <option value="1">1 Star+</option>
     <option value="2">2 Stars+</option>
     <option value="3">3 Stars+</option>
     <option value="4">4 Stars+</option>
     <option value="5">5 Stars</option>
    </select>
   </div>

   <div>
    <label for="ratingMax" class="form-label">
     Max Rating
    </label>
    <select
     id="ratingMax"
     bind:value={ratingMax}
     class="form-input"
    >
     <option value="">Any</option>
     <option value="1">1 Star</option>
     <option value="2">2 Stars</option>
     <option value="3">3 Stars</option>
     <option value="4">4 Stars</option>
     <option value="5">5 Stars</option>
    </select>
   </div>
  </div>

  <!-- Toggle Filters -->
  <div class="flex flex-wrap gap-4 mt-4">
   <label class="inline-flex items-center">
    <input
     type="checkbox"
     bind:checked={lovedOnly}
     class="form-checkbox h-4 w-4 text-primary-600 focus:ring-primary-500 border-gray-300 rounded"
    />
    <span class="ml-2 text-sm text-gray-700">
     <span class="text-red-500">♥</span> Only loved games
    </span>
   </label>

   <label class="inline-flex items-center">
    <input
     type="checkbox"
     bind:checked={hasNotesOnly}
     class="form-checkbox h-4 w-4 text-primary-600 focus:ring-primary-500 border-gray-300 rounded"
    />
    <span class="ml-2 text-sm text-gray-700">Games with notes</span>
   </label>
  </div>
 </div>

 <!-- Games Display -->
 {#if userGames.value.isLoading}
  <div class="flex items-center justify-center py-12">
   <div class="text-center">
    <svg class="mx-auto h-12 w-12 text-gray-400 loading" fill="none" viewBox="0 0 24 24" stroke="currentColor">
     <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15" />
    </svg>
    <p class="mt-2 text-sm text-gray-500">Loading games...</p>
   </div>
  </div>
 {:else if (userGames.value.userGames?.length ?? 0) === 0}
  <div class="text-center py-12">
   <svg class="mx-auto h-12 w-12 text-gray-400" fill="none" viewBox="0 0 24 24" stroke="currentColor">
    <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9.172 16.172a4 4 0 015.656 0M9 12h6m-6 4h6m2 5H7a2 2 0 01-2-2V5a2 2 0 012-2h5.586a1 1 0 01.707.293l5.414 5.414a1 1 0 01.293.707V19a2 2 0 01-2 2z" />
   </svg>
   <h3 class="mt-4 text-lg font-medium text-gray-900">
    {userGames.value.pagination.total === 0 ? 'No games in your collection yet' : 'No games match your filters'}
   </h3>
   <p class="mt-2 text-sm text-gray-500">
    {userGames.value.pagination.total === 0 ? 'Start building your unified game library by adding games from any platform or storefront.' : 'Try adjusting your search or filter criteria.'}
   </p>
   {#if userGames.value.pagination.total === 0}
    <div class="mt-6">
     <button
      onclick={handleAddGame}
      class="btn-primary inline-flex items-center gap-x-2"
     >
      <svg class="-ml-0.5 h-5 w-5" viewBox="0 0 20 20" fill="currentColor">
       <path d="M10.75 4.75a.75.75 0 00-1.5 0v4.5h-4.5a.75.75 0 000 1.5h4.5v4.5a.75.75 0 001.5 0v-4.5h4.5a.75.75 0 000-1.5h-4.5v-4.5z" />
      </svg>
      Add Your First Game
     </button>
    </div>
   {/if}
  </div>
 {:else}
  <!-- Grid View -->
  {#if viewMode === 'grid'}
   <div class="grid grid-cols-1 gap-6 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4">
    {#each userGames.value.userGames ?? [] as userGame (userGame.id)}
     <div
      onclick={(e) => {
       // Don't navigate if clicking on checkbox
       if ((e.target as HTMLInputElement).type !== 'checkbox') {
        handleGameClick(userGame.id);
       }
      }}
      onkeydown={(e) => e.key === 'Enter' && handleGameClick(userGame.id)}
      tabindex="0"
      role="button"
      data-testid="game-card"
      aria-label="{isBulkSelectionMode ? 'Select ' + userGame.game.title : 'View details for ' + userGame.game.title}"
      class="group relative flex flex-col overflow-hidden rounded-lg border border-gray-200 bg-white shadow-sm hover:shadow-md transition-all duration-300 {isBulkSelectionMode ? 'cursor-pointer' : 'cursor-pointer'} focus:outline-none focus:ring-2 focus:ring-primary-500 focus:ring-offset-2 {selectedGameIds.has(userGame.id) ? 'ring-2 ring-primary-500' : ''} {isBulkSelectionMode ? 'hover:ring-2 hover:ring-primary-300' : ''} {recentlyUpdatedGameIds.has(userGame.id) ? 'ring-2 ring-green-400 shadow-lg bg-green-50' : ''}"
     >
      <!-- Selection Checkbox -->
      <div class="absolute top-2 left-2 z-10">
       <input
        type="checkbox"
        checked={selectedGameIds.has(userGame.id)}
        onchange={() => toggleGameSelection(userGame.id)}
        onclick={(e) => e.stopPropagation()}
        class="h-4 w-4 text-primary-600 focus:ring-primary-500 border-gray-300 rounded"
        aria-label="Select {userGame.game.title}"
       />
      </div>
      <div class="aspect-[3/4] overflow-hidden bg-gray-100">
       {#if userGame.game.cover_art_url}
        <img
         src={resolveImageUrl(userGame.game.cover_art_url)}
         alt="Cover art for {userGame.game.title}"
         loading="lazy"
         class="h-full w-full object-cover object-center group-hover:scale-105 transition-transform duration-300"
         onerror={(e) => {
          const target = e.currentTarget as HTMLImageElement;
          const nextElement = target.nextElementSibling as HTMLElement;
          target.style.display = 'none';
          if (nextElement) {
            nextElement.style.display = 'flex';
          }
         }}
        />
        <div style="display: none;" class="h-full w-full flex items-center justify-center text-gray-400 text-sm">
         <div class="text-center">
          <svg class="mx-auto h-12 w-12 text-gray-300" fill="none" viewBox="0 0 24 24" stroke="currentColor">
           <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 16l4.586-4.586a2 2 0 012.828 0L16 16m-2-2l1.586-1.586a2 2 0 012.828 0L20 14m-6-6h.01M6 20h12a2 2 0 002-2V6a2 2 0 00-2-2H6a2 2 0 00-2 2v12a2 2 0 002 2z" />
          </svg>
          <p class="mt-2">No Image</p>
         </div>
        </div>
       {:else}
        <div class="h-full w-full flex items-center justify-center bg-gray-100 text-gray-400">
         <div class="text-center">
          <svg class="mx-auto h-12 w-12 text-gray-300" fill="none" viewBox="0 0 24 24" stroke="currentColor">
           <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 16l4.586-4.586a2 2 0 012.828 0L16 16m-2-2l1.586-1.586a2 2 0 012.828 0L20 14m-6-6h.01M6 20h12a2 2 0 002-2V6a2 2 0 00-2-2H6a2 2 0 00-2 2v12a2 2 0 002 2z" />
          </svg>
          <p class="mt-2 text-sm">No Cover</p>
         </div>
        </div>
       {/if}
       
       <!-- Status Badge -->
       <div class="absolute bottom-2 left-2">
        <span class="inline-flex items-center px-2 py-1 rounded-full text-xs font-medium status-{userGame.play_status.replace('_', '-')}">
         {getStatusLabel(userGame.play_status)}
        </span>
       </div>

       <!-- Top-right indicators -->
       <div class="absolute top-2 right-2 flex items-center space-x-1">
        
        <!-- Loved indicator -->
        {#if userGame.is_loved}
         <span class="inline-flex items-center justify-center w-6 h-6 rounded-full bg-red-100 text-red-600">
          ♥
         </span>
        {/if}
       </div>
      </div>
      
      <div class="flex flex-1 flex-col justify-between p-4">
       <div class="flex-1">
        <h3 class="text-sm font-medium text-gray-900 line-clamp-2" title="{userGame.game.title}">
         {userGame.game.title}
        </h3>
        <p class="mt-1 text-sm text-gray-500" title="{userGame.game.genre || 'Unknown Genre'}">
         {userGame.game.genre || 'Unknown Genre'}
        </p>
        <!-- ===== PLATFORM BADGES CONDITIONAL DEBUG ===== -->
        {#if userGame.platforms && userGame.platforms.length > 0}
         <!-- SHOULD RENDER PLATFORMBADGES: platforms={userGame.platforms?.length || 0} -->
         <div class="mt-3">
          <PlatformBadges 
            platforms={userGame.platforms} 
            compact={true} 
            maxVisible={3} 
            showStoreLinks={false}
          />
         </div>
        {:else}
         <!-- NO PLATFORMS FOR GAME: {userGame.game.title} -->
         <div class="mt-3 text-xs text-gray-500">
          DEBUG: No platforms for {userGame.game.title}
         </div>
        {/if}
       </div>
       
       <div class="mt-3 flex items-center justify-between">
        {#if userGame.personal_rating}
         <div class="flex items-center space-x-1">
          <span class="text-yellow-400">★</span>
          <span class="text-sm font-medium text-gray-900">
           {userGame.personal_rating}
          </span>
         </div>
        {:else}
         <div class="flex items-center space-x-1 text-gray-400">
          <span>☆</span>
          <span class="text-sm">Not rated</span>
         </div>
        {/if}
        
        <div class="text-right">
         <p class="text-sm text-gray-500">{userGame.hours_played || 0}h played</p>
         {#if userGame.game.release_date}
          <p class="text-xs text-gray-400">{new Date(userGame.game.release_date).getFullYear()}</p>
         {/if}
        </div>
       </div>
      </div>
     </div>
    {/each}
   </div>
  {:else}
   <!-- List View -->
   <div class="overflow-hidden">
    <div class="overflow-x-auto">
     <table class="min-w-full divide-y divide-gray-300">
      <thead class="bg-gray-50">
       <tr>
        <th scope="col" class="py-3.5 pl-4 pr-2 text-left text-sm font-semibold text-gray-900 sm:pl-6">
         <input
          type="checkbox"
          checked={isSelectingAll}
          onchange={() => {
           if (isSelectingAll) {
            clearSelection();
           } else {
            selectedGameIds = new Set(userGames.value.userGames?.map(game => game.id) ?? []);
            isSelectingAll = true;
            updateBulkActionsVisibility();
           }
          }}
          class="h-4 w-4 text-primary-600 focus:ring-primary-500 border-gray-300 rounded"
          aria-label="Select all games"
         />
        </th>
        <th scope="col" class="py-3.5 pl-2 pr-3 text-left text-sm font-semibold text-gray-900">
         Game
        </th>
        <th scope="col" class="px-3 py-3.5 text-left text-sm font-semibold text-gray-900">
         Genre
        </th>
        <th scope="col" class="px-3 py-3.5 text-left text-sm font-semibold text-gray-900">
         Platforms
        </th>
        <th scope="col" class="px-3 py-3.5 text-left text-sm font-semibold text-gray-900">
         Status
        </th>
        <th scope="col" class="px-3 py-3.5 text-left text-sm font-semibold text-gray-900">
         Rating
        </th>
        <th scope="col" class="px-3 py-3.5 text-left text-sm font-semibold text-gray-900">
         Hours
        </th>
       </tr>
      </thead>
      <tbody class="divide-y divide-gray-200 bg-white">
       {#each userGames.value.userGames ?? [] as userGame (userGame.id)}
        <tr
         onclick={(e) => {
          // Don't navigate if clicking on checkbox
          if ((e.target as HTMLInputElement).type !== 'checkbox') {
           handleGameClick(userGame.id);
          }
         }}
         onkeydown={(e) => e.key === 'Enter' && handleGameClick(userGame.id)}
         tabindex="0"
         data-testid="game-card"
         class="hover:bg-gray-50 cursor-pointer focus:outline-none focus:bg-gray-50 transition-all duration-300 {selectedGameIds.has(userGame.id) ? 'bg-primary-50' : ''} {isBulkSelectionMode ? 'hover:bg-primary-50' : ''} {recentlyUpdatedGameIds.has(userGame.id) ? 'bg-green-50 ring-1 ring-green-200' : ''}"
        >
         <td class="whitespace-nowrap py-4 pl-4 pr-2 text-sm sm:pl-6">
          <input
           type="checkbox"
           checked={selectedGameIds.has(userGame.id)}
           onchange={() => toggleGameSelection(userGame.id)}
           onclick={(e) => e.stopPropagation()}
           class="h-4 w-4 text-primary-600 focus:ring-primary-500 border-gray-300 rounded"
           aria-label="Select {userGame.game.title}"
          />
         </td>
         <td class="whitespace-nowrap py-4 pl-2 pr-3 text-sm">
          <div class="flex items-center space-x-4">
           <div class="relative h-12 w-9 flex-shrink-0">
            {#if userGame.game.cover_art_url}
             <img
              src={resolveImageUrl(userGame.game.cover_art_url)}
              alt={userGame.game.title}
              class="h-12 w-9 rounded object-cover"
             />
            {:else}
             <div class="h-12 w-9 rounded bg-gray-100 flex items-center justify-center">
              <svg class="h-6 w-6 text-gray-400" fill="none" viewBox="0 0 24 24" stroke="currentColor">
               <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 16l4.586-4.586a2 2 0 012.828 0L16 16m-2-2l1.586-1.586a2 2 0 012.828 0L20 14m-6-6h.01M6 20h12a2 2 0 002-2V6a2 2 0 00-2-2H6a2 2 0 00-2 2v12a2 2 0 002 2z" />
              </svg>
             </div>
            {/if}
            <!-- Top-right indicators for list view -->
            <div class="absolute -top-1 -right-1 flex items-center space-x-1">
             
             {#if userGame.is_loved}
              <div class="h-4 w-4 rounded-full bg-red-100 flex items-center justify-center">
               <span class="text-xs text-red-600">♥</span>
              </div>
             {/if}
            </div>
           </div>
           <div class="min-w-0 flex-1">
            <div class="text-sm font-medium text-gray-900 truncate">
             {userGame.game.title}
            </div>
            <div class="text-sm text-gray-500 truncate">
             {userGame.game.developer || 'Unknown Developer'}
            </div>
           </div>
          </div>
         </td>
         <td class="whitespace-nowrap px-3 py-4 text-sm text-gray-500">
          {userGame.game.genre || 'Unknown'}
         </td>
         <td class="px-3 py-4 text-sm text-gray-500">
          <!-- ===== LIST VIEW PLATFORM BADGES DEBUG ===== -->
          {#if userGame.platforms && userGame.platforms.length > 0}
           <!-- LIST VIEW SHOULD RENDER PLATFORMBADGES: platforms={userGame.platforms?.length || 0} -->
           <div class="max-w-48">
            <PlatformBadges 
              platforms={userGame.platforms} 
              compact={true} 
              maxVisible={2}
              showStoreLinks={false}
            />
           </div>
          {:else}
           <!-- LIST VIEW NO PLATFORMS: {userGame.game.title} -->
           <span class="text-gray-400">DEBUG: No platforms for {userGame.game.title}</span>
          {/if}
         </td>
         <td class="whitespace-nowrap px-3 py-4 text-sm">
          <span class="inline-flex items-center px-2 py-1 rounded-full text-xs font-medium status-{userGame.play_status.replace('_', '-')}">
           {getStatusLabel(userGame.play_status)}
          </span>
         </td>
         <td class="whitespace-nowrap px-3 py-4 text-sm text-gray-500">
          {#if userGame.personal_rating}
           <div class="flex items-center space-x-1">
            <span class="text-yellow-400">★</span>
            <span class="text-gray-900 font-medium">{userGame.personal_rating}</span>
           </div>
          {:else}
           <span class="text-gray-400">-</span>
          {/if}
         </td>
         <td class="whitespace-nowrap px-3 py-4 text-sm text-gray-500">
          {userGame.hours_played || 0}h
         </td>
        </tr>
       {/each}
      </tbody>
     </table>
    </div>
   </div>
  {/if}
  
  <!-- Pagination -->
  <div class="mt-8 border-t border-gray-200 pt-6">
   <Pagination 
    currentPage={userGames.value.pagination.page}
    totalPages={userGames.value.pagination.pages}
    totalItems={userGames.value.pagination.total}
    itemsPerPage={userGames.value.pagination.per_page}
    onPageChange={handlePageChange}
    onItemsPerPageChange={handleItemsPerPageChange}
   />
  </div>
 {/if}
</div>

<!-- Bulk Operations Modal -->
{#if showBulkModal}
 <div 
  class="fixed inset-0 bg-gray-500 bg-opacity-75 overflow-y-auto h-full w-full z-50" 
  role="button" 
  tabindex="0"
  aria-label="Close bulk operations modal"
  onclick={closeBulkModal} 
  onkeydown={(e) => (e.key === 'Escape' || e.key === 'Enter' || e.key === ' ') && closeBulkModal()}
 >
  <div 
    class="relative top-20 mx-auto p-5 border max-w-lg shadow-lg rounded-md bg-white" 
    role="dialog" 
    aria-modal="true" 
    aria-labelledby="bulk-modal-title"
  >
    <div class="bg-white px-4 pt-5 pb-4 sm:p-6 sm:pb-4">
     <div class="sm:flex sm:items-start">
      <div class="mx-auto flex-shrink-0 flex items-center justify-center h-12 w-12 rounded-full bg-primary-100 sm:mx-0 sm:h-10 sm:w-10">
       <svg class="h-6 w-6 text-primary-600" fill="none" viewBox="0 0 24 24" stroke-width="1.5" stroke="currentColor">
        <path stroke-linecap="round" stroke-linejoin="round" d="M16.862 4.487l1.687-1.688a1.875 1.875 0 112.652 2.652L10.582 16.07a4.5 4.5 0 01-1.897 1.13L6 18l.8-2.685a4.5 4.5 0 011.13-1.897l8.932-8.931zm0 0L19.5 7.125M18 14v4.75A2.25 2.25 0 0115.75 21H5.25A2.25 2.25 0 013 18.75V8.25A2.25 2.25 0 015.25 6H10" />
       </svg>
      </div>
      <div class="mt-3 text-center sm:mt-0 sm:ml-4 sm:text-left w-full">
       <h3 class="text-lg leading-6 font-medium text-gray-900" id="bulk-modal-title">
        Bulk Operations
       </h3>
       <div class="mt-2">
        <p class="text-sm text-gray-500">
         Update {selectedGameIds.size} selected game{selectedGameIds.size !== 1 ? 's' : ''}
        </p>
       </div>

       <!-- Bulk Operations Form -->
       <div class="mt-4 space-y-4">
        <!-- Status Update -->
        <div>
         <label for="bulkStatus" class="block text-sm font-medium text-gray-700">
          Play Status
         </label>
         <select
          id="bulkStatus"
          bind:value={bulkStatus}
          class="mt-1 block w-full border-gray-300 rounded-md shadow-sm focus:ring-primary-500 focus:border-primary-500 sm:text-sm"
         >
          <option value="">No change</option>
          <option value="not_started">Not Started</option>
          <option value="in_progress">In Progress</option>
          <option value="completed">Completed</option>
          <option value="mastered">Mastered</option>
          <option value="dominated">Dominated</option>
          <option value="shelved">Shelved</option>
          <option value="dropped">Dropped</option>
          <option value="replay">Replay</option>
         </select>
        </div>

        <!-- Ownership Status Update -->
        <div>
         <label for="bulkOwnershipStatus" class="block text-sm font-medium text-gray-700">
          Ownership Status
         </label>
         <select
          id="bulkOwnershipStatus"
          bind:value={bulkOwnershipStatus}
          class="mt-1 block w-full border-gray-300 rounded-md shadow-sm focus:ring-primary-500 focus:border-primary-500 sm:text-sm"
         >
          <option value="">No change</option>
          <option value="owned">Owned</option>
          <option value="borrowed">Borrowed</option>
          <option value="rented">Rented</option>
          <option value="subscription">Subscription</option>
          <option value="no_longer_owned">No Longer Owned</option>
         </select>
        </div>

        <!-- Rating Update -->
        <div>
         <label for="bulkRating" class="block text-sm font-medium text-gray-700">
          Rating
         </label>
         <select
          id="bulkRating"
          bind:value={bulkRating}
          class="mt-1 block w-full border-gray-300 rounded-md shadow-sm focus:ring-primary-500 focus:border-primary-500 sm:text-sm"
         >
          <option value="">No change</option>
          <option value="1">1 Star</option>
          <option value="2">2 Stars</option>
          <option value="3">3 Stars</option>
          <option value="4">4 Stars</option>
          <option value="5">5 Stars</option>
         </select>
        </div>

        <!-- Loved Toggle -->
        <div>
         <label class="flex items-center">
          <input
           type="checkbox"
           bind:checked={bulkIsLoved}
           class="h-4 w-4 text-primary-600 focus:ring-primary-500 border-gray-300 rounded"
          />
          <span class="ml-2 text-sm text-gray-700">Mark as loved</span>
         </label>
        </div>
       </div>
      </div>
     </div>
    </div>
    <div class="bg-gray-50 px-4 py-3 sm:px-6 sm:flex sm:flex-row-reverse">
     <button
      type="button"
      disabled={isBulkUpdating}
      onclick={applyBulkOperations}
      class="w-full inline-flex justify-center rounded-md border border-transparent shadow-sm px-4 py-2 bg-primary-600 text-base font-medium text-white hover:bg-primary-700 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-primary-500 sm:ml-3 sm:w-auto sm:text-sm disabled:opacity-50 disabled:cursor-not-allowed"
     >
      {#if isBulkUpdating}
       <svg class="animate-spin -ml-1 mr-3 h-5 w-5 text-white" xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24">
        <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
        <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path>
       </svg>
       Updating...
      {:else}
       Apply Changes
      {/if}
     </button>
     <button
      type="button"
      disabled={isBulkUpdating}
      onclick={showBulkDeleteConfirmation}
      class="w-full inline-flex justify-center rounded-md border border-transparent shadow-sm px-4 py-2 bg-red-600 text-base font-medium text-white hover:bg-red-700 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-red-500 sm:ml-3 sm:w-auto sm:text-sm disabled:opacity-50 disabled:cursor-not-allowed"
     >
      Delete Selected
     </button>
     <button
      type="button"
      disabled={isBulkUpdating}
      onclick={closeBulkModal}
      class="mt-3 w-full inline-flex justify-center rounded-md border border-gray-300 shadow-sm px-4 py-2 bg-white text-base font-medium text-gray-700 hover:bg-gray-50 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-primary-500 sm:mt-0 sm:ml-3 sm:w-auto sm:text-sm disabled:opacity-50 disabled:cursor-not-allowed"
     >
      Cancel
     </button>
    </div>
   </div>
  </div>
{/if}

<!-- Bulk Delete Confirmation Modal -->
{#if showDeleteConfirmation}
 <div 
  class="fixed inset-0 bg-gray-500 bg-opacity-75 overflow-y-auto h-full w-full z-50" 
  role="button" 
  tabindex="0"
  aria-label="Close delete confirmation"
  onclick={cancelBulkDelete} 
  onkeydown={(e) => (e.key === 'Escape' || e.key === 'Enter' || e.key === ' ') && cancelBulkDelete()}
 >
  <div 
    class="relative top-20 mx-auto p-5 border max-w-lg shadow-lg rounded-md bg-white" 
    role="dialog" 
    aria-modal="true" 
    aria-labelledby="bulk-delete-title"
  >
    <div class="bg-white px-4 pt-5 pb-4 sm:p-6 sm:pb-4">
     <div class="sm:flex sm:items-start">
      <div class="mx-auto flex-shrink-0 flex items-center justify-center h-12 w-12 rounded-full bg-red-100 sm:mx-0 sm:h-10 sm:w-10">
       <svg class="h-6 w-6 text-red-600" fill="none" viewBox="0 0 24 24" stroke-width="1.5" stroke="currentColor">
        <path stroke-linecap="round" stroke-linejoin="round" d="M12 9v3.75m-9.303 3.376c-.866 1.5.217 3.374 1.948 3.374h14.71c1.73 0 2.813-1.874 1.948-3.374L13.949 3.378c-.866-1.5-3.032-1.5-3.898 0L2.697 16.126zM12 15.75h.007v.008H12v-.008z" />
       </svg>
      </div>
      <div class="mt-3 text-center sm:mt-0 sm:ml-4 sm:text-left">
       <h3 class="text-lg leading-6 font-medium text-gray-900" id="bulk-delete-title">
        Delete Selected Games
       </h3>
       <div class="mt-2">
        <p class="text-sm text-gray-500">
         Are you sure you want to delete {selectedGameIds.size} selected game{selectedGameIds.size !== 1 ? 's' : ''}? This action cannot be undone.
        </p>
       </div>
      </div>
     </div>
    </div>
    <div class="bg-gray-50 px-4 py-3 sm:px-6 sm:flex sm:flex-row-reverse">
     <button
      type="button"
      disabled={isDeletingBulk}
      onclick={confirmBulkDelete}
      class="w-full inline-flex justify-center rounded-md border border-transparent shadow-sm px-4 py-2 bg-red-600 text-base font-medium text-white hover:bg-red-700 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-red-500 sm:ml-3 sm:w-auto sm:text-sm disabled:opacity-50 disabled:cursor-not-allowed"
     >
      {#if isDeletingBulk}
       <svg class="animate-spin -ml-1 mr-3 h-5 w-5 text-white" xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24">
        <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
        <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path>
       </svg>
       Deleting...
      {:else}
       Delete
      {/if}
     </button>
     <button
      type="button"
      disabled={isDeletingBulk}
      onclick={cancelBulkDelete}
      class="mt-3 w-full inline-flex justify-center rounded-md border border-gray-300 shadow-sm px-4 py-2 bg-white text-base font-medium text-gray-700 hover:bg-gray-50 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-primary-500 sm:mt-0 sm:mr-3 sm:w-auto sm:text-sm disabled:opacity-50 disabled:cursor-not-allowed"
     >
      Cancel
     </button>
    </div>
   </div>
  </div>
{/if}

<!-- Bulk Add Platforms Modal -->
{#if showBulkPlatformAddModal}
 <div 
  class="fixed inset-0 bg-gray-500 bg-opacity-75 overflow-y-auto h-full w-full z-50" 
  role="button" 
  tabindex="0"
  aria-label="Close add platforms modal"
  onclick={closeBulkPlatformAddModal} 
  onkeydown={(e) => (e.key === 'Escape' || e.key === 'Enter' || e.key === ' ') && closeBulkPlatformAddModal()}
 >
  <div 
    class="relative top-10 mx-auto p-5 border max-w-4xl shadow-lg rounded-md bg-white" 
    role="dialog" 
    aria-modal="true" 
    aria-labelledby="bulk-platform-add-title"
  >
    <div class="bg-white px-4 pt-5 pb-4 sm:p-6 sm:pb-4">
     <div class="sm:flex sm:items-start">
      <div class="mx-auto flex-shrink-0 flex items-center justify-center h-12 w-12 rounded-full bg-green-100 sm:mx-0 sm:h-10 sm:w-10">
       <svg class="h-6 w-6 text-green-600" fill="none" viewBox="0 0 24 24" stroke-width="1.5" stroke="currentColor">
        <path stroke-linecap="round" stroke-linejoin="round" d="M12 4.5v15m7.5-7.5h-15" />
       </svg>
      </div>
      <div class="mt-3 text-center sm:mt-0 sm:ml-4 sm:text-left w-full">
       <h3 class="text-lg leading-6 font-medium text-gray-900" id="bulk-platform-add-title">
        Add Platforms to Selected Games
       </h3>
       <div class="mt-2">
        <p class="text-sm text-gray-500">
         Add platform/storefront associations to {selectedGameIds.size} selected game{selectedGameIds.size !== 1 ? 's' : ''}
        </p>
       </div>

       <div class="mt-6">
        <PlatformSelector
          bind:selectedPlatforms={bulkSelectedPlatforms}
          bind:platformStorefronts={bulkPlatformStorefronts}
          bind:platformStoreUrls={bulkPlatformStoreUrls}
          igdbPlatformNames={[]}
          onplatformtoggle={(e: any) => toggleBulkPlatform(e.detail.platformId)}
          onstorefronttoggle={(e: any) => toggleBulkStorefrontForPlatform(e.detail.platformId, e.detail.storefrontId)}
          onstoreurlchange={(e: any) => setBulkStoreUrlForPlatform(e.detail.platformId, e.detail.url)}
        />
       </div>
      </div>
     </div>
    </div>
    <div class="bg-gray-50 px-4 py-3 sm:px-6 sm:flex sm:flex-row-reverse">
     <button
      type="button"
      disabled={isProcessingBulkPlatforms || bulkSelectedPlatforms.size === 0}
      onclick={applyBulkAddPlatforms}
      class="w-full inline-flex justify-center rounded-md border border-transparent shadow-sm px-4 py-2 bg-green-600 text-base font-medium text-white hover:bg-green-700 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-green-500 sm:ml-3 sm:w-auto sm:text-sm disabled:opacity-50 disabled:cursor-not-allowed"
     >
      {#if isProcessingBulkPlatforms}
       <svg class="animate-spin -ml-1 mr-3 h-5 w-5 text-white" xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24">
        <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
        <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path>
       </svg>
       Adding...
      {:else}
       Add Platforms
      {/if}
     </button>
     <button
      type="button"
      disabled={isProcessingBulkPlatforms}
      onclick={closeBulkPlatformAddModal}
      class="mt-3 w-full inline-flex justify-center rounded-md border border-gray-300 shadow-sm px-4 py-2 bg-white text-base font-medium text-gray-700 hover:bg-gray-50 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-primary-500 sm:mt-0 sm:mr-3 sm:w-auto sm:text-sm disabled:opacity-50 disabled:cursor-not-allowed"
     >
      Cancel
     </button>
    </div>
   </div>
  </div>
{/if}

<!-- Bulk Remove Platforms Modal -->
{#if showBulkPlatformRemoveModal}
 <div 
  class="fixed inset-0 bg-gray-500 bg-opacity-75 overflow-y-auto h-full w-full z-50" 
  role="button" 
  tabindex="0"
  aria-label="Close remove platforms modal"
  onclick={closeBulkPlatformRemoveModal} 
  onkeydown={(e) => (e.key === 'Escape' || e.key === 'Enter' || e.key === ' ') && closeBulkPlatformRemoveModal()}
 >
  <div 
    class="relative top-10 mx-auto p-5 border max-w-4xl shadow-lg rounded-md bg-white" 
    role="dialog" 
    aria-modal="true" 
    aria-labelledby="bulk-platform-remove-title"
  >
    <div class="bg-white px-4 pt-5 pb-4 sm:p-6 sm:pb-4">
     <div class="sm:flex sm:items-start">
      <div class="mx-auto flex-shrink-0 flex items-center justify-center h-12 w-12 rounded-full bg-red-100 sm:mx-0 sm:h-10 sm:w-10">
       <svg class="h-6 w-6 text-red-600" fill="none" viewBox="0 0 24 24" stroke-width="1.5" stroke="currentColor">
        <path stroke-linecap="round" stroke-linejoin="round" d="M19.5 12h-15" />
       </svg>
      </div>
      <div class="mt-3 text-center sm:mt-0 sm:ml-4 sm:text-left w-full">
       <h3 class="text-lg leading-6 font-medium text-gray-900" id="bulk-platform-remove-title">
        Remove Platforms from Selected Games
       </h3>
       <div class="mt-2">
        <p class="text-sm text-gray-500">
         Select platform/storefront associations to remove from {selectedGameIds.size} selected game{selectedGameIds.size !== 1 ? 's' : ''}
        </p>
       </div>

       <div class="mt-6">
        <PlatformRemovalSelector
          bind:availablePlatformAssociations={availablePlatformAssociations}
          bind:selectedAssociationIds={selectedAssociationIds}
          onselectionchange={handleSelectionChange}
        />
       </div>
      </div>
     </div>
    </div>
    <div class="bg-gray-50 px-4 py-3 sm:px-6 sm:flex sm:flex-row-reverse">
     <button
      type="button"
      disabled={isProcessingBulkPlatforms || selectedAssociationIds.size === 0}
      onclick={handleBulkRemovePlatforms}
      class="w-full inline-flex justify-center rounded-md border border-transparent shadow-sm px-4 py-2 bg-red-600 text-base font-medium text-white hover:bg-red-700 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-red-500 sm:ml-3 sm:w-auto sm:text-sm disabled:opacity-50 disabled:cursor-not-allowed"
     >
      {#if isProcessingBulkPlatforms}
       <svg class="animate-spin -ml-1 mr-3 h-5 w-5 text-white" xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24">
        <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
        <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path>
       </svg>
       Processing...
      {:else}
       Remove Platforms ({selectedAssociationIds.size})
      {/if}
     </button>
     <button
      type="button"
      onclick={closeBulkPlatformRemoveModal}
      class="mt-3 w-full inline-flex justify-center rounded-md border border-gray-300 shadow-sm px-4 py-2 bg-white text-base font-medium text-gray-700 hover:bg-gray-50 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-primary-500 sm:mt-0 sm:mr-3 sm:w-auto sm:text-sm"
     >
      Cancel
     </button>
    </div>
   </div>
  </div>
{/if}

</RouteGuard>