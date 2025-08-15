<script lang="ts">
  import ImportGameCard from './ImportGameCard.svelte';
  import type { ImportGame, ImportGameAction, ImportSearchResult } from '$lib/types/import';
  
  interface Props {
    title: string;
    description: string;
    icon: string;
    games: ImportGame[];
    actions?: ImportGameAction[];
    emptyMessage?: string;
    onRefresh?: () => Promise<void>;
    collapsible?: boolean;
    collapsed?: boolean;
    onToggleCollapse?: () => void;
    sourcePrefix?: string; // e.g., "Steam" for display purposes
    showGameLink?: boolean;
    // Game search/matching functionality
    searchService?: {
      searchGames: (query: string) => Promise<ImportSearchResult[]>;
      matchGame: (gameId: string, searchResult: ImportSearchResult) => Promise<void>;
    };
    enableInlineMatching?: boolean;
  }

  let {
    title,
    description,
    icon,
    games,
    actions = [],
    emptyMessage = 'No games in this section',
    onRefresh,
    collapsible = false,
    collapsed = false,
    onToggleCollapse,
    sourcePrefix = 'External',
    showGameLink = false,
    searchService,
    enableInlineMatching = false
  }: Props = $props();

  // State for game matching
  let matchingGameId = $state<string | null>(null);
  let showMatchWidget = $state(false);
  let loadingGames = $state<Set<string>>(new Set());

  // Search state
  let searchQuery = $state('');
  let searchResults = $state<ImportSearchResult[]>([]);
  let isSearching = $state(false);

  async function handleMatch(game: ImportGame) {
    if (!searchService) return;
    
    matchingGameId = game.id;
    showMatchWidget = true;
    searchQuery = game.name;
    await performSearch();
  }

  async function performSearch() {
    if (!searchService || !searchQuery.trim()) {
      searchResults = [];
      return;
    }
    
    try {
      isSearching = true;
      searchResults = await searchService.searchGames(searchQuery.trim());
    } catch (error) {
      console.error('Search failed:', error);
      searchResults = [];
    } finally {
      isSearching = false;
    }
  }

  async function handleGameSelected(selectedGame: ImportSearchResult) {
    if (!matchingGameId || !searchService) return;
    
    try {
      setGameLoading(matchingGameId, true);
      await searchService.matchGame(matchingGameId, selectedGame);
      await onRefresh?.();
    } catch (error) {
      console.error('Matching failed:', error);
    } finally {
      setGameLoading(matchingGameId, false);
      showMatchWidget = false;
      matchingGameId = null;
      searchQuery = '';
      searchResults = [];
    }
  }

  function handleCancelMatch() {
    showMatchWidget = false;
    matchingGameId = null;
    searchQuery = '';
    searchResults = [];
  }

  function setGameLoading(gameId: string, loading: boolean) {
    if (loading) {
      loadingGames.add(gameId);
    } else {
      loadingGames.delete(gameId);
    }
    loadingGames = new Set(loadingGames); // Trigger reactivity
  }

  function isGameLoading(gameId: string): boolean {
    return loadingGames.has(gameId);
  }

  // Create enhanced actions with loading state handling
  const enhancedActions = $derived(
    actions.map(action => ({
      ...action,
      handler: async (game: ImportGame) => {
        try {
          setGameLoading(game.id, true);
          await action.handler(game);
          await onRefresh?.();
        } catch (error) {
          console.error(`${action.type} failed:`, error);
        } finally {
          setGameLoading(game.id, false);
        }
      }
    }))
  );

  // Add match action if search service is provided and not already in actions
  const allActions = $derived(() => {
    let actionsWithMatch = [...enhancedActions];
    
    if (searchService && enableInlineMatching && !actions.find(a => a.type === 'match')) {
      const matchAction: ImportGameAction = {
        type: 'match',
        label: 'Match',
        icon: '🔗',
        enabled: (game: ImportGame) => !game.igdb_id && !game.ignored,
        handler: async (game: ImportGame) => { handleMatch(game); },
        buttonClass: 'btn-secondary',
        title: 'Match to IGDB game'
      };
      actionsWithMatch.unshift(matchAction);
    }
    
    return actionsWithMatch;
  });

  const matchingGame = $derived(
    matchingGameId ? games.find(g => g.id === matchingGameId) : null
  );
</script>

