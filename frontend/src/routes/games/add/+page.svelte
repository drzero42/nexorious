<script lang="ts">
  import { games } from '$lib/stores';
  import { userGames } from '$lib/stores/user-games.svelte';
  import type { UserGamePlatform, UserGame } from '$lib/stores/user-games.svelte';
  import { platforms } from '$lib/stores/platforms.svelte';
  import { notifications } from '$lib/stores/notifications.svelte';
  import { gameAdditionService, type GameFormData } from '$lib/services/game-addition';
  import { goto } from '$app/navigation';
  import { RouteGuard } from '$lib/components';
  import GameSearchStep from '$lib/components/GameSearchStep.svelte';
  import GameConfirmStep from '$lib/components/GameConfirmStep.svelte';
  import MetadataConfirmStep from '$lib/components/MetadataConfirmStep.svelte';
  import type { IGDBGameCandidate } from '$lib/stores/games.svelte';
  import { onMount, onDestroy } from 'svelte';
  import type { GameId } from '$lib/types/game';

  let searchQuery = '';
  let isSearching = false;
  let addingGameId: GameId | null = null;
  let searchResults: IGDBGameCandidate[] = [];
  let selectedGame: IGDBGameCandidate | null = null;
  let step: 'search' | 'confirm' | 'metadata-confirm' = 'search';

  // Form data for new game (personal data only, IGDB metadata is read-only)
  let gameData: GameFormData = {
    // Personal data (editable)
    personal_rating: null,
    play_status: 'not_started',
    hours_played: 0,
    personal_notes: '',
    ownership_status: 'owned',
    is_loved: false,
    // IGDB metadata (read-only, populated from selected game for display only)
    title: '',
    description: '',
    release_date: '',
    cover_art_url: ''
  };

  // Platform association data
  let selectedPlatforms = new Set<string>();
  let platformStorefronts = new Map<string, Set<string>>(); // platform_id -> Set<storefront_id>
  let platformStoreUrls = new Map<string, string>(); // platform_id -> store_url

  // Cleanup timeouts on component destroy
  const redirectTimeouts: ReturnType<typeof setTimeout>[] = [];
  
  onDestroy(() => {
    redirectTimeouts.forEach(timeoutId => clearTimeout(timeoutId));
    redirectTimeouts.length = 0;
  });

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
  function isGameOwned(igdbId: GameId): boolean {
    return userGames.value.userGames.some((userGame: UserGame) => 
      userGame.game.igdb_id === igdbId
    );
  }

  function getOwnedPlatformDetailsForGame(igdbId: GameId): UserGamePlatform[] {
    const userGame = userGames.value.userGames.find((ug: UserGame) => ug.game.igdb_id === igdbId);
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
      const platform = platforms.value?.platforms?.find(p => p.id === platformId);
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
    // Populate IGDB metadata for display only (these fields are read-only)
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
      const result = await gameAdditionService.addGameComplete(
        selectedGame,
        gameData,
        selectedPlatforms,
        platformStorefronts,
        platformStoreUrls
      );

      if (result.success) {
        // Brief delay to show success message before redirect
        const timeoutId = setTimeout(() => {
          goto('/games');
        }, 1000);
        redirectTimeouts.push(timeoutId);
      } else {
        // Brief delay before redirect on failure
        const timeoutId = setTimeout(() => {
          goto('/games');
        }, 2000);
        redirectTimeouts.push(timeoutId);
      }
    } catch (error) {
      console.error('Unexpected error during game addition:', error);
      notifications.showError('An unexpected error occurred. Please try again.');
    } finally {
      addingGameId = null;
    }
  }

  function goBack() {
    if (step === 'metadata-confirm') {
      step = 'confirm';
    } else if (step === 'confirm') {
      step = 'search';
      searchResults = [];
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
        onclick={() => goto('/games')}
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
        <div class="{step === 'search' ? 'bg-primary-500 text-white' : step === 'confirm' || step === 'metadata-confirm' ? 'bg-primary-100 text-primary-700' : 'bg-gray-200 text-gray-500'} rounded-full w-8 h-8 flex items-center justify-center text-sm font-medium">
          1
        </div>
        <span class="ml-2 text-sm font-medium {step === 'search' ? 'text-gray-900' : 'text-gray-500'}">Search</span>
      </div>
      <svg class="w-5 h-5 text-gray-300" fill="currentColor" viewBox="0 0 20 20">
        <path fill-rule="evenodd" d="M7.293 14.707a1 1 0 010-1.414L10.586 10 7.293 6.707a1 1 0 011.414-1.414l4 4a1 1 0 010 1.414l-4 4a1 1 0 01-1.414 0z" clip-rule="evenodd" />
      </svg>
      <div class="flex items-center">
        <div class="{step === 'confirm' || step === 'metadata-confirm' ? 'bg-primary-500 text-white' : 'bg-gray-200 text-gray-500'} rounded-full w-8 h-8 flex items-center justify-center text-sm font-medium">
          2
        </div>
        <span class="ml-2 text-sm font-medium {step === 'confirm' || step === 'metadata-confirm' ? 'text-gray-900' : 'text-gray-500'}">Add Game</span>
      </div>
    </div>
  </div>

  <!-- Step Content -->
  {#if step === 'search'}
    <GameSearchStep
      bind:searchQuery
      bind:isSearching
      onsearch={handleSearch}
    />
  {:else if step === 'confirm'}
    <GameConfirmStep
      {searchResults}
      {addingGameId}
      {isGameOwned}
      {getOwnedPlatformDetailsForGame}
      onback={goBack}
      ongameclick={handleGameClick}
    />
  {:else if step === 'metadata-confirm'}
    <MetadataConfirmStep
      {selectedGame}
      bind:gameData
      {addingGameId}
      bind:selectedPlatforms
      bind:platformStorefronts
      bind:platformStoreUrls
      onback={goBack}
      onconfirm={confirmGameAddition}
      onplatformtoggle={handlePlatformToggle}
      onstorefronttoggle={handleStorefrontToggle}
      onstoreurlchange={handleStoreUrlChange}
    />
  {/if}
</div>
</RouteGuard>