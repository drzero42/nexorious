<script lang="ts">
  import { page } from '$app/stores';
  import { auth, userGames } from '$lib/stores';
  import { onMount } from 'svelte';
  import { goto } from '$app/navigation';
  import { RouteGuard } from '$lib/components';

  let game = null;
  let isLoading = true;
  let isEditing = false;
  let editData = {};

  $: gameId = $page.params.id;

  onMount(async () => {
    // Load game details - authentication is handled by RouteGuard
    await loadGame();
  });

  async function loadGame() {
    try {
      isLoading = true;
      // In a real app, this would fetch the specific game
      // For now, find it in the loaded games
      const games = userGames.value.games;
      game = games.find(g => g.id === gameId);
      
      if (!game) {
        // Try to fetch from API
        await userGames.fetchUserGames();
        game = userGames.value.games.find(g => g.id === gameId);
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
    editData = {
      personal_rating: game.personal_rating,
      play_status: game.play_status,
      hours_played: game.hours_played,
      personal_notes: game.personal_notes,
      is_loved: game.is_loved,
      ownership_status: game.ownership_status,
      is_physical: game.is_physical
    };
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
      // Update game in backend
      await userGames.updateUserGame(gameId, editData);
      
      // Update local game object
      game = { ...game, ...editData };
      isEditing = false;
    } catch (error) {
      console.error('Failed to save changes:', error);
    }
  }

  async function deleteGame() {
    if (confirm('Are you sure you want to remove this game from your collection?')) {
      try {
        await userGames.deleteUserGame(gameId);
        goto('/games');
      } catch (error) {
        console.error('Failed to delete game:', error);
      }
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

  function renderStars(rating: number) {
    const stars = [];
    for (let i = 1; i <= 5; i++) {
      stars.push(i <= rating ? '★' : '☆');
    }
    return stars.join('');
  }
</script>

<svelte:head>
  <title>{game?.title || 'Game Details'} - Nexorious</title>
</svelte:head>

<RouteGuard requireAuth={true}>
{#if isLoading}
  <div class="text-center py-8">
    <div class="text-gray-500 dark:text-gray-400">Loading game details...</div>
  </div>
{:else if !game}
  <div class="text-center py-8">
    <div class="text-gray-500 dark:text-gray-400">Game not found</div>
    <button
      on:click={() => goto('/games')}
      class="mt-4 text-blue-600 hover:text-blue-700 dark:text-blue-400 dark:hover:text-blue-300"
    >
      Back to Games
    </button>
  </div>
{:else}
  <div class="max-w-4xl mx-auto">
    <!-- Header -->
    <div class="flex items-center justify-between mb-6">
      <button
        on:click={() => goto('/games')}
        class="text-blue-600 hover:text-blue-700 dark:text-blue-400 dark:hover:text-blue-300"
      >
        ← Back to Games
      </button>
      <div class="flex space-x-2">
        {#if !isEditing}
          <button
            on:click={startEditing}
            class="px-4 py-2 bg-blue-600 hover:bg-blue-700 text-white rounded-md transition-colors"
          >
            Edit
          </button>
        {/if}
        <button
          on:click={deleteGame}
          class="px-4 py-2 bg-red-600 hover:bg-red-700 text-white rounded-md transition-colors"
        >
          Remove
        </button>
      </div>
    </div>

    <div class="bg-white dark:bg-gray-800 rounded-lg shadow overflow-hidden">
      <div class="md:flex">
        <!-- Cover Art -->
        <div class="md:flex-shrink-0">
          <div class="h-48 w-full md:h-full md:w-48 bg-gray-200 dark:bg-gray-700">
            {#if game.cover_art_url}
              <img
                src={game.cover_art_url}
                alt={game.title}
                class="h-full w-full object-cover"
              />
            {:else}
              <div class="h-full w-full flex items-center justify-center text-gray-400">
                No Cover
              </div>
            {/if}
          </div>
        </div>

        <!-- Game Info -->
        <div class="p-6 flex-1">
          <div class="flex items-start justify-between">
            <div>
              <h1 class="text-2xl font-bold text-gray-900 dark:text-white mb-2">
                {game.title}
                {#if game.is_loved}
                  <span class="text-red-500 ml-2">❤️</span>
                {/if}
              </h1>
              <div class="text-sm text-gray-600 dark:text-gray-400 space-y-1">
                {#if game.developer}
                  <p><strong>Developer:</strong> {game.developer}</p>
                {/if}
                {#if game.publisher}
                  <p><strong>Publisher:</strong> {game.publisher}</p>
                {/if}
                {#if game.genre}
                  <p><strong>Genre:</strong> {game.genre}</p>
                {/if}
                {#if game.release_date}
                  <p><strong>Release Date:</strong> {new Date(game.release_date).toLocaleDateString()}</p>
                {/if}
              </div>
            </div>
          </div>

          {#if game.description}
            <div class="mt-4">
              <h3 class="text-lg font-semibold text-gray-900 dark:text-white mb-2">Description</h3>
              <p class="text-gray-600 dark:text-gray-400">{game.description}</p>
            </div>
          {/if}

          <!-- Personal Information -->
          <div class="mt-6 border-t border-gray-200 dark:border-gray-700 pt-6">
            <h3 class="text-lg font-semibold text-gray-900 dark:text-white mb-4">Your Information</h3>
            
            {#if isEditing}
              <!-- Edit Form -->
              <form on:submit|preventDefault={saveChanges} class="space-y-4">
                <div class="grid grid-cols-1 md:grid-cols-2 gap-4">
                  <div>
                    <label for="play_status" class="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">
                      Play Status
                    </label>
                    <select
                      id="play_status"
                      bind:value={editData.play_status}
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
                      bind:value={editData.ownership_status}
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
                      bind:value={editData.personal_rating}
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
                      bind:value={editData.hours_played}
                      class="w-full px-3 py-2 border border-gray-300 dark:border-gray-600 rounded-md shadow-sm focus:outline-none focus:ring-blue-500 focus:border-blue-500 dark:bg-gray-700 dark:text-white"
                    />
                  </div>
                </div>

                <div class="flex items-center space-x-4">
                  <label class="flex items-center">
                    <input
                      type="checkbox"
                      bind:checked={editData.is_physical}
                      class="rounded border-gray-300 dark:border-gray-600 text-blue-600 focus:ring-blue-500"
                    />
                    <span class="ml-2 text-sm text-gray-700 dark:text-gray-300">Physical copy</span>
                  </label>

                  <label class="flex items-center">
                    <input
                      type="checkbox"
                      bind:checked={editData.is_loved}
                      class="rounded border-gray-300 dark:border-gray-600 text-blue-600 focus:ring-blue-500"
                    />
                    <span class="ml-2 text-sm text-gray-700 dark:text-gray-300">Loved game</span>
                  </label>
                </div>

                <div>
                  <label for="personal_notes" class="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">
                    Personal Notes
                  </label>
                  <textarea
                    id="personal_notes"
                    bind:value={editData.personal_notes}
                    rows="3"
                    placeholder="Add your personal notes about this game..."
                    class="w-full px-3 py-2 border border-gray-300 dark:border-gray-600 rounded-md shadow-sm focus:outline-none focus:ring-blue-500 focus:border-blue-500 dark:bg-gray-700 dark:text-white"
                  ></textarea>
                </div>

                <div class="flex justify-end space-x-2 pt-4">
                  <button
                    type="button"
                    on:click={cancelEditing}
                    class="px-4 py-2 text-gray-700 dark:text-gray-300 hover:text-gray-900 dark:hover:text-white"
                  >
                    Cancel
                  </button>
                  <button
                    type="submit"
                    class="px-4 py-2 bg-blue-600 hover:bg-blue-700 text-white rounded-md transition-colors"
                  >
                    Save Changes
                  </button>
                </div>
              </form>
            {:else}
              <!-- Display Mode -->
              <div class="grid grid-cols-1 md:grid-cols-2 gap-6">
                <div class="space-y-3">
                  <div class="flex items-center justify-between">
                    <span class="text-sm font-medium text-gray-700 dark:text-gray-300">Status:</span>
                    <span class="inline-flex items-center px-2 py-1 rounded-full text-xs font-medium {getStatusColor(game.play_status)}">
                      {getStatusLabel(game.play_status)}
                    </span>
                  </div>

                  <div class="flex items-center justify-between">
                    <span class="text-sm font-medium text-gray-700 dark:text-gray-300">Ownership:</span>
                    <span class="text-sm text-gray-600 dark:text-gray-400 capitalize">
                      {game.ownership_status}
                      {#if game.is_physical}
                        (Physical)
                      {/if}
                    </span>
                  </div>

                  <div class="flex items-center justify-between">
                    <span class="text-sm font-medium text-gray-700 dark:text-gray-300">Rating:</span>
                    <span class="text-sm text-gray-600 dark:text-gray-400">
                      {#if game.personal_rating}
                        <span class="text-yellow-400">{renderStars(game.personal_rating)}</span>
                        ({game.personal_rating}/5)
                      {:else}
                        Not rated
                      {/if}
                    </span>
                  </div>

                  <div class="flex items-center justify-between">
                    <span class="text-sm font-medium text-gray-700 dark:text-gray-300">Hours Played:</span>
                    <span class="text-sm text-gray-600 dark:text-gray-400">
                      {game.hours_played || 0}h
                    </span>
                  </div>
                </div>

                <div class="space-y-3">
                  {#if game.last_played}
                    <div class="flex items-center justify-between">
                      <span class="text-sm font-medium text-gray-700 dark:text-gray-300">Last Played:</span>
                      <span class="text-sm text-gray-600 dark:text-gray-400">
                        {new Date(game.last_played).toLocaleDateString()}
                      </span>
                    </div>
                  {/if}

                  {#if game.acquired_date}
                    <div class="flex items-center justify-between">
                      <span class="text-sm font-medium text-gray-700 dark:text-gray-300">Acquired:</span>
                      <span class="text-sm text-gray-600 dark:text-gray-400">
                        {new Date(game.acquired_date).toLocaleDateString()}
                      </span>
                    </div>
                  {/if}
                </div>
              </div>

              {#if game.personal_notes}
                <div class="mt-4">
                  <h4 class="text-sm font-medium text-gray-700 dark:text-gray-300 mb-2">Personal Notes</h4>
                  <p class="text-sm text-gray-600 dark:text-gray-400 whitespace-pre-wrap">{game.personal_notes}</p>
                </div>
              {/if}
            {/if}
          </div>
        </div>
      </div>
    </div>
  </div>
{/if}
</RouteGuard>