<script lang="ts">
  import { games } from '$lib/stores';
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
      searchResults = response.candidates;
      
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

  async function selectGame(game: IGDBGameCandidate) {
    selectedGame = game;
    isSearching = true;
    
    try {
      // Import the game directly from IGDB with full metadata
      await games.createFromIGDB(game.igdb_id);
      
      // Redirect to the games page after successful import
      goto('/games');
    } catch (error) {
      console.error('Failed to import game:', error);
      
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
<div>
  <!-- Header -->
  <div>
    <div>
      <div>
        <h1>Add Game</h1>
        <p>Add a new game to your collection</p>
      </div>
      <button
        on:click={() => goto('/games')}
      >
        Cancel
      </button>
    </div>
  </div>

  <!-- Step 1: Search -->
  {#if step === 'search'}
    <div>
      <div>
        <div>
          <label for="search">
            Search for a game
          </label>
          <div>
            <input
              id="search"
              type="text"
              bind:value={searchQuery}
              on:keydown={handleKeydown}
              placeholder="Enter game title..."
            />
            <button
              on:click={handleSearch}
              disabled={isSearching || !searchQuery.trim()}
            >
              {isSearching ? 'Searching...' : 'Search'}
            </button>
          </div>
        </div>

        <div>
          <p>Search for games using the IGDB database. Selecting a game will automatically add it to your collection with full metadata.</p>
          <p>If you can't find your game, you can manually add it by clicking "Add Manually" below.</p>
        </div>

        {#if games.value.error}
          <div>
            <p>Search Error</p>
            <p>{games.value.error}</p>
          </div>
        {/if}

        <div>
          <button
            on:click={() => step = 'details'}
          >
            Add Manually Instead
          </button>
        </div>
      </div>
    </div>
  {/if}

  <!-- Step 2: Confirm Game Selection -->
  {#if step === 'confirm'}
    <div>
      <div>
        <h2>
          Select a game
        </h2>
        <p>
          Choose the correct game from the search results
        </p>
      </div>

      <div>
        {#if searchResults.length === 0}
          <div>
            <div>
              No games found
            </div>
            <p>
              Try a different search term or add the game manually
            </p>
          </div>
        {:else}
          {#each searchResults as game}
            <button
              on:click={() => selectGame(game)}
              disabled={isSearching}
            >
              <div>
                <div>
                  {#if game.cover_art_url}
                    <img
                      src={game.cover_art_url}
                      alt={game.title}
                      loading="lazy"
                    />
                  {:else}
                    <div>
                      No Cover
                    </div>
                  {/if}
                </div>
                <div>
                  <h3>
                    {game.title}
                  </h3>
                  
                  {#if game.platforms && game.platforms.length > 0}
                    <div>
                      {#each game.platforms as platform}
                        <span>
                          {platform}
                        </span>
                      {/each}
                    </div>
                  {/if}
                  
                  <p>
                    Released: {game.release_date ? new Date(game.release_date).getFullYear() : 'Unknown'}
                  </p>
                  
                  {#if game.howlongtobeat_main || game.howlongtobeat_extra || game.howlongtobeat_completionist}
                    <div>
                      <span>Time to beat:</span>
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
                    <p style="display: -webkit-box; -webkit-line-clamp: 3; -webkit-box-orient: vertical;">
                      {game.description}
                    </p>
                  {/if}
                  
                  {#if isSearching}
                    <div>
                      Adding to collection...
                    </div>
                  {/if}
                </div>
              </div>
            </button>
          {/each}
        {/if}
      </div>

      <div>
        <button
          on:click={goBack}
        >
          Back to Search
        </button>
        <button
          on:click={() => step = 'details'}
        >
          Add Manually Instead
        </button>
      </div>
    </div>
  {/if}

  <!-- Step 3: Game Details -->
  {#if step === 'details'}
    <div>
      <div>
        <h2>
          Game Details
        </h2>
        <p>
          {selectedGame ? 'Review and customize the game information' : 'Enter the game information manually'}
        </p>
      </div>

      <form on:submit|preventDefault={handleSubmit}>
        <!-- Basic Information -->
        <div>
          <div>
            <label for="title">
              Title *
            </label>
            <input
              id="title"
              type="text"
              bind:value={gameData.title}
              required
            />
          </div>

          <div>
            <label for="genre">
              Genre
            </label>
            <input
              id="genre"
              type="text"
              bind:value={gameData.genre}
            />
          </div>

          <div>
            <label for="developer">
              Developer
            </label>
            <input
              id="developer"
              type="text"
              bind:value={gameData.developer}
            />
          </div>

          <div>
            <label for="publisher">
              Publisher
            </label>
            <input
              id="publisher"
              type="text"
              bind:value={gameData.publisher}
            />
          </div>

          <div>
            <label for="release_date">
              Release Date
            </label>
            <input
              id="release_date"
              type="date"
              bind:value={gameData.release_date}
            />
          </div>

          <div>
            <label for="cover_art_url">
              Cover Art URL
            </label>
            <input
              id="cover_art_url"
              type="url"
              bind:value={gameData.cover_art_url}
            />
          </div>
        </div>

        <!-- Description -->
        <div>
          <label for="description">
            Description
          </label>
          <textarea
            id="description"
            bind:value={gameData.description}
            rows="3"
          ></textarea>
        </div>

        <!-- Personal Information -->
        <div>
          <h3>
            Personal Information
          </h3>

          <div>
            <div>
              <label for="play_status">
                Play Status
              </label>
              <select
                id="play_status"
                bind:value={gameData.play_status}
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
                bind:value={gameData.ownership_status}
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
                bind:value={gameData.personal_rating}
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
                bind:value={gameData.hours_played}
              />
            </div>
          </div>

          <div>
            <label>
              <input
                type="checkbox"
                bind:checked={gameData.is_physical}
              />
              <span>Physical copy</span>
            </label>

            <label>
              <input
                type="checkbox"
                bind:checked={gameData.is_loved}
              />
              <span>Loved game</span>
            </label>
          </div>
        </div>

        <!-- Personal Notes -->
        <div>
          <label for="personal_notes">
            Personal Notes
          </label>
          <textarea
            id="personal_notes"
            bind:value={gameData.personal_notes}
            rows="3"
            placeholder="Add your personal notes about this game..."
          ></textarea>
        </div>

        <!-- Actions -->
        <div>
          <button
            type="button"
            on:click={goBack}
          >
            {selectedGame ? 'Back to Selection' : 'Back to Search'}
          </button>
          <button
            type="submit"
          >
            Add Game
          </button>
        </div>
      </form>
    </div>
  {/if}
</div>
</RouteGuard>