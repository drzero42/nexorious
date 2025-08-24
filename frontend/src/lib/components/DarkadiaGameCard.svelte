<script lang="ts">
  import type { DarkadiaGameResponse } from '$lib/types/darkadia';
  
  interface Props {
    game: DarkadiaGameResponse;
    onMatch?: (() => void) | undefined;
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
    onSync,
    onIgnore,
    onUnignore,
    onUnmatch,
    onUnsync,
    showActions = true,
    isLoading = false,
    showGameLink = false
  }: Props = $props();

  function formatDate(date: Date): string {
    return date.toLocaleDateString();
  }

  /**
   * Checks if original and IGDB titles should show both titles
   * Shows both titles unless they are exactly identical
   */
  function shouldShowBothTitles(originalTitle: string, igdbTitle: string | null): boolean {
    if (!igdbTitle) return false;
    
    // Show both titles unless they are exactly the same
    return originalTitle !== igdbTitle;
  }

  function getStatusDisplay(): { label: string; color: string; icon: string } {
    if (game.ignored) {
      return { label: 'Ignored', color: 'bg-gray-100 text-gray-600', icon: '🚫' };
    } else if (game.game_id) {
      return { label: 'Synced', color: 'bg-blue-100 text-blue-600', icon: '🔥' };
    } else if (game.igdb_id) {
      return { label: 'Matched', color: 'bg-green-100 text-green-600', icon: '✅' };
    } else {
      return { label: 'Unmatched', color: 'bg-yellow-100 text-yellow-600', icon: '🔍' };
    }
  }

  function getPlatformStatusDisplay(): { label: string; color: string; icon: string; tooltip: string } | null {
    // If we have resolved platform name, show it
    if (game.platform_name) {
      return {
        label: game.platform_name,
        color: 'bg-green-100 text-green-600 border-green-200',
        icon: '✅',
        tooltip: `Platform resolved: ${game.platform_name}`
      };
    }
    
    // Fall back to resolution status indicators for unresolved platforms
    if (!game.platform_resolution_status && !game.original_platform_name) {
      return null;
    }
    
    switch (game.platform_resolution_status) {
      case 'resolved':
        return { 
          label: game.original_platform_name || 'Platform', 
          color: 'bg-green-100 text-green-600 border-green-200', 
          icon: '✅', 
          tooltip: `Platform resolved: ${game.original_platform_name}` 
        };
      case 'pending':
      case 'mapped':
        return { 
          label: `Needs Resolution: ${game.original_platform_name}`, 
          color: 'bg-yellow-100 text-yellow-600 border-yellow-200', 
          icon: '⚠️', 
          tooltip: `Platform needs resolution: ${game.original_platform_name}` 
        };
      case 'ignored':
        return { 
          label: 'Platform Ignored', 
          color: 'bg-gray-100 text-gray-600 border-gray-200', 
          icon: '🚫', 
          tooltip: `Platform resolution was ignored: ${game.original_platform_name || 'Unknown'}` 
        };
      case 'conflict':
        return { 
          label: `Multiple Matches`, 
          color: 'bg-red-100 text-red-600 border-red-200', 
          icon: '❌', 
          tooltip: `Multiple platform matches found: ${game.original_platform_name}` 
        };
      default:
        // If we have a platform name but no status, assume pending
        if (game.original_platform_name) {
          return { 
            label: `Needs Resolution: ${game.original_platform_name}`, 
            color: 'bg-yellow-100 text-yellow-600 border-yellow-200', 
            icon: '⚠️', 
            tooltip: `Platform needs resolution: ${game.original_platform_name}` 
          };
        }
        return null;
    }
  }

  function getStorefrontStatusDisplay(): { label: string; color: string; icon: string; tooltip: string } | null {
    // If we have resolved storefront name, show it
    if (game.storefront_name) {
      return {
        label: game.storefront_name,
        color: 'bg-blue-100 text-blue-600 border-blue-200',
        icon: '🏪',
        tooltip: `Storefront resolved: ${game.storefront_name}`
      };
    }
    
    // Fall back to storefront resolution status indicators
    if (!game.storefront_resolution_status && !game.original_storefront_name) {
      return null;
    }
    
    switch (game.storefront_resolution_status) {
      case 'resolved':
        return { 
          label: game.original_storefront_name || 'Storefront', 
          color: 'bg-blue-100 text-blue-600 border-blue-200', 
          icon: '🏪', 
          tooltip: `Storefront resolved: ${game.original_storefront_name}` 
        };
      case 'pending':
      case 'mapped':
        return { 
          label: `Needs Resolution: ${game.original_storefront_name}`, 
          color: 'bg-orange-100 text-orange-600 border-orange-200', 
          icon: '⚠️', 
          tooltip: `Storefront needs resolution: ${game.original_storefront_name}` 
        };
      case 'ignored':
        return { 
          label: 'Storefront Ignored', 
          color: 'bg-gray-100 text-gray-600 border-gray-200', 
          icon: '🚫', 
          tooltip: `Storefront resolution was ignored: ${game.original_storefront_name || 'Unknown'}` 
        };
      case 'conflict':
        return { 
          label: `Multiple Matches`, 
          color: 'bg-red-100 text-red-600 border-red-200', 
          icon: '❌', 
          tooltip: `Multiple storefront matches found: ${game.original_storefront_name}` 
        };
      default:
        // If we have a storefront name but no status, assume pending
        if (game.original_storefront_name) {
          return { 
            label: `Needs Resolution: ${game.original_storefront_name}`, 
            color: 'bg-orange-100 text-orange-600 border-orange-200', 
            icon: '⚠️', 
            tooltip: `Storefront needs resolution: ${game.original_storefront_name}` 
          };
        }
        return null;
    }
  }

  const status = $derived(getStatusDisplay());
  const platformStatus = $derived(getPlatformStatusDisplay());
  const storefrontStatus = $derived(getStorefrontStatusDisplay());
  const canSync = $derived(game.igdb_id && !game.game_id && !game.ignored);
  const canMatch = $derived(!game.igdb_id && !game.ignored);
  const canIgnore = $derived(!game.ignored);
  const canUnignore = $derived(game.ignored);
  const canUnmatch = $derived(game.igdb_id !== null && !game.game_id); // Only matched, not synced
  const canUnsync = $derived(game.game_id !== null); // Only synced games
  const showBothTitles = $derived(shouldShowBothTitles(game.name, game.igdb_title || null));
  
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
    <!-- Game Info -->
    <div class="flex-1 min-w-0">
      <div class="flex items-center justify-between mb-2">
        <div class="flex-1 min-w-0 mr-3">
          {#if showBothTitles}
            <!-- Original Title -->
            <div class="flex items-center space-x-2 mb-1">
              <span class="text-xs text-gray-500 uppercase tracking-wide font-semibold">Original:</span>
              {#if showGameLink && game.user_game_id}
                <a 
                  href="/games/{game.user_game_id}" 
                  class="text-sm font-medium text-gray-900 truncate hover:text-blue-600 transition-colors duration-200"
                  aria-label="Original title: {game.name}"
                >
                  {game.name}
                </a>
              {:else}
                <h3 class="text-sm font-medium text-gray-900 truncate" aria-label="Original title: {game.name}">
                  {game.name}
                </h3>
              {/if}
            </div>
            
            <!-- IGDB Title -->
            <div class="flex items-center space-x-2">
              <span class="text-xs text-green-600 uppercase tracking-wide font-semibold">IGDB:</span>
              <span class="text-sm font-medium text-green-700 truncate" aria-label="IGDB title: {game.igdb_title}">
                {game.igdb_title}
              </span>
            </div>
          {:else}
            <!-- Single Title Display -->
            {#if showGameLink && game.user_game_id}
              <a 
                href="/games/{game.user_game_id}" 
                class="text-sm font-medium text-gray-900 truncate hover:text-blue-600 transition-colors duration-200 block"
              >
                {game.igdb_title || game.name}
              </a>
            {:else}
              <h3 class="text-sm font-medium text-gray-900 truncate">
                {game.igdb_title || game.name}
              </h3>
            {/if}
          {/if}
        </div>
        
        <div class="flex flex-col space-y-1 items-end">
          <span class="inline-flex items-center px-2.5 py-0.5 rounded-full text-xs font-medium {status.color} flex-shrink-0">
            <span class="mr-1">{status.icon}</span>
            {status.label}
          </span>
          
          {#if platformStatus}
            <span 
              class="inline-flex items-center px-2 py-0.5 rounded border text-xs font-medium {platformStatus.color} flex-shrink-0"
              title={platformStatus.tooltip}
            >
              <span class="mr-1">{platformStatus.icon}</span>
              {platformStatus.label}
            </span>
          {/if}
          
          {#if storefrontStatus}
            <span 
              class="inline-flex items-center px-2 py-0.5 rounded border text-xs font-medium {storefrontStatus.color} flex-shrink-0"
              title={storefrontStatus.tooltip}
            >
              <span class="mr-1">{storefrontStatus.icon}</span>
              {storefrontStatus.label}
            </span>
          {/if}
        </div>
      </div>

      <div class="space-y-1 text-xs text-gray-500">
        <div class="flex items-center space-x-4">
          <span class="inline-flex items-center">
            <span class="mr-1">🎮</span>
            CSV ID: {game.external_id}
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
          Imported: {formatDate(game.created_at)}
          {#if game.updated_at.getTime() !== game.created_at.getTime()}
            • Updated: {formatDate(game.updated_at)}
          {/if}
        </div>
      </div>

      <!-- Status Info Cards -->
      {#if game.igdb_id && !game.game_id && !game.ignored}
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

      {#if game.ignored}
        <div class="mt-3 p-2 bg-gray-50 border border-gray-200 rounded text-xs">
          <div class="flex items-center text-gray-600">
            <svg class="h-4 w-4 mr-1" fill="none" viewBox="0 0 24 24" stroke="currentColor">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M18.364 18.364A9 9 0 005.636 5.636m12.728 12.728L5.636 5.636m12.728 12.728L18.364 5.636M5.636 18.364l12.728-12.728" />
            </svg>
            Game is ignored and won't be processed
          </div>
        </div>
      {/if}

      {#if game.game_id}
        <div class="mt-3 p-2 bg-green-50 border border-green-200 rounded text-xs">
          <div class="flex items-center text-green-700">
            <svg class="h-4 w-4 mr-1" fill="none" viewBox="0 0 24 24" stroke="currentColor">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 13l4 4L19 7" />
            </svg>
            Game is in your collection
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
                <path class="opacity-75" fill="currentColor" d="m4 12a8 8 0 818-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 714 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path>
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
                <path class="opacity-75" fill="currentColor" d="m4 12a8 8 0 818-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 714 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path>
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

        {#if showGameLink && game.user_game_id}
          <a
            href="/games/{game.user_game_id}"
            class="text-xs btn-secondary text-center"
            title="View in collection"
          >
            <span class="mr-1">🔗</span>
            View
          </a>
        {/if}
      </div>
    {/if}
  </div>
</div>

<!-- Unmatch Confirmation Dialog -->
{#if showUnmatchConfirm}
  <div class="fixed inset-0 bg-gray-600 bg-opacity-50 flex items-center justify-center p-4 z-50" role="dialog" aria-modal="true" aria-labelledby="unmatch-confirm-title">
    <div class="bg-white rounded-lg shadow-xl max-w-md w-full">
      <div class="p-4 border-b border-gray-200">
        <h3 id="unmatch-confirm-title" class="text-lg font-medium text-gray-900">
          Confirm Unmatch
        </h3>
        <p class="text-sm text-gray-500 mt-1">
          Are you sure you want to unmatch this game from IGDB? This will remove the IGDB association and move the game back to "Unmatched".
        </p>
      </div>
      <div class="p-4 space-y-3">
        <div class="text-sm">
          <strong>Game:</strong> {game.name}
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
  <div class="fixed inset-0 bg-gray-600 bg-opacity-50 flex items-center justify-center p-4 z-50" role="dialog" aria-modal="true" aria-labelledby="unsync-confirm-title">
    <div class="bg-white rounded-lg shadow-xl max-w-md w-full">
      <div class="p-4 border-b border-gray-200">
        <h3 id="unsync-confirm-title" class="text-lg font-medium text-gray-900">
          Confirm Unsync
        </h3>
        <p class="text-sm text-gray-500 mt-1">
          Are you sure you want to remove this game from your collection? The IGDB match will remain intact and you can re-sync it later.
        </p>
      </div>
      <div class="p-4 space-y-3">
        <div class="text-sm">
          <strong>Game:</strong> {game.name}
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