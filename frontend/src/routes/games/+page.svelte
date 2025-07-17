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

 function getStatusColor(status: string) {
  const colors: Record<string, string> = {
   'not_started': 'bg-gray-100 text-gray-800',
   'in_progress': 'bg-blue-100 text-blue-800',
   'completed': 'bg-green-100 text-green-800',
   'mastered': 'bg-purple-100 text-purple-800',
   'dominated': 'bg-yellow-100 text-yellow-800',
   'shelved': 'bg-orange-100 text-orange-800',
   'dropped': 'bg-red-100 text-red-800',
   'replay': 'bg-indigo-100 text-indigo-800'
  };
  return colors[status] || 'bg-gray-100 text-gray-800';
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
 <div class="flex flex-col sm:flex-row sm:items-center sm:justify-between">
  <div>
   <h1 class="text-3xl font-bold text-gray-900">My Games</h1>
   <p class="text-gray-600 mt-1">
    {userGames.value.pagination.total} games in your collection
   </p>
  </div>
  <div class="mt-4 sm:mt-0">
   <button
    on:click={handleAddGame}
    class="inline-flex items-center justify-center font-medium rounded-lg transition-all duration-200 outline-none focus:ring-2 focus:ring-blue-500/50 px-6 py-3 text-base bg-blue-600 text-white hover:bg-blue-700"
   >
    <svg class="w-5 h-5 mr-2" fill="none" stroke="currentColor" viewBox="0 0 24 24">
     <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 6v6m0 0v6m0-6h6m-6 0H6" />
    </svg>
    Add Game
   </button>
  </div>
 </div>

 <!-- Filters and Search -->
 <div class="bg-white rounded-xl shadow-sm border border-gray-200">
  <div class="p-6">
   <div class="grid grid-cols-1 md:grid-cols-4 gap-4">
    <!-- Search -->
    <div>
     <label for="search" class="block text-sm font-medium text-gray-700 mb-1">
      Search
     </label>
     <input
      id="search"
      type="text"
      bind:value={searchQuery}
      placeholder="Search games..."
      class="block w-full px-3 py-2 border border-gray-300 rounded-lg shadow-sm bg-white text-gray-900 placeholder-gray-500 focus:outline-none focus:ring-2 focus:ring-blue-500/50 focus:border-blue-500"
     />
    </div>

    <!-- Status Filter -->
    <div>
     <label for="status" class="block text-sm font-medium text-gray-700 mb-1">
      Status
     </label>
     <select
      id="status"
      bind:value={selectedStatus}
      class="block w-full px-3 py-2 border border-gray-300 rounded-lg shadow-sm bg-white text-gray-900 placeholder-gray-500 focus:outline-none focus:ring-2 focus:ring-blue-500/50 focus:border-blue-500"
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
     <label for="sortBy" class="block text-sm font-medium text-gray-700 mb-1">
      Sort By
     </label>
     <select
      id="sortBy"
      bind:value={sortBy}
      class="block w-full px-3 py-2 border border-gray-300 rounded-lg shadow-sm bg-white text-gray-900 placeholder-gray-500 focus:outline-none focus:ring-2 focus:ring-blue-500/50 focus:border-blue-500"
     >
      <option value="title">Title</option>
      <option value="personal_rating">Rating</option>
      <option value="play_status">Status</option>
      <option value="genre">Genre</option>
      <option value="release_date">Release Date</option>
     </select>
    </div>

    <!-- View Mode -->
    <div>
     <label for="view-mode" class="block text-sm font-medium text-gray-700 mb-1">
      View
     </label>
     <div id="view-mode" class="flex rounded-lg shadow-sm" role="radiogroup" aria-labelledby="view-mode">
      <button
       on:click={() => viewMode = 'grid'}
       class="px-4 py-2 text-sm font-medium rounded-l-lg border transition-colors {viewMode === 'grid' ? 'bg-blue-600 text-white border-blue-600' : 'bg-white text-gray-700 border-gray-300 hover:bg-gray-50'}"
       role="radio"
       aria-checked={viewMode === 'grid'}
      >
       <svg class="w-4 h-4 mr-2 inline" fill="none" stroke="currentColor" viewBox="0 0 24 24">
        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 6a2 2 0 012-2h2a2 2 0 012 2v2a2 2 0 01-2 2H6a2 2 0 01-2-2V6zM14 6a2 2 0 012-2h2a2 2 0 012 2v2a2 2 0 01-2 2h-2a2 2 0 01-2-2V6zM4 16a2 2 0 012-2h2a2 2 0 012 2v2a2 2 0 01-2 2H6a2 2 0 01-2-2v-2zM14 16a2 2 0 012-2h2a2 2 0 012 2v2a2 2 0 01-2 2h-2a2 2 0 01-2-2v-2z" />
       </svg>
       Grid
      </button>
      <button
       on:click={() => viewMode = 'list'}
       class="px-4 py-2 text-sm font-medium rounded-r-lg border-t border-r border-b transition-colors {viewMode === 'list' ? 'bg-blue-600 text-white border-blue-600' : 'bg-white text-gray-700 border-gray-300 hover:bg-gray-50'}"
       role="radio"
       aria-checked={viewMode === 'list'}
      >
       <svg class="w-4 h-4 mr-2 inline" fill="none" stroke="currentColor" viewBox="0 0 24 24">
        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 6h16M4 10h16M4 14h16M4 18h16" />
       </svg>
       List
      </button>
     </div>
    </div>
   </div>
  </div>
 </div>

 <!-- Games Display -->
 {#if userGames.value.isLoading}
  <div class="text-center py-12">
   <div class="w-8 h-8 mx-auto mb-4 border-2 border-gray-200 border-t-blue-600 rounded-full animate-spin"></div>
   <div class="text-gray-500">Loading games...</div>
  </div>
 {:else if userGames.value.userGames.length === 0}
  <div class="text-center py-12">
   <div class="w-24 h-24 bg-gray-100 rounded-full flex items-center justify-center mx-auto mb-4">
    <svg class="w-12 h-12 text-gray-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">
     <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 11H5m14 0a2 2 0 012 2v6a2 2 0 01-2 2H5a2 2 0 01-2-2v-6a2 2 0 012-2m14 0V9a2 2 0 00-2-2M5 11V9a2 2 0 012-2m0 0V5a2 2 0 012-2h6a2 2 0 012 2v2M7 7h10" />
    </svg>
   </div>
   <h3 class="text-lg font-medium text-gray-900 mb-2">
    {userGames.value.pagination.total === 0 ? 'No games in your collection yet' : 'No games match your filters'}
   </h3>
   <p class="text-gray-500 mb-6">
    {userGames.value.pagination.total === 0 ? 'Start building your game library by adding your first game.' : 'Try adjusting your search or filter criteria.'}
   </p>
   {#if userGames.value.pagination.total === 0}
    <button
     on:click={handleAddGame}
     class="inline-flex items-center justify-center font-medium rounded-lg transition-all duration-200 outline-none focus:ring-2 focus:ring-blue-500/50 px-6 py-3 text-base bg-blue-600 text-white hover:bg-blue-700"
    >
     <svg class="w-5 h-5 mr-2" fill="none" stroke="currentColor" viewBox="0 0 24 24">
      <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 6v6m0 0v6m0-6h6m-6 0H6" />
     </svg>
     Add Your First Game
    </button>
   {/if}
  </div>
 {:else}
  <!-- Grid View -->
  {#if viewMode === 'grid'}
   <div class="grid grid-cols-1 sm:grid-cols-2 md:grid-cols-3 lg:grid-cols-4 xl:grid-cols-5 gap-6">
    {#each userGames.value.userGames as userGame (userGame.id)}
     <div
      class="bg-white rounded-xl shadow-sm border border-gray-200 cursor-pointer transition-all duration-200 hover:shadow-md hover:scale-105 group animate-fade-in"
      on:click={() => handleGameClick(userGame.id)}
      on:keydown={(e) => e.key === 'Enter' && handleGameClick(userGame.id)}
      tabindex="0"
      role="button"
      aria-label="View details for {userGame.game.title}"
     >
      <div class="aspect-[3/4] bg-gray-200 rounded-t-xl overflow-hidden relative">
       {#if userGame.game.cover_art_url}
        <img
         src={userGame.game.cover_art_url}
         alt="Cover art for {userGame.game.title}"
         class="w-full h-full object-cover transition-transform duration-200 group-hover:scale-105"
         loading="lazy"
         on:error={(e) => {
          const target = e.currentTarget as HTMLImageElement;
          const nextElement = target.nextElementSibling as HTMLElement;
          target.style.display = 'none';
          if (nextElement) {
            nextElement.style.display = 'flex';
          }
         }}
        />
        <div class="w-full h-full flex items-center justify-center text-gray-400 hidden">
         <svg class="w-12 h-12" fill="currentColor" viewBox="0 0 20 20">
          <path fill-rule="evenodd" d="M4 3a2 2 0 00-2 2v10a2 2 0 002 2h12a2 2 0 002-2V5a2 2 0 00-2-2H4zm12 12H4l4-8 3 6 2-4 3 6z" clip-rule="evenodd" />
         </svg>
        </div>
       {:else}
        <div class="w-full h-full flex flex-col items-center justify-center text-gray-400">
         <svg class="w-12 h-12 mb-2" fill="currentColor" viewBox="0 0 20 20">
          <path fill-rule="evenodd" d="M4 3a2 2 0 00-2 2v10a2 2 0 002 2h12a2 2 0 002-2V5a2 2 0 00-2-2H4zm12 12H4l4-8 3 6 2-4 3 6z" clip-rule="evenodd" />
         </svg>
         <span class="text-xs">No Cover</span>
        </div>
       {/if}
       
       <!-- Loved indicator -->
       {#if userGame.is_loved}
        <div class="absolute top-2 right-2 bg-red-500 text-white rounded-full p-1.5 shadow-md">
         <svg class="w-4 h-4" fill="currentColor" viewBox="0 0 20 20">
          <path fill-rule="evenodd" d="M3.172 5.172a4 4 0 015.656 0L10 6.343l1.172-1.171a4 4 0 115.656 5.656L10 17.657l-6.828-6.829a4 4 0 010-5.656z" clip-rule="evenodd" />
         </svg>
        </div>
       {/if}
      </div>
      
      <div class="p-4">
       <h3 class="font-semibold text-gray-900 text-sm mb-1 truncate" title="{userGame.game.title}">
        {userGame.game.title}
       </h3>
       <p class="text-xs text-gray-500 mb-3 truncate" title="{userGame.game.genre || 'Unknown Genre'}">
        {userGame.game.genre || 'Unknown Genre'}
       </p>
       
       <div class="flex items-center justify-between mb-3">
        <span class="inline-flex items-center px-2.5 py-0.5 rounded-full text-xs font-medium {getStatusColor(userGame.play_status)}">
         {getStatusLabel(userGame.play_status)}
        </span>
        {#if userGame.personal_rating}
         <div class="flex items-center">
          <span class="text-yellow-400">★</span>
          <span class="text-xs text-gray-600 ml-1">
           {userGame.personal_rating}
          </span>
         </div>
        {/if}
       </div>
       
       <!-- Additional info -->
       <div class="flex items-center justify-between text-xs text-gray-500">
        <span>{userGame.hours_played || 0}h played</span>
        {#if userGame.game.release_date}
         <span>{new Date(userGame.game.release_date).getFullYear()}</span>
        {/if}
       </div>
      </div>
     </div>
    {/each}
   </div>
  {:else}
   <!-- List View -->
   <div class="bg-white rounded-xl shadow-sm border border-gray-200 overflow-hidden">
    <div class="overflow-x-auto">
     <table class="min-w-full divide-y divide-gray-200">
      <thead class="bg-gray-50">
       <tr>
        <th class="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">
         Game
        </th>
        <th class="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">
         Genre
        </th>
        <th class="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">
         Status
        </th>
        <th class="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">
         Rating
        </th>
        <th class="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">
         Hours
        </th>
       </tr>
      </thead>
      <tbody class="bg-white divide-y divide-gray-200">
       {#each userGames.value.userGames as userGame (userGame.id)}
        <tr
         class="hover:bg-gray-50 cursor-pointer transition-colors animate-fade-in"
         on:click={() => handleGameClick(userGame.id)}
         on:keydown={(e) => e.key === 'Enter' && handleGameClick(userGame.id)}
         tabindex="0"
        >
         <td class="px-6 py-4 whitespace-nowrap">
          <div class="flex items-center">
           <div class="flex-shrink-0 h-12 w-12 relative">
            {#if userGame.game.cover_art_url}
             <img
              src={userGame.game.cover_art_url}
              alt={userGame.game.title}
              class="h-12 w-12 rounded-lg object-cover"
             />
            {:else}
             <div class="h-12 w-12 rounded-lg bg-gray-300 flex items-center justify-center">
              <svg class="w-6 h-6 text-gray-400" fill="currentColor" viewBox="0 0 20 20">
               <path fill-rule="evenodd" d="M4 3a2 2 0 00-2 2v10a2 2 0 002 2h12a2 2 0 002-2V5a2 2 0 00-2-2H4zm12 12H4l4-8 3 6 2-4 3 6z" clip-rule="evenodd" />
              </svg>
             </div>
            {/if}
            {#if userGame.is_loved}
             <div class="absolute -top-1 -right-1 bg-red-500 text-white rounded-full p-1">
              <svg class="w-3 h-3" fill="currentColor" viewBox="0 0 20 20">
               <path fill-rule="evenodd" d="M3.172 5.172a4 4 0 015.656 0L10 6.343l1.172-1.171a4 4 0 115.656 5.656L10 17.657l-6.828-6.829a4 4 0 010-5.656z" clip-rule="evenodd" />
              </svg>
             </div>
            {/if}
           </div>
           <div class="ml-4">
            <div class="text-sm font-medium text-gray-900">
             {userGame.game.title}
            </div>
            <div class="text-sm text-gray-500">
             {userGame.game.developer || 'Unknown Developer'}
            </div>
           </div>
          </div>
         </td>
         <td class="px-6 py-4 whitespace-nowrap text-sm text-gray-900">
          {userGame.game.genre || 'Unknown'}
         </td>
         <td class="px-6 py-4 whitespace-nowrap">
          <span class="inline-flex items-center px-2.5 py-0.5 rounded-full text-xs font-medium {getStatusColor(userGame.play_status)}">
           {getStatusLabel(userGame.play_status)}
          </span>
         </td>
         <td class="px-6 py-4 whitespace-nowrap text-sm text-gray-900">
          {#if userGame.personal_rating}
           <div class="flex items-center">
            <span class="text-yellow-400">★</span>
            <span class="ml-1">{userGame.personal_rating}</span>
           </div>
          {:else}
           <span class="text-gray-400">-</span>
          {/if}
         </td>
         <td class="px-6 py-4 whitespace-nowrap text-sm text-gray-900">
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
  <Pagination 
   currentPage={userGames.value.pagination.page}
   totalPages={userGames.value.pagination.pages}
   totalItems={userGames.value.pagination.total}
   itemsPerPage={userGames.value.pagination.per_page}
   onPageChange={handlePageChange}
   onItemsPerPageChange={handleItemsPerPageChange}
  />
 {/if}
</div>
</RouteGuard>