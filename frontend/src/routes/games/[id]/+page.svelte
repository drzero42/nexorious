<script lang="ts">
  import { page } from '$app/stores';
  import { userGames } from '$lib/stores';
  import { onMount } from 'svelte';
  import { goto } from '$app/navigation';
  import { RouteGuard } from '$lib/components';
  import { resolveImageUrl } from '$lib/utils/image-url';
  import type { UserGame, PlayStatus, OwnershipStatus, UserGameUpdateRequest, ProgressUpdateRequest } from '$lib/stores/user-games.svelte';

  let game: UserGame | null = null;
  let isLoading = true;
  let isEditing = false;
  let editData: {
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

  $: gameId = $page.params.id!;

  onMount(async () => {
    // Load game details - authentication is handled by RouteGuard
    await loadGame();
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

  function resetEditData() {
    if (game) {
      editData = {
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
      
      // Update game in backend
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
              <div class="grid grid-cols-1 sm:grid-cols-2 gap-4">
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
              </div>
            </div>

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
            
        {#if isEditing}
          <!-- Edit Form -->
          <form on:submit|preventDefault={saveChanges} class="space-y-6">
            <div class="grid grid-cols-1 sm:grid-cols-2 gap-6">
              <div>
                <label for="play_status" class="form-label">
                  Play Status
                </label>
                <select
                  id="play_status"
                  bind:value={editData.play_status}
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
                <input
                  id="hours_played"
                  type="number"
                  min="0"
                  bind:value={editData.hours_played}
                  class="form-input"
                />
              </div>
            </div>

            <div class="space-y-4">
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
                <textarea
                  id="personal_notes"
                  bind:value={editData.personal_notes}
                  rows="3"
                  placeholder="Add your personal notes about this game..."
                  class="form-input"
                ></textarea>
              </div>
            </div>

            <div class="flex items-center justify-end space-x-3 pt-4 border-t border-gray-200">
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

            {#if game.last_played || game.acquired_date}
              <div class="grid grid-cols-1 sm:grid-cols-2 gap-6">
                {#if game.last_played}
                  <div class="bg-gray-50 p-4 rounded-lg">
                    <dt class="text-sm font-medium text-gray-500">Last Played</dt>
                    <dd class="mt-1 text-sm text-gray-900">
                      {new Date(game.last_played).toLocaleDateString()}
                    </dd>
                  </div>
                {/if}

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
                <p class="text-sm text-gray-900 leading-relaxed">{game.personal_notes}</p>
              </div>
            {/if}
          </div>
        {/if}
      </div>
    </div>
{/if}
</div>
</RouteGuard>