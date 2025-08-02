<script lang="ts">
  import { createEventDispatcher } from 'svelte';
  import type { IGDBGameCandidate } from '$lib/stores/games.svelte';
  import PlatformBadges from './PlatformBadges.svelte';
  import { resolveImageUrl } from '$lib/utils/image-url';

  export let searchResults: IGDBGameCandidate[] = [];
  export let addingGameId: string | null = null;
  export let isGameOwned: (igdbId: string) => boolean;
  export let getOwnedPlatformDetailsForGame: (igdbId: string) => any[];

  const dispatch = createEventDispatcher<{
    'back': void;
    'manual-add': void;
    'game-click': { game: IGDBGameCandidate; owned: boolean };
  }>();

  function handleBack() {
    dispatch('back');
  }

  function handleManualAdd() {
    dispatch('manual-add');
  }

  function handleGameClick(game: IGDBGameCandidate, owned: boolean) {
    dispatch('game-click', { game, owned });
  }
</script>

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
          {@const ownedPlatformDetails = getOwnedPlatformDetailsForGame(game.igdb_id)}
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
                        {#if ownedPlatformDetails.length > 0}
                          <div class="mt-1">
                            <PlatformBadges 
                              platforms={ownedPlatformDetails} 
                              compact={true}
                              maxVisible={2}
                              showDetailedTooltips={true}
                              enableHover={true}
                            />
                          </div>
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
      on:click={handleBack}
      class="btn-secondary inline-flex items-center gap-x-2"
    >
      <svg class="h-5 w-5" viewBox="0 0 20 20" fill="currentColor">
        <path fill-rule="evenodd" d="M12.79 5.23a.75.75 0 01-.02 1.06L8.832 10l3.938 3.71a.75.75 0 11-1.04 1.08l-4.5-4.25a.75.75 0 010-1.08l4.5-4.25a.75.75 0 011.06.02z" clip-rule="evenodd" />
      </svg>
      Back to Search
    </button>
    <button
      on:click={handleManualAdd}
      class="btn-secondary inline-flex items-center gap-x-2"
    >
      <svg class="h-5 w-5" viewBox="0 0 20 20" fill="currentColor">
        <path fill-rule="evenodd" d="M13.5 4.938a7 7 0 11-9.006 1.737c.202-.257.59-.218.793.039.278.352.594.672.943.954.332.269.786-.049.773-.476a5.977 5.977 0 01.572-2.759 6.026 6.026 0 012.486-2.665c.247-.14.55-.016.677.238A6.967 6.967 0 0013.5 4.938zM14 12a4 4 0 01-4 4c-1.913 0-3.52-1.398-3.91-3.182-.093-.429.44-.643.814-.413a4.043 4.043 0 001.601.564c.303.038.531-.24.51-.544a5.975 5.975 0 011.315-4.192.447.447 0 01.431-.16A4.001 4.001 0 0114 12z" clip-rule="evenodd" />
      </svg>
      Add Manually
    </button>
  </div>
</div>