<div class="card">
  <div class="border-b border-gray-200 pb-4 mb-4">
    {#if collapsible}
      <button
        type="button"
        onclick={onToggleCollapse}
        class="w-full flex items-center justify-between text-left hover:bg-gray-50 -mx-2 -my-1 px-2 py-1 rounded transition-colors duration-200"
      >
        <div>
          <h2 class="text-lg font-semibold text-gray-900 flex items-center">
            <span class="text-xl mr-2">{icon}</span>
            {title} ({games.length})
          </h2>
          <p class="text-sm text-gray-600 mt-1">
            {description}
          </p>
        </div>
        <svg class="h-5 w-5 text-gray-400 transition-transform duration-200 {collapsed ? '' : 'rotate-180'}" fill="none" viewBox="0 0 24 24" stroke="currentColor">
          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 9l-7 7-7-7" />
        </svg>
      </button>
    {:else}
      <h2 class="text-lg font-semibold text-gray-900 flex items-center">
        <span class="text-xl mr-2">{icon}</span>
        {title} ({games.length})
      </h2>
      <p class="text-sm text-gray-600 mt-1">
        {description}
      </p>
    {/if}
  </div>

  {#if !collapsed}
    {#if games.length === 0}
      <div class="text-center py-8">
        <div class="text-4xl mb-2">{icon}</div>
        <p class="text-gray-500 text-sm">{emptyMessage}</p>
      </div>
    {:else}
      <div class="space-y-3">
        {#each games as game (game.id)}
          <ImportGameCard
            {game}
            actions={allActions as unknown as ImportGameAction[]}
            {sourcePrefix}
            {showGameLink}
            isLoading={isGameLoading(game.id)}
          />
        {/each}
      </div>
    {/if}
  {/if}
</div>

<!-- Game Search/Match Modal -->
{#if showMatchWidget && matchingGame && searchService}
  <div class="fixed inset-0 bg-gray-600 bg-opacity-50 flex items-center justify-center p-4 z-50">
    <div class="max-w-2xl w-full max-h-[80vh] overflow-y-auto">
      <div class="bg-white rounded-lg shadow-xl">
        <div class="p-4 border-b border-gray-200">
          <h3 class="text-lg font-medium text-gray-900">
            Match "{matchingGame.name}" to IGDB
          </h3>
          <p class="text-sm text-gray-500 mt-1">
            Search for the correct game in the IGDB database
          </p>
        </div>
        <div class="p-4 space-y-4">
          <!-- Search Input -->
          <div class="relative">
            <input
              type="text"
              bind:value={searchQuery}
              oninput={() => performSearch()}
              placeholder="Search for games..."
              class="w-full px-4 py-2 border border-gray-300 rounded-md focus:ring-2 focus:ring-blue-500 focus:border-blue-500"
            />
            {#if isSearching}
              <div class="absolute right-3 top-1/2 transform -translate-y-1/2">
                <svg class="animate-spin h-5 w-5 text-gray-400" fill="none" viewBox="0 0 24 24">
                  <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
                  <path class="opacity-75" fill="currentColor" d="m4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 714 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path>
                </svg>
              </div>
            {/if}
          </div>

          <!-- Search Results -->
          <div class="max-h-64 overflow-y-auto space-y-2">
            {#if searchResults.length === 0 && searchQuery.trim() && !isSearching}
              <div class="text-center py-4 text-gray-500">
                No games found matching "{searchQuery}"
              </div>
            {:else if searchResults.length === 0 && !searchQuery.trim()}
              <div class="text-center py-4 text-gray-500">
                Enter a search query to find games
              </div>
            {:else}
              {#each searchResults as result (result.id)}
                <button
                  onclick={() => handleGameSelected(result)}
                  class="w-full text-left p-3 border border-gray-200 rounded-lg hover:bg-gray-50 transition-colors duration-200"
                >
                  <div class="flex items-center space-x-3">
                    {#if result.cover_url}
                      <img
                        src={result.cover_url}
                        alt="{result.name} cover"
                        class="w-12 h-16 object-cover rounded"
                      />
                    {:else}
                      <div class="w-12 h-16 bg-gray-200 rounded flex items-center justify-center">
                        <span class="text-gray-400 text-xs">No Cover</span>
                      </div>
                    {/if}
                    <div class="flex-1 min-w-0">
                      <h4 class="font-medium text-gray-900 truncate">{result.name}</h4>
                      {#if result.release_date}
                        <p class="text-sm text-gray-500">Released: {result.release_date}</p>
                      {/if}
                      {#if result.platforms}
                        <p class="text-xs text-gray-400 truncate">
                          Platforms: {result.platforms.join(', ')}
                        </p>
                      {/if}
                    </div>
                  </div>
                </button>
              {/each}
            {/if}
          </div>

          <!-- Action Buttons -->
          <div class="flex justify-end space-x-3 pt-4 border-t">
            <button
              onclick={handleCancelMatch}
              class="px-4 py-2 text-sm font-medium text-gray-700 bg-white border border-gray-300 rounded-md hover:bg-gray-50 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-blue-500"
            >
              Cancel
            </button>
          </div>
        </div>
      </div>
    </div>
  </div>
{/if}