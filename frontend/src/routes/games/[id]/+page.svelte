<script lang="ts">
  import { page } from '$app/stores';
  import { userGames } from '$lib/stores';
  import { games } from '$lib/stores/games.svelte';
  import { platforms } from '$lib/stores/platforms.svelte';
  import { notifications } from '$lib/stores/notifications.svelte';
  import { onMount } from 'svelte';
  import { goto } from '$app/navigation';
  import { RouteGuard, PlayStatusDropdown, TimeTrackingInput, RichTextEditor, GameProgressCard, PlatformBadges, PlatformSelector } from '$lib/components';
  import { resolveImageUrl } from '$lib/utils/image-url';
  import { formatOwnershipStatus, formatIgdbRating } from '$lib/utils/format-utils';
  import { groupPlatformsByPlatform } from '$lib/utils/platform-utils';
  import { OwnershipStatus } from '$lib/stores/user-games.svelte';
  import type { UserGame, PlayStatus, UserGameUpdateRequest, ProgressUpdateRequest, UserGamePlatformCreateRequest } from '$lib/stores/user-games.svelte';
  import { auth } from '$lib/stores/auth.svelte';

  let game: UserGame | null = null;
  let isLoading = true;
  let isEditing = false;
  let isUpdatingFromIGDB = false;
  let editData: {
    // Personal data
    personal_rating?: number | undefined;
    play_status: PlayStatus;
    hours_played: number;
    personal_notes?: string | undefined;
    is_loved: boolean;
    ownership_status: OwnershipStatus;
    is_physical: boolean;
  } = {
    play_status: 'not_started' as PlayStatus,
    hours_played: 0,
    is_loved: false,
    ownership_status: 'owned' as OwnershipStatus,
    is_physical: false
  };

  // PlatformSelector component data model
  let selectedPlatforms = new Set<string>();
  let platformStorefronts = new Map<string, Set<string>>();
  let platformStoreUrls = new Map<string, string>();
  let isAddingPlatform = false;
  let isRemovingPlatform = false;
  let platformToRemove: { platformAssociationId: string; platformName: string; storefrontName: string } | null = null;

  // IGDB platform data state  
  let igdbPlatformNames: string[] = [];

  // Platform management functions (copied from Add Game page for full functionality)
  function togglePlatform(platformId: string) {
    console.log('togglePlatform called with:', platformId, 'current selected:', selectedPlatforms);
    
    if (selectedPlatforms.has(platformId)) {
      console.log('Removing platform:', platformId);
      selectedPlatforms.delete(platformId);
      platformStorefronts.delete(platformId);
      platformStoreUrls.delete(platformId);
    } else {
      console.log('Adding platform:', platformId);
      selectedPlatforms.add(platformId);
      
      // Create storefronts set and auto-select default if available
      const storefronts = new Set<string>();
      const platform = $platforms.platforms.find(p => p.id === platformId);
      console.log('Found platform for auto-selection:', platform);
      
      if (platform?.default_storefront_id) {
        console.log('Auto-selecting default storefront:', platform.default_storefront_id);
        storefronts.add(platform.default_storefront_id);
      }
      
      platformStorefronts.set(platformId, storefronts);
    }
    
    // Trigger reactivity
    selectedPlatforms = new Set(selectedPlatforms);
    platformStorefronts = new Map(platformStorefronts);
    platformStoreUrls = new Map(platformStoreUrls);
    
    console.log('Platform toggle complete. Selected platforms:', selectedPlatforms, 'storefronts:', platformStorefronts);
  }

  function toggleStorefrontForPlatform(platformId: string, storefrontId: string) {
    console.log('toggleStorefrontForPlatform called:', { platformId, storefrontId });
    
    const storefronts = platformStorefronts.get(platformId) || new Set<string>();
    if (storefronts.has(storefrontId)) {
      console.log('Removing storefront:', storefrontId);
      storefronts.delete(storefrontId);
    } else {
      console.log('Adding storefront:', storefrontId);
      storefronts.add(storefrontId);
    }
    
    platformStorefronts.set(platformId, storefronts);
    platformStorefronts = new Map(platformStorefronts); // Trigger reactivity
    
    console.log('Updated storefronts for platform', platformId, ':', storefronts);
  }

  function setStoreUrlForPlatform(platformId: string, url: string) {
    console.log('setStoreUrlForPlatform called:', { platformId, url });
    
    if (url.trim()) {
      platformStoreUrls.set(platformId, url);
    } else {
      platformStoreUrls.delete(platformId);
    }
    platformStoreUrls = new Map(platformStoreUrls); // Trigger reactivity
    
    console.log('Updated store URLs:', platformStoreUrls);
  }

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
        // Load IGDB platform data for filtering from stored data
        loadIGDBPlatformData();
      }
    } catch (error) {
      console.error('Failed to load game:', error);
    } finally {
      isLoading = false;
    }
  }

  function loadIGDBPlatformData() {
    if (!game || !game.game.igdb_platform_names) {
      // No IGDB platform data available, reset platform filtering to show all platforms
      igdbPlatformNames = [];
      return;
    }

    try {
      // Parse stored platform names from the database
      igdbPlatformNames = JSON.parse(game.game.igdb_platform_names);
    } catch (error) {
      console.error('Failed to parse IGDB platform data:', error);
      // Fall back to showing all platforms
      igdbPlatformNames = [];
    }
  }

  async function loadPlatforms() {
    // The PlatformSelector component handles platform loading internally
    // through the platforms store, so we just need to ensure platforms are loaded
    try {
      await platforms.fetchAll();
    } catch (error) {
      console.error('Failed to load platforms:', error);
    }
  }

  async function addPlatforms() {
    console.log('addPlatforms called with selected platforms:', selectedPlatforms);
    console.log('Platform storefronts:', platformStorefronts);
    console.log('Platform store URLs:', platformStoreUrls);
    
    if (selectedPlatforms.size === 0 || !game) {
      console.log('Missing required data - selectedPlatforms:', selectedPlatforms.size, 'game:', !!game);
      return;
    }

    try {
      console.log('Starting platform addition...');
      isAddingPlatform = true;
      
      // Check if adding platform to no-longer-owned game
      const wasNoLongerOwned = game.ownership_status === 'no_longer_owned';

      let totalAddedPlatforms = 0;
      let totalAddedStorefronts = 0;

      // Process each selected platform
      for (const platformId of selectedPlatforms) {
        const selectedStorefronts = platformStorefronts.get(platformId) || new Set<string>();
        const storeUrl = platformStoreUrls.get(platformId) || '';
        
        console.log(`Processing platform ${platformId} with ${selectedStorefronts.size} storefronts`);

        // If no storefronts selected, add platform without storefront
        if (selectedStorefronts.size === 0) {
          const platformData: UserGamePlatformCreateRequest = {
            platform_id: platformId
          };

          // Only include optional fields if they have values
          if (storeUrl.trim()) {
            platformData.store_url = storeUrl.trim();
          }

          console.log('Sending platform data to API (no storefront):', platformData);
          await userGames.addPlatformToUserGame(game.id, platformData);
          totalAddedPlatforms++;
        } else {
          // Add platform-storefront combinations for each selected storefront
          for (const storefrontId of selectedStorefronts) {
            const platformData: UserGamePlatformCreateRequest = {
              platform_id: platformId,
              storefront_id: storefrontId
            };

            // Only include optional fields if they have values
            if (storeUrl.trim()) {
              platformData.store_url = storeUrl.trim();
            }

            console.log('Sending platform data to API:', platformData);
            await userGames.addPlatformToUserGame(game.id, platformData);
            totalAddedStorefronts++;
          }
          totalAddedPlatforms++;
        }
      }
      
      console.log('API calls successful, reloading game data...');
      
      // Immediately update dropdown if adding to no-longer-owned game
      if (wasNoLongerOwned && editData.ownership_status === OwnershipStatus.NO_LONGER_OWNED) {
        editData.ownership_status = OwnershipStatus.OWNED;
      }
      
      // Reload the game to get updated platform data
      await loadGame();
      
      // Clear all selections
      selectedPlatforms.clear();
      platformStorefronts.clear();
      platformStoreUrls.clear();
      selectedPlatforms = new Set(selectedPlatforms);
      platformStorefronts = new Map(platformStorefronts);
      platformStoreUrls = new Map(platformStoreUrls);

      console.log('Platform(s) added successfully');
      
      // Show success message
      if (totalAddedPlatforms === 1) {
        if (totalAddedStorefronts > 1) {
          notifications.showSuccess(`Platform added with ${totalAddedStorefronts} storefronts successfully`);
        } else {
          notifications.showSuccess(`Platform added successfully`);
        }
      } else {
        notifications.showSuccess(`${totalAddedPlatforms} platforms added successfully`);
      }
      
    } catch (error) {
      console.error('Failed to add platforms:', error);
      // Show error message
      notifications.showError('Failed to add platforms. Please try again.');
    } finally {
      isAddingPlatform = false;
    }
  }

  function confirmRemovePlatform(platformAssociationId: string, platformName: string, storefrontName: string) {
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
      
      // Check if this is the last platform before removal
      const isLastPlatform = game.platforms && game.platforms.length <= 1;
      
      // Call the API to remove the platform
      await userGames.removePlatformFromUserGame(game.id, platformToRemove.platformAssociationId);
      
      // Immediately update dropdown if this was the last platform
      if (isLastPlatform && game.ownership_status === OwnershipStatus.OWNED) {
        editData.ownership_status = OwnershipStatus.NO_LONGER_OWNED;
      }
      
      // Force fresh data fetch from server, then reload game with updated data
      await userGames.fetchUserGames(); // Force refresh from server
      await loadGame(); // Now uses fresh data
      
      // Clear the confirmation dialog
      platformToRemove = null;

      // Show appropriate success message
      if (isLastPlatform) {
        notifications.showSuccess('Platform removed successfully. Ownership status automatically changed to "No Longer Owned".');
      } else {
        notifications.showSuccess('Platform removed successfully');
      }
      
    } catch (error) {
      console.error('Failed to remove platform:', error);
      // Show error message
      notifications.showError('Failed to remove platform. Please try again.');
    } finally {
      isRemovingPlatform = false;
    }
  }

  // PlatformSelector event handlers (fixed to actually handle the events!)
  function handlePlatformToggle(event: CustomEvent<{ platformId: string }>) {
    console.log('handlePlatformToggle event received:', event.detail);
    togglePlatform(event.detail.platformId);
  }

  function handleStorefrontToggle(event: CustomEvent<{ platformId: string; storefrontId: string }>) {
    console.log('handleStorefrontToggle event received:', event.detail);
    toggleStorefrontForPlatform(event.detail.platformId, event.detail.storefrontId);
  }

  function handleStoreUrlChange(event: CustomEvent<{ platformId: string; url: string }>) {
    console.log('handleStoreUrlChange event received:', event.detail);
    setStoreUrlForPlatform(event.detail.platformId, event.detail.url);
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
        is_physical: game.is_physical
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
      // Split editData into user game update and progress update
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

      // Note: Game metadata (title, description, etc.) is managed by IGDB and cannot be edited directly.
      // Use "Update from IGDB" to refresh metadata.
      
      // Always update user-specific data (personal information)
      await userGames.updateUserGame(gameId, userGameUpdate);
      await userGames.updateProgress(gameId, progressUpdate);
      
      // Reload the game to get updated data
      await loadGame();
      isEditing = false;
      
      // Show success message
      notifications.showSuccess('Game information updated successfully.');
    } catch (error) {
      console.error('Failed to save changes:', error);
      notifications.showError('Failed to save changes. Please try again.');
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

  async function updateFromIGDB() {
    if (!game?.game.igdb_id) {
      notifications.showError('This game does not have an IGDB ID and cannot be updated.');
      return;
    }

    try {
      isUpdatingFromIGDB = true;
      
      // Call the games store refresh metadata function
      const result = await games.refreshMetadata(game.game.id);
      
      // Reload the game to get updated data
      await loadGame();
      
      // Show success message with updated fields info
      if (result && result.updated_fields && result.updated_fields.length > 0) {
        const fieldList = result.updated_fields.join(', ');
        notifications.showSuccess(`Game updated from IGDB! Updated fields: ${fieldList}`);
      } else {
        notifications.showSuccess('Game checked against IGDB - no updates needed.');
      }
      
    } catch (error) {
      console.error('Failed to update from IGDB:', error);
      notifications.showError('Failed to update game from IGDB. Please try again.');
    } finally {
      isUpdatingFromIGDB = false;
    }
  }

  // Helper function to check if user can update the game
  function canUpdateFromIGDB() {
    const currentUser = auth.value;
    return game?.game.igdb_id && currentUser.user?.isAdmin;
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
                <div class="flex-1">
                  <div class="flex items-center space-x-3">
                    <h1 class="text-3xl font-bold text-gray-900">
                      {game.game.title}
                    </h1>
                  </div>
                </div>
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
                <h3 class="text-lg font-medium text-gray-900 mb-3">Available On</h3>
                <div class="space-y-4">
                  <!-- Enhanced Platform Badges with detailed view -->
                  <div class="bg-gray-50 rounded-lg p-4 border border-gray-200">
                    <PlatformBadges 
                      platforms={game.platforms} 
                      compact={false} 
                      maxVisible={10} 
                      showDetails={true}
                      enableHover={true}
                    />
                  </div>
                  
                  <!-- Store Links Section -->
                  <div class="grid gap-3 sm:grid-cols-2 lg:grid-cols-1">
                    {#each groupPlatformsByPlatform(game.platforms) as groupedPlatform}
                      {#if groupedPlatform.storefronts.some(sf => sf.store_url && sf.storefront?.name !== 'physical')}
                        <div class="bg-white border border-gray-200 rounded-lg p-3 shadow-sm">
                          <h4 class="text-sm font-semibold text-gray-700 mb-2 flex items-center gap-2">
                            {#if groupedPlatform.platform.name?.toLowerCase().includes('playstation')}
                              <span role="img" aria-hidden="true">🎮</span>
                            {:else if groupedPlatform.platform.name?.toLowerCase().includes('xbox')}
                              <span role="img" aria-hidden="true">🎮</span>
                            {:else if groupedPlatform.platform.name?.toLowerCase().includes('nintendo')}
                              <span role="img" aria-hidden="true">🎮</span>
                            {:else if groupedPlatform.platform.name?.toLowerCase().includes('pc')}
                              <span role="img" aria-hidden="true">💻</span>
                            {:else if groupedPlatform.platform.name?.toLowerCase().includes('ios')}
                              <span role="img" aria-hidden="true">📱</span>
                            {:else if groupedPlatform.platform.name?.toLowerCase().includes('android')}
                              <span role="img" aria-hidden="true">📱</span>
                            {:else}
                              <span role="img" aria-hidden="true">🎯</span>
                            {/if}
                            {groupedPlatform.platform.display_name} Links
                          </h4>
                          <div class="flex flex-wrap gap-2">
                            {#each groupedPlatform.storefronts as storefront}
                              {#if storefront.store_url && storefront.storefront?.name !== 'physical'}
                                <a 
                                  href={storefront.store_url} 
                                  target="_blank" 
                                  rel="noopener noreferrer"
                                  class="inline-flex items-center gap-2 px-3 py-2 bg-blue-50 text-blue-700 rounded-md border border-blue-200 hover:bg-blue-100 hover:text-blue-800 transition-colors duration-200 text-sm font-medium"
                                  title="View in {storefront.storefront?.display_name || 'store'}"
                                  aria-label="View {groupedPlatform.platform.display_name} on {storefront.storefront?.display_name || 'store'}"
                                >
                                  <span role="img" aria-hidden="true">
                                    {#if storefront.storefront?.name?.toLowerCase().includes('steam')}🔥
                                    {:else if storefront.storefront?.name?.toLowerCase().includes('epic')}🎮
                                    {:else if storefront.storefront?.name?.toLowerCase().includes('gog')}🏪
                                    {:else if storefront.storefront?.name?.toLowerCase().includes('playstation')}🎮
                                    {:else if storefront.storefront?.name?.toLowerCase().includes('microsoft') || storefront.storefront?.name?.toLowerCase().includes('xbox')}🎮
                                    {:else if storefront.storefront?.name?.toLowerCase().includes('nintendo')}🎮
                                    {:else if storefront.storefront?.name?.toLowerCase().includes('app store') || storefront.storefront?.name?.toLowerCase().includes('apple')}📱
                                    {:else if storefront.storefront?.name?.toLowerCase().includes('google play')}🤖
                                    {:else if storefront.storefront?.name?.toLowerCase().includes('humble')}🎁
                                    {:else if storefront.storefront?.name?.toLowerCase().includes('itch')}🕹️
                                    {:else if storefront.storefront?.name?.toLowerCase().includes('origin') || storefront.storefront?.name?.toLowerCase().includes('ea')}🎮
                                    {:else}🏪
                                    {/if}
                                  </span>
                                  {storefront.storefront?.display_name || 'Unknown Store'}
                                  <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                    <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M10 6H6a2 2 0 00-2 2v10a2 2 0 002 2h10a2 2 0 002-2v-4M14 4h6m0 0v6m0-6L10 14"></path>
                                  </svg>
                                </a>
                              {/if}
                            {/each}
                          </div>
                        </div>
                      {/if}
                    {/each}
                  </div>
                </div>
              </div>
            {/if}

            <!-- IGDB Rating -->
            {#if game.game.rating_average}
              <div>
                <h3 class="text-lg font-medium text-gray-900">Game Rating</h3>
                <div class="mt-2 flex items-center space-x-4">
                  {#if game.game.rating_average}
                    <div class="flex items-center space-x-2">
                      <div class="flex items-center">
                        <span class="text-yellow-400 text-lg">★</span>
                        <span class="ml-1 text-sm font-medium text-gray-900">
                          {formatIgdbRating(game.game.rating_average) || 'N/A'}/10
                        </span>
                      </div>
                      {#if game.game.rating_count > 0}
                        <span class="text-xs text-gray-500">({game.game.rating_count.toLocaleString()} reviews)</span>
                      {/if}
                    </div>
                  {/if}
                  {#if canUpdateFromIGDB()}
                    <button
                      on:click={updateFromIGDB}
                      disabled={isUpdatingFromIGDB}
                      class="inline-flex items-center gap-x-2 px-3 py-2 text-sm font-medium text-blue-700 bg-blue-50 border border-blue-200 rounded-md hover:bg-blue-100 hover:text-blue-800 focus:ring-2 focus:ring-blue-500 focus:ring-offset-2 transition-colors duration-200 disabled:opacity-50 disabled:cursor-not-allowed"
                      title="Update game metadata from IGDB"
                      aria-label="Update {game.game.title} metadata from IGDB"
                    >
                      {#if isUpdatingFromIGDB}
                        <svg class="animate-spin h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15" />
                        </svg>
                        Updating...
                      {:else}
                        <svg class="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15" />
                        </svg>
                        Update from IGDB
                      {/if}
                    </button>
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
                    <option value="no_longer_owned">No Longer Owned</option>
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
                  <div class="bg-white border border-gray-200 rounded-lg p-4 mb-4">
                    <PlatformBadges 
                      platforms={game.platforms} 
                      compact={false} 
                      maxVisible={10} 
                      showDetails={true}
                      enableHover={false}
                    />
                  </div>
                  
                  <!-- Platform Management with Remove Buttons -->
                  <div class="space-y-3">
                    {#each groupPlatformsByPlatform(game.platforms) as groupedPlatform}
                      <div class="bg-red-50 border border-red-200 rounded-lg p-3">
                        <div class="flex items-start justify-between">
                          <div class="flex-1">
                            <span class="text-sm font-semibold text-red-900 mb-2 block flex items-center gap-2">
                              <span role="img" aria-hidden="true">
                                {#if groupedPlatform.platform.name?.toLowerCase().includes('playstation')}🎮
                                {:else if groupedPlatform.platform.name?.toLowerCase().includes('xbox')}🎮
                                {:else if groupedPlatform.platform.name?.toLowerCase().includes('nintendo')}🎮
                                {:else if groupedPlatform.platform.name?.toLowerCase().includes('pc')}💻
                                {:else if groupedPlatform.platform.name?.toLowerCase().includes('ios')}📱
                                {:else if groupedPlatform.platform.name?.toLowerCase().includes('android')}📱
                                {:else}🎯
                                {/if}
                              </span>
                              {groupedPlatform.platform.display_name} - Management
                            </span>
                            <div class="space-y-2">
                              {#each groupedPlatform.storefronts as storefront}
                                <div class="flex items-center justify-between bg-white border border-red-300 rounded-md p-2">
                                  <div class="flex items-center space-x-2">
                                    <span role="img" aria-hidden="true" class="text-sm">
                                      {#if storefront.storefront?.name?.toLowerCase().includes('steam')}🔥
                                      {:else if storefront.storefront?.name?.toLowerCase().includes('epic')}🎮
                                      {:else if storefront.storefront?.name?.toLowerCase().includes('gog')}🏪
                                      {:else if storefront.storefront?.name?.toLowerCase().includes('playstation')}🎮
                                      {:else if storefront.storefront?.name?.toLowerCase().includes('microsoft') || storefront.storefront?.name?.toLowerCase().includes('xbox')}🎮
                                      {:else if storefront.storefront?.name?.toLowerCase().includes('nintendo')}🎮
                                      {:else if storefront.storefront?.name?.toLowerCase().includes('app store') || storefront.storefront?.name?.toLowerCase().includes('apple')}📱
                                      {:else if storefront.storefront?.name?.toLowerCase().includes('google play')}🤖
                                      {:else if storefront.storefront?.name?.toLowerCase().includes('physical')}📦
                                      {:else if storefront.storefront?.name?.toLowerCase().includes('humble')}🎁
                                      {:else if storefront.storefront?.name?.toLowerCase().includes('itch')}🕹️
                                      {:else if storefront.storefront?.name?.toLowerCase().includes('origin') || storefront.storefront?.name?.toLowerCase().includes('ea')}🎮
                                      {:else}🏪
                                      {/if}
                                    </span>
                                    <span class="text-red-800 font-medium text-sm">
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
                                  <button 
                                    type="button"
                                    on:click={() => confirmRemovePlatform(storefront.id, groupedPlatform.platform.display_name, storefront.storefront?.display_name || 'Unknown Storefront')}
                                    class="inline-flex items-center gap-1 px-2 py-1 text-xs font-medium text-red-700 bg-red-100 border border-red-300 rounded hover:bg-red-200 hover:text-red-800 transition-colors duration-200"
                                    title={game && game.platforms && game.platforms.length <= 1 ? "Remove this platform/storefront combination (ownership will become 'No Longer Owned')" : "Remove this platform/storefront combination"}
                                    aria-label="Remove {groupedPlatform.platform.display_name} on {storefront.storefront?.display_name || 'store'}"
                                  >
                                    <svg class="w-3 h-3" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                      <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16"></path>
                                    </svg>
                                    Remove
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
                  <div class="bg-gray-50 border border-gray-200 rounded-lg p-4">
                    <p class="text-sm text-gray-500 italic flex items-center gap-2">
                      <span role="img" aria-hidden="true">❌</span>
                      No platforms added yet.
                    </p>
                  </div>
                {/if}
              </div>

              <!-- Add New Platform -->
              <div class="border border-gray-200 rounded-lg p-4">
                <h5 class="text-md font-medium text-gray-700 mb-3">Add New Platform</h5>
                
                <PlatformSelector
                  bind:selectedPlatforms
                  bind:platformStorefronts
                  bind:platformStoreUrls
                  igdbPlatformNames={igdbPlatformNames}
                  on:platform-toggle={handlePlatformToggle}
                  on:storefront-toggle={handleStorefrontToggle}
                  on:store-url-change={handleStoreUrlChange}
                />

                <!-- Add Platform Button -->
                <div class="mt-4">
                  <button
                    type="button"
                    on:click={addPlatforms}
                    disabled={selectedPlatforms.size === 0 || isAddingPlatform}
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
                      {selectedPlatforms.size > 1 ? `Add ${selectedPlatforms.size} Platforms` : 'Add Platform'}
                    {/if}
                  </button>
                </div>
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
                <dd class="mt-1 text-sm text-gray-900">
                  {formatOwnershipStatus(game.ownership_status)}
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