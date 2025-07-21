<script lang="ts">
 import { userGames } from '$lib/stores';
 import { onMount } from 'svelte';
 import { goto } from '$app/navigation';
 import { RouteGuard, Pagination } from '$lib/components';
 import type { UserGameFilters } from '$lib/stores';

 let viewMode: 'grid' | 'list' = 'grid';
 let searchQuery = '';
 let selectedPlatform = '';
 let selectedStatus = '';
 let sortBy = 'title';

 // Local state for debounced search
 let searchTimeout: ReturnType<typeof setTimeout>;

 onMount(() => {
  // Load user games - authentication is handled by RouteGuard
  loadGames();
 });

 // Build filters based on current selections
 $: filters = {
  ...(selectedStatus && { play_status: selectedStatus }),
  ...(selectedPlatform && { platform_id: selectedPlatform }),
  ...(searchQuery && { q: searchQuery })
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
 $: if (selectedStatus || selectedPlatform) {
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
  goto(`/games/${gameId}`);
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
    {userGames.value.pagination.total} games in your collection
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

 <!-- Filters and Search -->
 <div class="border-b border-gray-200 pb-5">
  <div class="sm:flex sm:items-center sm:justify-between">
   <div class="grid grid-cols-1 gap-4 sm:grid-cols-4 sm:gap-6">
    <!-- Search -->
    <div class="sm:col-span-2">
     <label for="search" class="form-label">
      Search
     </label>
     <div class="relative">
      <div class="pointer-events-none absolute inset-y-0 left-0 flex items-center pl-3">
       <svg class="h-5 w-5 text-gray-400" viewBox="0 0 20 20" fill="currentColor">
        <path fill-rule="evenodd" d="M9 3.5a5.5 5.5 0 100 11 5.5 5.5 0 000-11zM2 9a7 7 0 1112.452 4.391l3.328 3.329a.75.75 0 11-1.06 1.06l-3.329-3.328A7 7 0 012 9z" clip-rule="evenodd" />
       </svg>
      </div>
      <input
       id="search"
       type="text"
       bind:value={searchQuery}
       placeholder="Search games..."
       class="form-input pl-10"
      />
     </div>
    </div>

    <!-- Status Filter -->
    <div>
     <label for="status" class="form-label">
      Status
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

    <!-- Sort By -->
    <div>
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
     </select>
    </div>
   </div>

   <!-- View Mode -->
   <div class="mt-4 sm:ml-6 sm:mt-0">
    <span class="form-label">View</span>
    <div class="mt-1">
     <div class="inline-flex rounded-md shadow-sm" role="group">
      <button
       on:click={() => viewMode = 'grid'}
       class="{viewMode === 'grid' ? 'bg-primary-50 border-primary-500 text-primary-700' : 'bg-white border-gray-300 text-gray-700 hover:bg-gray-50'} relative inline-flex items-center rounded-l-md border px-3 py-2 text-sm font-medium focus:z-10 focus:border-primary-500 focus:outline-none focus:ring-1 focus:ring-primary-500"
      >
       <svg class="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke-width="1.5" stroke="currentColor">
        <path stroke-linecap="round" stroke-linejoin="round" d="M3.375 19.5h17.25m-17.25 0a1.125 1.125 0 01-1.125-1.125M3.375 19.5h17.25m-17.25 0a1.125 1.125 0 001.125 1.125m0 0V4.875c0-.621.504-1.125 1.125-1.125M3.375 19.5V4.875c0-.621.504-1.125 1.125-1.125m0 0h17.25m-17.25 0a1.125 1.125 0 011.125-1.125h15.75m0 0a1.125 1.125 0 011.125-1.125M18.375 2.25h-7.5A1.125 1.125 0 009.75 3.375v1.875m7.5-1.875A1.125 1.125 0 0118.375 3.375v1.875m-7.5 0V18.375m7.5-13.125V18.375m-7.5 0h7.5" />
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
 {:else if userGames.value.userGames.length === 0}
  <div class="text-center py-12">
   <svg class="mx-auto h-12 w-12 text-gray-400" fill="none" viewBox="0 0 24 24" stroke="currentColor">
    <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9.172 16.172a4 4 0 015.656 0M9 12h6m-6 4h6m2 5H7a2 2 0 01-2-2V5a2 2 0 012-2h5.586a1 1 0 01.707.293l5.414 5.414a1 1 0 01.293.707V19a2 2 0 01-2 2z" />
   </svg>
   <h3 class="mt-4 text-lg font-medium text-gray-900">
    {userGames.value.pagination.total === 0 ? 'No games in your collection yet' : 'No games match your filters'}
   </h3>
   <p class="mt-2 text-sm text-gray-500">
    {userGames.value.pagination.total === 0 ? 'Start building your game library by adding your first game.' : 'Try adjusting your search or filter criteria.'}
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
    {#each userGames.value.userGames as userGame (userGame.id)}
     <div
      on:click={() => handleGameClick(userGame.id)}
      on:keydown={(e) => e.key === 'Enter' && handleGameClick(userGame.id)}
      tabindex="0"
      role="button"
      aria-label="View details for {userGame.game.title}"
      class="group relative flex flex-col overflow-hidden rounded-lg border border-gray-200 bg-white shadow-sm hover:shadow-md transition-shadow cursor-pointer focus:outline-none focus:ring-2 focus:ring-primary-500 focus:ring-offset-2"
     >
      <div class="aspect-[3/4] overflow-hidden bg-gray-100">
       {#if userGame.game.cover_art_url}
        <img
         src={userGame.game.cover_art_url}
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
       <div class="absolute top-2 left-2">
        <span class="inline-flex items-center px-2 py-1 rounded-full text-xs font-medium status-{userGame.play_status.replace('_', '-')}">
         {getStatusLabel(userGame.play_status)}
        </span>
       </div>

       <!-- Loved indicator -->
       {#if userGame.is_loved}
        <div class="absolute top-2 right-2">
         <span class="inline-flex items-center justify-center w-6 h-6 rounded-full bg-red-100 text-red-600">
          ♥
         </span>
        </div>
       {/if}
      </div>
      
      <div class="flex flex-1 flex-col justify-between p-4">
       <div class="flex-1">
        <h3 class="text-sm font-medium text-gray-900 line-clamp-2" title="{userGame.game.title}">
         {userGame.game.title}
        </h3>
        <p class="mt-1 text-sm text-gray-500" title="{userGame.game.genre || 'Unknown Genre'}">
         {userGame.game.genre || 'Unknown Genre'}
        </p>
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
        <th scope="col" class="py-3.5 pl-4 pr-3 text-left text-sm font-semibold text-gray-900 sm:pl-6">
         Game
        </th>
        <th scope="col" class="px-3 py-3.5 text-left text-sm font-semibold text-gray-900">
         Genre
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
       {#each userGames.value.userGames as userGame (userGame.id)}
        <tr
         on:click={() => handleGameClick(userGame.id)}
         on:keydown={(e) => e.key === 'Enter' && handleGameClick(userGame.id)}
         tabindex="0"
         class="hover:bg-gray-50 cursor-pointer focus:outline-none focus:bg-gray-50"
        >
         <td class="whitespace-nowrap py-4 pl-4 pr-3 text-sm sm:pl-6">
          <div class="flex items-center space-x-4">
           <div class="relative h-12 w-9 flex-shrink-0">
            {#if userGame.game.cover_art_url}
             <img
              src={userGame.game.cover_art_url}
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
            {#if userGame.is_loved}
             <div class="absolute -top-1 -right-1 h-4 w-4 rounded-full bg-red-100 flex items-center justify-center">
              <span class="text-xs text-red-600">♥</span>
             </div>
            {/if}
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
</RouteGuard>