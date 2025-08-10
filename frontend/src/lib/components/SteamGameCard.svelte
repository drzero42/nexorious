<script lang="ts">
  import type { SteamGameResponse } from '$lib/stores/steam-games.svelte';
  
  interface Props {
    game: SteamGameResponse;
    onMatch?: (() => void) | undefined;
    onAutoMatch?: (() => void) | undefined;
    onSync?: (() => void) | undefined;
    onIgnore?: (() => void) | undefined;
    onUnignore?: (() => void) | undefined;
    onUnmatch?: (() => void) | undefined;
    onUnsync?: (() => void) | undefined;
    showActions?: boolean;
    isLoading?: boolean;
    showGameLink?: boolean;
  }

  let {
    game,
    onMatch,
    onAutoMatch,
    onSync,
    onIgnore,
    onUnignore,
    onUnmatch,
    onUnsync,
    showActions = true,
    isLoading = false,
    showGameLink = false
  }: Props = $props();

  function formatDate(dateString: string): string {
    return new Date(dateString).toLocaleDateString();
  }

  function getStatusDisplay(): { label: string; color: string; icon: string } {
    if (game.ignored) {
      return { label: 'Ignored', color: 'bg-gray-100 text-gray-600', icon: '🚫' };
    } else if (game.game_id) {
      return { label: 'In Collection', color: 'bg-green-100 text-green-600', icon: '✅' };
    } else if (game.igdb_id) {
      return { label: 'Matched', color: 'bg-blue-100 text-blue-600', icon: '🔗' };
    } else {
      return { label: 'Unmatched', color: 'bg-yellow-100 text-yellow-600', icon: '❓' };
    }
  }

  const status = $derived(getStatusDisplay());
  const canSync = $derived(game.igdb_id && !game.game_id && !game.ignored);
  const canMatch = $derived(!game.igdb_id && !game.ignored);
  const canIgnore = $derived(!game.ignored);
  const canUnignore = $derived(game.ignored);
  const canUnmatch = $derived(game.igdb_id !== null && !game.game_id); // Only matched, not synced
  const canUnsync = $derived(game.game_id !== null); // Only synced games
  
  // State for confirmation dialogs
  let showUnmatchConfirm = $state(false);
  let showUnsyncConfirm = $state(false);
  
  function handleUnmatchClick() {
    showUnmatchConfirm = true;
  }
  
  function handleConfirmUnmatch() {
    showUnmatchConfirm = false;
    onUnmatch?.();
  }
  
  function handleCancelUnmatch() {
    showUnmatchConfirm = false;
  }
  
  function handleUnsyncClick() {
    showUnsyncConfirm = true;
  }
  
  function handleConfirmUnsync() {
    showUnsyncConfirm = false;
    onUnsync?.();
  }
  
  function handleCancelUnsync() {
    showUnsyncConfirm = false;
  }
</script>

