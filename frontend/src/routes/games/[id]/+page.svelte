<script lang="ts">
  import { page } from '$app/stores';
  import { userGames } from '$lib/stores';
  import { games } from '$lib/stores/games.svelte';
  import { platforms } from '$lib/stores/platforms.svelte';
  import { notifications } from '$lib/stores/notifications.svelte';
  import { onMount } from 'svelte';
  import { goto } from '$app/navigation';
  import { RouteGuard, PlayStatusDropdown, TimeTrackingInput, RichTextEditor, GameProgressCard } from '$lib/components';
  import { resolveImageUrl } from '$lib/utils/image-url';
  import { groupPlatformsByPlatform } from '$lib/utils/platform-utils';
  import type { UserGame, PlayStatus, OwnershipStatus, UserGameUpdateRequest, ProgressUpdateRequest, UserGamePlatformCreateRequest } from '$lib/stores/user-games.svelte';
  import type { Game } from '$lib/stores/games.svelte';
  import type { Platform, Storefront } from '$lib/stores/platforms.svelte';

  let game: UserGame | null = null;
  let isLoading = true;
  let isEditing = false;
  let editData: {
    // Personal data
    personal_rating?: number | undefined;
    play_status: PlayStatus;
    hours_played: number;
    personal_notes?: string | undefined;
    is_loved: boolean;
    ownership_status: OwnershipStatus;
    is_physical: boolean;
    // Game metadata
    title: string;
    description?: string | undefined;
    genre?: string | undefined;
    developer?: string | undefined;
    publisher?: string | undefined;
    release_date?: string | undefined;
    estimated_playtime_hours?: number | undefined;
  } = {
    play_status: 'not_started' as PlayStatus,
    hours_played: 0,
    is_loved: false,
    ownership_status: 'owned' as OwnershipStatus,
    is_physical: false,
    title: '',
    description: undefined,
    genre: undefined,
    developer: undefined,
    publisher: undefined,
    release_date: undefined,
    estimated_playtime_hours: undefined
  };

  // Platform management variables
  let availablePlatforms: Platform[] = [];
  let availableStorefronts: Storefront[] = [];
  let newPlatformData: {
    platform_id: string;
    storefront_id: string;
    store_url: string;
    store_game_id: string;
  } = {
    platform_id: '',
    storefront_id: '',
    store_url: '',
    store_game_id: ''
  };
  let isAddingPlatform = false;
  let isLoadingPlatforms = false;
  let isRemovingPlatform = false;
  let platformToRemove: { platformAssociationId: string; platformName: string; storefrontName: string } | null = null;

  $: gameId = $page.params.id!;

  onMount(async () => {
    // Load game details and platforms - authentication is handled by RouteGuard
    await Promise.all([
      loadGame(),
      loadPlatforms()
    ]);
  });

  async function loadGame() {
    try {
      isLoading = true;
      // In a real app, this would fetch the specific game
      // For now, find it in the loaded games
      const games = userGames.value.userGames;
      game = games.find(g => g.id === gameId) || null;
      
      if (!game) {
        // Try to fetch from API
        await userGames.fetchUserGames();
        game = userGames.value.userGames.find(g => g.id === gameId) || null;
      }
      
      if (game) {
        resetEditData();
      }
    } catch (error) {
      console.error('Failed to load game:', error);
    } finally {
      isLoading = false;
    }
  }

  async function loadPlatforms() {
    try {
      isLoadingPlatforms = true;
      const data = await platforms.fetchActivePlatformsAndStorefronts();
      availablePlatforms = data.platforms;
      availableStorefronts = data.storefronts;
    } catch (error) {
      console.error('Failed to load platforms:', error);
    } finally {
      isLoadingPlatforms = false;
    }
  }

  async function addPlatform() {
    console.log('addPlatform called with data:', newPlatformData);
    console.log('game object:', game);
    
    if (!newPlatformData.platform_id || !game) {
      console.log('Missing required data - platform_id:', newPlatformData.platform_id, 'game:', !!game);
      return;
    }

    try {
      console.log('Starting platform addition...');
      isAddingPlatform = true;
      
      // Create the platform data for the API call
      const platformData: UserGamePlatformCreateRequest = {
        platform_id: newPlatformData.platform_id
      };

      // Only include storefront_id if it has a value
      if (newPlatformData.storefront_id && newPlatformData.storefront_id.trim()) {
        platformData.storefront_id = newPlatformData.storefront_id;
      }

      // Only include optional fields if they have values
      if (newPlatformData.store_url.trim()) {
        platformData.store_url = newPlatformData.store_url.trim();
      }
      if (newPlatformData.store_game_id.trim()) {
        platformData.store_game_id = newPlatformData.store_game_id.trim();
      }

      console.log('Sending platform data to API:', platformData);

      // Call the API to add the platform
      await userGames.addPlatformToUserGame(game.id, platformData);
      
      console.log('API call successful, reloading game data...');
      
      // Reload the game to get updated platform data
      await loadGame();
      
      // Reset form data
      newPlatformData = {
        platform_id: '',
        storefront_id: '',
        store_url: '',
        store_game_id: ''
      };

      console.log('Platform added successfully');
      // Show success message
      notifications.showSuccess('Platform added successfully');
      
    } catch (error) {
      console.error('Failed to add platform:', error);
      // Show error message
      notifications.showError('Failed to add platform. Please try again.');
    } finally {
      isAddingPlatform = false;
    }
  }

  function confirmRemovePlatform(platformAssociationId: string, platformName: string, storefrontName: string) {
    // Check if this would leave the game with no platform associations
    if (game && game.platforms && game.platforms.length <= 1) {
      notifications.showError('Cannot remove the last platform. Games must have at least one platform.');
      return;
    }
    
    platformToRemove = { platformAssociationId, platformName, storefrontName };
  }

  function cancelRemovePlatform() {
    platformToRemove = null;
  }

  async function removePlatform() {
    if (!platformToRemove || !game) {
      return;
    }

    try {
      isRemovingPlatform = true;
      
      // Call the API to remove the platform
      await userGames.removePlatformFromUserGame(game.id, platformToRemove.platformAssociationId);
      
      // Reload the game to get updated platform data
      await loadGame();
      
      // Clear the confirmation dialog
      platformToRemove = null;

      // Show success message
      notifications.showSuccess('Platform removed successfully');
      
    } catch (error) {
      console.error('Failed to remove platform:', error);
      // Show error message
      notifications.showError('Failed to remove platform. Please try again.');
    } finally {
      isRemovingPlatform = false;
    }
  }

  function resetEditData() {
    if (game) {
      editData = {
        // Personal data
        personal_rating: game.personal_rating || undefined,
        play_status: game.play_status,
        hours_played: game.hours_played,
        personal_notes: game.personal_notes || undefined,
        is_loved: game.is_loved,
        ownership_status: game.ownership_status,
        is_physical: game.is_physical,
        // Game metadata
        title: game.game.title,
        description: game.game.description || undefined,
        genre: game.game.genre || undefined,
        developer: game.game.developer || undefined,
        publisher: game.game.publisher || undefined,
        release_date: game.game.release_date || undefined,
        estimated_playtime_hours: game.game.estimated_playtime_hours || undefined
      };
    }
  }

  function startEditing() {
    isEditing = true;
    resetEditData();
  }

  function cancelEditing() {
    isEditing = false;
    resetEditData();
  }

  async function saveChanges() {
    try {
      // Split editData into user game update, progress update, and game metadata update
      const userGameUpdate: UserGameUpdateRequest = {
        ownership_status: editData.ownership_status,
        is_physical: editData.is_physical,
        is_loved: editData.is_loved
      };
      
      if (editData.personal_rating !== undefined && editData.personal_rating !== null) {
        userGameUpdate.personal_rating = editData.personal_rating;
      }
      
      const progressUpdate: ProgressUpdateRequest = {
        play_status: editData.play_status,
        hours_played: editData.hours_played
      };
      
      if (editData.personal_notes !== undefined) {
        progressUpdate.personal_notes = editData.personal_notes;
      }

      // Game metadata update - only include defined values
      const gameMetadataUpdate: Partial<Game> = {
        title: editData.title
      };
      
      if (editData.description !== undefined) {
        gameMetadataUpdate.description = editData.description;
      }
      if (editData.genre !== undefined) {
        gameMetadataUpdate.genre = editData.genre;
      }
      if (editData.developer !== undefined) {
        gameMetadataUpdate.developer = editData.developer;
      }
      if (editData.publisher !== undefined) {
        gameMetadataUpdate.publisher = editData.publisher;
      }
      if (editData.release_date !== undefined) {
        gameMetadataUpdate.release_date = editData.release_date;
      }
      if (editData.estimated_playtime_hours !== undefined) {
        gameMetadataUpdate.estimated_playtime_hours = editData.estimated_playtime_hours;
      }
      
      // Update game metadata first
      if (game?.game.id) {
        await games.updateGame(game.game.id, gameMetadataUpdate);
      }
      
      // Update user game data
      await userGames.updateUserGame(gameId, userGameUpdate);
      await userGames.updateProgress(gameId, progressUpdate);
      
      // Reload the game to get updated data
      await loadGame();
      isEditing = false;
    } catch (error) {
      console.error('Failed to save changes:', error);
    }
  }

  async function deleteGame() {
    if (confirm('Are you sure you want to remove this game from your collection?')) {
      try {
        await userGames.removeFromCollection(gameId);
        goto('/games');
      } catch (error) {
        console.error('Failed to delete game:', error);
      }
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

  function renderStars(rating: number) {
    const stars = [];
    for (let i = 1; i <= 5; i++) {
      stars.push(i <= rating ? '★' : '☆');
    }
    return stars.join('');
  }
</script>

<svelte:head>
  <title>{game?.game.title || 'Game Details'} - Nexorious</title>
</svelte:head>

<RouteGuard requireAuth={true}>
<div class="space-y-6">
{#if isLoading}
  <div class="flex items-center justify-center py-12">
    <div class="text-center">
      <svg class="mx-auto h-12 w-12 text-gray-400 loading" fill="none" viewBox="0 0 24 24" stroke="currentColor">
        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15" />
      </svg>
      <p class="mt-2 text-sm text-gray-500">Loading game details...</p>
    </div>
  </div>
{:else if !game}
  <div class="text-center py-12">
    <div class="mx-auto max-w-md">
      <svg class="mx-auto h-12 w-12 text-gray-400" fill="none" viewBox="0 0 24 24" stroke="currentColor">
        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9.172 16.172a4 4 0 015.656 0M9 12h6m-6 4h6m2 5H7a2 2 0 01-2-2V5a2 2 0 012-2h5.586a1 1 0 01.707.293l5.414 5.414a1 1 0 01.293.707V19a2 2 0 01-2 2z" />
      </svg>
      <h3 class="mt-4 text-lg font-medium text-gray-900">Game not found</h3>
      <p class="mt-2 text-sm text-gray-500">The requested game could not be found in your collection.</p>
      <div class="mt-6">
        <button
          on:click={() => goto('/games')}
          class="btn-primary inline-flex items-center gap-x-2"
        >
          <svg class="-ml-0.5 h-5 w-5" viewBox="0 0 20 20" fill="currentColor">
            <path fill-rule="evenodd" d="M17 10a.75.75 0 01-.75.75H5.612l4.158 3.96a.75.75 0 11-1.04 1.08l-5.5-5.25a.75.75 0 010-1.08l5.5-5.25a.75.75 0 111.04 1.08L5.612 9.25H16.25A.75.75 0 0117 10z" clip-rule="evenodd" />
          </svg>
          Back to Games
        </button>
      </div>
    </div>
  </div>
{:else}
    <!-- Header -->
    <div class="sm:flex sm:items-center sm:justify-between">
      <div class="flex items-center space-x-4">
        <button
          on:click={() => goto('/games')}
          class="btn-secondary inline-flex items-center gap-x-2"
        >
          <svg class="-ml-0.5 h-5 w-5" viewBox="0 0 20 20" fill="currentColor">
            <path fill-rule="evenodd" d="M17 10a.75.75 0 01-.75.75H5.612l4.158 3.96a.75.75 0 11-1.04 1.08l-5.5-5.25a.75.75 0 010-1.08l5.5-5.25a.75.75 0 111.04 1.08L5.612 9.25H16.25A.75.75 0 0117 10z" clip-rule="evenodd" />
          </svg>
          Back to Games
        </button>
      </div>
      <div class="mt-4 sm:mt-0 flex items-center space-x-3">
        {#if !isEditing}
          <button
            on:click={startEditing}
            class="btn-primary inline-flex items-center gap-x-2"
          >
            <svg class="-ml-0.5 h-5 w-5" viewBox="0 0 20 20" fill="currentColor">
              <path d="M2.695 14.763l-1.262 3.154a.5.5 0 00.65.65l3.155-1.262a4 4 0 001.343-.885L17.5 5.5a2.121 2.121 0 00-3-3L3.58 13.42a4 4 0 00-.885 1.343z" />
            </svg>
            Edit
          </button>
        {/if}
        <button
          on:click={deleteGame}
          class="bg-red-600 text-white px-4 py-2 rounded-md hover:bg-red-700 focus:ring-2 focus:ring-red-500 focus:ring-offset-2 transition-colors duration-200 inline-flex items-center gap-x-2"
        >
          <svg class="-ml-0.5 h-5 w-5" viewBox="0 0 20 20" fill="currentColor">
            <path fill-rule="evenodd" d="M8.75 1A2.75 2.75 0 006 3.75v.443c-.795.077-1.584.176-2.365.298a.75.75 0 10.23 1.482l.149-.022.841 10.518A2.75 2.75 0 007.596 19h4.807a2.75 2.75 0 002.742-2.53l.841-10.52.149.023a.75.75 0 00.23-1.482A41.03 41.03 0 0014 4.193V3.75A2.75 2.75 0 0011.25 1h-2.5zM10 4c.84 0 1.673.025 2.5.075V3.75c0-.69-.56-1.25-1.25-1.25h-2.5c-.69 0-1.25.56-1.25 1.25v.325C8.327 4.025 9.16 4 10 4zM8.58 7.72a.75.75 0 00-1.5.06l.3 7.5a.75.75 0 101.5-.06l-.3-7.5zm4.34.06a.75.75 0 10-1.5-.06l-.3 7.5a.75.75 0 101.5.06l.3-7.5z" clip-rule="evenodd" />
          </svg>
          Remove
        </button>
      </div>
    </div>

    <!-- Main Content -->
    <div class="card">
      <div class="lg:grid lg:grid-cols-3 lg:gap-8 p-6">
        <!-- Cover Art -->
        <div class="lg:col-span-1">
          <div class="aspect-[3/4] overflow-hidden rounded-lg bg-gray-100 shadow-lg">
            {#if game.game.cover_art_url}
              <img
                src={resolveImageUrl(game.game.cover_art_url)}
                alt={game.game.title}
                class="h-full w-full object-cover object-center"
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
              <div style="display: none;" class="h-full w-full flex items-center justify-center text-gray-400">
                <div class="text-center">
                  <svg class="mx-auto h-16 w-16 text-gray-300" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                    <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 16l4.586-4.586a2 2 0 012.828 0L16 16m-2-2l1.586-1.586a2 2 0 012.828 0L20 14m-6-6h.01M6 20h12a2 2 0 002-2V6a2 2 0 00-2-2H6a2 2 0 00-2 2v12a2 2 0 002 2z" />
                  </svg>
                  <p class="mt-2 text-sm">No Cover</p>
                </div>
              </div>
            {:else}
              <div class="h-full w-full flex items-center justify-center text-gray-400">
                <div class="text-center">
                  <svg class="mx-auto h-16 w-16 text-gray-300" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                    <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 16l4.586-4.586a2 2 0 012.828 0L16 16m-2-2l1.586-1.586a2 2 0 012.828 0L20 14m-6-6h.01M6 20h12a2 2 0 002-2V6a2 2 0 00-2-2H6a2 2 0 00-2 2v12a2 2 0 002 2z" />
                  </svg>
                  <p class="mt-2 text-sm">No Cover</p>
                </div>
              </div>
            {/if}
          </div>
        </div>

        <!-- Game Info -->
        <div class="lg:col-span-2 mt-6 lg:mt-0">
          <div class="space-y-6">
            <div class="space-y-4">
              <div class="flex items-start justify-between">
                <h1 class="text-3xl font-bold text-gray-900">
                  {game.game.title}
                </h1>
                {#if game.is_loved}
                  <span class="inline-flex items-center justify-center w-8 h-8 rounded-full bg-red-100 text-red-600 text-lg">♥</span>
                {/if}
              </div>
              <div class="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-4">
                {#if game.game.developer}
                  <div>
                    <dt class="text-sm font-medium text-gray-500">Developer</dt>
                    <dd class="mt-1 text-sm text-gray-900">{game.game.developer}</dd>
                  </div>
                {/if}
                {#if game.game.publisher}
                  <div>
                    <dt class="text-sm font-medium text-gray-500">Publisher</dt>
                    <dd class="mt-1 text-sm text-gray-900">{game.game.publisher}</dd>
                  </div>
                {/if}
                {#if game.game.genre}
                  <div>
                    <dt class="text-sm font-medium text-gray-500">Genre</dt>
                    <dd class="mt-1 text-sm text-gray-900">{game.game.genre}</dd>
                  </div>
                {/if}
                {#if game.game.release_date}
                  <div>
                    <dt class="text-sm font-medium text-gray-500">Release Date</dt>
                    <dd class="mt-1 text-sm text-gray-900">{new Date(game.game.release_date).toLocaleDateString()}</dd>
                  </div>
                {/if}
                {#if game.game.estimated_playtime_hours}
                  <div>
                    <dt class="text-sm font-medium text-gray-500">Estimated Playtime</dt>
                    <dd class="mt-1 text-sm text-gray-900">{game.game.estimated_playtime_hours} hours</dd>
                  </div>
                {/if}
                {#if game.game.igdb_id}
                  <div>
                    <dt class="text-sm font-medium text-gray-500">IGDB ID</dt>
                    <dd class="mt-1 text-sm text-gray-900">
                      {#if game.game.igdb_slug}
                        <a 
                          href="https://www.igdb.com/games/{game.game.igdb_slug}" 
                          target="_blank" 
                          rel="noopener noreferrer"
                          class="text-blue-600 hover:text-blue-800 inline-flex items-center"
                        >
                          {game.game.igdb_id}
                          <svg class="w-3 h-3 ml-1" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M10 6H6a2 2 0 00-2 2v10a2 2 0 002 2h10a2 2 0 002-2v-4M14 4h6m0 0v6m0-6L10 14"></path>
                          </svg>
                        </a>
                      {:else}
                        <span>{game.game.igdb_id}</span>
                      {/if}
                    </dd>
                  </div>
                {/if}
              </div>
            </div>

            <!-- Platform Information -->
            {#if game.platforms && game.platforms.length > 0}
              <div>
                <h3 class="text-lg font-medium text-gray-900">Available On</h3>
                <div class="mt-2 space-y-3">
                  {#each groupPlatformsByPlatform(game.platforms) as groupedPlatform}
                    <div class="bg-blue-50 border border-blue-200 rounded-lg p-3">
                      <div class="flex items-start justify-between">
                        <span class="text-sm font-semibold text-blue-900 mb-2 block">{groupedPlatform.platform.display_name}</span>
                      </div>
                      <div class="flex flex-wrap gap-2">
                        {#each groupedPlatform.storefronts as storefront}
                          <div class="inline-flex items-center space-x-2 px-2 py-1 bg-white border border-blue-300 rounded text-xs">
                            <span class="text-blue-800 font-medium">
                              {storefront.storefront?.display_name || 'Unknown Storefront'}
                            </span>
                            {#if storefront.store_url && storefront.storefront?.name !== 'physical'}
                              <a 
                                href={storefront.store_url} 
                                target="_blank" 
                                rel="noopener noreferrer"
                                class="text-blue-600 hover:text-blue-800 flex-shrink-0"
                                title="View in {storefront.storefront?.display_name || 'store'}"
                                aria-label="View {groupedPlatform.platform.display_name} on {storefront.storefront?.display_name || 'store'}"
                              >
                                <svg class="w-3 h-3" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                  <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M10 6H6a2 2 0 00-2 2v10a2 2 0 002 2h10a2 2 0 002-2v-4M14 4h6m0 0v6m0-6L10 14"></path>
                                </svg>
                              </a>
                            {/if}
                          </div>
                        {/each}
                      </div>
                    </div>
                  {/each}
                </div>
              </div>
            {/if}

            <!-- IGDB Rating and Verification -->
            {#if game.game.rating_average || game.game.is_verified}
              <div>
                <h3 class="text-lg font-medium text-gray-900">Game Rating</h3>
                <div class="mt-2 flex items-center space-x-4">
                  {#if game.game.rating_average}
                    <div class="flex items-center space-x-2">
                      <div class="flex items-center">
                        <span class="text-yellow-400 text-lg">★</span>
                        <span class="ml-1 text-sm font-medium text-gray-900">
                          {Number(game.game.rating_average).toFixed(1)}/10
                        </span>
                      </div>
                      {#if game.game.rating_count > 0}
                        <span class="text-xs text-gray-500">({game.game.rating_count.toLocaleString()} reviews)</span>
                      {/if}
                    </div>
                  {/if}
                  {#if game.game.is_verified}
                    <span class="inline-flex items-center px-2 py-1 rounded-full text-xs font-medium bg-green-100 text-green-800">
                      <svg class="w-3 h-3 mr-1" fill="currentColor" viewBox="0 0 20 20">
                        <path fill-rule="evenodd" d="M16.707 5.293a1 1 0 010 1.414l-8 8a1 1 0 01-1.414 0l-4-4a1 1 0 011.414-1.414L8 12.586l7.293-7.293a1 1 0 011.414 0z" clip-rule="evenodd"></path>
                      </svg>
                      Verified
                    </span>
                  {/if}
                </div>
              </div>
            {/if}

            <!-- How Long to Beat -->
            {#if game.game.howlongtobeat_main || game.game.howlongtobeat_extra || game.game.howlongtobeat_completionist}
              <div>
                <h3 class="text-lg font-medium text-gray-900">How Long to Beat</h3>
                <div class="mt-2 grid grid-cols-1 sm:grid-cols-3 gap-4">
                  {#if game.game.howlongtobeat_main}
                    <div class="bg-blue-50 p-3 rounded-lg text-center">
                      <div class="text-sm font-medium text-blue-900">Main Story</div>
                      <div class="text-lg font-bold text-blue-800">{game.game.howlongtobeat_main}h</div>
                    </div>
                  {/if}
                  {#if game.game.howlongtobeat_extra}
                    <div class="bg-green-50 p-3 rounded-lg text-center">
                      <div class="text-sm font-medium text-green-900">Main + Extra</div>
                      <div class="text-lg font-bold text-green-800">{game.game.howlongtobeat_extra}h</div>
                    </div>
                  {/if}
                  {#if game.game.howlongtobeat_completionist}
                    <div class="bg-purple-50 p-3 rounded-lg text-center">
                      <div class="text-sm font-medium text-purple-900">Completionist</div>
                      <div class="text-lg font-bold text-purple-800">{game.game.howlongtobeat_completionist}h</div>
                    </div>
                  {/if}
                </div>
              </div>
            {/if}

            {#if game.game.description}
              <div>
                <h3 class="text-lg font-medium text-gray-900">Description</h3>
                <p class="mt-2 text-sm text-gray-700 leading-relaxed">{game.game.description}</p>
              </div>
            {/if}
          </div>
        </div>
      </div>
    </div>

    <!-- Personal Information -->
    <div class="card">
      <div class="px-6 py-4 border-b border-gray-200">
        <h3 class="text-lg font-medium text-gray-900">Your Information</h3>
      </div>
      <div class="p-6">
        <!-- Progress Visualization -->
        {#if !isEditing}
          <div class="mb-6">
            <GameProgressCard userGame={game} />
          </div>
        {/if}
            
        {#if isEditing}
          <!-- Edit Form -->
          <form on:submit|preventDefault={saveChanges} class="space-y-8">
            <!-- Game Metadata Section -->
            <div>
              <h4 class="text-lg font-medium text-gray-900 mb-4">Game Information</h4>
              <div class="grid grid-cols-1 lg:grid-cols-2 gap-6">
                <div>
                  <label for="title" class="form-label">
                    Title
                  </label>
                  <input
                    id="title"
                    type="text"
                    bind:value={editData.title}
                    class="form-input"
                    required
                  />
                </div>

                <div>
                  <label for="genre" class="form-label">
                    Genre
                  </label>
                  <input
                    id="genre"
                    type="text"
                    bind:value={editData.genre}
                    class="form-input"
                    placeholder="e.g., Action, RPG, Strategy"
                  />
                </div>

                <div>
                  <label for="developer" class="form-label">
                    Developer
                  </label>
                  <input
                    id="developer"
                    type="text"
                    bind:value={editData.developer}
                    class="form-input"
                    placeholder="e.g., FromSoftware"
                  />
                </div>

                <div>
                  <label for="publisher" class="form-label">
                    Publisher
                  </label>
                  <input
                    id="publisher"
                    type="text"
                    bind:value={editData.publisher}
                    class="form-input"
                    placeholder="e.g., Bandai Namco"
                  />
                </div>

                <div>
                  <label for="release_date" class="form-label">
                    Release Date
                  </label>
                  <input
                    id="release_date"
                    type="date"
                    bind:value={editData.release_date}
                    class="form-input"
                  />
                </div>

                <div>
                  <label for="estimated_playtime_hours" class="form-label">
                    Estimated Playtime (hours)
                  </label>
                  <input
                    id="estimated_playtime_hours"
                    type="number"
                    min="0"
                    bind:value={editData.estimated_playtime_hours}
                    class="form-input"
                    placeholder="e.g., 40"
                  />
                </div>

                <div class="lg:col-span-2">
                  <label for="description" class="form-label">
                    Description
                  </label>
                  <textarea
                    id="description"
                    bind:value={editData.description}
                    rows="4"
                    class="form-input"
                    placeholder="Enter a description of the game..."
                  ></textarea>
                </div>
              </div>
            </div>

            <!-- Personal Information Section -->
            <div class="pt-6 border-t border-gray-200">
              <h4 class="text-lg font-medium text-gray-900 mb-4">Your Information</h4>
              <div class="grid grid-cols-1 sm:grid-cols-2 gap-6">
                <div>
                  <label for="play_status" class="form-label">
                    Play Status
                  </label>
                  <PlayStatusDropdown
                    id="play_status"
                    bind:value={editData.play_status}
                    on:change={(e: CustomEvent<{ value: PlayStatus }>) => {
                      editData.play_status = e.detail.value;
                    }}
                  />
                </div>

                <div>
                  <label for="ownership_status" class="form-label">
                    Ownership Status
                  </label>
                  <select
                    id="ownership_status"
                    bind:value={editData.ownership_status}
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
                    bind:value={editData.personal_rating}
                    class="form-input"
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
                  <label for="hours_played" class="form-label">
                    Hours Played
                  </label>
                  <TimeTrackingInput
                    id="hours_played"
                    bind:value={editData.hours_played}
                    on:change={(e: CustomEvent<{ value: number }>) => {
                      editData.hours_played = e.detail.value;
                    }}
                  />
                </div>
              </div>

              <div class="mt-6 space-y-4">
                <div class="flex items-center space-x-6">
                  <label class="inline-flex items-center">
                    <input
                      type="checkbox"
                      bind:checked={editData.is_physical}
                      class="form-checkbox h-4 w-4 text-primary-600 focus:ring-primary-500 border-gray-300 rounded"
                    />
                    <span class="ml-2 text-sm text-gray-700">Physical copy</span>
                  </label>

                  <label class="inline-flex items-center">
                    <input
                      type="checkbox"
                      bind:checked={editData.is_loved}
                      class="form-checkbox h-4 w-4 text-primary-600 focus:ring-primary-500 border-gray-300 rounded"
                    />
                    <span class="ml-2 text-sm text-gray-700">
                      <span class="text-red-500">♥</span> Loved game
                    </span>
                  </label>
                </div>

                <div>
                  <label for="personal_notes" class="form-label">
                    Personal Notes
                  </label>
                  <RichTextEditor
                    bind:value={editData.personal_notes}
                    placeholder="Add your personal notes about this game..."
                    editable={true}
                    onchange={(e: CustomEvent<{ value: string }>) => {
                      editData.personal_notes = e.detail.value;
                    }}
                  />
                </div>
              </div>
            </div>

            <!-- Platform Management Section -->
            <div class="pt-6 border-t border-gray-200">
              <h4 class="text-lg font-medium text-gray-900 mb-4">Platform Management</h4>
              
              <!-- Current Platforms -->
              <div class="mb-6">
                <h5 class="text-md font-medium text-gray-700 mb-3">Current Platforms</h5>
                {#if game && game.platforms && game.platforms.length > 0}
                  <div class="space-y-3">
                    {#each groupPlatformsByPlatform(game.platforms) as groupedPlatform}
                      <div class="bg-blue-50 border border-blue-200 rounded-lg p-3">
                        <div class="flex items-start justify-between">
                          <div class="flex-1">
                            <span class="text-sm font-semibold text-blue-900 mb-2 block">{groupedPlatform.platform.display_name}</span>
                            <div class="flex flex-wrap gap-2">
                              {#each groupedPlatform.storefronts as storefront}
                                <div class="inline-flex items-center space-x-2 px-2 py-1 bg-white border border-blue-300 rounded text-xs">
                                  <span class="text-blue-800 font-medium">
                                    {storefront.storefront?.display_name || 'Unknown Storefront'}
                                  </span>
                                  {#if storefront.store_url && storefront.storefront?.name !== 'physical'}
                                    <a 
                                      href={storefront.store_url} 
                                      target="_blank" 
                                      rel="noopener noreferrer"
                                      class="text-blue-600 hover:text-blue-800 flex-shrink-0"
                                      title="View in {storefront.storefront?.display_name || 'store'}"
                                      aria-label="View {groupedPlatform.platform.display_name} on {storefront.storefront?.display_name || 'store'}"
                                    >
                                      <svg class="w-3 h-3" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M10 6H6a2 2 0 00-2 2v10a2 2 0 002 2h10a2 2 0 002-2v-4M14 4h6m0 0v6m0-6L10 14"></path>
                                      </svg>
                                    </a>
                                  {/if}
                                  <button 
                                    type="button"
                                    on:click={() => confirmRemovePlatform(storefront.id, groupedPlatform.platform.display_name, storefront.storefront?.display_name || 'Unknown Storefront')}
                                    class="text-red-600 hover:text-red-800 flex-shrink-0 ml-1 disabled:opacity-50 disabled:cursor-not-allowed"
                                    title={game && game.platforms && game.platforms.length <= 1 ? "Cannot remove - game must have at least one platform" : "Remove this platform/storefront combination"}
                                    aria-label="Remove {groupedPlatform.platform.display_name} on {storefront.storefront?.display_name || 'store'}"
                                    disabled={game && game.platforms && game.platforms.length <= 1}
                                  >
                                    <svg class="w-3 h-3" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                      <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16"></path>
                                    </svg>
                                  </button>
                                </div>
                              {/each}
                            </div>
                          </div>
                        </div>
                      </div>
                    {/each}
                  </div>
                {:else}
                  <p class="text-sm text-gray-500 italic">No platforms added yet.</p>
                {/if}
              </div>

              <!-- Add New Platform -->
              <div class="border border-gray-200 rounded-lg p-4">
                <h5 class="text-md font-medium text-gray-700 mb-3">Add New Platform</h5>
                {#if isLoadingPlatforms}
                  <div class="flex items-center justify-center py-4">
                    <svg class="animate-spin h-6 w-6 text-gray-400" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                      <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15" />
                    </svg>
                    <span class="ml-2 text-sm text-gray-500">Loading platforms...</span>
                  </div>
                {:else if availablePlatforms.length === 0}
                  <p class="text-sm text-gray-500 italic">No platforms available to add.</p>
                {:else}
                  <div class="grid grid-cols-1 lg:grid-cols-2 gap-4">
                    <!-- Platform Selection -->
                    <div>
                      <label for="new_platform" class="form-label">Platform</label>
                      <select
                        id="new_platform"
                        bind:value={newPlatformData.platform_id}
                        class="form-input"
                        on:change={() => {
                          // Reset storefront when platform changes
                          newPlatformData.storefront_id = '';
                          newPlatformData.store_url = '';
                          newPlatformData.store_game_id = '';
                          
                          // Auto-select default storefront if available
                          const selectedPlatform = availablePlatforms.find(p => p.id === newPlatformData.platform_id);
                          if (selectedPlatform && selectedPlatform.default_storefront_id) {
                            newPlatformData.storefront_id = selectedPlatform.default_storefront_id;
                          }
                        }}
                      >
                        <option value="">Select a platform...</option>
                        {#each availablePlatforms as platform}
                          <option value={platform.id}>{platform.display_name}</option>
                        {/each}
                      </select>
                    </div>

                    <!-- Storefront Selection -->
                    <div>
                      <label for="new_storefront" class="form-label">Storefront</label>
                      <select
                        id="new_storefront"
                        bind:value={newPlatformData.storefront_id}
                        class="form-input"
                        disabled={!newPlatformData.platform_id}
                      >
                        <option value="">Select a storefront...</option>
                        {#each availableStorefronts as storefront}
                          <option value={storefront.id}>{storefront.display_name}</option>
                        {/each}
                      </select>
                    </div>

                    <!-- Store URL (Optional) -->
                    <div>
                      <label for="new_store_url" class="form-label">Store URL (Optional)</label>
                      <input
                        id="new_store_url"
                        type="url"
                        bind:value={newPlatformData.store_url}
                        class="form-input"
                        placeholder="https://store.example.com/game/..."
                      />
                    </div>

                    <!-- Store Game ID (Optional) -->
                    <div>
                      <label for="new_store_game_id" class="form-label">Store Game ID (Optional)</label>
                      <input
                        id="new_store_game_id"
                        type="text"
                        bind:value={newPlatformData.store_game_id}
                        class="form-input"
                        placeholder="Game ID in the store"
                      />
                    </div>
                  </div>

                  <!-- Add Platform Button -->
                  <div class="mt-4">
                    <button
                      type="button"
                      on:click={addPlatform}
                      disabled={!newPlatformData.platform_id || isAddingPlatform}
                      class="btn-secondary inline-flex items-center gap-x-2"
                    >
                      {#if isAddingPlatform}
                        <svg class="animate-spin h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15" />
                        </svg>
                        Adding...
                      {:else}
                        <svg class="-ml-0.5 h-4 w-4" viewBox="0 0 20 20" fill="currentColor">
                          <path d="M10.75 4.75a.75.75 0 00-1.5 0v4.5h-4.5a.75.75 0 000 1.5h4.5v4.5a.75.75 0 001.5 0v-4.5h4.5a.75.75 0 000-1.5h-4.5v-4.5z" />
                        </svg>
                        Add Platform
                      {/if}
                    </button>
                  </div>
                {/if}
              </div>
            </div>

            <div class="flex items-center justify-end space-x-3 pt-6 border-t border-gray-200">
              <button
                type="button"
                on:click={cancelEditing}
                class="btn-secondary"
              >
                Cancel
              </button>
              <button
                type="submit"
                class="btn-primary inline-flex items-center gap-x-2"
              >
                <svg class="-ml-0.5 h-5 w-5" viewBox="0 0 20 20" fill="currentColor">
                  <path fill-rule="evenodd" d="M16.704 4.153a.75.75 0 01.143 1.052l-8 10.5a.75.75 0 01-1.127.075l-4.5-4.5a.75.75 0 011.06-1.06l3.894 3.893 7.48-9.817a.75.75 0 011.05-.143z" clip-rule="evenodd" />
                </svg>
                Save Changes
              </button>
            </div>
          </form>
        {:else}
          <!-- Display Mode -->
          <div class="space-y-6">
            <div class="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-6">
              <div class="bg-gray-50 p-4 rounded-lg">
                <dt class="text-sm font-medium text-gray-500">Status</dt>
                <dd class="mt-1">
                  <span class="inline-flex items-center px-2.5 py-0.5 rounded-full text-xs font-medium status-{game.play_status.replace('_', '-')}">
                    {getStatusLabel(game.play_status)}
                  </span>
                </dd>
              </div>

              <div class="bg-gray-50 p-4 rounded-lg">
                <dt class="text-sm font-medium text-gray-500">Ownership</dt>
                <dd class="mt-1 text-sm text-gray-900 capitalize">
                  {game.ownership_status}
                  {#if game.is_physical}
                    <span class="inline-flex items-center px-2 py-0.5 rounded text-xs font-medium bg-blue-100 text-blue-800 ml-2">
                      Physical
                    </span>
                  {/if}
                </dd>
              </div>

              <div class="bg-gray-50 p-4 rounded-lg">
                <dt class="text-sm font-medium text-gray-500">Rating</dt>
                <dd class="mt-1">
                  {#if game.personal_rating}
                    <div class="flex items-center space-x-1">
                      <span class="text-yellow-400 text-lg">{renderStars(game.personal_rating)}</span>
                      <span class="text-sm font-medium text-gray-900">({game.personal_rating}/5)</span>
                    </div>
                  {:else}
                    <span class="text-sm text-gray-500">Not rated</span>
                  {/if}
                </dd>
              </div>

              <div class="bg-gray-50 p-4 rounded-lg">
                <dt class="text-sm font-medium text-gray-500">Hours Played</dt>
                <dd class="mt-1 text-sm font-medium text-gray-900">
                  {game.hours_played || 0}h
                </dd>
              </div>
            </div>

            {#if game.acquired_date}
              <div class="grid grid-cols-1 gap-6">

                {#if game.acquired_date}
                  <div class="bg-gray-50 p-4 rounded-lg">
                    <dt class="text-sm font-medium text-gray-500">Acquired</dt>
                    <dd class="mt-1 text-sm text-gray-900">
                      {new Date(game.acquired_date).toLocaleDateString()}
                    </dd>
                  </div>
                {/if}
              </div>
            {/if}

            {#if game.personal_notes}
              <div class="bg-gray-50 p-4 rounded-lg">
                <h4 class="text-sm font-medium text-gray-500 mb-2">Personal Notes</h4>
                <div class="prose prose-sm max-w-none text-gray-900">
                  <RichTextEditor
                    value={game.personal_notes}
                    editable={false}
                  />
                </div>
              </div>
            {/if}
          </div>
        {/if}
      </div>
    </div>
{/if}
</div>

<!-- Platform Removal Confirmation Dialog -->
{#if platformToRemove}
  <div class="fixed inset-0 bg-gray-600 bg-opacity-50 overflow-y-auto h-full w-full z-50">
    <div class="relative top-20 mx-auto p-5 border w-96 shadow-lg rounded-md bg-white">
      <div class="mt-3 text-center">
        <div class="mx-auto flex items-center justify-center h-12 w-12 rounded-full bg-red-100">
          <svg class="h-6 w-6 text-red-600" fill="none" stroke="currentColor" viewBox="0 0 24 24" xmlns="http://www.w3.org/2000/svg">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-2.5L13.732 4c-.77-.833-1.964-.833-2.732 0L3.732 16.5c-.77.833.192 2.5 1.732 2.5z"></path>
          </svg>
        </div>
        <h3 class="text-lg font-medium text-gray-900 mt-2">Remove Platform</h3>
        <div class="mt-2 px-7 py-3">
          <p class="text-sm text-gray-500">
            Are you sure you want to remove <strong>{platformToRemove.platformName}</strong> on <strong>{platformToRemove.storefrontName}</strong> from this game?
          </p>
          <p class="text-xs text-gray-400 mt-2">
            This action cannot be undone.
          </p>
        </div>
        <div class="flex gap-4 px-7 py-3">
          <button
            type="button"
            on:click={cancelRemovePlatform}
            class="btn-secondary flex-1"
            disabled={isRemovingPlatform}
          >
            Cancel
          </button>
          <button
            type="button"
            on:click={removePlatform}
            disabled={isRemovingPlatform}
            class="flex-1 bg-red-600 text-white px-4 py-2 rounded-md hover:bg-red-700 focus:ring-2 focus:ring-red-500 focus:ring-offset-2 transition-colors duration-200 disabled:opacity-50 disabled:cursor-not-allowed inline-flex items-center justify-center gap-x-2"
          >
            {#if isRemovingPlatform}
              <svg class="animate-spin h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15" />
              </svg>
              Removing...
            {:else}
              Remove
            {/if}
          </button>
        </div>
      </div>
    </div>
  </div>
{/if}
</RouteGuard>