<script lang="ts">
  import { auth, userGames, search } from '$lib/stores';
  import { onMount } from 'svelte';
  import { goto } from '$app/navigation';
  import { RouteGuard, Pagination } from '$lib/components';
  import type { UserGameFilters } from '$lib/stores';

  let viewMode: 'grid' | 'list' = 'grid';
  let searchQuery = '';
  let selectedPlatform = '';
  let selectedStatus = '';
  let sortBy = 'title';
  let sortOrder: 'asc' | 'desc' = 'asc';

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
    const colors = {
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
    const labels = {
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
      <h1 class="text-2xl font-bold text-gray-900 dark:text-white">My Games</h1>
      <p class="text-gray-600 dark:text-gray-400">
        {userGames.value.pagination.total} games in your collection
      </p>
    </div>
    <div class="mt-4 sm:mt-0">
      <button
        on:click={handleAddGame}
        class="bg-blue-600 hover:bg-blue-700 text-white px-4 py-2 rounded-lg transition-colors"
      >
        Add Game
      </button>
    </div>
  </div>

  <!-- Filters and Search -->
  <div class="bg-white dark:bg-gray-800 rounded-lg shadow p-4">
    <div class="grid grid-cols-1 md:grid-cols-4 gap-4">
      <!-- Search -->
      <div>
        <label for="search" class="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">
          Search
        </label>
        <input
          id="search"
          type="text"
          bind:value={searchQuery}
          placeholder="Search games..."
          class="w-full px-3 py-2 border border-gray-300 dark:border-gray-600 rounded-md shadow-sm focus:outline-none focus:ring-blue-500 focus:border-blue-500 dark:bg-gray-700 dark:text-white"
        />
      </div>

      <!-- Status Filter -->
      <div>
        <label for="status" class="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">
          Status
        </label>
        <select
          id="status"
          bind:value={selectedStatus}
          class="w-full px-3 py-2 border border-gray-300 dark:border-gray-600 rounded-md shadow-sm focus:outline-none focus:ring-blue-500 focus:border-blue-500 dark:bg-gray-700 dark:text-white"
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
        <label for="sortBy" class="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">
          Sort By
        </label>
        <select
          id="sortBy"
          bind:value={sortBy}
          class="w-full px-3 py-2 border border-gray-300 dark:border-gray-600 rounded-md shadow-sm focus:outline-none focus:ring-blue-500 focus:border-blue-500 dark:bg-gray-700 dark:text-white"
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
        <label class="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">
          View
        </label>
        <div class="flex rounded-md shadow-sm">
          <button
            on:click={() => viewMode = 'grid'}
            class="px-4 py-2 text-sm font-medium rounded-l-md border {viewMode === 'grid' ? 'bg-blue-600 text-white border-blue-600' : 'bg-white dark:bg-gray-700 text-gray-700 dark:text-gray-300 border-gray-300 dark:border-gray-600'}"
          >
            Grid
          </button>
          <button
            on:click={() => viewMode = 'list'}
            class="px-4 py-2 text-sm font-medium rounded-r-md border-t border-r border-b {viewMode === 'list' ? 'bg-blue-600 text-white border-blue-600' : 'bg-white dark:bg-gray-700 text-gray-700 dark:text-gray-300 border-gray-300 dark:border-gray-600'}"
          >
            List
          </button>
        </div>
      </div>
    </div>
  </div>

  <!-- Games Display -->
  {#if userGames.value.isLoading}
    <div class="text-center py-8">
      <div class="text-gray-500 dark:text-gray-400">Loading games...</div>
    </div>
  {:else if userGames.value.userGames.length === 0}
    <div class="text-center py-8">
      <div class="text-gray-500 dark:text-gray-400">
        {userGames.value.pagination.total === 0 ? 'No games in your collection yet.' : 'No games match your filters.'}
      </div>
      {#if userGames.value.pagination.total === 0}
        <button
          on:click={handleAddGame}
          class="mt-4 bg-blue-600 hover:bg-blue-700 text-white px-4 py-2 rounded-lg transition-colors"
        >
          Add Your First Game
        </button>
      {/if}
    </div>
  {:else}
    <!-- Grid View -->
    {#if viewMode === 'grid'}
      <div class="grid grid-cols-1 sm:grid-cols-2 md:grid-cols-3 lg:grid-cols-4 gap-4">
        {#each userGames.value.userGames as userGame (userGame.id)}
          <div
            class="bg-white dark:bg-gray-800 rounded-lg shadow hover:shadow-md transition-shadow cursor-pointer"
            on:click={() => handleGameClick(userGame.id)}
            on:keydown={(e) => e.key === 'Enter' && handleGameClick(userGame.id)}
            tabindex="0"
          >
            <div class="aspect-[3/4] bg-gray-200 dark:bg-gray-700 rounded-t-lg">
              {#if userGame.game.cover_art_url}
                <img
                  src={userGame.game.cover_art_url}
                  alt={userGame.game.title}
                  class="w-full h-full object-cover rounded-t-lg"
                />
              {:else}
                <div class="w-full h-full flex items-center justify-center text-gray-400">
                  No Cover
                </div>
              {/if}
            </div>
            <div class="p-3">
              <h3 class="font-semibold text-gray-900 dark:text-white text-sm mb-1 truncate">
                {userGame.game.title}
              </h3>
              <p class="text-xs text-gray-500 dark:text-gray-400 mb-2">
                {userGame.game.genre || 'Unknown Genre'}
              </p>
              <div class="flex items-center justify-between">
                <span class="inline-flex items-center px-2 py-1 rounded-full text-xs font-medium {getStatusColor(userGame.play_status)}">
                  {getStatusLabel(userGame.play_status)}
                </span>
                {#if userGame.personal_rating}
                  <div class="flex items-center">
                    <span class="text-yellow-400">★</span>
                    <span class="text-xs text-gray-600 dark:text-gray-400 ml-1">
                      {userGame.personal_rating}
                    </span>
                  </div>
                {/if}
              </div>
            </div>
          </div>
        {/each}
      </div>
    {:else}
      <!-- List View -->
      <div class="bg-white dark:bg-gray-800 rounded-lg shadow overflow-hidden">
        <div class="overflow-x-auto">
          <table class="min-w-full divide-y divide-gray-200 dark:divide-gray-700">
            <thead class="bg-gray-50 dark:bg-gray-700">
              <tr>
                <th class="px-6 py-3 text-left text-xs font-medium text-gray-500 dark:text-gray-300 uppercase tracking-wider">
                  Game
                </th>
                <th class="px-6 py-3 text-left text-xs font-medium text-gray-500 dark:text-gray-300 uppercase tracking-wider">
                  Genre
                </th>
                <th class="px-6 py-3 text-left text-xs font-medium text-gray-500 dark:text-gray-300 uppercase tracking-wider">
                  Status
                </th>
                <th class="px-6 py-3 text-left text-xs font-medium text-gray-500 dark:text-gray-300 uppercase tracking-wider">
                  Rating
                </th>
                <th class="px-6 py-3 text-left text-xs font-medium text-gray-500 dark:text-gray-300 uppercase tracking-wider">
                  Hours
                </th>
              </tr>
            </thead>
            <tbody class="bg-white dark:bg-gray-800 divide-y divide-gray-200 dark:divide-gray-700">
              {#each userGames.value.userGames as userGame (userGame.id)}
                <tr
                  class="hover:bg-gray-50 dark:hover:bg-gray-700 cursor-pointer"
                  on:click={() => handleGameClick(userGame.id)}
                  on:keydown={(e) => e.key === 'Enter' && handleGameClick(userGame.id)}
                  tabindex="0"
                >
                  <td class="px-6 py-4 whitespace-nowrap">
                    <div class="flex items-center">
                      <div class="flex-shrink-0 h-10 w-10">
                        {#if userGame.game.cover_art_url}
                          <img
                            src={userGame.game.cover_art_url}
                            alt={userGame.game.title}
                            class="h-10 w-10 rounded object-cover"
                          />
                        {:else}
                          <div class="h-10 w-10 rounded bg-gray-300 dark:bg-gray-600"></div>
                        {/if}
                      </div>
                      <div class="ml-4">
                        <div class="text-sm font-medium text-gray-900 dark:text-white">
                          {userGame.game.title}
                        </div>
                        <div class="text-sm text-gray-500 dark:text-gray-400">
                          {userGame.game.developer || 'Unknown Developer'}
                        </div>
                      </div>
                    </div>
                  </td>
                  <td class="px-6 py-4 whitespace-nowrap text-sm text-gray-900 dark:text-white">
                    {userGame.game.genre || 'Unknown'}
                  </td>
                  <td class="px-6 py-4 whitespace-nowrap">
                    <span class="inline-flex items-center px-2 py-1 rounded-full text-xs font-medium {getStatusColor(userGame.play_status)}">
                      {getStatusLabel(userGame.play_status)}
                    </span>
                  </td>
                  <td class="px-6 py-4 whitespace-nowrap text-sm text-gray-900 dark:text-white">
                    {#if userGame.personal_rating}
                      <div class="flex items-center">
                        <span class="text-yellow-400">★</span>
                        <span class="ml-1">{userGame.personal_rating}</span>
                      </div>
                    {:else}
                      <span class="text-gray-400">-</span>
                    {/if}
                  </td>
                  <td class="px-6 py-4 whitespace-nowrap text-sm text-gray-900 dark:text-white">
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