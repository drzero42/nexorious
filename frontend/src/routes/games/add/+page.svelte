<script lang="ts">
  import { games } from '$lib/stores';
  import { userGames, OwnershipStatus, PlayStatus } from '$lib/stores/user-games.svelte';
  import type { UserGamePlatform } from '$lib/stores/user-games.svelte';
  import { platforms } from '$lib/stores/platforms.svelte';
  import { notifications } from '$lib/stores/notifications.svelte';
  import { goto } from '$app/navigation';
  import { RouteGuard } from '$lib/components';
  import GameSearchStep from '$lib/components/GameSearchStep.svelte';
  import GameConfirmStep from '$lib/components/GameConfirmStep.svelte';
  import MetadataConfirmStep from '$lib/components/MetadataConfirmStep.svelte';
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
  let platformStorefronts = new Map<string, Set<string>>(); // platform_id -> Set<storefront_id>
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

  // Helper functions for ownership detection
  function isGameOwned(igdbId: string): boolean {
    return userGames.value.userGames.some((userGame: any) => 
      userGame.game.igdb_id === igdbId
    );
  }

  function getOwnedPlatformDetailsForGame(igdbId: string): UserGamePlatform[] {
    const userGame = userGames.value.userGames.find((ug: any) => ug.game.igdb_id === igdbId);
    if (!userGame || !userGame.platforms) return [];
    
    // Return the actual UserGamePlatform objects, not flattened objects
    return userGame.platforms;
  }

  // Platform management functions
  function togglePlatform(platformId: string) {
    if (selectedPlatforms.has(platformId)) {
      selectedPlatforms.delete(platformId);
      platformStorefronts.delete(platformId);
      platformStoreUrls.delete(platformId);
    } else {
      selectedPlatforms.add(platformId);
      
      // Create storefronts set and auto-select default if available
      const storefronts = new Set<string>();
      const platform = $platforms.platforms.find(p => p.id === platformId);
      if (platform?.default_storefront_id) {
        storefronts.add(platform.default_storefront_id);
      }
      
      platformStorefronts.set(platformId, storefronts);
    }
    selectedPlatforms = new Set(selectedPlatforms); // Trigger reactivity
    platformStorefronts = new Map(platformStorefronts);
    platformStoreUrls = new Map(platformStoreUrls);
  }

  function toggleStorefrontForPlatform(platformId: string, storefrontId: string) {
    const storefronts = platformStorefronts.get(platformId) || new Set<string>();
    if (storefronts.has(storefrontId)) {
      storefronts.delete(storefrontId);
    } else {
      storefronts.add(storefrontId);
    }
    platformStorefronts.set(platformId, storefronts);
    platformStorefronts = new Map(platformStorefronts); // Trigger reactivity
  }

  function setStoreUrlForPlatform(platformId: string, url: string) {
    if (url.trim()) {
      platformStoreUrls.set(platformId, url);
    } else {
      platformStoreUrls.delete(platformId);
    }
    platformStoreUrls = new Map(platformStoreUrls); // Trigger reactivity
  }

  async function handleSearch(event: CustomEvent<{ query: string }>) {
    searchQuery = event.detail.query;
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

  function handleGameClick(event: CustomEvent<{ game: IGDBGameCandidate; owned: boolean }>) {
    const { game, owned } = event.detail;
    
    if (owned) {
      // If game is already owned, show details/edit mode
      notifications.showInfo(`"${game.title}" is already in your collection. Click to view details.`);
      // Could redirect to the game's detail page here
      return;
    }
    
    // Select game for addition
    selectGame(game);
  }

  function selectGame(game: IGDBGameCandidate) {
    selectedGame = game;
    // Pre-populate gameData with the selected game's information
    gameData.title = game.title;
    gameData.description = game.description || '';
    gameData.release_date = game.release_date || '';
    gameData.cover_art_url = game.cover_art_url || '';
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
      notifications.showSuccess(`Adding "${createdGame.title}" to your collection`);
      
      try {
        // Add the game to the user's collection with form values
        const platformData: any[] = [];
        for (const platformId of selectedPlatforms) {
          const storefronts = platformStorefronts.get(platformId) || new Set<string>();
          
          if (storefronts.size === 0) {
            // No storefronts selected for this platform
            platformData.push({
              platform_id: platformId,
              storefront_id: null,
              store_game_id: null,
              store_url: platformStoreUrls.get(platformId) || null,
              is_available: true
            });
          } else {
            // Create an entry for each selected storefront
            for (const storefrontId of storefronts) {
              platformData.push({
                platform_id: platformId,
                storefront_id: storefrontId,
                store_game_id: null,
                store_url: platformStoreUrls.get(platformId) || null,
                is_available: true
              });
            }
          }
        }
        
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
        notifications.showError(`Game was imported but couldn't be added to your collection. You can try adding it manually from your games list.`);
        // Brief delay before redirect
        setTimeout(() => {
          goto('/games');
        }, 2000);
      }
    } catch (error) {
      console.error('Failed to create game:', error);
      notifications.showError('Failed to import game from IGDB. You can add it manually with custom details.');
      
      // If import fails, fall back to manual entry with current form data
      // Ensure gameData is populated from selectedGame if available
      if (selectedGame) {
        // Update properties individually to ensure Svelte reactivity
        gameData.title = selectedGame.title;
        gameData.description = selectedGame.description || '';
        gameData.release_date = selectedGame.release_date || '';
        gameData.cover_art_url = selectedGame.cover_art_url || '';
      }
      
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

  function handleManualAdd() {
    // Reset game data for manual entry
    selectedGame = null;
    gameData = {
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
    step = 'details';
  }

  async function createManualGame() {
    if (!gameData.title.trim()) return;

    try {
      // Create game manually with form data
      const createdGame = await games.createGame({
        title: gameData.title.trim(),
        description: gameData.description || '',
        genre: gameData.genre || '',
        developer: gameData.developer || '',
        publisher: gameData.publisher || '',
        release_date: gameData.release_date || '',
        cover_art_url: gameData.cover_art_url || '',
        game_metadata: gameData.game_metadata || ''
      });

      notifications.showSuccess('Game created successfully');

      try {
        // Add the game to the user's collection with form values
        const platformData: any[] = [];
        for (const platformId of selectedPlatforms) {
          const storefronts = platformStorefronts.get(platformId) || new Set<string>();
          
          if (storefronts.size === 0) {
            // No storefronts selected for this platform
            platformData.push({
              platform_id: platformId,
              storefront_id: null,
              store_game_id: null,
              store_url: platformStoreUrls.get(platformId) || null,
              is_available: true
            });
          } else {
            // Create an entry for each selected storefront
            for (const storefrontId of storefronts) {
              platformData.push({
                platform_id: platformId,
                storefront_id: storefrontId,
                store_game_id: null,
                store_url: platformStoreUrls.get(platformId) || null,
                is_available: true
              });
            }
          }
        }
        
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

  // Platform event handlers
  function handlePlatformToggle(event: CustomEvent<{ platformId: string }>) {
    togglePlatform(event.detail.platformId);
  }

  function handleStorefrontToggle(event: CustomEvent<{ platformId: string; storefrontId: string }>) {
    toggleStorefrontForPlatform(event.detail.platformId, event.detail.storefrontId);
  }

  function handleStoreUrlChange(event: CustomEvent<{ platformId: string; url: string }>) {
    setStoreUrlForPlatform(event.detail.platformId, event.detail.url);
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

  <!-- Step Content -->
  {#if step === 'search'}
    <GameSearchStep
      bind:searchQuery
      bind:isSearching
      on:search={handleSearch}
      on:manual-add={handleManualAdd}
    />
  {:else if step === 'confirm'}
    <GameConfirmStep
      {searchResults}
      {addingGameId}
      {isGameOwned}
      {getOwnedPlatformDetailsForGame}
      on:back={goBack}
      on:manual-add={handleManualAdd}
      on:game-click={handleGameClick}
    />
  {:else if step === 'metadata-confirm'}
    <MetadataConfirmStep
      {selectedGame}
      bind:gameData
      {addingGameId}
      bind:selectedPlatforms
      bind:platformStorefronts
      bind:platformStoreUrls
      on:back={goBack}
      on:edit-details={() => step = 'details'}
      on:confirm={confirmGameAddition}
      on:platform-toggle={handlePlatformToggle}
      on:storefront-toggle={handleStorefrontToggle}
      on:store-url-change={handleStoreUrlChange}
    />
  {:else if step === 'details'}
    <!-- TODO: Extract GameDetailsStep component -->
    <div class="card p-6">
      <div class="text-center mb-6">
        <h2 class="text-xl font-semibold text-gray-900">
          {selectedGame ? 'Review & Customize' : 'Manual Entry'}
        </h2>
        <p class="mt-2 text-sm text-gray-600">
          {selectedGame ? 'Review and customize the game information' : 'Enter the game information manually'}
        </p>
      </div>
      
      <div class="space-y-4">
        <div>
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
          <label for="description" class="form-label">
            Description
          </label>
          <textarea
            id="description"
            bind:value={gameData.description}
            rows="3"
            placeholder="Game description..."
            class="form-input resize-none"
          ></textarea>
        </div>

        <div class="flex gap-3">
          <button
            type="button"
            on:click={goBack}
            class="btn-secondary flex-1"
          >
            {selectedGame ? 'Back to Selection' : 'Back to Search'}
          </button>
          <button
            type="button"
            on:click={selectedGame ? () => step = 'metadata-confirm' : createManualGame}
            class="btn-primary flex-1"
            disabled={!gameData.title.trim()}
          >
            {selectedGame ? 'Continue' : 'Add to Collection'}
          </button>
        </div>
      </div>
    </div>
  {/if}
</div>
</RouteGuard>