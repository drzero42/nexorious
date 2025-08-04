<script lang="ts">
 import { userGames, platforms, ui } from '$lib/stores';
 import { onMount } from 'svelte';
 import { goto } from '$app/navigation';
 import { RouteGuard, Pagination, PlatformBadges } from '$lib/components';
 import { resolveImageUrl } from '$lib/utils/image-url';
 import type { UserGameFilters } from '$lib/stores';
 import { PlayStatus, type BulkStatusUpdateRequest, type BulkDeleteRequest } from '$lib/stores/user-games.svelte';

 let viewMode: 'grid' | 'list' = 'grid';
 let searchQuery = '';
 let selectedPlatform = '';
 let selectedStorefront = '';
 let selectedStatus = '';
 let selectedOwnershipStatus = '';
 let lovedOnly = false;
 let hasNotesOnly = false;
 let ratingMin = '';
 let ratingMax = '';
 let sortBy = 'title';
 let sortOrder: 'asc' | 'desc' = 'asc';

 // Bulk selection state
 let selectedGameIds: Set<string> = new Set();
 let isSelectingAll = false;
 // let showBulkActions = false; // Not yet implemented
 let showBulkModal = false;

 // Computed state for bulk selection mode
 $: isBulkSelectionMode = selectedGameIds.size > 0;

 // Bulk operations modal state
 let bulkStatus = '';
 let bulkRating = '';
 let bulkIsLoved = false;
 let isBulkUpdating = false;
