<script lang="ts">
  import { games } from '$lib/stores';
  import { userGames, OwnershipStatus, PlayStatus } from '$lib/stores/user-games.svelte';
  import { goto } from '$app/navigation';
  import { RouteGuard } from '$lib/components';
  import type { IGDBGameCandidate } from '$lib/stores/games.svelte';

  let searchQuery = '';
  let isSearching = false;
  let searchResults: IGDBGameCandidate[] = [];
  let selectedGame: IGDBGameCandidate | null = null;
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
    game_metadata: '',
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
      searchResults = response.games;
      
      // Always go to confirm step to show results (or "no games found" message)
      step = 'confirm';
    } catch (error) {
      console.error('Search failed:', error);
      games.clearError(); // Clear any existing error
    } finally {
      isSearching = false;
    }
  }

  async function selectGame(game: IGDBGameCandidate) {
    selectedGame = game;
    isSearching = true;
    
    try {
      // Import the game directly from IGDB with full metadata
      const createdGame = await games.createFromIGDB(game.igdb_id);
      
      try {
        // Add the game to the user's collection with default values
        await userGames.addGameToCollection({
          game_id: createdGame.id,
          ownership_status: OwnershipStatus.OWNED,
          is_physical: false
        });
        
        // Redirect to the games page after successful import and collection addition
        goto('/games');
      } catch (collectionError) {
        console.error('Failed to add game to collection:', collectionError);
        // Game was created but couldn't be added to collection - show error but still redirect
        // The user can manually add it to their collection later
        goto('/games');
      }
    } catch (error) {
      console.error('Failed to import game from IGDB:', error);
      
      // If import fails, fall back to manual entry with pre-filled data
      gameData = {
        ...gameData,
        title: game.title,
        description: game.description || '',
        genre: '', // Genre will be populated from full metadata
        developer: '', // Developer will be populated from full metadata
        publisher: '', // Publisher will be populated from full metadata
        release_date: game.release_date || '',
        cover_art_url: game.cover_art_url || '',
        game_metadata: JSON.stringify({})
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
      // Create the game first
      const createdGame = await games.createGame(gameData);
      
      try {
        // Then add it to the user's collection with personal information
        const userGame = await userGames.addGameToCollection({
          game_id: createdGame.id,
          ownership_status: gameData.ownership_status as OwnershipStatus || OwnershipStatus.OWNED,
          is_physical: gameData.is_physical || false
        });
        
        // Update progress with personal information if any were provided
        if (gameData.play_status !== 'not_started' || gameData.hours_played > 0 || gameData.personal_notes) {
          try {
            await userGames.updateProgress(userGame.id, {
              play_status: gameData.play_status as PlayStatus || PlayStatus.NOT_STARTED,
              hours_played: gameData.hours_played || 0,
              personal_notes: gameData.personal_notes || ''
            });
          } catch (progressError) {
            console.error('Failed to update progress, but game was added to collection:', progressError);
          }
        }
        
        // Update user game details (rating and loved status) if any were provided
        if (gameData.personal_rating || gameData.is_loved) {
          try {
            const updateData: any = {
              is_loved: gameData.is_loved || false
            };
            
            // Only include personal_rating if it has a value to avoid TypeScript strict mode issues
            if (gameData.personal_rating) {
              updateData.personal_rating = gameData.personal_rating;
            }
            
            await userGames.updateUserGame(userGame.id, updateData);
          } catch (updateError) {
            console.error('Failed to update game details, but game was added to collection:', updateError);
          }
        }
        
        goto('/games');
      } catch (collectionError) {
        console.error('Failed to add game to collection:', collectionError);
        // Game was created but couldn't be added to collection - show error but still redirect
        // The user can manually add it to their collection later
        goto('/games');
      }
    } catch (error) {
      console.error('Failed to create game:', error);
      // Show error to user - they can try again or modify the data
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
<div class="space-y-6">
  <!-- Header -->
  <div class="sm:flex sm:items-center sm:justify-between">
    <div>
      <h1 class="text-2xl font-bold leading-7 text-gray-900 sm:truncate sm:text-3xl sm:tracking-tight">Add Game</h1>
      <p class="mt-1 text-sm text-gray-500">Add a new game to your collection</p>
    </div>
    <div class="mt-4 sm:ml-16 sm:mt-0 sm:flex-none">
      <button
        on:click={() => goto('/games')}
        class="btn-secondary inline-flex items-center gap-x-2"
      >
        <svg class="-ml-0.5 h-5 w-5" viewBox="0 0 20 20" fill="currentColor">
          <path fill-rule="evenodd" d="M7.72 12.53a.75.75 0 010-1.06L10.94 8.25H3.75a.75.75 0 010-1.5h7.19L7.72 3.53a.75.75 0 011.06-1.06l4.25 4.25a.75.75 0 010 1.06l-4.25 4.25a.75.75 0 01-1.06 0z" clip-rule="evenodd" />
        </svg>
        Back to Games
      </button>
    </div>
  </div>

  <!-- Step 1: Search -->
  {#if step === 'search'}
    <div class="card p-6">
      <div class="space-y-6">
        <div>
          <label for="search" class="form-label">
            Search for a game
          </label>
          <div class="flex space-x-3">
            <div class="flex-1 relative">
              <div class="pointer-events-none absolute inset-y-0 left-0 flex items-center pl-3">
                <svg class="h-5 w-5 text-gray-400" viewBox="0 0 20 20" fill="currentColor">
                  <path fill-rule="evenodd" d="M9 3.5a5.5 5.5 0 100 11 5.5 5.5 0 000-11zM2 9a7 7 0 1112.452 4.391l3.328 3.329a.75.75 0 11-1.06 1.06l-3.329-3.328A7 7 0 012 9z" clip-rule="evenodd" />
                </svg>
              </div>
              <input
                id="search"
                type="text"
                bind:value={searchQuery}
                on:keydown={handleKeydown}
                placeholder="Enter game title..."
                class="form-input pl-10"
                disabled={isSearching}
              />
            </div>
            <button
              on:click={handleSearch}
              disabled={isSearching || !searchQuery.trim()}
              class="btn-primary flex items-center gap-x-2"
            >
              {#if isSearching}
                <svg class="animate-spin -ml-1 h-4 w-4 text-white" xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24">
                  <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
                  <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path>
                </svg>
                Searching...
              {:else}
                <svg class="-ml-0.5 h-5 w-5" viewBox="0 0 20 20" fill="currentColor">
                  <path fill-rule="evenodd" d="M9 3.5a5.5 5.5 0 100 11 5.5 5.5 0 000-11zM2 9a7 7 0 1112.452 4.391l3.328 3.329a.75.75 0 11-1.06 1.06l-3.329-3.328A7 7 0 012 9z" clip-rule="evenodd" />
                </svg>
                Search
              {/if}
            </button>
          </div>
        </div>

        <div class="bg-blue-50 border border-blue-200 rounded-md p-4">
          <div class="flex">
            <div class="flex-shrink-0">
              <svg class="h-5 w-5 text-blue-400" viewBox="0 0 20 20" fill="currentColor">
                <path fill-rule="evenodd" d="M18 10a8 8 0 11-16 0 8 8 0 0116 0zm-7-4a1 1 0 11-2 0 1 1 0 012 0zM9 9a1 1 0 000 2v3a1 1 0 001 1h1a1 1 0 100-2v-3a1 1 0 00-1-1H9z" clip-rule="evenodd" />
              </svg>
            </div>
            <div class="ml-3">
              <h3 class="text-sm font-medium text-blue-800">How game search works</h3>
              <div class="mt-2 text-sm text-blue-700 space-y-1">
                <p>Search for games using the IGDB database. Selecting a game will automatically add it to your collection with full metadata.</p>
                <p>If you can't find your game, you can manually add it by clicking "Add Manually" below.</p>
              </div>
            </div>
          </div>
        </div>

        {#if games.value.error}
          <div class="rounded-md bg-red-50 p-4">
            <div class="flex">
              <div class="flex-shrink-0">
                <svg class="h-5 w-5 text-red-400" viewBox="0 0 20 20" fill="currentColor">
                  <path fill-rule="evenodd" d="M10 18a8 8 0 100-16 8 8 0 000 16zM8.28 7.22a.75.75 0 00-1.06 1.06L8.94 10l-1.72 1.72a.75.75 0 101.06 1.06L10 11.06l1.72 1.72a.75.75 0 101.06-1.06L11.06 10l1.72-1.72a.75.75 0 00-1.06-1.06L10 8.94 8.28 7.22z" clip-rule="evenodd" />
                </svg>
              </div>
              <div class="ml-3">
                <h3 class="text-sm font-medium text-red-800">Search Error</h3>
                <p class="mt-2 text-sm text-red-700">{games.value.error}</p>
              </div>
            </div>
          </div>
        {/if}

        <div class="flex justify-center">
          <button
            on:click={() => step = 'details'}
            class="btn-secondary inline-flex items-center gap-x-2"
          >
            <svg class="-ml-0.5 h-5 w-5" viewBox="0 0 20 20" fill="currentColor">
              <path d="M10.75 4.75a.75.75 0 00-1.5 0v4.5h-4.5a.75.75 0 000 1.5h4.5v4.5a.75.75 0 001.5 0v-4.5h4.5a.75.75 0 000-1.5h-4.5v-4.5z" />
            </svg>
            Add Manually Instead
          </button>
        </div>
      </div>
    </div>
  {/if}

  <!-- Step 2: Confirm Game Selection -->
  {#if step === 'confirm'}
    <div class="space-y-6">
      <div class="text-center">
        <h2 class="text-xl font-semibold text-gray-900">
          Select a game
        </h2>
        <p class="mt-2 text-sm text-gray-600">
          Choose the correct game from the search results
        </p>
      </div>

      <div class="space-y-4">
        {#if searchResults.length === 0}
          <div class="card p-8">
            <div class="text-center">
              <svg class="mx-auto h-12 w-12 text-gray-400" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9.172 16.172a4 4 0 015.656 0M9 12h6m-6 4h6m2 5H7a2 2 0 01-2-2V5a2 2 0 012-2h5.586a1 1 0 01.707.293l5.414 5.414a1 1 0 01.293.707V19a2 2 0 01-2 2z" />
              </svg>
              <h3 class="mt-4 text-lg font-medium text-gray-900">
                No games found
              </h3>
              <p class="mt-2 text-sm text-gray-500">
                Try a different search term or add the game manually
              </p>
            </div>
          </div>
        {:else}
          {#each searchResults as game}
            <button
              on:click={() => selectGame(game)}
              disabled={isSearching}
              class="card w-full p-4 text-left hover:shadow-md hover:border-primary-300 focus:outline-none focus:ring-2 focus:ring-primary-500 focus:ring-offset-2 transition-all duration-200 disabled:opacity-50 disabled:cursor-not-allowed"
            >
              <div class="flex space-x-4">
                <div class="flex-shrink-0">
                  {#if game.cover_art_url}
                    <img
                      src={game.cover_art_url}
                      alt={game.title}
                      loading="lazy"
                      class="h-24 w-16 object-cover rounded border border-gray-200"
                    />
                  {:else}
                    <div class="h-24 w-16 bg-gray-100 border border-gray-200 rounded flex items-center justify-center">
                      <div class="text-center text-xs text-gray-400">
                        <svg class="mx-auto h-6 w-6 text-gray-300 mb-1" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 16l4.586-4.586a2 2 0 012.828 0L16 16m-2-2l1.586-1.586a2 2 0 012.828 0L20 14m-6-6h.01M6 20h12a2 2 0 002-2V6a2 2 0 00-2-2H6a2 2 0 00-2 2v12a2 2 0 002 2z" />
                        </svg>
                        No Cover
                      </div>
                    </div>
                  {/if}
                </div>
                <div class="flex-1 min-w-0">
                  <h3 class="text-lg font-medium text-gray-900 mb-2">
                    {game.title}
                  </h3>
                  
                  {#if game.platforms && game.platforms.length > 0}
                    <div class="mb-2 flex flex-wrap gap-1">
                      {#each game.platforms as platform}
                        <span class="inline-flex items-center px-2 py-1 rounded-full text-xs font-medium bg-gray-100 text-gray-800">
                          {platform}
                        </span>
                      {/each}
                    </div>
                  {/if}
                  
                  <p class="text-sm text-gray-600 mb-2">
                    Released: {game.release_date ? new Date(game.release_date).getFullYear() : 'Unknown'}
                  </p>
                  
                  {#if game.howlongtobeat_main || game.howlongtobeat_extra || game.howlongtobeat_completionist}
                    <div class="text-sm text-gray-600 mb-2">
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
                    <p class="text-sm text-gray-500 line-clamp-3 overflow-hidden">
                      {game.description}
                    </p>
                  {/if}
                  
                  {#if isSearching}
                    <div class="mt-3 flex items-center text-sm text-primary-600">
                      <svg class="animate-spin -ml-1 mr-2 h-4 w-4 text-primary-600" xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24">
                        <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
                        <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path>
                      </svg>
                      Adding to collection...
                    </div>
                  {/if}
                </div>
              </div>
            </button>
          {/each}
        {/if}
      </div>

      <div class="flex justify-between">
        <button
          on:click={goBack}
          class="btn-secondary inline-flex items-center gap-x-2"
        >
          <svg class="-ml-0.5 h-5 w-5" viewBox="0 0 20 20" fill="currentColor">
            <path fill-rule="evenodd" d="M12.79 5.23a.75.75 0 01-.02 1.06L8.832 10l3.938 3.71a.75.75 0 11-1.04 1.08l-4.5-4.25a.75.75 0 010-1.08l4.5-4.25a.75.75 0 011.06.02z" clip-rule="evenodd" />
          </svg>
          Back to Search
        </button>
        <button
          on:click={() => step = 'details'}
          class="btn-secondary inline-flex items-center gap-x-2"
        >
          <svg class="-ml-0.5 h-5 w-5" viewBox="0 0 20 20" fill="currentColor">
            <path d="M10.75 4.75a.75.75 0 00-1.5 0v4.5h-4.5a.75.75 0 000 1.5h4.5v4.5a.75.75 0 001.5 0v-4.5h4.5a.75.75 0 000-1.5h-4.5v-4.5z" />
          </svg>
          Add Manually Instead
        </button>
      </div>
    </div>
  {/if}

  <!-- Step 3: Game Details -->
  {#if step === 'details'}
    <div class="space-y-6">
      <div class="text-center">
        <h2 class="text-xl font-semibold text-gray-900">
          Game Details
        </h2>
        <p class="mt-2 text-sm text-gray-600">
          {selectedGame ? 'Review and customize the game information' : 'Enter the game information manually'}
        </p>
      </div>

      <form on:submit|preventDefault={handleSubmit} class="space-y-8">
        <!-- Basic Information -->
        <div class="card p-6">
          <h3 class="text-lg font-medium text-gray-900 mb-4">Basic Information</h3>
          <div class="grid grid-cols-1 gap-6 sm:grid-cols-2">
            <div class="sm:col-span-2">
              <label for="title" class="form-label">
                Title *
              </label>
              <input
                id="title"
                type="text"
                bind:value={gameData.title}
                required
                placeholder="Enter game title"
                class="form-input"
              />
            </div>

            <div>
              <label for="genre" class="form-label">
                Genre
              </label>
              <input
                id="genre"
                type="text"
                bind:value={gameData.genre}
                placeholder="e.g., Action, RPG, Strategy"
                class="form-input"
              />
            </div>

            <div>
              <label for="release_date" class="form-label">
                Release Date
              </label>
              <input
                id="release_date"
                type="date"
                bind:value={gameData.release_date}
                class="form-input"
              />
            </div>

            <div>
              <label for="developer" class="form-label">
                Developer
              </label>
              <input
                id="developer"
                type="text"
                bind:value={gameData.developer}
                placeholder="Game developer"
                class="form-input"
              />
            </div>

            <div>
              <label for="publisher" class="form-label">
                Publisher
              </label>
              <input
                id="publisher"
                type="text"
                bind:value={gameData.publisher}
                placeholder="Game publisher"
                class="form-input"
              />
            </div>

            <div class="sm:col-span-2">
              <label for="cover_art_url" class="form-label">
                Cover Art URL
              </label>
              <input
                id="cover_art_url"
                type="url"
                bind:value={gameData.cover_art_url}
                placeholder="https://example.com/cover.jpg"
                class="form-input"
              />
            </div>
          </div>
        </div>

        <!-- Description -->
        <div class="card p-6">
          <h3 class="text-lg font-medium text-gray-900 mb-4">Description</h3>
          <div>
            <label for="description" class="form-label">
              Game Description
            </label>
            <textarea
              id="description"
              bind:value={gameData.description}
              rows="4"
              placeholder="Describe the game..."
              class="form-input"
            ></textarea>
          </div>
        </div>

        <!-- Personal Information -->
        <div class="card p-6">
          <h3 class="text-lg font-medium text-gray-900 mb-4">Personal Information</h3>

          <div class="grid grid-cols-1 gap-6 sm:grid-cols-2">
            <div>
              <label for="play_status" class="form-label">
                Play Status
              </label>
              <select
                id="play_status"
                bind:value={gameData.play_status}
                class="form-input"
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
              <label for="ownership_status" class="form-label">
                Ownership Status
              </label>
              <select
                id="ownership_status"
                bind:value={gameData.ownership_status}
                class="form-input"
              >
                <option value="owned">Owned</option>
                <option value="borrowed">Borrowed</option>
                <option value="rented">Rented</option>
                <option value="subscription">Subscription</option>
              </select>
            </div>

            <div>
              <label for="personal_rating" class="form-label">
                Personal Rating
              </label>
              <select
                id="personal_rating"
                bind:value={gameData.personal_rating}
                class="form-input"
              >
                <option value={null}>No Rating</option>
                <option value={1}>★ 1 Star</option>
                <option value={2}>★★ 2 Stars</option>
                <option value={3}>★★★ 3 Stars</option>
                <option value={4}>★★★★ 4 Stars</option>
                <option value={5}>★★★★★ 5 Stars</option>
              </select>
            </div>

            <div>
              <label for="hours_played" class="form-label">
                Hours Played
              </label>
              <input
                id="hours_played"
                type="number"
                min="0"
                step="0.1"
                bind:value={gameData.hours_played}
                placeholder="0"
                class="form-input"
              />
            </div>
          </div>

          <div class="mt-6 space-y-4">
            <div class="flex items-center">
              <input
                id="is_physical"
                type="checkbox"
                bind:checked={gameData.is_physical}
                class="h-4 w-4 text-primary-600 focus:ring-primary-500 border-gray-300 rounded"
              />
              <label for="is_physical" class="ml-2 block text-sm text-gray-900">
                Physical copy
              </label>
            </div>

            <div class="flex items-center">
              <input
                id="is_loved"
                type="checkbox"
                bind:checked={gameData.is_loved}
                class="h-4 w-4 text-primary-600 focus:ring-primary-500 border-gray-300 rounded"
              />
              <label for="is_loved" class="ml-2 block text-sm text-gray-900">
                <span class="flex items-center gap-1">
                  <span>Loved game</span>
                  <span class="text-red-500">♥</span>
                </span>
              </label>
            </div>
          </div>
        </div>

        <!-- Personal Notes -->
        <div class="card p-6">
          <h3 class="text-lg font-medium text-gray-900 mb-4">Personal Notes</h3>
          <div>
            <label for="personal_notes" class="form-label">
              Your Notes
            </label>
            <textarea
              id="personal_notes"
              bind:value={gameData.personal_notes}
              rows="4"
              placeholder="Add your personal notes about this game..."
              class="form-input"
            ></textarea>
          </div>
        </div>

        <!-- Actions -->
        <div class="flex justify-between pt-6 border-t border-gray-200">
          <button
            type="button"
            on:click={goBack}
            class="btn-secondary inline-flex items-center gap-x-2"
          >
            <svg class="-ml-0.5 h-5 w-5" viewBox="0 0 20 20" fill="currentColor">
              <path fill-rule="evenodd" d="M12.79 5.23a.75.75 0 01-.02 1.06L8.832 10l3.938 3.71a.75.75 0 11-1.04 1.08l-4.5-4.25a.75.75 0 010-1.08l4.5-4.25a.75.75 0 011.06.02z" clip-rule="evenodd" />
            </svg>
            {selectedGame ? 'Back to Selection' : 'Back to Search'}
          </button>
          <button
            type="submit"
            class="btn-primary inline-flex items-center gap-x-2"
          >
            <svg class="-ml-0.5 h-5 w-5" viewBox="0 0 20 20" fill="currentColor">
              <path d="M10.75 4.75a.75.75 0 00-1.5 0v4.5h-4.5a.75.75 0 000 1.5h4.5v4.5a.75.75 0 001.5 0v-4.5h4.5a.75.75 0 000-1.5h-4.5v-4.5z" />
            </svg>
            Add Game to Collection
          </button>
        </div>
      </form>
    </div>
  {/if}
</div>
</RouteGuard>