<div class="bg-white border border-gray-200 rounded-lg p-4 hover:shadow-md transition-shadow duration-200">
  <div class="flex items-start space-x-4">
    <!-- Steam Game Info -->
    <div class="flex-1 min-w-0">
      <div class="flex items-center justify-between mb-2">
        {#if showGameLink && game.user_game_id}
          <a 
            href="/games/{game.user_game_id}" 
            class="text-sm font-medium text-gray-900 truncate hover:text-blue-600 transition-colors duration-200 block"
          >
            {game.game_name}
          </a>
        {:else}
          <h3 class="text-sm font-medium text-gray-900 truncate">
            {game.game_name}
          </h3>
        {/if}
        <span class="inline-flex items-center px-2.5 py-0.5 rounded-full text-xs font-medium {status.color}">
          <span class="mr-1">{status.icon}</span>
          {status.label}
        </span>
      </div>

      <div class="space-y-1 text-xs text-gray-500">
        <div class="flex items-center space-x-4">
          <span class="inline-flex items-center">
            <span class="mr-1">🎮</span>
            Steam ID: {game.steam_appid}
          </span>
          {#if game.igdb_id}
            <span class="inline-flex items-center">
              <span class="mr-1">🔗</span>
              IGDB Matched
            </span>
          {/if}
          {#if game.game_id}
            <span class="inline-flex items-center">
              <span class="mr-1">📚</span>
              In Collection
            </span>
          {/if}
        </div>
        <div class="text-xs text-gray-400">
          Added: {formatDate(game.created_at)}
          {#if game.updated_at !== game.created_at}
            • Updated: {formatDate(game.updated_at)}
          {/if}
        </div>
      </div>

      {#if game.igdb_id && !game.game_id}
        <div class="mt-3 p-2 bg-blue-50 border border-blue-200 rounded text-xs">
          <div class="flex items-center text-blue-700">
            <svg class="h-4 w-4 mr-1" fill="none" viewBox="0 0 24 24" stroke="currentColor">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M13 16h-1v-4h-1m1-4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z" />
            </svg>
            Ready to sync to your collection
          </div>
        </div>
      {/if}

      {#if !game.igdb_id && !game.ignored}
        <div class="mt-3 p-2 bg-yellow-50 border border-yellow-200 rounded text-xs">
          <div class="flex items-center text-yellow-700">
            <svg class="h-4 w-4 mr-1" fill="none" viewBox="0 0 24 24" stroke="currentColor">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-2.5L13.732 4c-.77-.833-1.964-.833-2.732 0L3.732 16.5c-.77.833.192 2.5 1.732 2.5z" />
            </svg>
            Needs IGDB matching before sync
          </div>
        </div>
      {/if}
    </div>

    <!-- Actions -->
    {#if showActions}
      <div class="flex-shrink-0 flex flex-col space-y-2">
        {#if canMatch && onMatch}
          <button
            onclick={onMatch}
            disabled={isLoading}
            class="text-xs btn-secondary disabled:opacity-50"
            title="Match to IGDB game"
          >
            {#if isLoading}
              <svg class="animate-spin h-3 w-3 mr-1" fill="none" viewBox="0 0 24 24">
                <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
                <path class="opacity-75" fill="currentColor" d="m4 12a8 8 0 818-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 714 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path>
              </svg>
            {:else}
              <span class="mr-1">🔗</span>
            {/if}
            Match
          </button>
        {/if}

        {#if canMatch && onAutoMatch}
          <button
            onclick={onAutoMatch}
            disabled={isLoading}
            class="text-xs btn-primary disabled:opacity-50"
            title="Automatically match to IGDB using AI"
          >
            {#if isLoading}
              <svg class="animate-spin h-3 w-3 mr-1" fill="none" viewBox="0 0 24 24">
                <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
                <path class="opacity-75" fill="currentColor" d="m4 12a8 8 0 818-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 714 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path>
              </svg>
            {:else}
              <span class="mr-1">🤖</span>
            {/if}
            Auto-match
          </button>
        {/if}

        {#if canSync && onSync}
          <button
            onclick={onSync}
            disabled={isLoading}
            class="text-xs btn-primary disabled:opacity-50"
            title="Add to collection"
          >
            {#if isLoading}
              <svg class="animate-spin h-3 w-3 mr-1" fill="none" viewBox="0 0 24 24">
                <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
                <path class="opacity-75" fill="currentColor" d="m4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 714 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path>
              </svg>
            {:else}
              <span class="mr-1">➕</span>
            {/if}
            Sync
          </button>
        {/if}

        {#if canIgnore && onIgnore}
          <button
            onclick={onIgnore}
            disabled={isLoading}
            class="text-xs btn-secondary text-gray-600 hover:text-red-600 disabled:opacity-50"
            title="Mark as ignored"
          >
            {#if isLoading}
              <svg class="animate-spin h-3 w-3 mr-1" fill="none" viewBox="0 0 24 24">
                <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
                <path class="opacity-75" fill="currentColor" d="m4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 714 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path>
              </svg>
            {:else}
              <span class="mr-1">🚫</span>
            {/if}
            Ignore
          </button>
        {/if}

        {#if canUnignore && onUnignore}
          <button
            onclick={onUnignore}
            disabled={isLoading}
            class="text-xs btn-secondary disabled:opacity-50"
            title="Remove from ignored"
          >
            {#if isLoading}
              <svg class="animate-spin h-3 w-3 mr-1" fill="none" viewBox="0 0 24 24">
                <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
                <path class="opacity-75" fill="currentColor" d="m4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 714 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path>
              </svg>
            {:else}
              <span class="mr-1">↩️</span>
            {/if}
            Unignore
          </button>
        {/if}
        {#if canUnmatch && onUnmatch}
          <button
            onclick={handleUnmatchClick}
            disabled={isLoading}
            class="text-xs btn-secondary text-gray-600 hover:text-orange-600 disabled:opacity-50"
            title="Remove IGDB match"
          >
            {#if isLoading}
              <svg class="animate-spin h-3 w-3 mr-1" fill="none" viewBox="0 0 24 24">
                <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
                <path class="opacity-75" fill="currentColor" d="m4 12a8 8 0 818-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 714 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path>
              </svg>
            {:else}
              <span class="mr-1">🔓</span>
            {/if}
            Unmatch
          </button>
        {/if}

        {#if canUnsync && onUnsync}
          <button
            onclick={handleUnsyncClick}
            disabled={isLoading}
            class="text-xs btn-secondary text-gray-600 hover:text-red-600 disabled:opacity-50"
            title="Remove from collection (keeps IGDB match)"
          >
            {#if isLoading}
              <svg class="animate-spin h-3 w-3 mr-1" fill="none" viewBox="0 0 24 24">
                <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
                <path class="opacity-75" fill="currentColor" d="m4 12a8 8 0 818-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 714 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path>
              </svg>
            {:else}
              <span class="mr-1">📤</span>
            {/if}
            Unsync
          </button>
        {/if}
      </div>
    {/if}
  </div>
</div>

<!-- Unmatch Confirmation Dialog -->
{#if showUnmatchConfirm}
  <div class="fixed inset-0 bg-gray-600 bg-opacity-50 flex items-center justify-center p-4 z-50">
    <div class="bg-white rounded-lg shadow-xl max-w-md w-full">
      <div class="p-4 border-b border-gray-200">
        <h3 class="text-lg font-medium text-gray-900">
          Confirm Unmatch
        </h3>
        <p class="text-sm text-gray-500 mt-1">
          Are you sure you want to unmatch this game from IGDB? This will remove the IGDB association and move the game back to "Needs Attention".
        </p>
      </div>
      <div class="p-4 space-y-3">
        <div class="text-sm">
          <strong>Game:</strong> {game.game_name}
        </div>
        <div class="flex space-x-3 justify-end">
          <button
            onclick={handleCancelUnmatch}
            class="btn-secondary text-sm"
          >
            Cancel
          </button>
          <button
            onclick={handleConfirmUnmatch}
            class="btn-secondary text-sm text-orange-600 hover:text-orange-700 border-orange-300 hover:border-orange-400"
          >
            Unmatch
          </button>
        </div>
      </div>
    </div>
  </div>
{/if}

<!-- Unsync Confirmation Dialog -->
{#if showUnsyncConfirm}
  <div class="fixed inset-0 bg-gray-600 bg-opacity-50 flex items-center justify-center p-4 z-50">
    <div class="bg-white rounded-lg shadow-xl max-w-md w-full">
      <div class="p-4 border-b border-gray-200">
        <h3 class="text-lg font-medium text-gray-900">
          Confirm Unsync
        </h3>
        <p class="text-sm text-gray-500 mt-1">
          Are you sure you want to remove this game from your collection? The IGDB match will remain intact and you can re-sync it later.
        </p>
      </div>
      <div class="p-4 space-y-3">
        <div class="text-sm">
          <strong>Game:</strong> {game.game_name}
        </div>
        <div class="flex space-x-3 justify-end">
          <button
            onclick={handleCancelUnsync}
            class="btn-secondary text-sm"
          >
            Cancel
          </button>
          <button
            onclick={handleConfirmUnsync}
            class="btn-secondary text-sm text-red-600 hover:text-red-700 border-red-300 hover:border-red-400"
          >
            Unsync
          </button>
        </div>
      </div>
    </div>
  </div>
{/if}