let showDeleteConfirmation = false;
let isDeletingBulk = false;

 // Local state for debounced search
 let searchTimeout: ReturnType<typeof setTimeout>;

 onMount(() => {
  // Load user games and platforms - authentication is handled by RouteGuard
  loadGames();
  platforms.fetchAll();
 });

 // Build filters based on current selections
 $: filters = {
  ...(searchQuery && { q: searchQuery }),
  ...(selectedStatus && { play_status: selectedStatus }),
  ...(selectedOwnershipStatus && { ownership_status: selectedOwnershipStatus }),
  ...(selectedPlatform && { platform_id: selectedPlatform }),
  ...(selectedStorefront && { storefront_id: selectedStorefront }),
  ...(lovedOnly && { is_loved: true }),
  ...(hasNotesOnly && { has_notes: true }),
  ...(ratingMin && { rating_min: parseInt(ratingMin) }),
  ...(ratingMax && { rating_max: parseInt(ratingMax) }),
  sort_by: sortBy,
  sort_order: sortOrder
 } as UserGameFilters;

 // Load games with current filters and pagination
 async function loadGames() {
  try {
   await userGames.loadUserGames(
    filters,
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
   await userGames.loadUserGames(filters, 1, userGames.value.pagination.per_page);
  } catch (error) {
   console.error('Failed to load games:', error);
  }
 }

 // Watch for filter changes
 $: if (selectedStatus || selectedPlatform || selectedStorefront || selectedOwnershipStatus || lovedOnly || hasNotesOnly || ratingMin || ratingMax || sortBy || sortOrder) {
  handleFilterChange();
 }

 // Watch for search query changes
 $: if (searchQuery !== undefined) {
  handleSearch();
 }

 function handleAddGame() {
  goto('/games/add');
 }

 function handleGameClick(gameId: string) {
  if (isBulkSelectionMode) {
   // In bulk selection mode, toggle selection instead of navigating
   toggleGameSelection(gameId);
  } else {
   // Normal mode, navigate to game detail
   goto(`/games/${gameId}`);
  }
 }

 // Pagination handlers
 async function handlePageChange(page: number) {
  try {
   await userGames.loadUserGames(filters, page, userGames.value.pagination.per_page);
  } catch (error) {
   console.error('Failed to load games:', error);
  }
 }

 async function handleItemsPerPageChange(perPage: number) {
  try {
   await userGames.loadUserGames(filters, 1, perPage);
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
 $: hasActiveFilters = searchQuery || selectedPlatform || selectedStorefront || selectedStatus || selectedOwnershipStatus || lovedOnly || hasNotesOnly || ratingMin || ratingMax;

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
 $: if (userGames.value.userGames && Array.isArray(userGames.value.userGames)) {
  // Remove any selected IDs that are no longer in current results
  const currentGameIds = new Set(userGames.value.userGames.map(game => game.id));
  selectedGameIds = new Set([...selectedGameIds].filter(id => currentGameIds.has(id)));
  
  // Update "select all" state
  isSelectingAll = selectedGameIds.size > 0 && selectedGameIds.size === userGames.value.userGames.length;
  
  updateBulkActionsVisibility();
 }

 // Bulk Operations Functions
 function resetBulkModal() {
  bulkStatus = '';
  bulkRating = '';
  bulkIsLoved = false;
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
</script>

<svelte:head>
 <title>My Games - Nexorious</title>
</svelte:head>

<RouteGuard requireAuth={true}>
<div class="space-y-6">
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
    on:click={handleAddGame}
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
      on:click={clearAllFilters}
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
        on:click={() => {
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
        on:click={clearSelection}
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
        on:click={() => showBulkModal = true}
        class="btn-secondary text-sm px-3 py-1"
       >
        Bulk Actions
       </button>
      {/if}
     </div>
    {/if}
    
    <!-- View Mode Toggle -->
    <div class="inline-flex rounded-md shadow-sm" role="group">
     <button
      on:click={() => viewMode = 'grid'}
      class="{viewMode === 'grid' ? 'bg-primary-50 border-primary-500 text-primary-700' : 'bg-white border-gray-300 text-gray-700 hover:bg-gray-50'} relative inline-flex items-center rounded-l-md border px-3 py-2 text-sm font-medium focus:z-10 focus:border-primary-500 focus:outline-none focus:ring-1 focus:ring-primary-500"
     >
      <svg class="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke-width="1.5" stroke="currentColor">
       <path stroke-linecap="round" stroke-linejoin="round" d="M3.375 19.5h17.25m-17.25 0a1.125 1.125 0 01-1.125-1.125M3.375 19.5h17.25m-17.25 0a1.125 1.125 0 001.125 1.125m0 0V4.875c0-.621.504-1.125 1.125-1.125M3.375 19.5V4.875c0-.621.504-1.125 1.125-1.125m0 0h17.25m-17.25 0a1.125 1.125 0 011.125-1.125h15.75m0 0a1.125 1.125 0 011.125-1.125M18.375 2.25h-7.5A1.125 1.125 0 009.75 3.375v1.875m7.5-1.875A1.125 1.125 0 0018.375 3.375v1.875m-7.5 0V18.375m7.5-13.125V18.375m-7.5 0h7.5" />
      </svg>
      <span class="sr-only">Grid view</span>
     </button>
     <button
      on:click={() => viewMode = 'list'}
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
     {#each $platforms.platforms as platform (platform.id)}
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
     {#each $platforms.storefronts as storefront (storefront.id)}
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
      on:click={handleAddGame}
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
      on:click={(e) => {
       // Don't navigate if clicking on checkbox
       if ((e.target as HTMLInputElement).type !== 'checkbox') {
        handleGameClick(userGame.id);
       }
      }}
      on:keydown={(e) => e.key === 'Enter' && handleGameClick(userGame.id)}
      tabindex="0"
      role="button"
      aria-label="{isBulkSelectionMode ? 'Select ' + userGame.game.title : 'View details for ' + userGame.game.title}"
      class="group relative flex flex-col overflow-hidden rounded-lg border border-gray-200 bg-white shadow-sm hover:shadow-md transition-shadow {isBulkSelectionMode ? 'cursor-pointer' : 'cursor-pointer'} focus:outline-none focus:ring-2 focus:ring-primary-500 focus:ring-offset-2 {selectedGameIds.has(userGame.id) ? 'ring-2 ring-primary-500' : ''} {isBulkSelectionMode ? 'hover:ring-2 hover:ring-primary-300' : ''}"
     >
      <!-- Selection Checkbox -->
      <div class="absolute top-2 left-2 z-10">
       <input
        type="checkbox"
        checked={selectedGameIds.has(userGame.id)}
        on:change={() => toggleGameSelection(userGame.id)}
        on:click={(e) => e.stopPropagation()}
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
         on:error={(e) => {
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
          on:change={() => {
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
         on:click={(e) => {
          // Don't navigate if clicking on checkbox
          if ((e.target as HTMLInputElement).type !== 'checkbox') {
           handleGameClick(userGame.id);
          }
         }}
         on:keydown={(e) => e.key === 'Enter' && handleGameClick(userGame.id)}
         tabindex="0"
         class="hover:bg-gray-50 cursor-pointer focus:outline-none focus:bg-gray-50 {selectedGameIds.has(userGame.id) ? 'bg-primary-50' : ''} {isBulkSelectionMode ? 'hover:bg-primary-50' : ''}"
        >
         <td class="whitespace-nowrap py-4 pl-4 pr-2 text-sm sm:pl-6">
          <input
           type="checkbox"
           checked={selectedGameIds.has(userGame.id)}
           on:change={() => toggleGameSelection(userGame.id)}
           on:click={(e) => e.stopPropagation()}
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
 <!-- svelte-ignore a11y-click-events-have-key-events -->
 <!-- svelte-ignore a11y-no-static-element-interactions -->
 <div 
  class="fixed inset-0 bg-gray-500 bg-opacity-75 overflow-y-auto h-full w-full z-50" 
  role="dialog" 
  aria-modal="true" 
  aria-labelledby="modal-title"
  tabindex="-1" 
  on:click={closeBulkModal} 
  on:keydown={(e) => e.key === 'Escape' && closeBulkModal()}
 >
  <div class="relative top-20 mx-auto p-5 border max-w-lg shadow-lg rounded-md bg-white" on:click|stopPropagation>
    <div class="bg-white px-4 pt-5 pb-4 sm:p-6 sm:pb-4">
     <div class="sm:flex sm:items-start">
      <div class="mx-auto flex-shrink-0 flex items-center justify-center h-12 w-12 rounded-full bg-primary-100 sm:mx-0 sm:h-10 sm:w-10">
       <svg class="h-6 w-6 text-primary-600" fill="none" viewBox="0 0 24 24" stroke-width="1.5" stroke="currentColor">
        <path stroke-linecap="round" stroke-linejoin="round" d="M16.862 4.487l1.687-1.688a1.875 1.875 0 112.652 2.652L10.582 16.07a4.5 4.5 0 01-1.897 1.13L6 18l.8-2.685a4.5 4.5 0 011.13-1.897l8.932-8.931zm0 0L19.5 7.125M18 14v4.75A2.25 2.25 0 0115.75 21H5.25A2.25 2.25 0 013 18.75V8.25A2.25 2.25 0 015.25 6H10" />
       </svg>
      </div>
      <div class="mt-3 text-center sm:mt-0 sm:ml-4 sm:text-left w-full">
       <h3 class="text-lg leading-6 font-medium text-gray-900" id="modal-title">
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
      on:click={applyBulkOperations}
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
      on:click={showBulkDeleteConfirmation}
      class="w-full inline-flex justify-center rounded-md border border-transparent shadow-sm px-4 py-2 bg-red-600 text-base font-medium text-white hover:bg-red-700 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-red-500 sm:ml-3 sm:w-auto sm:text-sm disabled:opacity-50 disabled:cursor-not-allowed"
     >
      Delete Selected
     </button>
     <button
      type="button"
      disabled={isBulkUpdating}
      on:click={closeBulkModal}
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
 <!-- svelte-ignore a11y-click-events-have-key-events -->
 <!-- svelte-ignore a11y-no-static-element-interactions -->
 <div 
  class="fixed inset-0 bg-gray-500 bg-opacity-75 overflow-y-auto h-full w-full z-50" 
  role="dialog" 
  aria-modal="true" 
  tabindex="-1" 
  on:click={cancelBulkDelete} 
  on:keydown={(e) => e.key === 'Escape' && cancelBulkDelete()}
 >
  <div class="relative top-20 mx-auto p-5 border max-w-lg shadow-lg rounded-md bg-white" on:click|stopPropagation>
    <div class="bg-white px-4 pt-5 pb-4 sm:p-6 sm:pb-4">
     <div class="sm:flex sm:items-start">
      <div class="mx-auto flex-shrink-0 flex items-center justify-center h-12 w-12 rounded-full bg-red-100 sm:mx-0 sm:h-10 sm:w-10">
       <svg class="h-6 w-6 text-red-600" fill="none" viewBox="0 0 24 24" stroke-width="1.5" stroke="currentColor">
        <path stroke-linecap="round" stroke-linejoin="round" d="M12 9v3.75m-9.303 3.376c-.866 1.5.217 3.374 1.948 3.374h14.71c1.73 0 2.813-1.874 1.948-3.374L13.949 3.378c-.866-1.5-3.032-1.5-3.898 0L2.697 16.126zM12 15.75h.007v.008H12v-.008z" />
       </svg>
      </div>
      <div class="mt-3 text-center sm:mt-0 sm:ml-4 sm:text-left">
       <h3 class="text-lg leading-6 font-medium text-gray-900" id="modal-title">
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
      on:click={confirmBulkDelete}
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
      on:click={cancelBulkDelete}
      class="mt-3 w-full inline-flex justify-center rounded-md border border-gray-300 shadow-sm px-4 py-2 bg-white text-base font-medium text-gray-700 hover:bg-gray-50 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-primary-500 sm:mt-0 sm:mr-3 sm:w-auto sm:text-sm disabled:opacity-50 disabled:cursor-not-allowed"
     >
      Cancel
     </button>
    </div>
   </div>
  </div>
{/if}

</RouteGuard>