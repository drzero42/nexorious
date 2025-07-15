<script lang="ts">
  import { auth, games } from '$lib/stores';
  import { onMount } from 'svelte';
  import { goto } from '$app/navigation';
  import { RouteGuard } from '$lib/components';

  let searchQuery = '';
  let isSearching = false;
  let searchResults: any[] = [];
  let selectedGame: any = null;
  let step: 'search' | 'confirm' | 'details' = 'search';

  // Form data for new game
  let gameData = {
    title: '',
    description: '',
    genre: '',
    developer: '',
    publisher: '',
    release_date: '',
    cover_art_url: '',
    personal_rating: null,
    play_status: 'not_started',
    hours_played: 0,
    personal_notes: '',
    ownership_status: 'owned',
    is_physical: false,
    is_loved: false
  };

  async function handleSearch() {
    if (!searchQuery.trim()) return;

    isSearching = true;
    try {
      // Call the IGDB search API through our backend
      const response = await games.searchIGDB(searchQuery, 10);
      
      // Convert IGDB candidates to search results format
      searchResults = response.candidates.map(candidate => ({
        id: candidate.igdb_id,
        title: candidate.title,
        description: candidate.description,
        genre: '', // Genre will be populated from full metadata
        developer: '', // Developer will be populated from full metadata
        publisher: '', // Publisher will be populated from full metadata
        release_date: candidate.release_date,
        cover_art_url: candidate.cover_art_url,
        igdb_id: candidate.igdb_id,
        platforms: candidate.platforms,
        howlongtobeat_main: candidate.howlongtobeat_main,
        howlongtobeat_extra: candidate.howlongtobeat_extra,
        howlongtobeat_completionist: candidate.howlongtobeat_completionist
      }));
      
      if (searchResults.length > 0) {
        step = 'confirm';
      }
    } catch (error) {
      console.error('Search failed:', error);
      games.clearError(); // Clear any existing error
    } finally {
      isSearching = false;
    }
  }

  async function selectGame(game) {
    selectedGame = game;
    isSearching = true;
    
    try {
      // Import the game directly from IGDB with full metadata
      const importedGame = await games.createFromIGDB(game.igdb_id);
      
      // Redirect to the games page after successful import
      goto('/games');
    } catch (error) {
      console.error('Failed to import game:', error);
      
      // If import fails, fall back to manual entry with pre-filled data
      gameData = {
        ...gameData,
        title: game.title,
        description: game.description || '',
        genre: game.genre || '',
        developer: game.developer || '',
        publisher: game.publisher || '',
        release_date: game.release_date || '',
        cover_art_url: game.cover_art_url || ''
      };
      step = 'details';
    } finally {
      isSearching = false;
    }
  }

  function goBack() {
    if (step === 'details') {
      step = 'confirm';
    } else if (step === 'confirm') {
      step = 'search';
      searchResults = [];
    }
  }

  async function handleSubmit() {
    try {
      // Add game to user's collection
      await games.createGame(gameData);
      goto('/games');
    } catch (error) {
      console.error('Failed to add game:', error);
    }
  }

  function handleKeydown(event: KeyboardEvent) {
    if (event.key === 'Enter' && step === 'search') {
      handleSearch();
    }
  }
</script>

<svelte:head>
  <title>Add Game - Nexorious</title>
</svelte:head>

