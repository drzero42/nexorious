<script lang="ts">
  import { createEventDispatcher } from 'svelte';
  import { games } from '$lib/stores';

  export let searchQuery = '';
  export let isSearching = false;

  const dispatch = createEventDispatcher<{
    search: { query: string };
  }>();

  async function handleSearch() {
    if (!searchQuery.trim()) return;
    dispatch('search', { query: searchQuery });
  }

  function handleKeydown(event: KeyboardEvent) {
    if (event.key === 'Enter') {
      handleSearch();
    }
  }

</script>

<div class="card p-6">
  <div class="space-y-6">
    <div>
      <h2 class="text-lg font-semibold text-gray-900 mb-4">Search for a Game</h2>
      <div class="flex gap-3">
        <div class="flex-1 relative">
          <div class="pointer-events-none absolute inset-y-0 left-0 flex items-center pl-3">
            <svg class="h-5 w-5 text-gray-400" viewBox="0 0 20 20" fill="currentColor">
              <path fill-rule="evenodd" d="M9 3.5a5.5 5.5 0 100 11 5.5 5.5 0 000-11zM2 9a7 7 0 1112.452 4.391l3.328 3.329a.75.75 0 11-1.06 1.06l-3.329-3.328A7 7 0 012 9z" clip-rule="evenodd" />
            </svg>
          </div>
          <input
            id="search"
            type="text"
            bind:value={searchQuery}
            on:keydown={handleKeydown}
            placeholder="Enter game title..."
            class="form-input pl-10 focus:ring-2 focus:ring-primary-500"
            disabled={isSearching}
          />
        </div>
        <button
          on:click={handleSearch}
          disabled={isSearching || !searchQuery.trim()}
          class="btn-primary inline-flex items-center gap-x-2 disabled:opacity-50 disabled:cursor-not-allowed transition-all duration-200"
        >
          {#if isSearching}
            <svg class="animate-spin h-4 w-4 text-white" xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24">
              <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
              <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path>
            </svg>
            Searching...
          {:else}
            <svg class="h-5 w-5" viewBox="0 0 20 20" fill="currentColor">
              <path fill-rule="evenodd" d="M9 3.5a5.5 5.5 0 100 11 5.5 5.5 0 000-11zM2 9a7 7 0 1112.452 4.391l3.328 3.329a.75.75 0 11-1.06 1.06l-3.329-3.328A7 7 0 012 9z" clip-rule="evenodd" />
            </svg>
            Search
          {/if}
        </button>
      </div>
    </div>

    <div class="bg-blue-50 border border-blue-200 rounded-lg p-4">
      <div class="flex">
        <div class="flex-shrink-0">
          <svg class="h-5 w-5 text-blue-600" viewBox="0 0 20 20" fill="currentColor">
            <path fill-rule="evenodd" d="M18 10a8 8 0 11-16 0 8 8 0 0116 0zm-7-4a1 1 0 11-2 0 1 1 0 012 0zM9 9a1 1 0 000 2v3a1 1 0 001 1h1a1 1 0 100-2v-3a1 1 0 00-1-1H9z" clip-rule="evenodd" />
          </svg>
        </div>
        <div class="ml-3 flex-1">
          <h3 class="text-sm font-medium text-blue-900">How game search works</h3>
          <div class="mt-2 text-sm text-blue-800 space-y-1">
            <p>• Search for games using the IGDB database with automatic metadata</p>
            <p>• Only games from the IGDB database can be added to your collection</p>
          </div>
        </div>
      </div>
    </div>

    {#if games.value.error}
      <div class="rounded-lg bg-red-50 border border-red-200 p-4">
        <div class="flex">
          <div class="flex-shrink-0">
            <svg class="h-5 w-5 text-red-600" viewBox="0 0 20 20" fill="currentColor">
              <path fill-rule="evenodd" d="M10 18a8 8 0 100-16 8 8 0 000 16zM8.28 7.22a.75.75 0 00-1.06 1.06L8.94 10l-1.72 1.72a.75.75 0 101.06 1.06L10 11.06l1.72 1.72a.75.75 0 101.06-1.06L11.06 10l1.72-1.72a.75.75 0 00-1.06-1.06L10 8.94 8.28 7.22z" clip-rule="evenodd" />
            </svg>
          </div>
          <div class="ml-3 flex-1">
            <h3 class="text-sm font-medium text-red-900">Search Error</h3>
            <p class="mt-1 text-sm text-red-800">{games.value.error}</p>
          </div>
        </div>
      </div>
    {/if}

  </div>
</div>