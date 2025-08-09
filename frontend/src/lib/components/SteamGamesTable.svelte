<script lang="ts">
  import SteamGameCard from './SteamGameCard.svelte';
  import { steamGames, type SteamGameResponse } from '$lib/stores/steam-games.svelte';
  import IGDBSearchWidget from './steam/IGDBSearchWidget.svelte';
  
  interface Props {
    title: string;
    description: string;
    icon: string;
    games: SteamGameResponse[];
    emptyMessage?: string;
    showMatchButton?: boolean;
    showSyncButton?: boolean;
    showIgnoreButton?: boolean;
    showUnignoreButton?: boolean;
    showUnmatchButton?: boolean;
    onRefresh?: () => Promise<void>;
    collapsible?: boolean;
    collapsed?: boolean;
    onToggleCollapse?: () => void;
  }

  let {
    title,
    description,
    icon,
    games,
    emptyMessage = 'No games in this section',
    showMatchButton = false,
    showSyncButton = false,
    showIgnoreButton = false,
    showUnignoreButton = false,
    showUnmatchButton = false,
    onRefresh,
    collapsible = false,
    collapsed = false,
    onToggleCollapse
  }: Props = $props();

  // State for IGDB matching
  let matchingGameId = $state<string | null>(null);
  let showMatchWidget = $state(false);
  let loadingGames = $state<Set<string>>(new Set());

  async function handleMatch(game: SteamGameResponse) {
    matchingGameId = game.id;
    showMatchWidget = true;
  }

  async function handleAutoMatch(game: SteamGameResponse) {
    try {
      console.log('🎯 [SINGLE AUTO-MATCH] Starting single game auto-match for:', game.game_name);
      setGameLoading(game.id, true);
      
      console.log('🔄 [SINGLE AUTO-MATCH] Calling steamGames.autoMatchSingleGame()...');
      const result = await steamGames.autoMatchSingleGame(game.id);
      console.log('✅ [SINGLE AUTO-MATCH] Auto-match result:', result);
      
      console.log('🔄 [SINGLE AUTO-MATCH] Calling onRefresh callback...');
      await onRefresh?.();
      console.log('✅ [SINGLE AUTO-MATCH] Refresh completed');
    } catch (error) {
      console.error('❌ [SINGLE AUTO-MATCH] Error:', error);
      // Error handled in store
    } finally {
      setGameLoading(game.id, false);
      console.log('✅ [SINGLE AUTO-MATCH] Single auto-match completed');
    }
  }

  async function handleGameSelected(selectedGame: any) {
    if (!matchingGameId) return;
    
    try {
      setGameLoading(matchingGameId, true);
      await steamGames.matchSteamGameToIGDB(matchingGameId, selectedGame.igdb_id);
      await onRefresh?.();
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

  async function handleSync(game: SteamGameResponse) {
    try {
      setGameLoading(game.id, true);
      await steamGames.syncSteamGameToCollection(game.id);
      await onRefresh?.();
    } catch (error) {
      // Error handled in store
    } finally {
      setGameLoading(game.id, false);
    }
  }

  async function handleIgnore(game: SteamGameResponse) {
    try {
      setGameLoading(game.id, true);
      await steamGames.toggleSteamGameIgnored(game.id);
      await onRefresh?.();
    } catch (error) {
      // Error handled in store
    } finally {
      setGameLoading(game.id, false);
    }
  }

  async function handleUnignore(game: SteamGameResponse) {
    try {
      setGameLoading(game.id, true);
      await steamGames.toggleSteamGameIgnored(game.id);
      await onRefresh?.();
    } catch (error) {
      // Error handled in store
    } finally {
      setGameLoading(game.id, false);
    }
  }

  async function handleUnmatch(game: SteamGameResponse) {
    try {
      setGameLoading(game.id, true);
      await steamGames.matchSteamGameToIGDB(game.id, null);
      await onRefresh?.();
    } catch (error) {
      // Error handled in store
    } finally {
      setGameLoading(game.id, false);
    }
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
          <SteamGameCard
            {game}
            onMatch={showMatchButton ? () => handleMatch(game) : undefined}
            onAutoMatch={showMatchButton ? () => handleAutoMatch(game) : undefined}
            onSync={showSyncButton ? () => handleSync(game) : undefined}
            onIgnore={showIgnoreButton ? () => handleIgnore(game) : undefined}
            onUnignore={showUnignoreButton ? () => handleUnignore(game) : undefined}
            onUnmatch={showUnmatchButton ? () => handleUnmatch(game) : undefined}
            isLoading={isGameLoading(game.id)}
          />
        {/each}
      </div>
    {/if}
  {/if}
</div>

<!-- IGDB Match Modal -->
{#if showMatchWidget && matchingGame}
  <div class="fixed inset-0 bg-gray-600 bg-opacity-50 flex items-center justify-center p-4 z-50">
    <div class="max-w-2xl w-full max-h-[80vh] overflow-y-auto">
      <div class="bg-white rounded-lg shadow-xl">
        <div class="p-4 border-b border-gray-200">
          <h3 class="text-lg font-medium text-gray-900">
            Match "{matchingGame.game_name}" to IGDB
          </h3>
          <p class="text-sm text-gray-500 mt-1">
            Search for the correct game in the IGDB database
          </p>
        </div>
        <div class="p-4">
          <IGDBSearchWidget
            initialQuery={matchingGame.game_name}
            onGameSelected={handleGameSelected}
            onCancel={handleCancelMatch}
          />
        </div>
      </div>
    </div>
  </div>
{/if}