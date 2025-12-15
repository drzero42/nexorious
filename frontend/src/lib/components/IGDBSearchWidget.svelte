<script lang="ts">
  import { onMount } from 'svelte';
  import { config } from '$lib/env';
  import { auth } from '$lib/stores';

  interface Props {
    initialQuery: string;
    onGameSelected: (game: IGDBGame) => void;
    onCancel: () => void;
  }

  interface IGDBGame {
    igdb_id: string;
    igdb_slug?: string;
    title: string;
    release_date?: string;
    cover_art_url?: string;
    description?: string;
    platforms: string[];
    howlongtobeat_main?: number;
    howlongtobeat_extra?: number;
    howlongtobeat_completionist?: number;
  }

  let { initialQuery, onGameSelected, onCancel }: Props = $props();

  let searchQuery = $state(initialQuery);
  let searchResults: IGDBGame[] = $state([]);
  let isSearching = $state(false);
  let searchError = $state<string | null>(null);
  let selectedGameId = $state<string | null>(null);
  let searchTimeout: NodeJS.Timeout | null = null;

  // Auto-search on mount with initial query
  onMount(async () => {
    if (initialQuery.trim()) {
      await performSearch(initialQuery);
    }
  });

  // Debounced search using effect instead of reactive statement
  $effect(() => {
    if (searchQuery.trim() !== initialQuery || searchQuery.trim().length > 0) {
      if (searchTimeout) {
        clearTimeout(searchTimeout);
      }

      if (searchQuery.trim().length >= 2) {
        searchTimeout = setTimeout(() => {
          performSearch(searchQuery);
        }, 500);
      } else {
        searchResults = [];
        searchError = null;
      }
    }
  });

  async function performSearch(query: string) {
    if (!query.trim() || query.length < 2) {
      searchResults = [];
      return;
    }

    isSearching = true;
    searchError = null;

    try {
      const response = await fetch(`${config.apiUrl}/games/search/igdb`, {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          'Authorization': `Bearer ${auth.value.accessToken}`
        },
        body: JSON.stringify({
          query: query,
          limit: 10
        })
      });

      if (!response.ok) {
        if (response.status === 401) {
          await auth.refreshAuth();
          return performSearch(query);
        }
        throw new Error('Failed to search games');
      }

      const data = await response.json();
      searchResults = data.games || [];
    } catch (error) {
      searchError = error instanceof Error ? error.message : 'Search failed';
      searchResults = [];
    } finally {
      isSearching = false;
    }
  }

  function handleGameSelect(game: IGDBGame) {
    selectedGameId = game.igdb_id;
  }

  function handleConfirmSelection() {
    const selectedGame = searchResults.find(game => game.igdb_id === selectedGameId);
    if (selectedGame) {
      onGameSelected(selectedGame);
    }
  }

  function formatReleaseDate(dateString?: string): string {
    if (!dateString) return '';
    return new Date(dateString).getFullYear().toString();
  }

  function getCoverImageUrl(url?: string): string {
    return url || '';
  }

  function handleKeyDown(event: KeyboardEvent) {
    if (event.key === 'Escape') {
      onCancel();
    }
  }
</script>

<svelte:window onkeydown={handleKeyDown} />

