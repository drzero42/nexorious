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
{#if isLoading}
  <div>
    <div>Loading game details...</div>
  </div>
{:else if !game}
  <div>
    <div>Game not found</div>
    <button
      on:click={() => goto('/games')}
    >
      Back to Games
    </button>
  </div>
{:else}
  <div>
    <!-- Header -->
    <div>
      <button
        on:click={() => goto('/games')}
      >
        ← Back to Games
      </button>
      <div>
        {#if !isEditing}
          <button
            on:click={startEditing}
          >
            Edit
          </button>
        {/if}
        <button
          on:click={deleteGame}
        >
          Remove
        </button>
      </div>
    </div>

    <div>
      <div>
        <!-- Cover Art -->
        <div>
          <div>
            {#if game.game.cover_art_url}
              <img
                src={resolveImageUrl(game.game.cover_art_url)}
                alt={game.game.title}
              />
            {:else}
              <div>
                No Cover
              </div>
            {/if}
          </div>
        </div>

        <!-- Game Info -->
        <div>
          <div>
            <div>
              <h1>
                {game.game.title}
                {#if game.is_loved}
                  <span>❤️</span>
                {/if}
              </h1>
              <div>
                {#if game.game.developer}
                  <p><strong>Developer:</strong> {game.game.developer}</p>
                {/if}
                {#if game.game.publisher}
                  <p><strong>Publisher:</strong> {game.game.publisher}</p>
                {/if}
                {#if game.game.genre}
                  <p><strong>Genre:</strong> {game.game.genre}</p>
                {/if}
                {#if game.game.release_date}
                  <p><strong>Release Date:</strong> {new Date(game.game.release_date).toLocaleDateString()}</p>
                {/if}
              </div>
            </div>
          </div>

          {#if game.game.description}
            <div>
              <h3>Description</h3>
              <p>{game.game.description}</p>
            </div>
          {/if}

          <!-- Personal Information -->
          <div>
            <h3>Your Information</h3>
            
            {#if isEditing}
              <!-- Edit Form -->
              <form on:submit|preventDefault={saveChanges}>
                <div>
                  <div>
                    <label for="play_status">
                      Play Status
                    </label>
                    <select
                      id="play_status"
                      bind:value={editData.play_status}
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
                    <label for="ownership_status">
                      Ownership Status
                    </label>
                    <select
                      id="ownership_status"
                      bind:value={editData.ownership_status}
                    >
                      <option value="owned">Owned</option>
                      <option value="borrowed">Borrowed</option>
                      <option value="rented">Rented</option>
                      <option value="subscription">Subscription</option>
                    </select>
                  </div>

                  <div>
                    <label for="personal_rating">
                      Personal Rating
                    </label>
                    <select
                      id="personal_rating"
                      bind:value={editData.personal_rating}
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
                    <label for="hours_played">
                      Hours Played
                    </label>
                    <input
                      id="hours_played"
                      type="number"
                      min="0"
                      bind:value={editData.hours_played}
                    />
                  </div>
                </div>

                <div>
                  <label>
                    <input
                      type="checkbox"
                      bind:checked={editData.is_physical}
                    />
                    <span>Physical copy</span>
                  </label>

                  <label>
                    <input
                      type="checkbox"
                      bind:checked={editData.is_loved}
                    />
                    <span>Loved game</span>
                  </label>
                </div>

                <div>
                  <label for="personal_notes">
                    Personal Notes
                  </label>
                  <textarea
                    id="personal_notes"
                    bind:value={editData.personal_notes}
                    rows="3"
                    placeholder="Add your personal notes about this game..."
                  ></textarea>
                </div>

                <div>
                  <button
                    type="button"
                    on:click={cancelEditing}
                  >
                    Cancel
                  </button>
                  <button
                    type="submit"
                  >
                    Save Changes
                  </button>
                </div>
              </form>
            {:else}
              <!-- Display Mode -->
              <div>
                <div>
                  <div>
                    <span>Status:</span>
                    <span>
                      {getStatusLabel(game.play_status)}
                    </span>
                  </div>

                  <div>
                    <span>Ownership:</span>
                    <span>
                      {game.ownership_status}
                      {#if game.is_physical}
                        (Physical)
                      {/if}
                    </span>
                  </div>

                  <div>
                    <span>Rating:</span>
                    <span>
                      {#if game.personal_rating}
                        <span>{renderStars(game.personal_rating)}</span>
                        ({game.personal_rating}/5)
                      {:else}
                        Not rated
                      {/if}
                    </span>
                  </div>

                  <div>
                    <span>Hours Played:</span>
                    <span>
                      {game.hours_played || 0}h
                    </span>
                  </div>
                </div>

                <div>
                  {#if game.last_played}
                    <div>
                      <span>Last Played:</span>
                      <span>
                        {new Date(game.last_played).toLocaleDateString()}
                      </span>
                    </div>
                  {/if}

                  {#if game.acquired_date}
                    <div>
                      <span>Acquired:</span>
                      <span>
                        {new Date(game.acquired_date).toLocaleDateString()}
                      </span>
                    </div>
                  {/if}
                </div>
              </div>

              {#if game.personal_notes}
                <div>
                  <h4>Personal Notes</h4>
                  <p>{game.personal_notes}</p>
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