<RouteGuard requireAuth={true}>
<div class="max-w-2xl mx-auto">
  <!-- Header -->
  <div class="mb-6">
    <div class="flex items-center justify-between">
      <div>
        <h1 class="text-2xl font-bold text-gray-900 dark:text-white">Add Game</h1>
        <p class="text-gray-600 dark:text-gray-400">Add a new game to your collection</p>
      </div>
      <button
        on:click={() => goto('/games')}
        class="text-gray-500 hover:text-gray-700 dark:text-gray-400 dark:hover:text-gray-200"
      >
        Cancel
      </button>
    </div>
  </div>

  <!-- Step 1: Search -->
  {#if step === 'search'}
    <div class="bg-white dark:bg-gray-800 rounded-lg shadow p-6">
      <div class="space-y-4">
        <div>
          <label for="search" class="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-2">
            Search for a game
          </label>
          <div class="flex space-x-2">
            <input
              id="search"
              type="text"
              bind:value={searchQuery}
              on:keydown={handleKeydown}
              placeholder="Enter game title..."
              class="flex-1 px-3 py-2 border border-gray-300 dark:border-gray-600 rounded-md shadow-sm focus:outline-none focus:ring-blue-500 focus:border-blue-500 dark:bg-gray-700 dark:text-white"
            />
            <button
              on:click={handleSearch}
              disabled={isSearching || !searchQuery.trim()}
              class="px-4 py-2 bg-blue-600 hover:bg-blue-700 text-white rounded-md disabled:opacity-50 disabled:cursor-not-allowed"
            >
              {isSearching ? 'Searching...' : 'Search'}
            </button>
          </div>
        </div>

        <div class="text-sm text-gray-600 dark:text-gray-400">
          <p>Search for games using the IGDB database. Selecting a game will automatically add it to your collection with full metadata.</p>
          <p class="mt-1">If you can't find your game, you can manually add it by clicking "Add Manually" below.</p>
        </div>

        {#if games.value.error}
          <div class="mt-4 p-4 bg-red-100 dark:bg-red-900 text-red-700 dark:text-red-200 rounded-md">
            <p class="font-medium">Search Error</p>
            <p class="text-sm">{games.value.error}</p>
          </div>
        {/if}

        <div class="pt-4">
          <button
            on:click={() => step = 'details'}
            class="text-blue-600 hover:text-blue-700 dark:text-blue-400 dark:hover:text-blue-300 text-sm"
          >
            Add Manually Instead
          </button>
        </div>
      </div>
    </div>
  {/if}

  <!-- Step 2: Confirm Game Selection -->
  {#if step === 'confirm'}
    <div class="bg-white dark:bg-gray-800 rounded-lg shadow p-6">
      <div class="mb-4">
        <h2 class="text-lg font-semibold text-gray-900 dark:text-white mb-2">
          Select a game
        </h2>
        <p class="text-gray-600 dark:text-gray-400">
          Choose the correct game from the search results
        </p>
      </div>

      <div class="space-y-4">
        {#if searchResults.length === 0}
          <div class="text-center py-8">
            <div class="text-gray-400 dark:text-gray-500 text-lg mb-2">
              No games found
            </div>
            <p class="text-sm text-gray-600 dark:text-gray-400">
              Try a different search term or add the game manually
            </p>
          </div>
        {:else}
          {#each searchResults as game}
            <button
              class="w-full text-left border border-gray-200 dark:border-gray-700 rounded-lg p-4 hover:bg-gray-50 dark:hover:bg-gray-700 transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
              on:click={() => selectGame(game)}
              disabled={isSearching}
            >
              <div class="flex">
                <div class="flex-shrink-0 w-20 h-28 bg-gray-200 dark:bg-gray-600 rounded">
                  {#if game.cover_art_url}
                    <img
                      src={game.cover_art_url}
                      alt={game.title}
                      class="w-full h-full object-cover rounded"
                      loading="lazy"
                    />
                  {:else}
                    <div class="w-full h-full flex items-center justify-center text-gray-400 text-xs">
                      No Cover
                    </div>
                  {/if}
                </div>
                <div class="ml-4 flex-1">
                  <h3 class="font-semibold text-gray-900 dark:text-white">
                    {game.title}
                  </h3>
                  
                  {#if game.platforms && game.platforms.length > 0}
                    <div class="flex flex-wrap gap-1 mt-1">
                      {#each game.platforms as platform}
                        <span class="inline-block bg-blue-100 dark:bg-blue-900 text-blue-800 dark:text-blue-200 text-xs px-2 py-1 rounded">
                          {platform}
                        </span>
                      {/each}
                    </div>
                  {/if}
                  
                  <p class="text-sm text-gray-500 dark:text-gray-500 mt-1">
                    Released: {game.release_date ? new Date(game.release_date).getFullYear() : 'Unknown'}
                  </p>
                  
                  {#if game.howlongtobeat_main || game.howlongtobeat_extra || game.howlongtobeat_completionist}
                    <div class="text-sm text-gray-600 dark:text-gray-400 mt-1">
                      <span class="font-medium">Time to beat:</span>
                      {#if game.howlongtobeat_main}
                        Main: {game.howlongtobeat_main}h
                      {/if}
                      {#if game.howlongtobeat_extra}
                        • Extra: {game.howlongtobeat_extra}h
                      {/if}
                      {#if game.howlongtobeat_completionist}
                        • Complete: {game.howlongtobeat_completionist}h
                      {/if}
                    </div>
                  {/if}
                  
                  {#if game.description}
                    <p class="text-sm text-gray-600 dark:text-gray-400 mt-2 overflow-hidden" style="display: -webkit-box; -webkit-line-clamp: 3; -webkit-box-orient: vertical;">
                      {game.description}
                    </p>
                  {/if}
                  
                  {#if isSearching}
                    <div class="mt-2 text-sm text-blue-600 dark:text-blue-400">
                      Adding to collection...
                    </div>
                  {/if}
                </div>
              </div>
            </button>
          {/each}
        {/if}
      </div>

      <div class="mt-6 flex justify-between">
        <button
          on:click={goBack}
          class="px-4 py-2 text-gray-700 dark:text-gray-300 hover:text-gray-900 dark:hover:text-white"
        >
          Back to Search
        </button>
        <button
          on:click={() => step = 'details'}
          class="px-4 py-2 bg-gray-600 hover:bg-gray-700 text-white rounded-md"
        >
          Add Manually Instead
        </button>
      </div>
    </div>
  {/if}

  <!-- Step 3: Game Details -->
  {#if step === 'details'}
    <div class="bg-white dark:bg-gray-800 rounded-lg shadow p-6">
      <div class="mb-4">
        <h2 class="text-lg font-semibold text-gray-900 dark:text-white mb-2">
          Game Details
        </h2>
        <p class="text-gray-600 dark:text-gray-400">
          {selectedGame ? 'Review and customize the game information' : 'Enter the game information manually'}
        </p>
      </div>

      <form on:submit|preventDefault={handleSubmit} class="space-y-6">
        <!-- Basic Information -->
        <div class="grid grid-cols-1 md:grid-cols-2 gap-4">
          <div>
            <label for="title" class="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">
              Title *
            </label>
            <input
              id="title"
              type="text"
              bind:value={gameData.title}
              required
              class="w-full px-3 py-2 border border-gray-300 dark:border-gray-600 rounded-md shadow-sm focus:outline-none focus:ring-blue-500 focus:border-blue-500 dark:bg-gray-700 dark:text-white"
            />
          </div>

          <div>
            <label for="genre" class="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">
              Genre
            </label>
            <input
              id="genre"
              type="text"
              bind:value={gameData.genre}
              class="w-full px-3 py-2 border border-gray-300 dark:border-gray-600 rounded-md shadow-sm focus:outline-none focus:ring-blue-500 focus:border-blue-500 dark:bg-gray-700 dark:text-white"
            />
          </div>

          <div>
            <label for="developer" class="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">
              Developer
            </label>
            <input
              id="developer"
              type="text"
              bind:value={gameData.developer}
              class="w-full px-3 py-2 border border-gray-300 dark:border-gray-600 rounded-md shadow-sm focus:outline-none focus:ring-blue-500 focus:border-blue-500 dark:bg-gray-700 dark:text-white"
            />
          </div>

          <div>
            <label for="publisher" class="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">
              Publisher
            </label>
            <input
              id="publisher"
              type="text"
              bind:value={gameData.publisher}
              class="w-full px-3 py-2 border border-gray-300 dark:border-gray-600 rounded-md shadow-sm focus:outline-none focus:ring-blue-500 focus:border-blue-500 dark:bg-gray-700 dark:text-white"
            />
          </div>

          <div>
            <label for="release_date" class="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">
              Release Date
            </label>
            <input
              id="release_date"
              type="date"
              bind:value={gameData.release_date}
              class="w-full px-3 py-2 border border-gray-300 dark:border-gray-600 rounded-md shadow-sm focus:outline-none focus:ring-blue-500 focus:border-blue-500 dark:bg-gray-700 dark:text-white"
            />
          </div>

          <div>
            <label for="cover_art_url" class="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">
              Cover Art URL
            </label>
            <input
              id="cover_art_url"
              type="url"
              bind:value={gameData.cover_art_url}
              class="w-full px-3 py-2 border border-gray-300 dark:border-gray-600 rounded-md shadow-sm focus:outline-none focus:ring-blue-500 focus:border-blue-500 dark:bg-gray-700 dark:text-white"
            />
          </div>
        </div>

        <!-- Description -->
        <div>
          <label for="description" class="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">
            Description
          </label>
          <textarea
            id="description"
            bind:value={gameData.description}
            rows="3"
            class="w-full px-3 py-2 border border-gray-300 dark:border-gray-600 rounded-md shadow-sm focus:outline-none focus:ring-blue-500 focus:border-blue-500 dark:bg-gray-700 dark:text-white"
          ></textarea>
        </div>

        <!-- Personal Information -->
        <div class="border-t border-gray-200 dark:border-gray-700 pt-6">
          <h3 class="text-lg font-medium text-gray-900 dark:text-white mb-4">
            Personal Information
          </h3>

          <div class="grid grid-cols-1 md:grid-cols-2 gap-4">
            <div>
              <label for="play_status" class="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">
                Play Status
              </label>
              <select
                id="play_status"
                bind:value={gameData.play_status}
                class="w-full px-3 py-2 border border-gray-300 dark:border-gray-600 rounded-md shadow-sm focus:outline-none focus:ring-blue-500 focus:border-blue-500 dark:bg-gray-700 dark:text-white"
              >
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

            <div>
              <label for="ownership_status" class="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">
                Ownership Status
              </label>
              <select
                id="ownership_status"
                bind:value={gameData.ownership_status}
                class="w-full px-3 py-2 border border-gray-300 dark:border-gray-600 rounded-md shadow-sm focus:outline-none focus:ring-blue-500 focus:border-blue-500 dark:bg-gray-700 dark:text-white"
              >
                <option value="owned">Owned</option>
                <option value="borrowed">Borrowed</option>
                <option value="rented">Rented</option>
                <option value="subscription">Subscription</option>
              </select>
            </div>

            <div>
              <label for="personal_rating" class="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">
                Personal Rating
              </label>
              <select
                id="personal_rating"
                bind:value={gameData.personal_rating}
                class="w-full px-3 py-2 border border-gray-300 dark:border-gray-600 rounded-md shadow-sm focus:outline-none focus:ring-blue-500 focus:border-blue-500 dark:bg-gray-700 dark:text-white"
              >
                <option value={null}>No Rating</option>
                <option value={1}>1 Star</option>
                <option value={2}>2 Stars</option>
                <option value={3}>3 Stars</option>
                <option value={4}>4 Stars</option>
                <option value={5}>5 Stars</option>
              </select>
            </div>

            <div>
              <label for="hours_played" class="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">
                Hours Played
              </label>
              <input
                id="hours_played"
                type="number"
                min="0"
                bind:value={gameData.hours_played}
                class="w-full px-3 py-2 border border-gray-300 dark:border-gray-600 rounded-md shadow-sm focus:outline-none focus:ring-blue-500 focus:border-blue-500 dark:bg-gray-700 dark:text-white"
              />
            </div>
          </div>

          <div class="mt-4 flex items-center space-x-4">
            <label class="flex items-center">
              <input
                type="checkbox"
                bind:checked={gameData.is_physical}
                class="rounded border-gray-300 dark:border-gray-600 text-blue-600 focus:ring-blue-500"
              />
              <span class="ml-2 text-sm text-gray-700 dark:text-gray-300">Physical copy</span>
            </label>

            <label class="flex items-center">
              <input
                type="checkbox"
                bind:checked={gameData.is_loved}
                class="rounded border-gray-300 dark:border-gray-600 text-blue-600 focus:ring-blue-500"
              />
              <span class="ml-2 text-sm text-gray-700 dark:text-gray-300">Loved game</span>
            </label>
          </div>
        </div>

        <!-- Personal Notes -->
        <div>
          <label for="personal_notes" class="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">
            Personal Notes
          </label>
          <textarea
            id="personal_notes"
            bind:value={gameData.personal_notes}
            rows="3"
            placeholder="Add your personal notes about this game..."
            class="w-full px-3 py-2 border border-gray-300 dark:border-gray-600 rounded-md shadow-sm focus:outline-none focus:ring-blue-500 focus:border-blue-500 dark:bg-gray-700 dark:text-white"
          ></textarea>
        </div>

        <!-- Actions -->
        <div class="flex justify-between pt-6">
          <button
            type="button"
            on:click={goBack}
            class="px-4 py-2 text-gray-700 dark:text-gray-300 hover:text-gray-900 dark:hover:text-white"
          >
            {selectedGame ? 'Back to Selection' : 'Back to Search'}
          </button>
          <button
            type="submit"
            class="px-6 py-2 bg-blue-600 hover:bg-blue-700 text-white rounded-md transition-colors"
          >
            Add Game
          </button>
        </div>
      </form>
    </div>
  {/if}
</div>
</RouteGuard>