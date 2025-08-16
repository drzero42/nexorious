<script lang="ts">
  import { createEventDispatcher } from 'svelte';
  import type { IGDBGameCandidate } from '$lib/stores/games.svelte';
  import { resolveImageUrl } from '$lib/utils/image-url';
  import PlatformSelector from './PlatformSelector.svelte';
  import StarRating from './StarRating.svelte';

  export let selectedGame: IGDBGameCandidate | null;
  export let gameData: any;
  export let addingGameId: string | null = null;
  export let selectedPlatforms: Set<string>;
  export let platformStorefronts: Map<string, Set<string>>;
  export let platformStoreUrls: Map<string, string>;

  const dispatch = createEventDispatcher<{
    'back': void;
    'edit-details': void;
    'confirm': void;
    'platform-toggle': { platformId: string };
    'storefront-toggle': { platformId: string; storefrontId: string };
    'store-url-change': { platformId: string; url: string };
  }>();

  function handleBack() {
    dispatch('back');
  }

  function handleEditDetails() {
    dispatch('edit-details');
  }

  function handleConfirm() {
    dispatch('confirm');
  }

  function handlePlatformToggle(event: CustomEvent<{ platformId: string }>) {
    dispatch('platform-toggle', event.detail);
  }

  function handleStorefrontToggle(event: CustomEvent<{ platformId: string; storefrontId: string }>) {
    dispatch('storefront-toggle', event.detail);
  }

  function handleStoreUrlChange(event: CustomEvent<{ platformId: string; url: string }>) {
    dispatch('store-url-change', event.detail);
  }
</script>

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
        
          <!-- Read-only Description -->
          {#if gameData.description}
            <div>
              <h4 class="text-sm font-medium text-gray-500 mb-2">Description</h4>
              <p class="text-sm text-gray-900 leading-relaxed">{gameData.description}</p>
            </div>
          {/if}
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
        <div class="mt-1">
          <StarRating
            id="metadata-personal-rating"
            bind:value={gameData.personal_rating}
            size="md"
            clearable={true}
            showLabel={true}
            onchange={(e) => {
              gameData.personal_rating = e.detail.value;
            }}
          />
        </div>
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

  <!-- Platform & Storefront Section -->
  <div class="card p-6">
    <PlatformSelector
      bind:selectedPlatforms
      bind:platformStorefronts
      bind:platformStoreUrls
      igdbPlatformNames={selectedGame?.platforms || []}
      on:platform-toggle={handlePlatformToggle}
      on:storefront-toggle={handleStorefrontToggle}
      on:store-url-change={handleStoreUrlChange}
    />
  </div>

  <!-- Actions -->
  <div class="card p-4 bg-gray-50">
    <div class="flex flex-col sm:flex-row justify-between gap-3">
      <button
        on:click={handleBack}
        class="btn-secondary inline-flex items-center justify-center gap-x-2 order-2 sm:order-1"
      >
        <svg class="h-5 w-5" viewBox="0 0 20 20" fill="currentColor">
          <path fill-rule="evenodd" d="M12.79 5.23a.75.75 0 01-.02 1.06L8.832 10l3.938 3.71a.75.75 0 11-1.04 1.08l-4.5-4.25a.75.75 0 010-1.08l4.5-4.25a.75.75 0 011.06.02z" clip-rule="evenodd" />
        </svg>
        Back to Selection
      </button>
      
      <div class="flex flex-col sm:flex-row gap-3 order-1 sm:order-2">
        <button
          on:click={handleEditDetails}
          class="btn-secondary inline-flex items-center justify-center gap-x-2"
        >
          <svg class="h-5 w-5" viewBox="0 0 20 20" fill="currentColor">
            <path fill-rule="evenodd" d="M13.5 4.938a7 7 0 11-9.006 1.737c.202-.257.59-.218.793.039.278.352.594.672.943.954.332.269.786-.049.773-.476a5.977 5.977 0 01.572-2.759 6.026 6.026 0 012.486-2.665c.247-.14.55-.016.677.238A6.967 6.967 0 0013.5 4.938zM14 12a4 4 0 01-4 4c-1.913 0-3.52-1.398-3.91-3.182-.093-.429.44-.643.814-.413a4.043 4.043 0 001.601.564c.303.038.531-.24.51-.544a5.975 5.975 0 011.315-4.192.447.447 0 01.431-.16A4.001 4.001 0 0114 12z" clip-rule="evenodd" />
          </svg>
          Edit Details
        </button>
        
        <button
          on:click={handleConfirm}
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