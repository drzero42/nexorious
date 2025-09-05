<script lang="ts">
  import DarkadiaGameCard from './DarkadiaGameCard.svelte';
  import { darkadia } from '$lib/stores/darkadia.svelte';
  import { auth } from '$lib/stores/auth.svelte';
  import IGDBSearchWidget from './steam/IGDBSearchWidget.svelte';
  import type { DarkadiaGameResponse, DarkadiaPlatformInfo } from '$lib/types/darkadia';
  
  interface Props {
    title: string;
    description?: string;
    icon?: string;
    games: DarkadiaGameResponse[];
    emptyMessage?: string;
    
    // Action buttons to show
    showMatchButton?: boolean;
    showSyncButton?: boolean;
    showIgnoreButton?: boolean;
    showUnignoreButton?: boolean;
    showUnmatchButton?: boolean;
    showUnsyncButton?: boolean;
    showGameLink?: boolean;
    
    // Callbacks
    onRefresh?: () => void;
    onGameAction?: (gameId: string, action: string) => void;
    
    // Collapsible functionality
    collapsible?: boolean;
    collapsed?: boolean;
    onToggleCollapse?: () => void;
    
    // Loading states
    isLoading?: boolean;
  }

  let {
    title,
    description = '',
    icon = '',
    games,
    emptyMessage = 'No games in this section',
    showMatchButton = false,
    showSyncButton = false,
    showIgnoreButton = false,
    showUnignoreButton = false,
    showUnmatchButton = false,
    showUnsyncButton = false,
    showGameLink = false,
    onRefresh,
    onGameAction,
    collapsible = false,
    collapsed = false,
    onToggleCollapse,
    isLoading = false
  }: Props = $props();

  // State for IGDB matching
  let matchingGameId = $state<string | null>(null);
  let showMatchWidget = $state(false);
  let loadingGames = $state<Set<string>>(new Set());

  // Individual game action handlers
  async function handleMatch(game: DarkadiaGameResponse) {
    matchingGameId = game.id;
    showMatchWidget = true;
  }

  async function handleGameSelected(selectedGame: any) {
    if (!matchingGameId || !auth.value.user?.id) return;
    
    try {
      setGameLoading(matchingGameId, true);
      await darkadia.matchGame(auth.value.user.id, matchingGameId, selectedGame.igdb_id);
      await onRefresh?.();
      onGameAction?.(matchingGameId, 'matched');
    } catch (error) {
      // Error handled in store
    } finally {
      setGameLoading(matchingGameId, false);
      showMatchWidget = false;
      matchingGameId = null;
    }
  }

  function handleCancelMatch() {
    showMatchWidget = false;
    matchingGameId = null;
  }

  async function handleSync(game: DarkadiaGameResponse) {
    if (!auth.value.user?.id) return;
    
    try {
      setGameLoading(game.id, true);
      await darkadia.syncGame(auth.value.user.id, game.id);
      await onRefresh?.();
      onGameAction?.(game.id, 'synced');
    } catch (error) {
      // Error handled in store
    } finally {
      setGameLoading(game.id, false);
    }
  }

  async function handleIgnore(game: DarkadiaGameResponse) {
    if (!auth.value.user?.id) return;
    
    try {
      setGameLoading(game.id, true);
      await darkadia.ignoreGame(auth.value.user.id, game.id);
      await onRefresh?.();
      onGameAction?.(game.id, 'ignored');
    } catch (error) {
      // Error handled in store
    } finally {
      setGameLoading(game.id, false);
    }
  }

  async function handleUnignore(game: DarkadiaGameResponse) {
    if (!auth.value.user?.id) return;
    
    try {
      setGameLoading(game.id, true);
      // Assuming the ignore API toggles the state
      await darkadia.ignoreGame(auth.value.user.id, game.id);
      await onRefresh?.();
      onGameAction?.(game.id, 'unignored');
    } catch (error) {
      // Error handled in store
    } finally {
      setGameLoading(game.id, false);
    }
  }

  async function handleUnmatch(game: DarkadiaGameResponse) {
    if (!auth.value.user?.id) return;
    
    try {
      setGameLoading(game.id, true);
      await darkadia.matchGame(auth.value.user.id, game.id, null);
      await onRefresh?.();
      onGameAction?.(game.id, 'unmatched');
    } catch (error) {
      // Error handled in store
    } finally {
      setGameLoading(game.id, false);
    }
  }

  async function handleUnsync(game: DarkadiaGameResponse) {
    // Note: This would need to be implemented in the darkadia store
    // For now, we'll call the callback to handle it at a higher level
    onGameAction?.(game.id, 'unsynced');
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

  function formatPlatformDisplay(platform: DarkadiaPlatformInfo): string {
    // Prioritize resolved names over original names
    const platformName = platform.resolved_platform_name || platform.original_platform_name || 'Unknown Platform';
    const storefrontName = platform.resolved_storefront_name || platform.original_storefront_name;
    
    if (storefrontName) {
      return `${platformName} (${storefrontName})`;
    } else {
      return platformName;
    }
  }

  const matchingGame = $derived(
    matchingGameId ? games.find(g => g.id === matchingGameId) : null
  );

  const gameCount = $derived(games.length);
  const hasGames = $derived(gameCount > 0);
</script>

<div class="card">
  <div class="border-b border-gray-200 pb-4 mb-4">
    {#if collapsible}
      <button
        type="button"
        onclick={onToggleCollapse}
        class="w-full flex items-center justify-between text-left hover:bg-gray-50 -mx-2 -my-1 px-2 py-1 rounded transition-colors duration-200"
        disabled={isLoading}
      >
        <div>
          <h2 class="text-lg font-semibold text-gray-900 flex items-center">
            {#if icon}
              <span class="text-xl mr-2">{icon}</span>
            {/if}
            {title} ({gameCount})
          </h2>
          {#if description}
            <p class="text-sm text-gray-600 mt-1">
              {description}
            </p>
          {/if}
        </div>
        <div class="flex items-center">
          {#if isLoading}
            <svg class="animate-spin h-5 w-5 text-gray-400 mr-2" fill="none" viewBox="0 0 24 24">
              <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
              <path class="opacity-75" fill="currentColor" d="m4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path>
            </svg>
          {/if}
          <svg class="h-5 w-5 text-gray-400 transition-transform duration-200 {collapsed ? '' : 'rotate-180'}" fill="none" viewBox="0 0 24 24" stroke="currentColor">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 9l-7 7-7-7" />
          </svg>
        </div>
      </button>
    {:else}
      <div class="flex items-center justify-between">
        <div>
          <h2 class="text-lg font-semibold text-gray-900 flex items-center">
            {#if icon}
              <span class="text-xl mr-2">{icon}</span>
            {/if}
            {title} ({gameCount})
          </h2>
          {#if description}
            <p class="text-sm text-gray-600 mt-1">
              {description}
            </p>
          {/if}
        </div>
        {#if isLoading}
          <svg class="animate-spin h-5 w-5 text-gray-400" fill="none" viewBox="0 0 24 24">
            <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
            <path class="opacity-75" fill="currentColor" d="m4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path>
          </svg>
        {/if}
      </div>
    {/if}
  </div>

  {#if !collapsed}
    {#if !hasGames}
      <div class="text-center py-8">
        {#if icon}
          <div class="text-4xl mb-2">{icon}</div>
        {/if}
        <p class="text-gray-500 text-sm">{emptyMessage}</p>
      </div>
    {:else}
      <!-- Desktop Table View -->
      <div class="hidden md:block overflow-x-auto">
        <table class="min-w-full divide-y divide-gray-200">
          <thead class="bg-gray-50">
            <tr>
              <th scope="col" class="px-4 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">
                Game
              </th>
              <th scope="col" class="px-4 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">
                Status
              </th>
              <th scope="col" class="px-4 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">
                Platform
              </th>
              <th scope="col" class="px-4 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">
                IGDB Match
              </th>
              <th scope="col" class="px-4 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">
                Import Date
              </th>
              <th scope="col" class="relative px-4 py-3">
                <span class="sr-only">Actions</span>
              </th>
            </tr>
          </thead>
          <tbody class="bg-white divide-y divide-gray-200">
            {#each games as game (game.id)}
              {@const gameLoading = isGameLoading(game.id)}
              {@const status = game.ignored ? 'ignored' : game.game_id ? 'synced' : game.igdb_id ? 'matched' : 'unmatched'}
              {@const statusConfig = {
                unmatched: { label: 'Unmatched', color: 'bg-yellow-100 text-yellow-800', icon: '🔍' },
                matched: { label: 'Matched', color: 'bg-green-100 text-green-800', icon: '✅' },
                ignored: { label: 'Ignored', color: 'bg-gray-100 text-gray-600', icon: '🚫' },
                synced: { label: 'Synced', color: 'bg-blue-100 text-blue-800', icon: '🔥' }
              }[status]}
              
              <tr class="hover:bg-gray-50 transition-colors duration-200">
                <!-- Game Name -->
                <td class="px-4 py-4 whitespace-nowrap">
                  <div class="flex items-center">
                    <div>
                      {#if showGameLink && game.user_game_id}
                        <a 
                          href="/games/{game.user_game_id}" 
                          class="text-sm font-medium text-gray-900 hover:text-blue-600 transition-colors duration-200"
                        >
                          {game.igdb_title || game.name}
                        </a>
                      {:else}
                        <div class="text-sm font-medium text-gray-900">
                          {game.igdb_title || game.name}
                        </div>
                      {/if}
                      {#if game.igdb_title && game.igdb_title !== game.name}
                        <div class="text-xs text-gray-500">
                          Original: {game.name}
                        </div>
                      {/if}
                      <div class="text-xs text-gray-400">
                        ID: {game.external_id}
                      </div>
                    </div>
                  </div>
                </td>

                <!-- Status -->
                <td class="px-4 py-4 whitespace-nowrap">
                  <span class="inline-flex items-center px-2.5 py-0.5 rounded-full text-xs font-medium {statusConfig.color}">
                    <span class="mr-1">{statusConfig.icon}</span>
                    {statusConfig.label}
                  </span>
                </td>

                <!-- Platform Status -->
                <td class="px-4 py-4">
                  {#if game.platforms && game.platforms.length > 0}
                    <div class="flex flex-wrap gap-1">
                      {#each game.platforms as platform}
                        {@const displayName = formatPlatformDisplay(platform)}
                        {@const status = platform.platform_resolution_status}
                        
                        {#if status === 'resolved'}
                          <span class="inline-flex items-center px-2 py-0.5 rounded border text-xs font-medium bg-green-100 text-green-600 border-green-200" 
                                title="Platform resolved: {displayName}">
                            <span class="mr-1">✅</span>
                            {displayName}
                          </span>
                        {:else if status === 'pending'}
                          <span class="inline-flex items-center px-2 py-0.5 rounded border text-xs font-medium bg-yellow-100 text-yellow-600 border-yellow-200" 
                                title="Platform needs resolution: {displayName}">
                            <span class="mr-1">⚠️</span>
                            {displayName}
                          </span>
                        {:else if status === 'ignored'}
                          <span class="inline-flex items-center px-2 py-0.5 rounded border text-xs font-medium bg-gray-100 text-gray-600 border-gray-200" 
                                title="Platform resolution ignored">
                            <span class="mr-1">🚫</span>
                            Ignored
                          </span>
                        {:else if status === 'conflict'}
                          <span class="inline-flex items-center px-2 py-0.5 rounded border text-xs font-medium bg-red-100 text-red-600 border-red-200" 
                                title="Multiple platform matches: {displayName}">
                            <span class="mr-1">❌</span>
                            Conflict
                          </span>
                        {:else}
                          <span class="inline-flex items-center px-2 py-0.5 rounded border text-xs font-medium bg-gray-100 text-gray-600 border-gray-200" 
                                title="Original platform: {displayName}">
                            <span class="mr-1">📱</span>
                            {displayName}
                          </span>
                        {/if}
                      {/each}
                    </div>
                  {:else}
                    <span class="text-gray-400 text-xs">No platforms</span>
                  {/if}
                </td>

                <!-- IGDB Match -->
                <td class="px-4 py-4 whitespace-nowrap text-sm text-gray-900">
                  {#if game.igdb_title}
                    <div class="flex items-center">
                      <span class="text-green-600 mr-1">✓</span>
                      {game.igdb_title}
                    </div>
                  {:else}
                    <span class="text-gray-400">No match</span>
                  {/if}
                </td>

                <!-- Import Date -->
                <td class="px-4 py-4 whitespace-nowrap text-sm text-gray-500">
                  {game.created_at.toLocaleDateString()}
                  {#if game.updated_at.getTime() !== game.created_at.getTime()}
                    <div class="text-xs">
                      Updated: {game.updated_at.toLocaleDateString()}
                    </div>
                  {/if}
                </td>

                <!-- Actions -->
                <td class="px-4 py-4 whitespace-nowrap text-right text-sm font-medium">
                  <div class="flex items-center justify-end space-x-2">
                    {#if gameLoading}
                      <svg class="animate-spin h-4 w-4 text-gray-400" fill="none" viewBox="0 0 24 24">
                        <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
                        <path class="opacity-75" fill="currentColor" d="m4 12a8 8 0 818-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 714 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path>
                      </svg>
                    {:else}
                      <!-- Match Button -->
                      {#if showMatchButton && !game.igdb_id && !game.ignored}
                        <button
                          onclick={() => handleMatch(game)}
                          class="inline-flex items-center px-2 py-1 border border-gray-300 rounded text-xs font-medium text-gray-700 bg-white hover:bg-gray-50 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-blue-500"
                          title="Match to IGDB"
                        >
                          🔗 Match
                        </button>
                      {/if}

                      <!-- Sync Button -->
                      {#if showSyncButton && game.igdb_id && !game.game_id && !game.ignored}
                        <button
                          onclick={() => handleSync(game)}
                          class="inline-flex items-center px-2 py-1 border border-transparent rounded text-xs font-medium text-white bg-blue-600 hover:bg-blue-700 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-blue-500"
                          title="Add to collection"
                        >
                          ➕ Sync
                        </button>
                      {/if}

                      <!-- Ignore Button -->
                      {#if showIgnoreButton && !game.ignored}
                        <button
                          onclick={() => handleIgnore(game)}
                          class="inline-flex items-center px-2 py-1 border border-gray-300 rounded text-xs font-medium text-gray-700 bg-white hover:bg-gray-50 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-gray-500"
                          title="Mark as ignored"
                        >
                          🚫 Ignore
                        </button>
                      {/if}

                      <!-- Unignore Button -->
                      {#if showUnignoreButton && game.ignored}
                        <button
                          onclick={() => handleUnignore(game)}
                          class="inline-flex items-center px-2 py-1 border border-gray-300 rounded text-xs font-medium text-gray-700 bg-white hover:bg-gray-50 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-gray-500"
                          title="Remove from ignored"
                        >
                          ↩️ Unignore
                        </button>
                      {/if}

                      <!-- Unmatch Button -->
                      {#if showUnmatchButton && game.igdb_id && !game.game_id}
                        <button
                          onclick={() => handleUnmatch(game)}
                          class="inline-flex items-center px-2 py-1 border border-gray-300 rounded text-xs font-medium text-orange-700 bg-white hover:bg-orange-50 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-orange-500"
                          title="Remove IGDB match"
                        >
                          🔓 Unmatch
                        </button>
                      {/if}

                      <!-- Unsync Button -->
                      {#if showUnsyncButton && game.game_id}
                        <button
                          onclick={() => handleUnsync(game)}
                          class="inline-flex items-center px-2 py-1 border border-gray-300 rounded text-xs font-medium text-red-700 bg-white hover:bg-red-50 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-red-500"
                          title="Remove from collection"
                        >
                          📤 Unsync
                        </button>
                      {/if}

                      <!-- Game Link -->
                      {#if showGameLink && game.user_game_id}
                        <a
                          href="/games/{game.user_game_id}"
                          class="inline-flex items-center px-2 py-1 border border-gray-300 rounded text-xs font-medium text-gray-700 bg-white hover:bg-gray-50 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-blue-500"
                          title="View in collection"
                        >
                          🔗 View
                        </a>
                      {/if}
                    {/if}
                  </div>
                </td>
              </tr>
            {/each}
          </tbody>
        </table>
      </div>

      <!-- Mobile Card View -->
      <div class="block md:hidden space-y-3">
        {#each games as game (game.id)}
          <DarkadiaGameCard
            {game}
            onMatch={showMatchButton ? () => handleMatch(game) : undefined}
            onSync={showSyncButton ? () => handleSync(game) : undefined}
            onIgnore={showIgnoreButton ? () => handleIgnore(game) : undefined}
            onUnignore={showUnignoreButton ? () => handleUnignore(game) : undefined}
            onUnmatch={showUnmatchButton ? () => handleUnmatch(game) : undefined}
            onUnsync={showUnsyncButton ? () => handleUnsync(game) : undefined}
            {showGameLink}
            isLoading={isGameLoading(game.id)}
          />
        {/each}
      </div>
    {/if}
  {/if}
</div>

<!-- IGDB Match Modal -->
{#if showMatchWidget && matchingGame}
  <div class="fixed inset-0 bg-gray-600 bg-opacity-50 flex items-center justify-center p-4 z-50" role="dialog" aria-modal="true" aria-labelledby="match-modal-title">
    <div class="max-w-2xl w-full max-h-[80vh] overflow-y-auto">
      <div class="bg-white rounded-lg shadow-xl">
        <div class="p-4 border-b border-gray-200">
          <div class="flex items-center justify-between">
            <h3 id="match-modal-title" class="text-lg font-medium text-gray-900">
              Match "{matchingGame.name}" to IGDB
            </h3>
            <button
              onclick={handleCancelMatch}
              class="text-gray-400 hover:text-gray-500 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-blue-500 rounded"
              aria-label="Close match dialog"
            >
              <svg class="h-6 w-6" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12" />
              </svg>
            </button>
          </div>
          <p class="text-sm text-gray-500 mt-1">
            Search for the correct game in the IGDB database
          </p>
        </div>
        <div class="p-4">
          <IGDBSearchWidget
            initialQuery={matchingGame.name}
            onGameSelected={handleGameSelected}
            onCancel={handleCancelMatch}
          />
        </div>
      </div>
    </div>
  </div>
{/if}