<div class="bg-white rounded-lg border border-gray-300 p-4 space-y-4">
  <!-- Search Input -->
  <div class="relative">
    <input
      type="text"
      bind:value={searchQuery}
      placeholder="Search for games..."
      class="w-full pl-10 pr-4 py-2 border border-gray-300 rounded-md focus:ring-blue-500 focus:border-blue-500 text-sm"
      class:border-red-300={searchError}
    />
    <div class="absolute inset-y-0 left-0 pl-3 flex items-center pointer-events-none">
      {#if isSearching}
        <svg class="animate-spin h-4 w-4 text-gray-400" fill="none" viewBox="0 0 24 24">
          <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
          <path class="opacity-75" fill="currentColor" d="m4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 714 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path>
        </svg>
      {:else}
        <svg class="h-4 w-4 text-gray-400" fill="none" viewBox="0 0 24 24" stroke="currentColor">
          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M21 21l-6-6m2-5a7 7 0 11-14 0 7 7 0 0114 0z" />
        </svg>
      {/if}
    </div>
  </div>

  <!-- Search Status -->
  {#if searchError}
    <div class="text-sm text-red-600 bg-red-50 p-2 rounded">
      {searchError}
    </div>
  {:else if isSearching && searchQuery.length >= 2}
    <div class="text-sm text-gray-600 text-center py-4">
      Searching for "{searchQuery}"...
    </div>
  {:else if searchQuery.length >= 2 && searchResults.length === 0 && !isSearching}
    <div class="text-sm text-gray-600 text-center py-4">
      No games found for "{searchQuery}". Try different search terms.
    </div>
  {/if}

  <!-- Search Results -->
  {#if searchResults.length > 0}
    <div class="space-y-2 max-h-96 overflow-y-auto">
      <div class="text-xs font-medium text-gray-700 px-2">
        Found {searchResults.length} game{searchResults.length === 1 ? '' : 's'}:
      </div>

      {#each searchResults as game (game.igdb_id)}
        <button
          class="w-full p-3 border rounded-lg text-left hover:bg-gray-50 focus:outline-none focus:ring-2 focus:ring-blue-500 focus:border-transparent transition-colors
                 {selectedGameId === game.igdb_id ? 'border-blue-500 bg-blue-50' : 'border-gray-200'}"
          onclick={() => handleGameSelect(game)}
        >
          <div class="flex items-start space-x-3">
            <!-- Game Cover -->
            <div class="flex-shrink-0 w-12 h-16 bg-gray-100 rounded overflow-hidden">
              {#if game.cover_art_url}
                <img
                  src={getCoverImageUrl(game.cover_art_url)}
                  alt="{game.title} cover"
                  class="w-full h-full object-cover"
                  loading="lazy"
                />
              {:else}
                <div class="w-full h-full flex items-center justify-center">
                  <svg class="w-6 h-6 text-gray-400" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                    <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 11H5m14 0a2 2 0 012 2v6a2 2 0 01-2 2H5a2 2 0 01-2-2v-6a2 2 0 012-2m14 0V9a2 2 0 00-2-2M5 11V9a2 2 0 012-2m0 0V5a2 2 0 012-2h6a2 2 0 012 2v2M7 7h10" />
                  </svg>
                </div>
              {/if}
            </div>

            <!-- Game Info -->
            <div class="flex-1 min-w-0">
              <div class="flex items-center space-x-2 mb-1">
                <h4 class="text-sm font-medium text-gray-900 truncate">
                  {game.title}
                </h4>
                {#if game.release_date}
                  <span class="text-xs text-gray-500 bg-gray-100 px-2 py-0.5 rounded">
                    {formatReleaseDate(game.release_date)}
                  </span>
                {/if}
              </div>

              {#if game.platforms && game.platforms.length > 0}
                <div class="mb-2">
                  <div class="flex flex-wrap gap-1">
                    {#each game.platforms.slice(0, 3) as platform}
                      <span class="text-xs text-gray-600 bg-gray-100 px-1.5 py-0.5 rounded">
                        {platform}
                      </span>
                    {/each}
                    {#if game.platforms.length > 3}
                      <span class="text-xs text-gray-500">
                        +{game.platforms.length - 3} more
                      </span>
                    {/if}
                  </div>
                </div>
              {/if}

              {#if game.description}
                <p class="text-xs text-gray-600 line-clamp-2">
                  {game.description}
                </p>
              {/if}
            </div>

            <!-- Selection Indicator -->
            <div class="flex-shrink-0">
              {#if selectedGameId === game.igdb_id}
                <div class="w-5 h-5 bg-blue-500 rounded-full flex items-center justify-center">
                  <svg class="w-3 h-3 text-white" fill="currentColor" viewBox="0 0 20 20">
                    <path fill-rule="evenodd" d="M16.707 5.293a1 1 0 010 1.414l-8 8a1 1 0 01-1.414 0l-4-4a1 1 0 011.414-1.414L8 12.586l7.293-7.293a1 1 0 011.414 0z" clip-rule="evenodd" />
                  </svg>
                </div>
              {:else}
                <div class="w-5 h-5 border-2 border-gray-300 rounded-full"></div>
              {/if}
            </div>
          </div>
        </button>
      {/each}
    </div>
  {/if}

  <!-- Action Buttons -->
  <div class="flex justify-between items-center pt-3 border-t">
    <button
      onclick={onCancel}
      class="btn-secondary text-sm"
    >
      Cancel
    </button>

    <button
      onclick={handleConfirmSelection}
      disabled={!selectedGameId}
      class="btn-primary text-sm disabled:opacity-50 disabled:cursor-not-allowed"
    >
      Use Selected Game
    </button>
  </div>
</div>

<style>
  .line-clamp-2 {
    display: -webkit-box;
    -webkit-line-clamp: 2;
    -webkit-box-orient: vertical;
    line-clamp: 2;
    overflow: hidden;
  }

  /* Custom scrollbar for search results */
  .max-h-96::-webkit-scrollbar {
    width: 6px;
  }

  .max-h-96::-webkit-scrollbar-track {
    background: #f1f5f9;
    border-radius: 3px;
  }

  .max-h-96::-webkit-scrollbar-thumb {
    background: #cbd5e1;
    border-radius: 3px;
  }

  .max-h-96::-webkit-scrollbar-thumb:hover {
    background: #94a3b8;
  }
</style>
