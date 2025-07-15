<script lang="ts">
  import { auth, userGames, search } from '$lib/stores';
  import { onMount } from 'svelte';
  import { goto } from '$app/navigation';

  let viewMode: 'grid' | 'list' = 'grid';
  let searchQuery = '';
  let selectedPlatform = '';
  let selectedStatus = '';
  let sortBy = 'title';
  let sortOrder: 'asc' | 'desc' = 'asc';

  onMount(() => {
    // Redirect if not authenticated
    if (!auth.value.user) {
      goto('/login');
      return;
    }

    // Load user games
    userGames.fetchUserGames();
  });

  // Filter and sort games based on current filters
  $: filteredGames = userGames.value.games
    .filter(game => {
      const matchesSearch = !searchQuery || 
        game.title.toLowerCase().includes(searchQuery.toLowerCase()) ||
        game.genre?.toLowerCase().includes(searchQuery.toLowerCase()) ||
        game.developer?.toLowerCase().includes(searchQuery.toLowerCase());
      
      const matchesStatus = !selectedStatus || game.play_status === selectedStatus;
      
      // Platform filtering would need to check user_game_platforms
      // For now, we'll skip platform filtering until we have that data structure
      
      return matchesSearch && matchesStatus;
    })
    .sort((a, b) => {
      let aValue = a[sortBy] || '';
      let bValue = b[sortBy] || '';
      
      if (sortBy === 'personal_rating') {
        aValue = a.personal_rating || 0;
        bValue = b.personal_rating || 0;
      }
      
      if (sortOrder === 'asc') {
        return aValue > bValue ? 1 : -1;
      } else {
        return aValue < bValue ? 1 : -1;
      }
    });

  function handleAddGame() {
    goto('/games/add');
  }

  function handleGameClick(gameId: string) {
    goto(`/games/${gameId}`);
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

<div class="space-y-6">
  <!-- Header -->
  <div class="flex flex-col sm:flex-row sm:items-center sm:justify-between">
    <div>
      <h1 class="text-2xl font-bold text-gray-900 dark:text-white">My Games</h1>
      <p class="text-gray-600 dark:text-gray-400">
        {userGames.value.games.length} games in your collection
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
  {:else if filteredGames.length === 0}
    <div class="text-center py-8">
      <div class="text-gray-500 dark:text-gray-400">
        {userGames.value.games.length === 0 ? 'No games in your collection yet.' : 'No games match your filters.'}
      </div>
      {#if userGames.value.games.length === 0}
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
        {#each filteredGames as game (game.id)}
          <div
            class="bg-white dark:bg-gray-800 rounded-lg shadow hover:shadow-md transition-shadow cursor-pointer"
            on:click={() => handleGameClick(game.id)}
            on:keydown={(e) => e.key === 'Enter' && handleGameClick(game.id)}
            tabindex="0"
          >
            <div class="aspect-[3/4] bg-gray-200 dark:bg-gray-700 rounded-t-lg">
              {#if game.cover_art_url}
                <img
                  src={game.cover_art_url}
                  alt={game.title}
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
                {game.title}
              </h3>
              <p class="text-xs text-gray-500 dark:text-gray-400 mb-2">
                {game.genre || 'Unknown Genre'}
              </p>
              <div class="flex items-center justify-between">
                <span class="inline-flex items-center px-2 py-1 rounded-full text-xs font-medium {getStatusColor(game.play_status)}">
                  {getStatusLabel(game.play_status)}
                </span>
                {#if game.personal_rating}
                  <div class="flex items-center">
                    <span class="text-yellow-400">★</span>
                    <span class="text-xs text-gray-600 dark:text-gray-400 ml-1">
                      {game.personal_rating}
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
              {#each filteredGames as game (game.id)}
                <tr
                  class="hover:bg-gray-50 dark:hover:bg-gray-700 cursor-pointer"
                  on:click={() => handleGameClick(game.id)}
                  on:keydown={(e) => e.key === 'Enter' && handleGameClick(game.id)}
                  tabindex="0"
                >
                  <td class="px-6 py-4 whitespace-nowrap">
                    <div class="flex items-center">
                      <div class="flex-shrink-0 h-10 w-10">
                        {#if game.cover_art_url}
                          <img
                            src={game.cover_art_url}
                            alt={game.title}
                            class="h-10 w-10 rounded object-cover"
                          />
                        {:else}
                          <div class="h-10 w-10 rounded bg-gray-300 dark:bg-gray-600"></div>
                        {/if}
                      </div>
                      <div class="ml-4">
                        <div class="text-sm font-medium text-gray-900 dark:text-white">
                          {game.title}
                        </div>
                        <div class="text-sm text-gray-500 dark:text-gray-400">
                          {game.developer || 'Unknown Developer'}
                        </div>
                      </div>
                    </div>
                  </td>
                  <td class="px-6 py-4 whitespace-nowrap text-sm text-gray-900 dark:text-white">
                    {game.genre || 'Unknown'}
                  </td>
                  <td class="px-6 py-4 whitespace-nowrap">
                    <span class="inline-flex items-center px-2 py-1 rounded-full text-xs font-medium {getStatusColor(game.play_status)}">
                      {getStatusLabel(game.play_status)}
                    </span>
                  </td>
                  <td class="px-6 py-4 whitespace-nowrap text-sm text-gray-900 dark:text-white">
                    {#if game.personal_rating}
                      <div class="flex items-center">
                        <span class="text-yellow-400">★</span>
                        <span class="ml-1">{game.personal_rating}</span>
                      </div>
                    {:else}
                      <span class="text-gray-400">-</span>
                    {/if}
                  </td>
                  <td class="px-6 py-4 whitespace-nowrap text-sm text-gray-900 dark:text-white">
                    {game.hours_played || 0}h
                  </td>
                </tr>
              {/each}
            </tbody>
          </table>
        </div>
      </div>
    {/if}
  {/if}
</div>