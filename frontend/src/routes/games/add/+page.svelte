<script lang="ts">
  import { games } from '$lib/stores';
  import { userGames, OwnershipStatus, PlayStatus } from '$lib/stores/user-games.svelte';
  import { platforms } from '$lib/stores/platforms.svelte';
  import { notifications } from '$lib/stores/notifications.svelte';
  import { goto } from '$app/navigation';
  import { RouteGuard } from '$lib/components';
  import { resolveImageUrl } from '$lib/utils/image-url';
  import type { IGDBGameCandidate } from '$lib/stores/games.svelte';
  import { onMount } from 'svelte';

  let searchQuery = '';
  let isSearching = false;
  let addingGameId: string | null = null;
  let searchResults: IGDBGameCandidate[] = [];
  let selectedGame: IGDBGameCandidate | null = null;
  let step: 'search' | 'confirm' | 'metadata-confirm' | 'details' = 'search';

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
    is_loved: false
  };

  // Platform association data
  let selectedPlatforms = new Set<string>();
  let platformStorefronts = new Map<string, string>(); // platform_id -> storefront_id
  let platformStoreUrls = new Map<string, string>(); // platform_id -> store_url

  // Load platforms and storefronts on component mount
  onMount(async () => {
    // Load platforms and storefronts first
    try {
      await platforms.fetchAll();
    } catch (error) {
      console.error('Failed to load platforms and storefronts:', error);
      notifications.showError('Failed to load platforms and storefronts. Some features may not work properly.');
    }

    // Load user's game collection to check ownership status (separate error handling)
    try {
      await userGames.fetchUserGames({}, 1, 100); // Load up to 100 games (backend limit)
    } catch (error) {
      console.error('Failed to load user game collection:', error);
      notifications.showWarning('Could not load your game collection. Ownership indicators may not be accurate.');
    }
  });

  // Reactive statements for active platforms and storefronts
  $: activePlatforms = $platforms.platforms.filter(platform => platform.is_active);
  $: activeStorefronts = $platforms.storefronts.filter(storefront => storefront.is_active);

  // IGDB platform filtering helpers
  function isPlatformInIGDB(platform: any, igdbPlatforms: string[]): boolean {
    if (!igdbPlatforms || igdbPlatforms.length === 0) return false;
    
    return igdbPlatforms.some(igdbPlatform => 
      igdbPlatform.toLowerCase() === platform.display_name.toLowerCase() ||
      igdbPlatform.toLowerCase() === platform.name.toLowerCase()
    );
  }

  function getIGDBPlatforms(platforms: any[], igdbPlatforms: string[]): any[] {
    if (!igdbPlatforms || igdbPlatforms.length === 0) return [];
    return platforms.filter(platform => isPlatformInIGDB(platform, igdbPlatforms));
  }

  function getOtherPlatforms(platforms: any[], igdbPlatforms: string[]): any[] {
    if (!igdbPlatforms || igdbPlatforms.length === 0) return platforms;
    return platforms.filter(platform => !isPlatformInIGDB(platform, igdbPlatforms));
  }

  // Reactive statements for filtered platforms
  $: igdbPlatformNames = selectedGame?.platforms || [];
  $: igdbPlatforms = getIGDBPlatforms(activePlatforms, igdbPlatformNames);
  $: otherPlatforms = getOtherPlatforms(activePlatforms, igdbPlatformNames);
  
  // Others section expand/collapse state
  let showOtherPlatforms = false;

  // Helper functions for ownership detection
  function isGameOwned(igdbId: string): boolean {
    return userGames.value.userGames.some((userGame: any) => 
      userGame.game.igdb_id === igdbId
    );
  }

  function getOwnedPlatformsForGame(igdbId: string): string[] {
    const userGame = userGames.value.userGames.find((userGame: any) => 
      userGame.game.igdb_id === igdbId
    );
    if (!userGame) return [];
    
    return userGame.platforms.map((platform: any) => platform.platform.display_name);
  }

  function handleGameClick(game: IGDBGameCandidate, isOwned: boolean) {
    if (isOwned) {
      // For owned games, navigate to the game detail view
      const userGame = userGames.value.userGames.find((userGame: any) => 
        userGame.game.igdb_id === game.igdb_id
      );
      if (userGame) {
        goto(`/games/${userGame.id}`);
      }
    } else {
      // For unowned games, proceed with the addition flow
      selectGame(game);
    }
  }

  // Platform selection helpers
  function togglePlatform(platformId: string) {
    if (selectedPlatforms.has(platformId)) {
      selectedPlatforms.delete(platformId);
      platformStorefronts.delete(platformId);
      platformStoreUrls.delete(platformId);
    } else {
      selectedPlatforms.add(platformId);
      
      // Automatically set the default storefront if one exists for this platform
      const platform = activePlatforms.find(p => p.id === platformId);
      if (platform && platform.default_storefront_id) {
        platformStorefronts.set(platformId, platform.default_storefront_id);
      }
    }
    selectedPlatforms = new Set(selectedPlatforms); // Trigger reactivity
    platformStorefronts = new Map(platformStorefronts);
    platformStoreUrls = new Map(platformStoreUrls);
  }

  function setStorefrontForPlatform(platformId: string, storefrontId: string) {
    if (storefrontId) {
      platformStorefronts.set(platformId, storefrontId);
    } else {
      platformStorefronts.delete(platformId);
    }
    platformStorefronts = new Map(platformStorefronts); // Trigger reactivity
  }

  function setStoreUrlForPlatform(platformId: string, storeUrl: string) {
    if (storeUrl.trim()) {
      platformStoreUrls.set(platformId, storeUrl.trim());
    } else {
      platformStoreUrls.delete(platformId);
    }
    platformStoreUrls = new Map(platformStoreUrls); // Trigger reactivity
  }

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
      notifications.showApiError(error, 'Failed to search for games. Please try again.');
    } finally {
      isSearching = false;
    }
  }

  function selectGame(game: IGDBGameCandidate) {
    selectedGame = game;
    // Pre-populate gameData with the selected game's information
    gameData = {
      ...gameData,
      title: game.title,
      description: game.description || '',
      release_date: game.release_date || '',
      cover_art_url: game.cover_art_url || ''
    };
    step = 'metadata-confirm';
  }

  async function confirmGameAddition() {
    if (!selectedGame) return;
    
    addingGameId = selectedGame.igdb_id;
    
    try {
      // Import the game from IGDB with any custom overrides from the form
      const customOverrides: Record<string, any> = {};
      
      // Only include overrides if they differ from the original IGDB data
      if (gameData.title !== selectedGame.title) {
        customOverrides.title = gameData.title;
      }
      if (gameData.description !== (selectedGame.description || '')) {
        customOverrides.description = gameData.description;
      }
      if (gameData.cover_art_url !== (selectedGame.cover_art_url || '')) {
        customOverrides.cover_art_url = gameData.cover_art_url;
      }
      
      const createdGame = await games.createFromIGDB(selectedGame.igdb_id, customOverrides);
      notifications.showSuccess('Game metadata imported successfully from IGDB');
      
      try {
        // Add the game to the user's collection with form values
        const platformData = Array.from(selectedPlatforms).map(platformId => ({
          platform_id: platformId,
          storefront_id: platformStorefronts.get(platformId) || null,
          store_game_id: null,
          store_url: platformStoreUrls.get(platformId) || null,
          is_available: true
        }));
        
        const addRequest: any = {
          game_id: createdGame.id,
          ownership_status: gameData.ownership_status as OwnershipStatus || OwnershipStatus.OWNED,
          platforms: platformData.length > 0 ? platformData : undefined
        };
        
        const userGame = await userGames.addGameToCollection(addRequest);

        // Platform details are now included in the initial request, so no separate API calls needed
        let partialErrors = [];
        
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
            partialErrors.push('Failed to save progress information');
          }
        }
        
        // Update user game details (rating and loved status) if any were provided
        if (gameData.personal_rating || gameData.is_loved) {
          try {
            const updateData: any = {
              is_loved: gameData.is_loved || false
            };
            
            if (gameData.personal_rating) {
              updateData.personal_rating = gameData.personal_rating;
            }
            
            await userGames.updateUserGame(userGame.id, updateData);
          } catch (updateError) {
            console.error('Failed to update game details, but game was added to collection:', updateError);
            partialErrors.push('Failed to save rating and favorite status');
          }
        }
        
        // Show success message with any partial error warnings
        if (partialErrors.length > 0) {
          notifications.showWarning(`"${createdGame.title}" added to collection, but some details couldn't be saved: ${partialErrors.join(', ')}`);
        } else {
          notifications.showSuccess(`"${createdGame.title}" successfully added to your collection!`);
        }
        
        // Brief delay to show success message before redirect
        setTimeout(() => {
          goto('/games');
        }, 1000);
      } catch (collectionError) {
        console.error('Failed to add game to collection:', collectionError);
        notifications.showError(`Game was imported but couldn't be added to your collection. You can try adding it manually from your games list.`);
        // Brief delay before redirect
        setTimeout(() => {
          goto('/games');
        }, 2000);
      }
    } catch (error) {
      console.error('Failed to import game from IGDB:', error);
      notifications.showError('Failed to import game from IGDB. You can add it manually with custom details.');
      // If import fails, fall back to manual entry with current form data
      step = 'details';
    } finally {
      addingGameId = null;
    }
  }

  function goBack() {
    if (step === 'details') {
      step = selectedGame ? 'metadata-confirm' : 'confirm';
    } else if (step === 'metadata-confirm') {
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
      notifications.showSuccess('Game created successfully');
      
      try {
        // Then add it to the user's collection with personal information
        const platformData = Array.from(selectedPlatforms).map(platformId => ({
          platform_id: platformId,
          storefront_id: platformStorefronts.get(platformId) || null,
          store_game_id: null,
          store_url: platformStoreUrls.get(platformId) || null,
          is_available: true
        }));
        
        const addRequest: any = {
          game_id: createdGame.id,
          ownership_status: gameData.ownership_status as OwnershipStatus || OwnershipStatus.OWNED,
          platforms: platformData.length > 0 ? platformData : undefined
        };
        
        const userGame = await userGames.addGameToCollection(addRequest);

        // Platform details are now included in the initial request, so no separate API calls needed
        let partialErrors = [];
        
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
            partialErrors.push('Failed to save progress information');
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
            partialErrors.push('Failed to save rating and favorite status');
          }
        }
        
        // Show success message with any partial error warnings
        if (partialErrors.length > 0) {
          notifications.showWarning(`"${createdGame.title}" added to collection, but some details couldn't be saved: ${partialErrors.join(', ')}`);
        } else {
          notifications.showSuccess(`"${createdGame.title}" successfully added to your collection!`);
        }
        
        // Brief delay to show success message before redirect
        setTimeout(() => {
          goto('/games');
        }, 1000);
      } catch (collectionError) {
        console.error('Failed to add game to collection:', collectionError);
        notifications.showError(`Game was created but couldn't be added to your collection. You can try adding it manually from your games list.`);
        // Brief delay before redirect
        setTimeout(() => {
          goto('/games');
        }, 2000);
      }
    } catch (error) {
      console.error('Failed to create game:', error);
      notifications.showApiError(error, 'Failed to create game. Please check your information and try again.');
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
          <path fill-rule="evenodd" d="M12.79 5.23a.75.75 0 01-.02 1.06L8.832 10l3.938 3.71a.75.75 0 11-1.04 1.08l-4.5-4.25a.75.75 0 010-1.08l4.5-4.25a.75.75 0 011.06.02z" clip-rule="evenodd" />
        </svg>
        Back to Games
      </button>
    </div>
  </div>

  <!-- Step Indicator -->
  <div class="flex items-center justify-center">
    <div class="flex items-center space-x-4">
      <div class="flex items-center">
        <div class="{step === 'search' ? 'bg-primary-500 text-white' : step === 'confirm' || step === 'metadata-confirm' || step === 'details' ? 'bg-primary-100 text-primary-700' : 'bg-gray-200 text-gray-500'} rounded-full w-8 h-8 flex items-center justify-center text-sm font-medium">
          1
        </div>
        <span class="ml-2 text-sm font-medium {step === 'search' ? 'text-gray-900' : 'text-gray-500'}">Search</span>
      </div>
      <svg class="w-5 h-5 text-gray-300" fill="currentColor" viewBox="0 0 20 20">
        <path fill-rule="evenodd" d="M7.293 14.707a1 1 0 010-1.414L10.586 10 7.293 6.707a1 1 0 011.414-1.414l4 4a1 1 0 010 1.414l-4 4a1 1 0 01-1.414 0z" clip-rule="evenodd" />
      </svg>
      <div class="flex items-center">
        <div class="{step === 'confirm' || step === 'metadata-confirm' ? 'bg-primary-500 text-white' : step === 'details' ? 'bg-primary-100 text-primary-700' : 'bg-gray-200 text-gray-500'} rounded-full w-8 h-8 flex items-center justify-center text-sm font-medium">
          2
        </div>
        <span class="ml-2 text-sm font-medium {step === 'confirm' || step === 'metadata-confirm' ? 'text-gray-900' : 'text-gray-500'}">Select</span>
      </div>
      <svg class="w-5 h-5 text-gray-300" fill="currentColor" viewBox="0 0 20 20">
        <path fill-rule="evenodd" d="M7.293 14.707a1 1 0 010-1.414L10.586 10 7.293 6.707a1 1 0 011.414-1.414l4 4a1 1 0 010 1.414l-4 4a1 1 0 01-1.414 0z" clip-rule="evenodd" />
      </svg>
      <div class="flex items-center">
        <div class="{step === 'details' || step === 'metadata-confirm' ? 'bg-primary-500 text-white' : 'bg-gray-200 text-gray-500'} rounded-full w-8 h-8 flex items-center justify-center text-sm font-medium">
          3
        </div>
        <span class="ml-2 text-sm font-medium {step === 'details' || step === 'metadata-confirm' ? 'text-gray-900' : 'text-gray-500'}">Details</span>
      </div>
    </div>
  </div>

  <!-- Step 1: Search -->
  {#if step === 'search'}
    <div class="card p-6">
      <div class="space-y-6">
        <div>
          <h2 class="text-lg font-semibold text-gray-900 mb-4">Search for a Game</h2>
          <div class="flex gap-3">
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
                class="form-input pl-10 focus:ring-2 focus:ring-primary-500"
                disabled={isSearching}
              />
            </div>
            <button
              on:click={handleSearch}
              disabled={isSearching || !searchQuery.trim()}
              class="btn-primary inline-flex items-center gap-x-2 disabled:opacity-50 disabled:cursor-not-allowed transition-all duration-200"
            >
              {#if isSearching}
                <svg class="animate-spin h-4 w-4 text-white" xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24">
                  <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
                  <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path>
                </svg>
                Searching...
              {:else}
                <svg class="h-5 w-5" viewBox="0 0 20 20" fill="currentColor">
                  <path fill-rule="evenodd" d="M9 3.5a5.5 5.5 0 100 11 5.5 5.5 0 000-11zM2 9a7 7 0 1112.452 4.391l3.328 3.329a.75.75 0 11-1.06 1.06l-3.329-3.328A7 7 0 012 9z" clip-rule="evenodd" />
                </svg>
                Search
              {/if}
            </button>
          </div>
        </div>

        <div class="bg-blue-50 border border-blue-200 rounded-lg p-4">
          <div class="flex">
            <div class="flex-shrink-0">
              <svg class="h-5 w-5 text-blue-600" viewBox="0 0 20 20" fill="currentColor">
                <path fill-rule="evenodd" d="M18 10a8 8 0 11-16 0 8 8 0 0116 0zm-7-4a1 1 0 11-2 0 1 1 0 012 0zM9 9a1 1 0 000 2v3a1 1 0 001 1h1a1 1 0 100-2v-3a1 1 0 00-1-1H9z" clip-rule="evenodd" />
              </svg>
            </div>
            <div class="ml-3 flex-1">
              <h3 class="text-sm font-medium text-blue-900">How game search works</h3>
              <div class="mt-2 text-sm text-blue-800 space-y-1">
                <p>• Search for games using the IGDB database with automatic metadata</p>
                <p>• Can't find your game? Click "Add Manually" to enter details yourself</p>
              </div>
            </div>
          </div>
        </div>

        {#if games.value.error}
          <div class="rounded-lg bg-red-50 border border-red-200 p-4">
            <div class="flex">
              <div class="flex-shrink-0">
                <svg class="h-5 w-5 text-red-600" viewBox="0 0 20 20" fill="currentColor">
                  <path fill-rule="evenodd" d="M10 18a8 8 0 100-16 8 8 0 000 16zM8.28 7.22a.75.75 0 00-1.06 1.06L8.94 10l-1.72 1.72a.75.75 0 101.06 1.06L10 11.06l1.72 1.72a.75.75 0 101.06-1.06L11.06 10l1.72-1.72a.75.75 0 00-1.06-1.06L10 8.94 8.28 7.22z" clip-rule="evenodd" />
                </svg>
              </div>
              <div class="ml-3 flex-1">
                <h3 class="text-sm font-medium text-red-900">Search Error</h3>
                <p class="mt-1 text-sm text-red-800">{games.value.error}</p>
              </div>
            </div>
          </div>
        {/if}

        <div class="pt-4 border-t border-gray-200 flex justify-center">
          <button
            on:click={() => step = 'details'}
            class="btn-secondary inline-flex items-center gap-x-2 hover:bg-gray-300 transition-colors duration-200"
          >
            <svg class="h-5 w-5" viewBox="0 0 20 20" fill="currentColor">
              <path fill-rule="evenodd" d="M13.5 4.938a7 7 0 11-9.006 1.737c.202-.257.59-.218.793.039.278.352.594.672.943.954.332.269.786-.049.773-.476a5.977 5.977 0 01.572-2.759 6.026 6.026 0 012.486-2.665c.247-.14.55-.016.677.238A6.967 6.967 0 0013.5 4.938zM14 12a4 4 0 01-4 4c-1.913 0-3.52-1.398-3.91-3.182-.093-.429.44-.643.814-.413a4.043 4.043 0 001.601.564c.303.038.531-.24.51-.544a5.975 5.975 0 011.315-4.192.447.447 0 01.431-.16A4.001 4.001 0 0114 12z" clip-rule="evenodd" />
            </svg>
            Add Game Manually
          </button>
        </div>
      </div>
    </div>
  {/if}

  <!-- Step 2: Confirm Game Selection -->
  {#if step === 'confirm'}
    <div class="space-y-6">
      <div class="card p-6">
        <div class="text-center mb-6">
          <h2 class="text-xl font-semibold text-gray-900">Select Your Game</h2>
          <p class="mt-2 text-sm text-gray-600">
            Choose the correct game from the search results below
          </p>
        </div>

        <div class="space-y-3">
          {#if searchResults.length === 0}
            <div class="py-12">
              <div class="text-center">
                <svg class="mx-auto h-12 w-12 text-gray-300" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                  <path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5" d="M9.172 16.172a4 4 0 015.656 0M9 12h6m-6 4h6m2 5H7a2 2 0 01-2-2V5a2 2 0 012-2h5.586a1 1 0 01.707.293l5.414 5.414a1 1 0 01.293.707V19a2 2 0 01-2 2z" />
                </svg>
                <h3 class="mt-4 text-lg font-medium text-gray-900">No games found</h3>
                <p class="mt-2 text-sm text-gray-500">
                  Try a different search term or add the game manually
                </p>
              </div>
            </div>
          {:else}
            {#each searchResults as game}
              {@const owned = isGameOwned(game.igdb_id)}
              {@const ownedPlatforms = getOwnedPlatformsForGame(game.igdb_id)}
              <button
                on:click={() => handleGameClick(game, owned)}
                disabled={addingGameId !== null}
                class="w-full p-4 bg-white border-2 rounded-lg text-left hover:shadow-md focus:outline-none focus:ring-2 focus:ring-primary-500 focus:ring-offset-2 transition-all duration-200 disabled:opacity-50 disabled:cursor-not-allowed group {owned ? 'border-green-300 bg-green-50' : 'border-gray-200 hover:border-primary-300'}"
              >
                <div class="flex gap-4">
                  <div class="flex-shrink-0">
                    {#if game.cover_art_url}
                      <img
                        src={resolveImageUrl(game.cover_art_url)}
                        alt={game.title}
                        loading="lazy"
                        class="h-24 w-16 object-cover rounded shadow-sm group-hover:shadow-md transition-shadow duration-200"
                      />
                    {:else}
                      <div class="h-24 w-16 bg-gray-100 rounded shadow-sm flex items-center justify-center group-hover:shadow-md transition-shadow duration-200">
                        <svg class="h-8 w-8 text-gray-300" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5" d="M4 16l4.586-4.586a2 2 0 012.828 0L16 16m-2-2l1.586-1.586a2 2 0 012.828 0L20 14m-6-6h.01M6 20h12a2 2 0 002-2V6a2 2 0 00-2-2H6a2 2 0 00-2 2v12a2 2 0 002 2z" />
                        </svg>
                      </div>
                    {/if}
                  </div>
                  <div class="flex-1 min-w-0">
                    <div class="flex items-start justify-between mb-2">
                      <div class="flex-1 min-w-0">
                        <h3 class="text-lg font-medium text-gray-900 group-hover:text-primary-600 transition-colors duration-200">
                          {game.title}
                        </h3>
                        {#if owned}
                          <div class="mt-1 flex items-center gap-2">
                            <span class="inline-flex items-center px-2 py-0.5 rounded-full text-xs font-medium bg-green-100 text-green-800 border border-green-200">
                              <svg class="h-3 w-3 mr-1" fill="currentColor" viewBox="0 0 20 20">
                                <path fill-rule="evenodd" d="M10 18a8 8 0 100-16 8 8 0 000 16zm3.707-9.293a1 1 0 00-1.414-1.414L9 10.586 7.707 9.293a1 1 0 00-1.414 1.414l2 2a1 1 0 001.414 0l4-4z" clip-rule="evenodd" />
                              </svg>
                              Already Owned
                            </span>
                            {#if ownedPlatforms.length > 0}
                              <span class="text-xs text-gray-600">on {ownedPlatforms.join(', ')}</span>
                            {/if}
                          </div>
                          <div class="mt-2 text-xs text-blue-600 flex items-center">
                            <svg class="h-3 w-3 mr-1" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M15 12a3 3 0 11-6 0 3 3 0 016 0z" />
                              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M2.458 12C3.732 7.943 7.523 5 12 5c4.478 0 8.268 2.943 9.542 7-1.274 4.057-5.064 7-9.542 7-4.477 0-8.268-2.943-9.542-7z" />
                            </svg>
                            Click to view details
                          </div>
                        {:else}
                          <div class="mt-2 text-xs text-primary-600 flex items-center">
                            <svg class="h-3 w-3 mr-1" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 6v6m0 0v6m0-6h6m-6 0H6" />
                            </svg>
                            Click to add to collection
                          </div>
                        {/if}
                      </div>
                      <svg class="h-5 w-5 text-gray-400 group-hover:text-primary-500 transition-colors duration-200 flex-shrink-0 ml-2" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 5l7 7-7 7" />
                      </svg>
                    </div>
                    
                    {#if game.platforms && game.platforms.length > 0}
                      <div class="mb-2 flex flex-wrap gap-1">
                        {#each game.platforms as platform}
                          <span class="inline-flex items-center px-2 py-0.5 rounded-full text-xs font-medium bg-primary-50 text-primary-700">
                            {platform}
                          </span>
                        {/each}
                      </div>
                    {/if}
                    
                    <div class="flex flex-wrap gap-x-4 text-sm text-gray-600 mb-2">
                      <span>Released: {game.release_date ? new Date(game.release_date).getFullYear() : 'Unknown'}</span>
                      {#if game.howlongtobeat_main}
                        <span>• {game.howlongtobeat_main}h to beat</span>
                      {/if}
                    </div>
                    
                    {#if game.description}
                      <p class="text-sm text-gray-500 line-clamp-2">
                        {game.description}
                      </p>
                    {/if}
                    
                    {#if addingGameId === game.igdb_id}
                      <div class="mt-3 flex items-center text-sm text-primary-600">
                        <svg class="animate-spin h-4 w-4 mr-2" xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24">
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
      </div>

      <div class="flex justify-between">
        <button
          on:click={goBack}
          class="btn-secondary inline-flex items-center gap-x-2"
        >
          <svg class="h-5 w-5" viewBox="0 0 20 20" fill="currentColor">
            <path fill-rule="evenodd" d="M12.79 5.23a.75.75 0 01-.02 1.06L8.832 10l3.938 3.71a.75.75 0 11-1.04 1.08l-4.5-4.25a.75.75 0 010-1.08l4.5-4.25a.75.75 0 011.06.02z" clip-rule="evenodd" />
          </svg>
          Back to Search
        </button>
        <button
          on:click={() => step = 'details'}
          class="btn-secondary inline-flex items-center gap-x-2"
        >
          <svg class="h-5 w-5" viewBox="0 0 20 20" fill="currentColor">
            <path fill-rule="evenodd" d="M13.5 4.938a7 7 0 11-9.006 1.737c.202-.257.59-.218.793.039.278.352.594.672.943.954.332.269.786-.049.773-.476a5.977 5.977 0 01.572-2.759 6.026 6.026 0 012.486-2.665c.247-.14.55-.016.677.238A6.967 6.967 0 0013.5 4.938zM14 12a4 4 0 01-4 4c-1.913 0-3.52-1.398-3.91-3.182-.093-.429.44-.643.814-.413a4.043 4.043 0 001.601.564c.303.038.531-.24.51-.544a5.975 5.975 0 011.315-4.192.447.447 0 01.431-.16A4.001 4.001 0 0114 12z" clip-rule="evenodd" />
          </svg>
          Add Manually
        </button>
      </div>
    </div>
  {/if}

  <!-- Step 2.5: Metadata Confirmation -->
  {#if step === 'metadata-confirm'}
    <div class="space-y-6">
      <!-- Game Overview Card -->
      <div class="card overflow-hidden">
        <div class="bg-gradient-to-r from-primary-50 to-primary-100 px-6 py-4 border-b border-primary-200">
          <h2 class="text-xl font-semibold text-gray-900">Confirm Game Details</h2>
          <p class="mt-1 text-sm text-gray-600">Review and customize the information before adding to your collection</p>
        </div>
        
        <div class="p-6">
          <div class="flex flex-col sm:flex-row gap-6">
            <!-- Cover Art -->
            <div class="flex-shrink-0 mx-auto sm:mx-0">
              {#if gameData.cover_art_url}
                <img
                  src={resolveImageUrl(gameData.cover_art_url)}
                  alt={gameData.title}
                  loading="lazy"
                  class="h-48 w-32 object-cover rounded-lg shadow-md"
                />
              {:else}
                <div class="h-48 w-32 bg-gray-100 rounded-lg shadow-md flex items-center justify-center">
                  <div class="text-center">
                    <svg class="mx-auto h-12 w-12 text-gray-300" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                      <path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5" d="M4 16l4.586-4.586a2 2 0 012.828 0L16 16m-2-2l1.586-1.586a2 2 0 012.828 0L20 14m-6-6h.01M6 20h12a2 2 0 002-2V6a2 2 0 00-2-2H6a2 2 0 00-2 2v12a2 2 0 002 2z" />
                    </svg>
                    <p class="mt-2 text-xs text-gray-400">No Cover</p>
                  </div>
                </div>
              {/if}
            </div>

            <!-- Game Information -->
            <div class="flex-1 space-y-4">
              <div>
                <h3 class="text-2xl font-bold text-gray-900">
                  {gameData.title}
                </h3>
                
                {#if selectedGame?.platforms && selectedGame.platforms.length > 0}
                  <div class="mt-3 flex flex-wrap gap-1">
                    {#each selectedGame.platforms as platform}
                      <span class="inline-flex items-center px-2.5 py-1 rounded-full text-xs font-medium bg-primary-50 text-primary-700 border border-primary-200">
                        {platform}
                      </span>
                    {/each}
                  </div>
                {/if}
              </div>
              
              <div class="grid grid-cols-2 gap-4 text-sm">
                <div>
                  <span class="text-gray-500">Release Year:</span>
                  <span class="ml-2 font-medium text-gray-900">
                    {gameData.release_date ? new Date(gameData.release_date).getFullYear() : 'Unknown'}
                  </span>
                </div>
                
                {#if selectedGame?.howlongtobeat_main}
                  <div>
                    <span class="text-gray-500">Time to Beat:</span>
                    <span class="ml-2 font-medium text-gray-900">{selectedGame.howlongtobeat_main}h</span>
                  </div>
                {/if}
              </div>
              
              {#if selectedGame?.howlongtobeat_extra || selectedGame?.howlongtobeat_completionist}
                <div class="flex gap-4 text-sm">
                  {#if selectedGame.howlongtobeat_extra}
                    <div>
                      <span class="text-gray-500">Extra:</span>
                      <span class="ml-1 font-medium text-gray-900">{selectedGame.howlongtobeat_extra}h</span>
                    </div>
                  {/if}
                  {#if selectedGame.howlongtobeat_completionist}
                    <div>
                      <span class="text-gray-500">Completionist:</span>
                      <span class="ml-1 font-medium text-gray-900">{selectedGame.howlongtobeat_completionist}h</span>
                    </div>
                  {/if}
                </div>
              {/if}
            
              <!-- Editable Description -->
              <div>
                <label for="metadata-description" class="form-label">
                  Description
                </label>
                <textarea
                  id="metadata-description"
                  bind:value={gameData.description}
                  rows="3"
                  placeholder="Game description..."
                  class="form-input resize-none"
                ></textarea>
              </div>
            </div>
          </div>
        </div>
      </div>

      <!-- Personal Information Card -->
      <div class="card p-6">
        <h3 class="text-lg font-medium text-gray-900 mb-4 flex items-center">
          <svg class="h-5 w-5 text-gray-400 mr-2" fill="none" viewBox="0 0 24 24" stroke="currentColor">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M16 7a4 4 0 11-8 0 4 4 0 018 0zM12 14a7 7 0 00-7 7h14a7 7 0 00-7-7z" />
          </svg>
          Personal Information
        </h3>
          
        <div class="grid grid-cols-1 gap-6 sm:grid-cols-2">
          <div>
            <label for="metadata-play-status" class="form-label">
              Play Status
            </label>
            <select
              id="metadata-play-status"
              bind:value={gameData.play_status}
              class="form-input"
            >
              <option value="not_started">🆕 Not Started</option>
              <option value="in_progress">🎮 In Progress</option>
              <option value="completed">✅ Completed</option>
              <option value="mastered">🏆 Mastered</option>
              <option value="dominated">👑 Dominated</option>
              <option value="shelved">📚 Shelved</option>
              <option value="dropped">❌ Dropped</option>
              <option value="replay">🔄 Replay</option>
            </select>
          </div>

          <div>
            <label for="metadata-ownership-status" class="form-label">
              Ownership Status
            </label>
            <select
              id="metadata-ownership-status"
              bind:value={gameData.ownership_status}
              class="form-input"
            >
              <option value="owned">💿 Owned</option>
              <option value="borrowed">🤝 Borrowed</option>
              <option value="rented">📅 Rented</option>
              <option value="subscription">📱 Subscription</option>
              <option value="no_longer_owned">📦 No Longer Owned</option>
            </select>
            <p class="mt-1 text-xs text-gray-500">
              {#if gameData.ownership_status === 'owned'}
                You own this game permanently
              {:else if gameData.ownership_status === 'borrowed'}
                Temporarily borrowed from someone
              {:else if gameData.ownership_status === 'rented'}
                Rented from a store or service
              {:else if gameData.ownership_status === 'subscription'}
                Available through a subscription service
              {:else if gameData.ownership_status === 'no_longer_owned'}
                Previously owned but no longer have access
              {/if}
            </p>
          </div>

          <div>
            <label for="metadata-personal-rating" class="form-label">
              Personal Rating
            </label>
            <select
              id="metadata-personal-rating"
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
            <label for="metadata-hours-played" class="form-label">
              Hours Played
            </label>
            <div class="relative">
              <input
                id="metadata-hours-played"
                type="number"
                min="0"
                step="0.1"
                bind:value={gameData.hours_played}
                placeholder="0"
                class="form-input pr-10"
              />
              <div class="absolute inset-y-0 right-0 pr-3 flex items-center pointer-events-none">
                <span class="text-gray-500 sm:text-sm">hrs</span>
              </div>
            </div>
          </div>
        </div>

        <div class="mt-6">
          <label class="flex items-center p-3 bg-gray-50 rounded-lg border border-gray-200 cursor-pointer hover:bg-gray-100 transition-colors duration-200">
            <input
              id="metadata-is-loved"
              type="checkbox"
              bind:checked={gameData.is_loved}
              class="h-4 w-4 text-primary-600 focus:ring-primary-500 border-gray-300 rounded"
            />
            <span class="ml-3 text-sm font-medium text-gray-900 flex items-center gap-1">
              <span>Loved Game</span>
              <span class="text-red-500">♥</span>
            </span>
          </label>
        </div>

        <!-- Personal Notes -->
        <div class="mt-6">
          <label for="metadata-personal-notes" class="form-label">
            Personal Notes
          </label>
          <textarea
            id="metadata-personal-notes"
            bind:value={gameData.personal_notes}
            rows="3"
            placeholder="Add your thoughts, memories, or notes about this game..."
            class="form-input resize-none"
          ></textarea>
        </div>
      </div>

      <!-- Platform & Storefront Card -->
      <div class="card p-6">
        <h3 class="text-lg font-medium text-gray-900 mb-2 flex items-center">
          <svg class="h-5 w-5 text-gray-400 mr-2" fill="none" viewBox="0 0 24 24" stroke="currentColor">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9.75 17L9 20l-1 1h8l-1-1-.75-3M3 13h18M5 17h14a2 2 0 002-2V5a2 2 0 00-2-2H5a2 2 0 00-2 2v10a2 2 0 002 2z" />
          </svg>
          Platforms & Storefronts
        </h3>
        <p class="text-sm text-gray-600 mb-4">Select where you own this game and optionally add store details.</p>
        
        {#if $platforms.isLoading}
          <div class="text-center py-8">
            <svg class="animate-spin h-8 w-8 text-gray-400 mx-auto" xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24">
              <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
              <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path>
            </svg>
            <p class="mt-2 text-sm text-gray-500">Loading platforms...</p>
          </div>
        {:else if $platforms.error}
          <div class="rounded-lg bg-red-50 border border-red-200 p-4">
            <div class="flex">
              <div class="flex-shrink-0">
                <svg class="h-5 w-5 text-red-600" viewBox="0 0 20 20" fill="currentColor">
                  <path fill-rule="evenodd" d="M10 18a8 8 0 100-16 8 8 0 000 16zM8.28 7.22a.75.75 0 00-1.06 1.06L8.94 10l-1.72 1.72a.75.75 0 101.06 1.06L10 11.06l1.72 1.72a.75.75 0 101.06-1.06L11.06 10l1.72-1.72a.75.75 0 00-1.06-1.06L10 8.94 8.28 7.22z" clip-rule="evenodd" />
                </svg>
              </div>
              <div class="ml-3">
                <h3 class="text-sm font-medium text-red-900">Platform Loading Error</h3>
                <p class="mt-1 text-sm text-red-800">{$platforms.error}</p>
              </div>
            </div>
          </div>
        {:else}
          <div class="space-y-3">
            <!-- IGDB Platforms Section -->
            {#if igdbPlatforms.length > 0}
              <div class="mb-4">
                <h4 class="text-sm font-medium text-gray-700 mb-3 flex items-center">
                  <svg class="h-4 w-4 text-primary-500 mr-2" fill="currentColor" viewBox="0 0 20 20">
                    <path fill-rule="evenodd" d="M10 18a8 8 0 100-16 8 8 0 000 16zm3.707-9.293a1 1 0 00-1.414-1.414L9 10.586 7.707 9.293a1 1 0 00-1.414 1.414l2 2a1 1 0 001.414 0l4-4z" clip-rule="evenodd" />
                  </svg>
                  Available on these platforms
                </h4>
                <div class="space-y-3">
                  {#each igdbPlatforms as platform (platform.id)}
                    <div class="border border-gray-200 rounded-lg overflow-hidden transition-all duration-200 {selectedPlatforms.has(platform.id) ? 'border-primary-300 shadow-sm' : ''}">
                      <!-- Platform Header -->
                      <label class="flex items-center p-4 cursor-pointer hover:bg-gray-50 transition-colors duration-200">
                        <input
                          id="platform-{platform.id}"
                          type="checkbox"
                          checked={selectedPlatforms.has(platform.id)}
                          on:change={() => togglePlatform(platform.id)}
                          class="h-4 w-4 text-primary-600 focus:ring-primary-500 border-gray-300 rounded"
                        />
                        <div class="ml-3 flex items-center gap-2 flex-1">
                          {#if platform.icon_url}
                            <img src={platform.icon_url} alt={platform.display_name} class="w-6 h-6 object-contain" />
                          {/if}
                          <span class="text-sm font-medium text-gray-900">{platform.display_name}</span>
                        </div>
                        {#if selectedPlatforms.has(platform.id)}
                          <svg class="h-5 w-5 text-primary-500" fill="currentColor" viewBox="0 0 20 20">
                            <path fill-rule="evenodd" d="M10 18a8 8 0 100-16 8 8 0 000 16zm3.707-9.293a1 1 0 00-1.414-1.414L9 10.586 7.707 9.293a1 1 0 00-1.414 1.414l2 2a1 1 0 001.414 0l4-4z" clip-rule="evenodd" />
                          </svg>
                        {/if}
                      </label>

                      <!-- Platform Details (shown when selected) -->
                      {#if selectedPlatforms.has(platform.id)}
                        <div class="px-4 pb-4 bg-gray-50 border-t border-gray-200">
                          <div class="grid grid-cols-1 sm:grid-cols-2 gap-3 pt-3">
                            <!-- Storefront Selection -->
                            <div>
                              <label for="storefront-{platform.id}" class="block text-xs font-medium text-gray-700 mb-1">
                                Storefront (optional)
                              </label>
                              <select
                                id="storefront-{platform.id}"
                                value={platformStorefronts.get(platform.id) || ''}
                                on:change={(e) => setStorefrontForPlatform(platform.id, e.currentTarget.value)}
                                class="form-input text-sm py-1.5"
                              >
                                <option value="">No specific storefront</option>
                                {#each activeStorefronts as storefront (storefront.id)}
                                  <option value={storefront.id}>{storefront.display_name}</option>
                                {/each}
                              </select>
                            </div>

                            <!-- Store URL -->
                            <div>
                              <label for="store-url-{platform.id}" class="block text-xs font-medium text-gray-700 mb-1">
                                Store URL (optional)
                              </label>
                              <input
                                id="store-url-{platform.id}"
                                type="url"
                                value={platformStoreUrls.get(platform.id) || ''}
                                on:input={(e) => setStoreUrlForPlatform(platform.id, e.currentTarget.value)}
                                placeholder="https://store.example.com/game"
                                class="form-input text-sm py-1.5"
                              />
                            </div>
                          </div>
                        </div>
                      {/if}
                    </div>
                  {/each}
                </div>
              </div>
            {/if}

            <!-- Others Section -->
            {#if otherPlatforms.length > 0}
              <div>
                <button
                  type="button"
                  on:click={() => showOtherPlatforms = !showOtherPlatforms}
                  class="w-full flex items-center justify-between p-3 bg-gray-50 border border-gray-200 rounded-lg hover:bg-gray-100 transition-colors duration-200"
                >
                  <span class="text-sm font-medium text-gray-700 flex items-center">
                    <svg class="h-4 w-4 text-gray-400 mr-2" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                      <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 11H5m14-7l-7 7-7-7m14 14l-7-7-7 7" />
                    </svg>
                    Other platforms ({otherPlatforms.length})
                  </span>
                  <svg class="h-4 w-4 text-gray-400 transition-transform duration-200 {showOtherPlatforms ? 'rotate-180' : ''}" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                    <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 9l-7 7-7-7" />
                  </svg>
                </button>
                
                {#if showOtherPlatforms}
                  <div class="mt-3 space-y-3">
                    {#each otherPlatforms as platform (platform.id)}
                      <div class="border border-gray-200 rounded-lg overflow-hidden transition-all duration-200 {selectedPlatforms.has(platform.id) ? 'border-primary-300 shadow-sm' : ''}">
                        <!-- Platform Header -->
                        <label class="flex items-center p-4 cursor-pointer hover:bg-gray-50 transition-colors duration-200">
                          <input
                            id="platform-other-{platform.id}"
                            type="checkbox"
                            checked={selectedPlatforms.has(platform.id)}
                            on:change={() => togglePlatform(platform.id)}
                            class="h-4 w-4 text-primary-600 focus:ring-primary-500 border-gray-300 rounded"
                          />
                          <div class="ml-3 flex items-center gap-2 flex-1">
                            {#if platform.icon_url}
                              <img src={platform.icon_url} alt={platform.display_name} class="w-6 h-6 object-contain" />
                            {/if}
                            <span class="text-sm font-medium text-gray-900">{platform.display_name}</span>
                          </div>
                          {#if selectedPlatforms.has(platform.id)}
                            <svg class="h-5 w-5 text-primary-500" fill="currentColor" viewBox="0 0 20 20">
                              <path fill-rule="evenodd" d="M10 18a8 8 0 100-16 8 8 0 000 16zm3.707-9.293a1 1 0 00-1.414-1.414L9 10.586 7.707 9.293a1 1 0 00-1.414 1.414l2 2a1 1 0 001.414 0l4-4z" clip-rule="evenodd" />
                            </svg>
                          {/if}
                        </label>

                        <!-- Platform Details (shown when selected) -->
                        {#if selectedPlatforms.has(platform.id)}
                          <div class="px-4 pb-4 bg-gray-50 border-t border-gray-200">
                            <div class="grid grid-cols-1 sm:grid-cols-2 gap-3 pt-3">
                              <!-- Storefront Selection -->
                              <div>
                                <label for="storefront-other-{platform.id}" class="block text-xs font-medium text-gray-700 mb-1">
                                  Storefront (optional)
                                </label>
                                <select
                                  id="storefront-other-{platform.id}"
                                  value={platformStorefronts.get(platform.id) || ''}
                                  on:change={(e) => setStorefrontForPlatform(platform.id, e.currentTarget.value)}
                                  class="form-input text-sm py-1.5"
                                >
                                  <option value="">No specific storefront</option>
                                  {#each activeStorefronts as storefront (storefront.id)}
                                    <option value={storefront.id}>{storefront.display_name}</option>
                                  {/each}
                                </select>
                              </div>

                              <!-- Store URL -->
                              <div>
                                <label for="store-url-other-{platform.id}" class="block text-xs font-medium text-gray-700 mb-1">
                                  Store URL (optional)
                                </label>
                                <input
                                  id="store-url-other-{platform.id}"
                                  type="url"
                                  value={platformStoreUrls.get(platform.id) || ''}
                                  on:input={(e) => setStoreUrlForPlatform(platform.id, e.currentTarget.value)}
                                  placeholder="https://store.example.com/game"
                                  class="form-input text-sm py-1.5"
                                />
                              </div>
                            </div>
                          </div>
                        {/if}
                      </div>
                    {/each}
                  </div>
                {/if}
              </div>
            {/if}

            <!-- Fallback: Show all platforms if no IGDB data -->
            {#if igdbPlatforms.length === 0 && otherPlatforms.length === 0}
              {#each activePlatforms as platform (platform.id)}
                <div class="border border-gray-200 rounded-lg overflow-hidden transition-all duration-200 {selectedPlatforms.has(platform.id) ? 'border-primary-300 shadow-sm' : ''}">
                  <!-- Platform Header -->
                  <label class="flex items-center p-4 cursor-pointer hover:bg-gray-50 transition-colors duration-200">
                    <input
                      id="platform-{platform.id}"
                      type="checkbox"
                      checked={selectedPlatforms.has(platform.id)}
                      on:change={() => togglePlatform(platform.id)}
                      class="h-4 w-4 text-primary-600 focus:ring-primary-500 border-gray-300 rounded"
                    />
                    <div class="ml-3 flex items-center gap-2 flex-1">
                      {#if platform.icon_url}
                        <img src={platform.icon_url} alt={platform.display_name} class="w-6 h-6 object-contain" />
                      {/if}
                      <span class="text-sm font-medium text-gray-900">{platform.display_name}</span>
                    </div>
                    {#if selectedPlatforms.has(platform.id)}
                      <svg class="h-5 w-5 text-primary-500" fill="currentColor" viewBox="0 0 20 20">
                        <path fill-rule="evenodd" d="M10 18a8 8 0 100-16 8 8 0 000 16zm3.707-9.293a1 1 0 00-1.414-1.414L9 10.586 7.707 9.293a1 1 0 00-1.414 1.414l2 2a1 1 0 001.414 0l4-4z" clip-rule="evenodd" />
                      </svg>
                    {/if}
                  </label>

                  <!-- Platform Details (shown when selected) -->
                  {#if selectedPlatforms.has(platform.id)}
                    <div class="px-4 pb-4 bg-gray-50 border-t border-gray-200">
                      <div class="grid grid-cols-1 sm:grid-cols-2 gap-3 pt-3">
                        <!-- Storefront Selection -->
                        <div>
                          <label for="storefront-{platform.id}" class="block text-xs font-medium text-gray-700 mb-1">
                            Storefront (optional)
                          </label>
                          <select
                            id="storefront-{platform.id}"
                            value={platformStorefronts.get(platform.id) || ''}
                            on:change={(e) => setStorefrontForPlatform(platform.id, e.currentTarget.value)}
                            class="form-input text-sm py-1.5"
                          >
                            <option value="">No specific storefront</option>
                            {#each activeStorefronts as storefront (storefront.id)}
                              <option value={storefront.id}>{storefront.display_name}</option>
                            {/each}
                          </select>
                        </div>

                        <!-- Store URL -->
                        <div>
                          <label for="store-url-{platform.id}" class="block text-xs font-medium text-gray-700 mb-1">
                            Store URL (optional)
                          </label>
                          <input
                            id="store-url-{platform.id}"
                            type="url"
                            value={platformStoreUrls.get(platform.id) || ''}
                            on:input={(e) => setStoreUrlForPlatform(platform.id, e.currentTarget.value)}
                            placeholder="https://store.example.com/game"
                            class="form-input text-sm py-1.5"
                          />
                        </div>
                      </div>
                    </div>
                  {/if}
                </div>
              {/each}
            {/if}
            
            {#if activePlatforms.length === 0}
              <div class="text-center py-8">
                <svg class="mx-auto h-12 w-12 text-gray-300" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                  <path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5" d="M9.75 17L9 20l-1 1h8l-1-1-.75-3M3 13h18M5 17h14a2 2 0 002-2V5a2 2 0 00-2-2H5a2 2 0 00-2 2v10a2 2 0 002 2z" />
                </svg>
                <p class="mt-2 text-sm text-gray-500">No platforms available</p>
                <p class="text-xs text-gray-400">Contact an administrator to add platforms.</p>
              </div>
            {/if}
          </div>
        {/if}
      </div>

      <!-- Actions -->
      <div class="card p-4 bg-gray-50">
        <div class="flex flex-col sm:flex-row justify-between gap-3">
          <button
            on:click={goBack}
            class="btn-secondary inline-flex items-center justify-center gap-x-2 order-2 sm:order-1"
          >
            <svg class="h-5 w-5" viewBox="0 0 20 20" fill="currentColor">
              <path fill-rule="evenodd" d="M12.79 5.23a.75.75 0 01-.02 1.06L8.832 10l3.938 3.71a.75.75 0 11-1.04 1.08l-4.5-4.25a.75.75 0 010-1.08l4.5-4.25a.75.75 0 011.06.02z" clip-rule="evenodd" />
            </svg>
            Back to Selection
          </button>
          
          <div class="flex flex-col sm:flex-row gap-3 order-1 sm:order-2">
            <button
              on:click={() => step = 'details'}
              class="btn-secondary inline-flex items-center justify-center gap-x-2"
            >
              <svg class="h-5 w-5" viewBox="0 0 20 20" fill="currentColor">
                <path fill-rule="evenodd" d="M13.5 4.938a7 7 0 11-9.006 1.737c.202-.257.59-.218.793.039.278.352.594.672.943.954.332.269.786-.049.773-.476a5.977 5.977 0 01.572-2.759 6.026 6.026 0 012.486-2.665c.247-.14.55-.016.677.238A6.967 6.967 0 0013.5 4.938zM14 12a4 4 0 01-4 4c-1.913 0-3.52-1.398-3.91-3.182-.093-.429.44-.643.814-.413a4.043 4.043 0 001.601.564c.303.038.531-.24.51-.544a5.975 5.975 0 011.315-4.192.447.447 0 01.431-.16A4.001 4.001 0 0114 12z" clip-rule="evenodd" />
              </svg>
              Edit Details
            </button>
            
            <button
              on:click={confirmGameAddition}
              disabled={addingGameId !== null}
              class="btn-primary inline-flex items-center justify-center gap-x-2 disabled:opacity-50 disabled:cursor-not-allowed transition-all duration-200 font-medium"
            >
              {#if addingGameId}
                <svg class="animate-spin h-4 w-4 text-white" xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24">
                  <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
                  <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path>
                </svg>
                Adding to Collection...
              {:else}
                <svg class="h-5 w-5" viewBox="0 0 20 20" fill="currentColor">
                  <path fill-rule="evenodd" d="M10 18a8 8 0 100-16 8 8 0 000 16zm3.857-9.809a.75.75 0 00-1.214-.882l-3.236 4.53L8.107 10.5a.75.75 0 00-1.214 1.029l2.5 3.5a.75.75 0 001.214 0l4-5.5z" clip-rule="evenodd" />
                </svg>
                Add to Collection
              {/if}
            </button>
          </div>
        </div>
      </div>
    </div>
  {/if}

  <!-- Step 3: Game Details -->
  {#if step === 'details'}
    <div class="space-y-6">
      <div class="card p-6">
        <div class="text-center mb-6">
          <h2 class="text-xl font-semibold text-gray-900">
            {selectedGame ? 'Review & Customize' : 'Manual Entry'}
          </h2>
          <p class="mt-2 text-sm text-gray-600">
            {selectedGame ? 'Review and customize the game information' : 'Enter the game information manually'}
          </p>
        </div>

        <form on:submit|preventDefault={handleSubmit} class="space-y-6">
          <!-- Basic Information -->
          <div class="space-y-4">
            <div class="border-b border-gray-200 pb-2">
              <h3 class="text-lg font-medium text-gray-900 flex items-center">
                <svg class="h-5 w-5 text-gray-400 mr-2" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                  <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M13 16h-1v-4h-1m1-4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z" />
                </svg>
                Game Information
              </h3>
            </div>
            
            <div class="grid grid-cols-1 gap-4 sm:grid-cols-2">
              <div class="sm:col-span-2">
                <label for="title" class="form-label">
                  Game Title <span class="text-red-500">*</span>
                </label>
                <input
                  id="title"
                  type="text"
                  bind:value={gameData.title}
                  required
                  placeholder="Enter the full game title"
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
                  placeholder="Game developer/studio"
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
          <div class="space-y-4">
            <div class="border-b border-gray-200 pb-2">
              <h3 class="text-lg font-medium text-gray-900 flex items-center">
                <svg class="h-5 w-5 text-gray-400 mr-2" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                  <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 12h6m-6 4h6m2 5H7a2 2 0 01-2-2V5a2 2 0 012-2h5.586a1 1 0 01.707.293l5.414 5.414a1 1 0 01.293.707V19a2 2 0 01-2 2z" />
                </svg>
                Description
              </h3>
            </div>
            
            <div>
              <label for="description" class="form-label">
                Game Description
              </label>
              <textarea
                id="description"
                bind:value={gameData.description}
                rows="4"
                placeholder="What is this game about? Describe the gameplay, story, or main features..."
                class="form-input resize-none"
              ></textarea>
            </div>
          </div>

          <!-- Personal Information -->
          <div class="space-y-4">
            <div class="border-b border-gray-200 pb-2">
              <h3 class="text-lg font-medium text-gray-900 flex items-center">
                <svg class="h-5 w-5 text-gray-400 mr-2" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                  <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M16 7a4 4 0 11-8 0 4 4 0 018 0zM12 14a7 7 0 00-7 7h14a7 7 0 00-7-7z" />
                </svg>
                Personal Information
              </h3>
            </div>

            <div class="grid grid-cols-1 gap-4 sm:grid-cols-2">
              <div>
                <label for="play_status" class="form-label">
                  Play Status
                </label>
                <select
                  id="play_status"
                  bind:value={gameData.play_status}
                  class="form-input"
                >
                  <option value="not_started">🆕 Not Started</option>
                  <option value="in_progress">🎮 In Progress</option>
                  <option value="completed">✅ Completed</option>
                  <option value="mastered">🏆 Mastered</option>
                  <option value="dominated">👑 Dominated</option>
                  <option value="shelved">📚 Shelved</option>
                  <option value="dropped">❌ Dropped</option>
                  <option value="replay">🔄 Replay</option>
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
                  <option value="owned">💿 Owned</option>
                  <option value="borrowed">🤝 Borrowed</option>
                  <option value="rented">📅 Rented</option>
                  <option value="subscription">📱 Subscription</option>
                  <option value="no_longer_owned">📦 No Longer Owned</option>
                </select>
                <p class="mt-1 text-xs text-gray-500">
                  {#if gameData.ownership_status === 'owned'}
                    You own this game permanently
                  {:else if gameData.ownership_status === 'borrowed'}
                    Temporarily borrowed from someone
                  {:else if gameData.ownership_status === 'rented'}
                    Rented from a store or service
                  {:else if gameData.ownership_status === 'subscription'}
                    Available through a subscription service
                  {:else if gameData.ownership_status === 'no_longer_owned'}
                    Previously owned but no longer have access
                  {/if}
                </p>
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
                  <option value={1}>⭐ 1 Star</option>
                  <option value={2}>⭐⭐ 2 Stars</option>
                  <option value={3}>⭐⭐⭐ 3 Stars</option>
                  <option value={4}>⭐⭐⭐⭐ 4 Stars</option>
                  <option value={5}>⭐⭐⭐⭐⭐ 5 Stars</option>
                </select>
              </div>

              <div>
                <label for="hours_played" class="form-label">
                  Hours Played
                </label>
                <div class="relative">
                  <input
                    id="hours_played"
                    type="number"
                    min="0"
                    step="0.1"
                    bind:value={gameData.hours_played}
                    placeholder="0"
                    class="form-input pr-10"
                  />
                  <div class="absolute inset-y-0 right-0 pr-3 flex items-center pointer-events-none">
                    <span class="text-gray-500 sm:text-sm">hrs</span>
                  </div>
                </div>
              </div>
            </div>

            <div class="mt-4">
              <label class="flex items-center p-3 bg-gray-50 rounded-lg border border-gray-200 cursor-pointer hover:bg-gray-100 transition-colors duration-200">
                <input
                  id="is_loved"
                  type="checkbox"
                  bind:checked={gameData.is_loved}
                  class="h-4 w-4 text-primary-600 focus:ring-primary-500 border-gray-300 rounded"
                />
                <span class="ml-3 text-sm font-medium text-gray-900 flex items-center gap-1">
                  <span>Loved Game</span>
                  <span class="text-red-500">♥</span>
                </span>
              </label>
            </div>
          </div>

          <!-- Platforms & Storefronts -->
          <div class="space-y-4">
            <div class="border-b border-gray-200 pb-2">
              <h3 class="text-lg font-medium text-gray-900 flex items-center">
                <svg class="h-5 w-5 text-gray-400 mr-2" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                  <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9.75 17L9 20l-1 1h8l-1-1-.75-3M3 13h18M5 17h14a2 2 0 002-2V5a2 2 0 00-2-2H5a2 2 0 00-2 2v10a2 2 0 002 2z" />
                </svg>
                Platforms & Storefronts
              </h3>
            </div>
            
            {#if $platforms.isLoading}
              <div class="text-center py-6">
                <svg class="animate-spin h-8 w-8 text-gray-400 mx-auto" xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24">
                  <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
                  <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path>
                </svg>
                <p class="mt-2 text-sm text-gray-500">Loading platforms...</p>
              </div>
            {:else if $platforms.error}
              <div class="rounded-lg bg-red-50 border border-red-200 p-4">
                <div class="flex">
                  <div class="flex-shrink-0">
                    <svg class="h-5 w-5 text-red-600" viewBox="0 0 20 20" fill="currentColor">
                      <path fill-rule="evenodd" d="M10 18a8 8 0 100-16 8 8 0 000 16zM8.28 7.22a.75.75 0 00-1.06 1.06L8.94 10l-1.72 1.72a.75.75 0 101.06 1.06L10 11.06l1.72 1.72a.75.75 0 101.06-1.06L11.06 10l1.72-1.72a.75.75 0 00-1.06-1.06L10 8.94 8.28 7.22z" clip-rule="evenodd" />
                    </svg>
                  </div>
                  <div class="ml-3">
                    <h3 class="text-sm font-medium text-red-900">Platform Loading Error</h3>
                    <p class="mt-1 text-sm text-red-800">{$platforms.error}</p>
                  </div>
                </div>
              </div>
            {:else}
              <div class="space-y-3">
                <!-- IGDB Platforms Section -->
                {#if igdbPlatforms.length > 0}
                  <div class="mb-4">
                    <h4 class="text-sm font-medium text-gray-700 mb-3 flex items-center">
                      <svg class="h-4 w-4 text-primary-500 mr-2" fill="currentColor" viewBox="0 0 20 20">
                        <path fill-rule="evenodd" d="M10 18a8 8 0 100-16 8 8 0 000 16zm3.707-9.293a1 1 0 00-1.414-1.414L9 10.586 7.707 9.293a1 1 0 00-1.414 1.414l2 2a1 1 0 001.414 0l4-4z" clip-rule="evenodd" />
                      </svg>
                      Available on these platforms
                    </h4>
                    <div class="space-y-3">
                      {#each igdbPlatforms as platform (platform.id)}
                        <div class="border border-gray-200 rounded-lg overflow-hidden transition-all duration-200 {selectedPlatforms.has(platform.id) ? 'border-primary-300 shadow-sm' : ''}">
                          <!-- Platform Header -->
                          <label class="flex items-center p-4 cursor-pointer hover:bg-gray-50 transition-colors duration-200">
                            <input
                              id="platform-details-{platform.id}"
                              type="checkbox"
                              checked={selectedPlatforms.has(platform.id)}
                              on:change={() => togglePlatform(platform.id)}
                              class="h-4 w-4 text-primary-600 focus:ring-primary-500 border-gray-300 rounded"
                            />
                            <div class="ml-3 flex items-center gap-2 flex-1">
                              {#if platform.icon_url}
                                <img src={platform.icon_url} alt={platform.display_name} class="w-6 h-6 object-contain" />
                              {/if}
                              <span class="text-sm font-medium text-gray-900">{platform.display_name}</span>
                            </div>
                            {#if selectedPlatforms.has(platform.id)}
                              <svg class="h-5 w-5 text-primary-500" fill="currentColor" viewBox="0 0 20 20">
                                <path fill-rule="evenodd" d="M10 18a8 8 0 100-16 8 8 0 000 16zm3.707-9.293a1 1 0 00-1.414-1.414L9 10.586 7.707 9.293a1 1 0 00-1.414 1.414l2 2a1 1 0 001.414 0l4-4z" clip-rule="evenodd" />
                              </svg>
                            {/if}
                          </label>

                          <!-- Platform Details (shown when selected) -->
                          {#if selectedPlatforms.has(platform.id)}
                            <div class="px-4 pb-4 bg-gray-50 border-t border-gray-200">
                              <div class="grid grid-cols-1 sm:grid-cols-2 gap-3 pt-3">
                                <!-- Storefront Selection -->
                                <div>
                                  <label for="storefront-details-{platform.id}" class="block text-xs font-medium text-gray-700 mb-1">
                                    Storefront (optional)
                                  </label>
                                  <select
                                    id="storefront-details-{platform.id}"
                                    value={platformStorefronts.get(platform.id) || ''}
                                    on:change={(e) => setStorefrontForPlatform(platform.id, e.currentTarget.value)}
                                    class="form-input text-sm py-1.5"
                                  >
                                    <option value="">No specific storefront</option>
                                    {#each activeStorefronts as storefront (storefront.id)}
                                      <option value={storefront.id}>{storefront.display_name}</option>
                                    {/each}
                                  </select>
                                </div>

                                <!-- Store URL -->
                                <div>
                                  <label for="store-url-details-{platform.id}" class="block text-xs font-medium text-gray-700 mb-1">
                                    Store URL (optional)
                                  </label>
                                  <input
                                    id="store-url-details-{platform.id}"
                                    type="url"
                                    value={platformStoreUrls.get(platform.id) || ''}
                                    on:input={(e) => setStoreUrlForPlatform(platform.id, e.currentTarget.value)}
                                    placeholder="https://store.example.com/game"
                                    class="form-input text-sm py-1.5"
                                  />
                                </div>
                              </div>
                            </div>
                          {/if}
                        </div>
                      {/each}
                    </div>
                  </div>
                {/if}

                <!-- Others Section -->
                {#if otherPlatforms.length > 0}
                  <div>
                    <button
                      type="button"
                      on:click={() => showOtherPlatforms = !showOtherPlatforms}
                      class="w-full flex items-center justify-between p-3 bg-gray-50 border border-gray-200 rounded-lg hover:bg-gray-100 transition-colors duration-200"
                    >
                      <span class="text-sm font-medium text-gray-700 flex items-center">
                        <svg class="h-4 w-4 text-gray-400 mr-2" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 11H5m14-7l-7 7-7-7m14 14l-7-7-7 7" />
                        </svg>
                        Other platforms ({otherPlatforms.length})
                      </span>
                      <svg class="h-4 w-4 text-gray-400 transition-transform duration-200 {showOtherPlatforms ? 'rotate-180' : ''}" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 9l-7 7-7-7" />
                      </svg>
                    </button>
                    
                    {#if showOtherPlatforms}
                      <div class="mt-3 space-y-3">
                        {#each otherPlatforms as platform (platform.id)}
                          <div class="border border-gray-200 rounded-lg overflow-hidden transition-all duration-200 {selectedPlatforms.has(platform.id) ? 'border-primary-300 shadow-sm' : ''}">
                            <!-- Platform Header -->
                            <label class="flex items-center p-4 cursor-pointer hover:bg-gray-50 transition-colors duration-200">
                              <input
                                id="platform-details-other-{platform.id}"
                                type="checkbox"
                                checked={selectedPlatforms.has(platform.id)}
                                on:change={() => togglePlatform(platform.id)}
                                class="h-4 w-4 text-primary-600 focus:ring-primary-500 border-gray-300 rounded"
                              />
                              <div class="ml-3 flex items-center gap-2 flex-1">
                                {#if platform.icon_url}
                                  <img src={platform.icon_url} alt={platform.display_name} class="w-6 h-6 object-contain" />
                                {/if}
                                <span class="text-sm font-medium text-gray-900">{platform.display_name}</span>
                              </div>
                              {#if selectedPlatforms.has(platform.id)}
                                <svg class="h-5 w-5 text-primary-500" fill="currentColor" viewBox="0 0 20 20">
                                  <path fill-rule="evenodd" d="M10 18a8 8 0 100-16 8 8 0 000 16zm3.707-9.293a1 1 0 00-1.414-1.414L9 10.586 7.707 9.293a1 1 0 00-1.414 1.414l2 2a1 1 0 001.414 0l4-4z" clip-rule="evenodd" />
                                </svg>
                              {/if}
                            </label>

                            <!-- Platform Details (shown when selected) -->
                            {#if selectedPlatforms.has(platform.id)}
                              <div class="px-4 pb-4 bg-gray-50 border-t border-gray-200">
                                <div class="grid grid-cols-1 sm:grid-cols-2 gap-3 pt-3">
                                  <!-- Storefront Selection -->
                                  <div>
                                    <label for="storefront-details-other-{platform.id}" class="block text-xs font-medium text-gray-700 mb-1">
                                      Storefront (optional)
                                    </label>
                                    <select
                                      id="storefront-details-other-{platform.id}"
                                      value={platformStorefronts.get(platform.id) || ''}
                                      on:change={(e) => setStorefrontForPlatform(platform.id, e.currentTarget.value)}
                                      class="form-input text-sm py-1.5"
                                    >
                                      <option value="">No specific storefront</option>
                                      {#each activeStorefronts as storefront (storefront.id)}
                                        <option value={storefront.id}>{storefront.display_name}</option>
                                      {/each}
                                    </select>
                                  </div>

                                  <!-- Store URL -->
                                  <div>
                                    <label for="store-url-details-other-{platform.id}" class="block text-xs font-medium text-gray-700 mb-1">
                                      Store URL (optional)
                                    </label>
                                    <input
                                      id="store-url-details-other-{platform.id}"
                                      type="url"
                                      value={platformStoreUrls.get(platform.id) || ''}
                                      on:input={(e) => setStoreUrlForPlatform(platform.id, e.currentTarget.value)}
                                      placeholder="https://store.example.com/game"
                                      class="form-input text-sm py-1.5"
                                    />
                                  </div>
                                </div>
                              </div>
                            {/if}
                          </div>
                        {/each}
                      </div>
                    {/if}
                  </div>
                {/if}

                <!-- Fallback: Show all platforms if no IGDB data -->
                {#if igdbPlatforms.length === 0 && otherPlatforms.length === 0}
                  {#each activePlatforms as platform (platform.id)}
                    <div class="border border-gray-200 rounded-lg overflow-hidden transition-all duration-200 {selectedPlatforms.has(platform.id) ? 'border-primary-300 shadow-sm' : ''}">
                      <!-- Platform Header -->
                      <label class="flex items-center p-4 cursor-pointer hover:bg-gray-50 transition-colors duration-200">
                        <input
                          id="platform-details-{platform.id}"
                          type="checkbox"
                          checked={selectedPlatforms.has(platform.id)}
                          on:change={() => togglePlatform(platform.id)}
                          class="h-4 w-4 text-primary-600 focus:ring-primary-500 border-gray-300 rounded"
                        />
                        <div class="ml-3 flex items-center gap-2 flex-1">
                          {#if platform.icon_url}
                            <img src={platform.icon_url} alt={platform.display_name} class="w-6 h-6 object-contain" />
                          {/if}
                          <span class="text-sm font-medium text-gray-900">{platform.display_name}</span>
                        </div>
                        {#if selectedPlatforms.has(platform.id)}
                          <svg class="h-5 w-5 text-primary-500" fill="currentColor" viewBox="0 0 20 20">
                            <path fill-rule="evenodd" d="M10 18a8 8 0 100-16 8 8 0 000 16zm3.707-9.293a1 1 0 00-1.414-1.414L9 10.586 7.707 9.293a1 1 0 00-1.414 1.414l2 2a1 1 0 001.414 0l4-4z" clip-rule="evenodd" />
                          </svg>
                        {/if}
                      </label>

                      <!-- Platform Details (shown when selected) -->
                      {#if selectedPlatforms.has(platform.id)}
                        <div class="px-4 pb-4 bg-gray-50 border-t border-gray-200">
                          <div class="grid grid-cols-1 sm:grid-cols-2 gap-3 pt-3">
                            <!-- Storefront Selection -->
                            <div>
                              <label for="storefront-details-{platform.id}" class="block text-xs font-medium text-gray-700 mb-1">
                                Storefront (optional)
                              </label>
                              <select
                                id="storefront-details-{platform.id}"
                                value={platformStorefronts.get(platform.id) || ''}
                                on:change={(e) => setStorefrontForPlatform(platform.id, e.currentTarget.value)}
                                class="form-input text-sm py-1.5"
                              >
                                <option value="">No specific storefront</option>
                                {#each activeStorefronts as storefront (storefront.id)}
                                  <option value={storefront.id}>{storefront.display_name}</option>
                                {/each}
                              </select>
                            </div>

                            <!-- Store URL -->
                            <div>
                              <label for="store-url-details-{platform.id}" class="block text-xs font-medium text-gray-700 mb-1">
                                Store URL (optional)
                              </label>
                              <input
                                id="store-url-details-{platform.id}"
                                type="url"
                                value={platformStoreUrls.get(platform.id) || ''}
                                on:input={(e) => setStoreUrlForPlatform(platform.id, e.currentTarget.value)}
                                placeholder="https://store.example.com/game"
                                class="form-input text-sm py-1.5"
                              />
                            </div>
                          </div>
                        </div>
                      {/if}
                    </div>
                  {/each}
                {/if}
              </div>
            {/if}
            
            {#if activePlatforms.length === 0}
                  <div class="text-center py-8">
                    <svg class="mx-auto h-12 w-12 text-gray-300" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                      <path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5" d="M9.75 17L9 20l-1 1h8l-1-1-.75-3M3 13h18M5 17h14a2 2 0 002-2V5a2 2 0 00-2-2H5a2 2 0 00-2 2v10a2 2 0 002 2z" />
                    </svg>
                    <p class="mt-2 text-sm text-gray-500">No platforms available</p>
                    <p class="text-xs text-gray-400">Contact an administrator to add platforms.</p>
                  </div>
                {/if}
              </div>

          <!-- Personal Notes -->
          <div class="space-y-4">
            <div class="border-b border-gray-200 pb-2">
              <h3 class="text-lg font-medium text-gray-900 flex items-center">
                <svg class="h-5 w-5 text-gray-400 mr-2" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                  <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M11 5H6a2 2 0 00-2 2v11a2 2 0 002 2h11a2 2 0 002-2v-5m-1.414-9.414a2 2 0 112.828 2.828L11.828 15H9v-2.828l8.586-8.586z" />
                </svg>
                Personal Notes
              </h3>
            </div>
            
            <div>
              <label for="personal_notes" class="form-label">
                Your Notes
              </label>
              <textarea
                id="personal_notes"
                bind:value={gameData.personal_notes}
                rows="4"
                placeholder="Add your thoughts, memories, or notes about this game..."
                class="form-input resize-none"
              ></textarea>
            </div>
          </div>

          <!-- Actions -->
          <div class="pt-6 border-t border-gray-200">
            <div class="flex flex-col sm:flex-row justify-between gap-3">
              <button
                type="button"
                on:click={goBack}
                class="btn-secondary inline-flex items-center justify-center gap-x-2 order-2 sm:order-1"
              >
                <svg class="h-5 w-5" viewBox="0 0 20 20" fill="currentColor">
                  <path fill-rule="evenodd" d="M12.79 5.23a.75.75 0 01-.02 1.06L8.832 10l3.938 3.71a.75.75 0 11-1.04 1.08l-4.5-4.25a.75.75 0 010-1.08l4.5-4.25a.75.75 0 011.06.02z" clip-rule="evenodd" />
                </svg>
                {selectedGame ? 'Back to Selection' : 'Back to Search'}
              </button>
              <button
                type="submit"
                class="btn-primary inline-flex items-center justify-center gap-x-2 font-medium order-1 sm:order-2"
              >
                <svg class="h-5 w-5" viewBox="0 0 20 20" fill="currentColor">
                  <path fill-rule="evenodd" d="M10 18a8 8 0 100-16 8 8 0 000 16zm3.857-9.809a.75.75 0 00-1.214-.882l-3.236 4.53L8.107 10.5a.75.75 0 00-1.214 1.029l2.5 3.5a.75.75 0 001.214 0l4-5.5z" clip-rule="evenodd" />
                </svg>
                Add to Collection
              </button>
            </div>
          </div>
        </form>
      </div>
    </div>
  {/if}
</div>
</RouteGuard>